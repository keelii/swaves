package admin

import (
	"github.com/gofiber/fiber/v2"
)

func RequireAdmin(store *SessionStore, loginRoute string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == loginRoute {
			return c.Next()
		}

		sess, err := store.Get(c)
		if err != nil {
			return c.Redirect(loginRoute)
		}
		if sess.Get("admin") != true {
			return c.Redirect(loginRoute)
		}
		return c.Next()
	}
}
