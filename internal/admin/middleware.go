package admin

import (
	"github.com/gofiber/fiber/v2"
)

func RequireAdmin(store *SessionStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		sess, err := store.Get(c)
		if err != nil {
			return c.Redirect("/admin/login")
		}
		if sess.Get("admin") != true {
			return c.Redirect("/admin/login")
		}
		return c.Next()
	}
}
