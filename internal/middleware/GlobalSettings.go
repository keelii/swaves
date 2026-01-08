package middleware

import (
	"swaves/internal/store"

	"github.com/gofiber/fiber/v2"
)

func GlobalSettings(key string) fiber.Handler {
	if key == "" {
		key = "setting"
	}
	return func(c *fiber.Ctx) error {
		setting := store.GetSettingMap()
		c.Locals(key, setting)
		return c.Next()
	}
}
