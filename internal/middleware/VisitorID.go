package middleware

import (
	"crypto/rand"
	"encoding/base64"
	"strings"
	"swaves/internal/share"

	"github.com/gofiber/fiber/v3"
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

	return func(c fiber.Ctx) error {
		path := c.Path()
		if isSkipVisitorIDPath(path) {
			return c.Next()
		}

		GetOrCreateVisitorID(c, cookieName)
		return c.Next()
	}
}

func GetOrCreateVisitorID(c fiber.Ctx, cookieName string) string {
	if cookieName == "" {
		cookieName = DefaultVisitorIDCookieName
	}

	visitorID, _ := c.Locals(VisitorIDLocalKey).(string)
	if isValidVisitorID(visitorID) {
		return visitorID
	}

	visitorID = c.Cookies(cookieName)
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
	return visitorID
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

func isSkipVisitorIDPath(path string) bool {
	if path == "/metrics" {
		return true
	}

	prefixes := []string{share.GetAdminUrl(), "/api", "/static"}
	for _, prefix := range prefixes {
		if prefix == "/" {
			continue
		}
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}

	return false
}
