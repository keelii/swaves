package middleware

import (
	"net/url"
	"swaves/internal/shared/types"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

func RequireDash(store *types.SessionStore, loginRoute string) fiber.Handler {
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
