package middleware

import (
	"net/url"
	"swaves/internal/types"
	"swaves/internal/webutil"

	"github.com/gofiber/fiber/v3"
)

func RequireAdmin(store *types.SessionStore, loginRoute string) fiber.Handler {
	return func(c fiber.Ctx) error {
		if c.Path() == loginRoute {
			return c.Next()
		}

		succ := store.IsLogin(c)
		if succ {
			return c.Next()
		}
		dest := loginRoute + "?returnUrl=" + url.QueryEscape(c.OriginalURL())
		return webutil.RedirectTo(c, dest)
	}
}
