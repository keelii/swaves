package app

import (
	"errors"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/modules/api"
	dash "swaves/internal/modules/dash"
	"swaves/internal/modules/site"
	"swaves/internal/modules/sui"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/jobs"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/store"
	"swaves/internal/platform/view"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"
	"swaves/internal/shared/webutil"
	webassets "swaves/web"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/google/uuid"
)

type SwavesApp struct {
	App     *fiber.App
	Config  *types.AppConfig
	Store   *store.GlobalStore
	Tracker *middleware.RequestTracker
}

func NewApp(appCfg types.AppConfig) SwavesApp {
	if err := validateAppConfig(appCfg); err != nil {
		logger.Fatal("invalid app config: %v", err)
	}

	globalStore := store.NewGlobalStore(db.Open(db.Options{
		DSN:          appCfg.SqliteFile,
		EnableSQLLog: appCfg.EnableSQLLog,
	}), dash.NewSessionStore(appCfg.SqliteFile))

	store.InitSettings(globalStore)
	store.InitRedirects(globalStore)
	viewEngine, initURLResolver := newRuntimeViewEngine()
	siteViews, initSiteURLResolver := newSiteRuntimeViewEngine(globalStore.Model, appCfg.SqliteFile)
	requestTracker := middleware.NewRequestTracker()

	app := fiber.New(fiber.Config{
		AppName:       appCfg.AppName,
		CaseSensitive: true,
		Views:         viewEngine,
		BodyLimit:     10 * 1024 * 1024, // 10MB
		ReadTimeout:   15 * time.Second,
		IdleTimeout:   5 * time.Second,
		ProxyHeader:   fiber.HeaderXForwardedFor,
		TrustProxy:    true,
		TrustProxyConfig: fiber.TrustProxyConfig{
			Loopback: true,
		},
		EnableIPValidation: true,
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "Internal Server Error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			} else if err != nil {
				msg = err.Error()
			}

			if code == fiber.StatusNotFound && (c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead) {
				if redirect, ok := store.GetRedirect(c.Path()); ok {
					return webutil.RedirectTo(c, redirect.To, redirect.Status)
				}
			}

			logger.Error("[http] method=%s code=%d msg=%s path=%s ip=%s referer=%s ua=%s",
				c.Method(), code, msg, c.Path(), c.IP(), c.Referer(), c.UserAgent())

			if strings.HasPrefix(c.Path(), share.GetDashUrl()) {
				return c.Status(code).SendString(msg)
			}

			return c.Status(code).SendString(msg)
		},
	})
	// Send "Connection: close" on every response while the server is shutting
	// down (stop==1).  Without this, fasthttp breaks keep-alive loops via the
	// bare "if stop==1 { break }" path after sending a response with no
	// Connection header, so clients hit an EPIPE/ECONNRESET on their very
	// next write — an error class Go's net/http does not retry.  With this
	// flag, the response that races against shutdown carries "Connection:
	// close", the client reads it and opens a fresh connection to the new
	// worker instead of getting a write-path error.
	app.Server().CloseOnShutdown = true
	initURLResolver(app)
	initSiteURLResolver(app)
	app.Hooks().OnListen(func(_ fiber.ListenData) error {
		go job.InitRegistry(globalStore, appCfg)
		return nil
	})

	app.Use("/static", newStaticMiddleware())
	app.Use(recover.New())
	app.Use(middleware.InstallGate("/install"))

	app.Use(middleware.DashViewContext(globalStore.Session))
	app.Use(middleware.GlobalSettings(config.GlobalSettingKey))
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header: "X-Req-Id",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
	app.Use(requestTracker.Middleware())
	dash.RegisterRouter(app, globalStore)
	sui.RegisterRouter(app, globalStore)
	site.RegisterRouter(app, globalStore, siteViews)
	api.RegisterRouter(app)

	return SwavesApp{
		App:     app,
		Store:   globalStore,
		Config:  &appCfg,
		Tracker: requestTracker,
	}
}

