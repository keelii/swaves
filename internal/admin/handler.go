package admin

import (
	"swaves/internal/db"
	"swaves/internal/tpl"

	"github.com/gofiber/fiber/v2"
)

// GetLoginHandler GET /admin/login
func GetLoginHandler(c *fiber.Ctx) error {
	return tpl.RenderTemplate(c, "admin_login", map[string]string{
		"Title": "Admin Login",
		"Error": "",
	})
}

// PostLoginHandler POST /admin/login
func PostLoginHandler(dbConn *db.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		password := c.FormValue("password")
		config, err := db.GetConfig(dbConn)
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).SendString("internal error")
		}

		if err := config.CheckPassword(password); err != nil {
			return tpl.RenderTemplate(c, "admin_login", map[string]string{
				"Title": "Admin Login",
				"Error": "Invalid password",
			})
		}

		// 登录成功，设置 cookie
		c.Cookie(&fiber.Cookie{
			Name:  LoginCookieName,
			Value: "1",
			Path:  "/",
		})
		return c.Redirect("/admin/dashboard")
	}
}

// RegisterAdminRoutes 注册后台登录路由
func RegisterAdminRoutes(app *fiber.App, dbConn *db.DB) {
	app.Get("/admin/login", GetLoginHandler)
	app.Post("/admin/login", PostLoginHandler(dbConn))
}
