package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const (
	DefaultVisitorIDCookieName = "vid"
	VisitorIDLocalKey          = "visitor_id"
	visitorIDBytes             = 12
	visitorIDEncodedLength     = 16
)

func EnsureVisitorID(cookieName string) fiber.Handler {
	if cookieName == "" {
		cookieName = DefaultVisitorIDCookieName
	}

	return func(c *fiber.Ctx) error {
		path := c.Path()
		if strings.HasPrefix(path, "/admin") || strings.HasPrefix(path, "/api") || strings.HasPrefix(path, "/static") || path == "/metrics" {
			return c.Next()
		}

		visitorID := c.Cookies(cookieName)
		if !isValidVisitorID(visitorID) {
			visitorID = newVisitorID()
			c.Cookie(&fiber.Cookie{
				Name:     cookieName,
				Value:    visitorID,
				Path:     "/",
				MaxAge:   60 * 60 * 24 * 365,
				HTTPOnly: true,
				Secure:   strings.EqualFold(c.Protocol(), "https"),
				SameSite: "Lax",
			})
		}

		c.Locals(VisitorIDLocalKey, visitorID)
		return c.Next()
	}
}

func newVisitorID() string {
	b := make([]byte, visitorIDBytes)
	if _, err := rand.Read(b); err != nil {
		id := uuid.New()
		return base64.RawURLEncoding.EncodeToString(id[:visitorIDBytes])
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func isValidVisitorID(visitorID string) bool {
	if len(visitorID) != visitorIDEncodedLength {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(visitorID)
	return err == nil && len(decoded) == visitorIDBytes
}
