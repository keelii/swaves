package main

import (
	"log"
	"swaves/internal/admin"

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
	engine.Reload(true)
	app := fiber.New(fiber.Config{
		AppName:               "swaves",
		DisableStartupMessage: true,
		Views:                 engine,
	})

	admin.RegisterRoutes(app, conn)

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
