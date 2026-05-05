package middleware

import (
	"strings"
	"swaves/internal/platform/store"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

func isStaticAssetPath(path string) bool {
	return path == "/static" || strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/sui")
}

func InstallGate(installPath string) fiber.Handler {
	return func(c fiber.Ctx) error {
		path := c.Path()
		if isStaticAssetPath(path) {
			return c.Next()
		}

		installed := !store.IsSettingEmpty()

		if path == installPath {
			if installed {
				return fiber.ErrNotFound
			}
			return c.Next()
		}

		if !installed {
			return webutil.RedirectTo(c, installPath, fiber.StatusFound)
		}

		return c.Next()
	}
}
