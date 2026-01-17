package middleware

import (
	"swaves/internal/types"

	"github.com/gofiber/fiber/v2"
)

func AdminViewContext(session *types.SessionStore) fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Locals("IsLogin", session.IsLogin(c))
		c.Locals("UrlPath", c.Path())

		return c.Next()
	}
}
