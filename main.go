package main

import (
	"log"
	"swaves/internal/admin"
	"swaves/internal/tpl"

	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
)

func main() {
	tpl.LoadTemplatesDir("web/templates")

	conn := db.Open(db.Options{
		DSN: "data.sqlite",
	})
	defer conn.Close()

	app := fiber.New(fiber.Config{
		AppName:               "swaves",
		DisableStartupMessage: true,
	})

	handler := admin.NewHandler(
		admin.NewService(conn),
		admin.NewSessionStore(conn),
	)

	adminGroup := app.Group("/admin")

	adminGroup.Get("/", handler.GetHome)
	adminGroup.Get("/login", handler.GetLoginHandler)
	adminGroup.Post("/login", handler.PostLoginHandler)
	adminGroup.Get("/logout", handler.GetLogoutHandler)

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
