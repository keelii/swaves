package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

const (
	dashCSRFSessionKey = "dash_csrf_token"
	DashCSRFFormField  = "_csrf_token"
	dashCSRFHeader     = "X-CSRF-Token"
	dashCSRFTokenBytes = 32
)

func DashCSRF(store *types.SessionStore) fiber.Handler {
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

		token := strings.TrimSpace(anyToString(sess.Get(dashCSRFSessionKey)))
		if token == "" {
			token, err = generateDashCSRFToken()
			if err != nil {
				logger.Error("[csrf] generate token failed: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("csrf unavailable")
			}
			sess.Set(dashCSRFSessionKey, token)
			if err = sess.Save(); err != nil {
				logger.Error("[csrf] persist token failed: %v", err)
				return c.Status(fiber.StatusInternalServerError).SendString("csrf unavailable")
			}
		}
		fiber.Locals(c, "CsrfToken", token)

		if isSafeMethod(c.Method()) {
			return c.Next()
		}

		candidate := strings.TrimSpace(c.Get(dashCSRFHeader))
		if candidate == "" {
			candidate = strings.TrimSpace(c.FormValue(DashCSRFFormField))
		}
		if !csrfTokenEqual(token, candidate) {
			logger.Warn("[csrf] validation failed: method=%s path=%s", c.Method(), c.Path())
			return c.Status(fiber.StatusForbidden).SendString("csrf token invalid")
		}
		return c.Next()
	}
}

func generateDashCSRFToken() (string, error) {
	buf := make([]byte, dashCSRFTokenBytes)
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
