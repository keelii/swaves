package site

import (
	"net/url"
	"strings"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	commentRememberCookieName = "swv_commenter"
	commentRememberCookieAge  = 3600 * 24 * 365
)

type commentFormDefaults struct {
	Author      string
	AuthorEmail string
	AuthorURL   string
	RememberMe  bool
}

func readCommentFormDefaults(c fiber.Ctx) commentFormDefaults {
	return parseCommentFormDefaults(c.Cookies(commentRememberCookieName))
}

func parseCommentFormDefaults(raw string) commentFormDefaults {
	defaults := commentFormDefaults{}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return defaults
	}

	values, err := url.ParseQuery(raw)
	if err != nil {
		return defaults
	}

	defaults.Author = strings.TrimSpace(values.Get("author"))
	defaults.AuthorEmail = strings.TrimSpace(values.Get("author_email"))
	defaults.AuthorURL = strings.TrimSpace(values.Get("author_url"))

	if len(defaults.Author) > 80 {
		defaults.Author = ""
	}
	if len(defaults.AuthorEmail) > 120 {
		defaults.AuthorEmail = ""
	}
	if len(defaults.AuthorURL) > 300 {
		defaults.AuthorURL = ""
	}

	defaults.RememberMe = defaults.Author != "" || defaults.AuthorEmail != "" || defaults.AuthorURL != ""
	return defaults
}

func isCommentRememberMeEnabled(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func saveCommentFormDefaults(c fiber.Ctx, defaults commentFormDefaults) {
	if !defaults.RememberMe {
		c.Cookie(&fiber.Cookie{
			Name:     commentRememberCookieName,
			Value:    "",
			Path:     getSiteFallbackPath(),
			HTTPOnly: true,
			SameSite: fiber.CookieSameSiteLaxMode,
			MaxAge:   -1,
			Expires:  time.Unix(0, 0),
		})
		return
	}

	values := url.Values{}
	values.Set("author", strings.TrimSpace(defaults.Author))
	values.Set("author_email", strings.TrimSpace(defaults.AuthorEmail))
	values.Set("author_url", strings.TrimSpace(defaults.AuthorURL))

	c.Cookie(&fiber.Cookie{
		Name:     commentRememberCookieName,
		Value:    values.Encode(),
		Path:     getSiteFallbackPath(),
		HTTPOnly: true,
		SameSite: fiber.CookieSameSiteLaxMode,
		MaxAge:   commentRememberCookieAge,
		Expires:  time.Now().Add(time.Second * commentRememberCookieAge),
	})
}
