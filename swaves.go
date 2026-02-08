package main

import (
	"log"
	"swaves/internal/admin"
	"swaves/internal/api"
	"swaves/internal/consts"
	"swaves/internal/db"
	"swaves/internal/jobs"
	"swaves/internal/middleware"
	"swaves/internal/store"
	"swaves/internal/types"
	"swaves/internal/ui"

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

	go job.InitRegistry(globalStore, config) // 每 5 秒扫描 pending

	store.InitSettings(globalStore)
	app := fiber.New(fiber.Config{
		AppName:               config.AppName,
		DisableStartupMessage: true,
		Views:                 NewViewEngine(),
	})

	// statics
	app.Static("/static", "./web/static")
	// metrics
	app.Get("/metrics", monitor.New(monitor.Config{
		Title:      config.AppName + " metrics",
		FontURL:    "/static/metrics/google-font.css",
		ChartJsURL: "/static/metrics/Chart.bundle.min.js",
	}))

	// Auth
	app.Use(middleware.RequireAdmin(globalStore.Session, consts.LoginRoutePath))

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
	app.Use(middleware.HttpErrorLogMiddleware(globalStore))
	//app.Use(logger.New(logger.Config{
	//	TimeFormat: TimeFormat,
	//	Format:     "${time} ${status} - ${method} ${path} ${queryParams} ${body}\n",
	//}))

	//fmt.Println(md.ParseMarkdown(``))

	admin.RegisterRoutes(app, globalStore)
	ui.RegisterRoutes(app, globalStore)
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
