package main

import (
	"log"
	"swaves/internal/tpl"

	"swaves/internal/admin"
	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
)

func main() {
	tpl.LoadTemplatesDir("web/templates")

	// 1. 打开数据库
	dbConn, err := db.Open(db.Options{
		DSN: "data.sqlite",
	})
	if err != nil {
		log.Fatalf("open db failed: %v", err)
	}
	defer dbConn.Close()

	// 2. migrate
	if err := db.Migrate(dbConn); err != nil {
		log.Fatalf("migrate failed: %v", err)
	}

	// 3. Fiber app
	app := fiber.New(fiber.Config{
		AppName: "swaves",
	})
	// 注册后台登录路由
	admin.RegisterAdminRoutes(app, dbConn)

	// 注册后台受保护路由
	adminGroup := app.Group("/admin", admin.RequireAdminLogin)
	adminGroup.Get("/dashboard", func(c *fiber.Ctx) error {
		return tpl.RenderTemplate(c, "admin_dashboard", map[string]string{
			"Title": "Admin Dashboard",
		})
	})

	log.Println("swaves listening on :3000")
	log.Fatal(app.Listen(":3000"))
}
