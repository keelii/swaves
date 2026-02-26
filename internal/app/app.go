package app

import (
	"os"
	"path/filepath"
	"swaves/internal/modules/admin"
	"swaves/internal/modules/api"
	"swaves/internal/modules/site"
	"swaves/internal/modules/sui"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/jobs"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/store"
	"swaves/internal/platform/view"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/google/uuid"
)

type SwavesApp struct {
	App    *fiber.App
	Config *types.AppConfig
	Store  *store.GlobalStore
}

func NewApp(appCfg types.AppConfig) SwavesApp {
	globalStore := store.NewGlobalStore(db.Open(db.Options{
		DSN:          appCfg.SqliteFile,
		EnableSQLLog: appCfg.EnableSQLLog,
	}), admin.NewSessionStore())

	store.InitSettings(globalStore)
	templateRoot := resolveProjectPath("web/templates")
	viewEngine, initURLResolver := view.NewViewEngine(templateRoot, config.TemplateReload)

	app := fiber.New(fiber.Config{
		AppName:       appCfg.AppName,
		CaseSensitive: true,
		Views:         viewEngine,
		BodyLimit:     10 * 1024 * 1024, // 10MB
		ErrorHandler: func(c fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "Internal Server Error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			} else if err != nil {
				msg = err.Error()
			}

			logger.Error("[http] code=%d msg=%s path=%s", code, msg, c.Path())
			return c.Status(code).SendString(msg)
		},
	})
	initURLResolver(app)
	app.Hooks().OnListen(func(_ fiber.ListenData) error {
		go job.InitRegistry(globalStore, appCfg)
		return nil
	})

	app.Use("/static", static.New(resolveProjectPath("web/static")))

	app.Use(middleware.AdminViewContext(globalStore.Session))
	app.Use(middleware.GlobalSettings(config.GlobalSettingKey))
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header: "X-Req-Id",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
	app.Use(recover.New())
	app.Use(middleware.HttpErrorLog(globalStore.Model))

	admin.RegisterRouter(app, globalStore)
	sui.RegisterRouter(app, globalStore)
	site.RegisterRouter(app, globalStore)
	api.RegisterRouter(app)

	return SwavesApp{
		App:    app,
		Store:  globalStore,
		Config: &appCfg,
	}
}

func resolveProjectPath(path string) string {
	primary := filepath.Clean(path)
	if _, err := os.Stat(primary); err == nil {
		return primary
	}

	fallback := filepath.Clean(filepath.Join("..", "..", path))
	if _, err := os.Stat(fallback); err == nil {
		return fallback
	}
	return primary
}

func (swv *SwavesApp) Listen(opts fiber.ListenConfig) {
	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, opts); err != nil {
		logger.Fatal("%v", err)
	}
}

func (swv *SwavesApp) Shutdown() {
	job.DestroyRegistry()
	swv.Store.Close()
}
