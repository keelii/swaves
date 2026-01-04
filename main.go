package main

import (
	"log"
	"swaves/internal/admin"
	"swaves/internal/api"
	"swaves/internal/middleware"
	"time"

	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/requestid"
	"github.com/gofiber/template/html/v3"
	"github.com/google/uuid"
)

const TimeFormat = "2006-01-02 15:04:05"

//const TimeFormatMs = "2006-01-02 15:04:05.000"

func main() {
	conn := db.Open(db.Options{
		DSN: "data.sqlite",
	})
	defer conn.Close()

	engine := html.New("./web/templates", ".html")
	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})
	engine.AddFunc("until", func(count int) []int {
		var step []int
		for i := 0; i < count; i++ {
			step = append(step, i)
		}
		return step
	})
	engine.AddFunc("formatTime", func(ts int64) string {
		if ts == 0 {
			return "-"
		}
		return time.Unix(ts, 0).Format(TimeFormat)
	})
	engine.Reload(true)
	app := fiber.New(fiber.Config{
		AppName:               "swaves",
		DisableStartupMessage: true,
		Views:                 engine,
	})

	app.Use(requestid.New())
	app.Use(middleware.PaginationMiddleware())
	app.Use(requestid.New(requestid.Config{
		Header:     "X-Req-Id",
		ContextKey: "reqId",
		Generator: func() string {
			return uuid.NewString()
		},
	}))
	app.Use(middleware.HttpErrorLogMiddleware(conn))
	//app.Use(logger.New(logger.Config{
	//	TimeFormat: TimeFormat,
	//	Format:     "${time} ${status} - ${method} ${path} ${queryParams} ${body}\n",
	//}))

	admin.RegisterRoutes(app, conn)
	api.RegisterRoutes(app, conn)

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
