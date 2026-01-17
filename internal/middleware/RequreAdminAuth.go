package middleware

import (
	"swaves/internal/types"

	"github.com/gofiber/fiber/v2"
)

func RequireAdmin(store *types.SessionStore, loginRoute string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if c.Path() == loginRoute {
			return c.Next()
		}

		succ := store.IsLogin(c)
		if succ {
			return c.Next()
		}
		return c.Redirect(loginRoute)
	}
}
