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

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/google/uuid"
)

type SwavesApp struct {
	App    *fiber.App
	Config *types.AppConfig
	Store  *store.GlobalStore
}

func NewApp(config types.AppConfig) SwavesApp {
	globalStore := store.NewGlobalStore(db.Open(db.Options{
		DSN: config.SqliteFile,
	}), admin.NewSessionStore())

	//defer globalStore.Close()

	go job.InitRegistry(globalStore, config) // 初始化定时任务

	store.InitSettings(globalStore)
	app := fiber.New(fiber.Config{
		AppName:               config.AppName,
		DisableStartupMessage: true,
		Views:                 NewViewEngine(),
		BodyLimit:             10 * 1024 * 1024, // 10MB
		ErrorHandler: func(c *fiber.Ctx, err error) error {
			code := fiber.StatusInternalServerError
			msg := "Internal Server Error"
			if e, ok := err.(*fiber.Error); ok {
				code = e.Code
				msg = e.Message
			} else if err != nil {
				msg = err.Error()
			}

			log.Println("[error]HTTP:", code, msg)
			return c.Status(code).SendString(msg)
		},
	})

	// statics
	app.Static("/static", "./web/static")
	// metrics
	app.Get("/metrics", monitor.New(monitor.Config{
		Title:      config.AppName + " metrics",
		FontURL:    "/static/admin/metrics/google-font.css",
		ChartJsURL: "/static/admin/metrics/Chart.bundle.min.js",
	}))

	//app.Use(limiter.New())
	app.Use(middleware.AdminViewContext(globalStore.Session))
	app.Use(middleware.GlobalSettings(consts.GlobalSettingKey))
	app.Use(requestid.New())
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header:     "X-Req-Id",
		ContextKey: "reqId",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
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

func (swv *SwavesApp) Listen() {
	log.Println(swv.Config.AppName + " listening on " + swv.Config.ListenAddr)
	log.Fatal(swv.App.Listen(swv.Config.ListenAddr))
}

func (swv *SwavesApp) Shutdown() {
	swv.Store.Close()
}
