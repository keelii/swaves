package main

import (
	"log"
	"swaves/internal/admin"
	"swaves/internal/api"
	"swaves/internal/consts"
	job "swaves/internal/jobs"
	"swaves/internal/middleware"
	"swaves/internal/store"
	"time"

	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/monitor"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/google/uuid"
)

const TimeFormat = "2006-01-02 15:04:05"

//const TimeFormatMs = "2006-01-02 15:04:05.000"

func main() {
	globalStore := store.NewGlobalStore(db.Open(db.Options{
		DSN: "data.sqlite",
	}), admin.NewSessionStore())

	defer globalStore.Close()

	job.InitRegistry(globalStore, 5*time.Second) // 每 5 秒扫描 pending
	job.RegisterJob("hello", job.HelloJob)
	job.RegisterJob("fdsa", job.HelloJob1)

	store.InitSettings(globalStore)

	app := fiber.New(fiber.Config{
		AppName:               "swaves",
		DisableStartupMessage: true,
		Views:                 NewViewEngine(),
	})

	// statics
	app.Static("/static", "./web/static")
	// metrics
	app.Get("/metrics", monitor.New(monitor.Config{Title: "swaves metrics"}))

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
	api.RegisterRoutes(app)

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
