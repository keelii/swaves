package main

import (
	"log"
	"swaves/internal/admin"
	"swaves/internal/api"
	"swaves/internal/middleware"
	"time"

	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/template/html/v3"
)

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
		return time.Unix(ts, 0).Format("2006-01-02 15:04:05")
	})
	engine.Reload(true)
	app := fiber.New(fiber.Config{
		AppName:               "swaves",
		DisableStartupMessage: true,
		Views:                 engine,
	})

	app.Use(middleware.PaginationMiddleware())

	admin.RegisterRoutes(app, conn)
	api.RegisterRoutes(app, conn)

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
