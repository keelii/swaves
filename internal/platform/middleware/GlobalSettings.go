package middleware

import (
	"swaves/internal/platform/store"

	"github.com/gofiber/fiber/v3"
)

func GlobalSettings(key string) fiber.Handler {
	if key == "" {
		key = "settings"
	}
	return func(c fiber.Ctx) error {
		setting := store.GetSettingMap()
		for k := range setting {
			c.Locals(key+"."+k, setting[k])
		}
		return c.Next()
	}
}
