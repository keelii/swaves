package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"swaves/internal/logger"
	"swaves/internal/types"

	"github.com/gofiber/fiber/v3"
)

const (
	adminCSRFSessionKey = "admin_csrf_token"
	AdminCSRFFormField  = "_csrf_token"
	adminCSRFHeader     = "X-CSRF-Token"
	adminCSRFTokenBytes = 32
)

func AdminCSRF(store *types.SessionStore) fiber.Handler {
	return func(c fiber.Ctx) error {
		if store == nil {
			logger.Error("[csrf] session store is nil")
			return c.Status(fiber.StatusInternalServerError).SendString("session unavailable")
		}

		sess, err := store.AcquireSession(c)
		if err != nil {
			logger.Error("[csrf] acquire session failed: %v", err)
			return c.Status(fiber.StatusInternalServerError).SendString("session unavailable")
		}
		defer sess.Release()

		token := strings.TrimSpace(anyToString(sess.Get(adminCSRFSessionKey)))
		if token == "" {
			token, err = generateAdminCSRFToken()
			if err != nil {
				logger.Error("[csrf] generate token failed: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("csrf unavailable")
			}
			sess.Set(adminCSRFSessionKey, token)
			if err = sess.Save(); err != nil {
				logger.Error("[csrf] persist token failed: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("csrf unavailable")
			}
		}
		fiber.Locals(c, "CsrfToken", token)

		if isSafeMethod(c.Method()) {
			return c.Next()
		}

		candidate := strings.TrimSpace(c.Get(adminCSRFHeader))
		if candidate == "" {
			candidate = strings.TrimSpace(c.FormValue(AdminCSRFFormField))
		}
		if !csrfTokenEqual(token, candidate) {
			logger.Warn("[csrf] validation failed: method=%s path=%s", c.Method(), c.Path())
			return c.Status(fiber.StatusForbidden).SendString("csrf token invalid")
		}
		return c.Next()
	}
}

func generateAdminCSRFToken() (string, error) {
	buf := make([]byte, adminCSRFTokenBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func isSafeMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions, fiber.MethodTrace:
		return true
	default:
		return false
	}
}

func csrfTokenEqual(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" || actual == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(expected), []byte(actual)) == 1
}

func anyToString(raw any) string {
	if raw == nil {
		return ""
	}
	switch v := raw.(type) {
	case string:
		return v
	default:
		return ""
	}
}
