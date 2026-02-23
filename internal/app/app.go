package app

import (
	"os"
	"path/filepath"
	"swaves/internal/modules/admin"
	"swaves/internal/modules/api"
	"swaves/internal/modules/site"
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

func NewApp(config types.AppConfig) SwavesApp {
	globalStore := store.NewGlobalStore(db.Open(db.Options{
		DSN:          config.SqliteFile,
		EnableSQLLog: config.EnableSQLLog,
	}), admin.NewSessionStore())

	//defer globalStore.Close()

	store.InitSettings(globalStore)
	templateRoot := resolveProjectPath("web/templates")
	viewEngine, initURLResolver, urlFor := view.NewViewEngine(templateRoot, consts.TemplateReload)

	app := fiber.New(fiber.Config{
		AppName:       config.AppName,
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
		go job.InitRegistry(globalStore, config) // 初始化定时任务
		return nil
	})

	// statics
	app.Use("/static", static.New(resolveProjectPath("web/static")))

	//app.Use(limiter.New())
	app.Use(middleware.AdminViewContext(globalStore.Session))
	app.Use(middleware.GlobalSettings(consts.GlobalSettingKey))
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header: "X-Req-Id",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
	app.Use(recover.New())
	app.Use(middleware.HttpErrorLog(globalStore.Model))
	//app.Use(logger.New(logger.Config{
	//	BaseTimeFormat: BaseTimeFormat,
	//	Format:     "${time} ${status} - ${method} ${path} ${queryParams} ${body}\n",
	//}))

	//fmt.Println(md.ParseMarkdown(``))

	admin.RegisterRoutes(app, globalStore, urlFor)
	site.RegisterRoutes(app, globalStore)
	api.RegisterRoutes(app)

	return SwavesApp{
		App:    app,
		Store:  globalStore,
		Config: &config,
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
