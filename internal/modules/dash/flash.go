package dash

import (
	"fmt"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

const (
	dashFlashNoticeKey = "_dash_flash_notice"
	dashFlashErrorKey  = "_dash_flash_error"
)

func dashSessionStore(c fiber.Ctx) *types.SessionStore {
	return fiber.Locals[*types.SessionStore](c, "DashSessionStore")
}

func (h *Handler) setFlashNotice(c fiber.Ctx, message string) {
	setDashFlash(c, h.Session, dashFlashNoticeKey, message)
}

func (h *Handler) setFlashError(c fiber.Ctx, message string) {
	setDashFlash(c, h.Session, dashFlashErrorKey, message)
}

func setDashFlash(c fiber.Ctx, sessionStore *types.SessionStore, key string, message string) {
	message = strings.TrimSpace(message)
	if sessionStore == nil || message == "" {
		return
	}

	sess, err := sessionStore.AcquireSession(c)
	if err != nil {
		logger.Error("[dash] acquire session for flash failed: key=%s err=%v", key, err)
		return
	}
	defer sess.Release()

	sess.Set(key, message)
	if err := sess.Save(); err != nil {
		logger.Error("[dash] save flash failed: key=%s err=%v", key, err)
	}
}

func popDashFlash(c fiber.Ctx, key string) string {
	sessionStore := dashSessionStore(c)
	if sessionStore == nil {
		return ""
	}

	sess, err := sessionStore.AcquireSession(c)
	if err != nil {
		logger.Error("[dash] acquire session for flash pop failed: key=%s err=%v", key, err)
		return ""
	}
	defer sess.Release()

	raw := sess.Get(key)
	if raw == nil {
		return ""
	}

	message := strings.TrimSpace(fmt.Sprintf("%v", raw))
	sess.Delete(key)
	if err := sess.Save(); err != nil {
		logger.Error("[dash] clear flash failed: key=%s err=%v", key, err)
	}
	return message
}

func injectDashFlash(c fiber.Ctx, data fiber.Map) {
	notice := popDashFlash(c, dashFlashNoticeKey)
	errorMessage := popDashFlash(c, dashFlashErrorKey)

	if valueAsTrimmedString(data["Notice"]) == "" && notice != "" {
		data["Notice"] = notice
	}
	if valueAsTrimmedString(data["Error"]) == "" && errorMessage != "" {
		data["Error"] = errorMessage
	}
}

func valueAsTrimmedString(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
