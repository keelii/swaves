package admin

import "github.com/gofiber/fiber/v2"

// RequireAdminLogin 中间件，保护后台路由
func RequireAdminLogin(c *fiber.Ctx) error {
	if c.Cookies(LoginCookieName) != "1" {
		return c.Redirect("/admin/login")
	}
	return c.Next()
}