func newRuntimeViewEngine() (fiber.Views, func(app *fiber.App)) {
	if config.TemplateReload {
		templateRoot := resolveProjectPath("web/templates")
		if pathExists(templateRoot) {
			logger.Info("[view] using local templates: %s", templateRoot)
			return view.NewViewEngine(templateRoot, true)
		}
		logger.Info("[view] local templates not found, using embedded templates")
	}
	return view.NewViewEngineFS(webassets.TemplateFS(), false)
}

func newSiteRuntimeViewEngine(model *db.DB, sqliteFile string) (fiber.Views, func(app *fiber.App)) {
	templateRoot := ""
	var templateFS fs.FS
	if config.TemplateReload {
		templateRoot = resolveProjectPath("web/templates")
		if !pathExists(templateRoot) {
			templateRoot = ""
		}
	}
	if templateRoot == "" {
		templateFS = webassets.TemplateFS()
	}
	if config.TemplateReload {
		if templateRoot != "" {
			logger.Info("[theme] using local templates: %s", templateRoot)
			return view.NewThemeDBViewEngineWithShared(model, templateRoot, true)
		}
		logger.Info("[theme] local templates not found, using embedded templates")
		return view.NewThemeDBViewEngineWithSharedFS(model, templateFS, true)
	}
	themeRoot, err := view.MaterializeCurrentThemeCache(model, sqliteFile, templateRoot, templateFS)
	if err != nil {
		if db.IsErrNotFound(err) {
			logger.Info("[theme] current theme not set, fallback to builtin theme templates")
		} else {
			logger.Warn("[theme] materialize current theme cache failed, fallback to builtin theme templates: %v", err)
		}
		themeRoot, err = view.MaterializeBuiltinThemeCache(sqliteFile, templateRoot, templateFS)
		if err != nil {
			logger.Fatal("[theme] materialize builtin theme cache failed: %v", err)
		}
	}

	logger.Info("[theme] serving from: root=%s", themeRoot)
	return view.NewViewEngine(themeRoot, config.TemplateReload)
}

func newStaticMiddleware() fiber.Handler {
	staticRoot := resolveProjectPath("web/static")
	if config.TemplateReload && pathExists(staticRoot) {
		return static.New(staticRoot)
	}
	return static.New("", static.Config{FS: webassets.StaticFS()})
}

func validateAppConfig(appCfg types.AppConfig) error {
	if strings.TrimSpace(appCfg.SqliteFile) == "" {
		return errors.New("sqlite file is required")
	}
	return nil
}

func resolveProjectPath(path string) string {
	rel := filepath.Clean(path)

	if filepath.IsAbs(rel) {
		if _, err := os.Stat(rel); err == nil {
			return rel
		}
		return rel
	}

	wd, err := os.Getwd()
	if err == nil {
		if resolved, ok := findPathUpward(wd, rel); ok {
			return resolved
		}
	}

	if _, err := os.Stat(rel); err == nil {
		return rel
	}
	return rel
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func findPathUpward(startDir, relPath string) (string, bool) {
	dir := filepath.Clean(startDir)
	for {
		candidate := filepath.Join(dir, relPath)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, true
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func (swv *SwavesApp) Listen(opts fiber.ListenConfig) {
	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, opts); err != nil {
		logger.Fatal("%v", err)
	}
}

func (swv *SwavesApp) Serve(listener net.Listener, opts fiber.ListenConfig) error {
	if listener == nil {
		return errors.New("listener is required")
	}
	return swv.App.Listener(listener, opts)
}

func (swv *SwavesApp) Shutdown() {
	if swv == nil {
		logger.Warn("[app] shutdown skipped: app is nil")
		return
	}
	appName := ""
	if swv.Config != nil {
		appName = strings.TrimSpace(swv.Config.AppName)
	}
	logger.Info("[app] shutdown start: app=%s", appName)
	job.DestroyRegistry()
	logger.Info("[app] shutdown jobs destroyed: app=%s", appName)
	if swv.Store != nil {
		swv.Store.Close()
		logger.Info("[app] shutdown store closed: app=%s", appName)
	} else {
		logger.Warn("[app] shutdown store close skipped: app=%s reason=nil_store", appName)
	}
	logger.Info("[app] shutdown complete: app=%s", appName)
}

func (swv *SwavesApp) PauseJobs() {
	if swv == nil {
		return
	}
	job.PauseRegistry()
}
