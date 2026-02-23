package middleware

import (
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

func AdminViewContext(session *types.SessionStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		fiber.Locals(c, "IsLogin", session.IsLogin(c))

		return c.Next()
	}
}
