package middleware

import (
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/logger"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

const (
	dashLoginRateLimitMax        = 10
	dashLoginRateLimitExpiration = 24 * time.Hour
)

type dashLoginRateLimitEntry struct {
	Count     int
	ExpiresAt time.Time
}

var dashLoginRateLimitStore = struct {
	sync.Mutex
	entries map[string]dashLoginRateLimitEntry
}{
	entries: map[string]dashLoginRateLimitEntry{},
}

func DashLoginRateLimitExceeded(c fiber.Ctx) bool {
	if !config.IsProduction {
		return false
	}

	key := dashLoginRateLimitKey(c)
	if key == "" {
		return false
	}

	now := time.Now()

	dashLoginRateLimitStore.Lock()
	defer dashLoginRateLimitStore.Unlock()

	entry, ok := dashLoginRateLimitStore.entries[key]
	if !ok {
		return false
	}
	if now.After(entry.ExpiresAt) {
		delete(dashLoginRateLimitStore.entries, key)
		return false
	}
	return entry.Count >= dashLoginRateLimitMax
}

func DashLoginRateLimitRecordFailure(c fiber.Ctx) bool {
	if !config.IsProduction {
		return false
	}

	key := dashLoginRateLimitKey(c)
	if key == "" {
		return false
	}

	now := time.Now()

	dashLoginRateLimitStore.Lock()
	defer dashLoginRateLimitStore.Unlock()

	entry, ok := dashLoginRateLimitStore.entries[key]
	if !ok || now.After(entry.ExpiresAt) {
		entry = dashLoginRateLimitEntry{}
	}

	entry.Count++
	entry.ExpiresAt = now.Add(dashLoginRateLimitExpiration)
	dashLoginRateLimitStore.entries[key] = entry

	return entry.Count >= dashLoginRateLimitMax
}

func DashLoginRateLimitReset(c fiber.Ctx) {
	key := dashLoginRateLimitKey(c)
	if key == "" {
		return
	}

	dashLoginRateLimitStore.Lock()
	defer dashLoginRateLimitStore.Unlock()

	delete(dashLoginRateLimitStore.entries, key)
}

func DashLoginRateLimitResetAll() {
	dashLoginRateLimitStore.Lock()
	defer dashLoginRateLimitStore.Unlock()

	dashLoginRateLimitStore.entries = map[string]dashLoginRateLimitEntry{}
}

func DashLoginRateLimitLog(c fiber.Ctx) {
	logger.Warn("[dash] login rate limited: ip=%s method=%s path=%s", c.IP(), c.Method(), c.Path())
}

func dashLoginRateLimitKey(c fiber.Ctx) string {
	return strings.TrimSpace(c.IP())
}
