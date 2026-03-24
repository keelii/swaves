package middleware

import (
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

func normalizeInstallPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" || path == "/" {
		return "/install"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		return "/install"
	}
	return path
}

func normalizeRequestPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		return "/"
	}
	return path
}

func isStaticAssetPath(path string) bool {
	return path == "/static" || strings.HasPrefix(path, "/static/")
}

func InstallGate(model *db.DB, installPath string) fiber.Handler {
	installPath = normalizeInstallPath(installPath)

	return func(c fiber.Ctx) error {
		path := normalizeRequestPath(c.Path())
		if isStaticAssetPath(path) {
			return c.Next()
		}

		installed, err := db.HasInstalledSettings(model)
		if err != nil {
			logger.Error("[install] resolve install state failed: path=%s err=%v", c.Path(), err)
			return c.Status(fiber.StatusInternalServerError).SendString("install state unavailable")
		}

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
