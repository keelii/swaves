package main

import (
	"log"
	"swaves/internal/admin"
	"swaves/internal/api"
	"swaves/internal/consts"
	"swaves/internal/db"
	"swaves/internal/jobs"
	"swaves/internal/middleware"
	"swaves/internal/site"
	"swaves/internal/store"
	"swaves/internal/types"

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
	view, initURLResolver := NewViewEngine()

	app := fiber.New(fiber.Config{
		AppName:       config.AppName,
		CaseSensitive: true,
		Views:         view,
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

			log.Println("[ERROR]HTTP:", code, msg, c.Path())
			return c.Status(code).SendString(msg)
		},
	})
	initURLResolver(app)
	app.Hooks().OnListen(func(_ fiber.ListenData) error {
		go job.InitRegistry(globalStore, config) // 初始化定时任务
		return nil
	})

	// statics
	app.Use("/static", static.New("./web/static"))

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
	//	TimeFormat: TimeFormat,
	//	Format:     "${time} ${status} - ${method} ${path} ${queryParams} ${body}\n",
	//}))

	//fmt.Println(md.ParseMarkdown(``))

	admin.RegisterRoutes(app, globalStore)
	site.RegisterRoutes(app, globalStore)
	api.RegisterRoutes(app)

	return SwavesApp{
		App:    app,
		Store:  globalStore,
		Config: &config,
	}
}

func (swv *SwavesApp) Listen(opts fiber.ListenConfig) {
	log.Println(swv.Config.AppName + " listening on " + swv.Config.ListenAddr)
	log.Fatal(swv.App.Listen(swv.Config.ListenAddr, opts))
}

func (swv *SwavesApp) Shutdown() {
	job.DestroyRegistry()
	swv.Store.Close()
}
