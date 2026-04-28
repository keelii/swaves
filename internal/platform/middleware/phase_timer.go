package middleware

import (
	"fmt"
	"strings"
	"time"

	"swaves/internal/platform/logger"
	"swaves/internal/platform/store"

	"github.com/gofiber/fiber/v3"
)

const phaseTimerLocalsKey = "phase_timer"

// PhaseEntry records the duration of one named request phase.
type PhaseEntry struct {
	Name     string
	Duration time.Duration
}

// PhaseTimer tracks per-phase durations within a single request.
// All methods are nil-safe; pass nil to disable tracking with zero overhead.
type PhaseTimer struct {
	startedAt time.Time
	entries   []PhaseEntry
}

// NewPhaseTimer creates a timer whose total clock starts immediately.
func NewPhaseTimer() *PhaseTimer {
	return &PhaseTimer{startedAt: time.Now()}
}

// Track wraps fn, records how long it ran under name, then returns.
// Calling Track on a nil *PhaseTimer still executes fn unchanged.
func (t *PhaseTimer) Track(name string, fn func()) {
	if t == nil {
		fn()
		return
	}
	start := time.Now()
	fn()
	t.entries = append(t.entries, PhaseEntry{
		Name:     name,
		Duration: time.Since(start),
	})
}

// Log emits a single INFO line with all phase timings when the
// "debug_request_timing" setting is enabled (1/true/yes/on).
// It is a no-op on a nil *PhaseTimer or when the setting is off.
func (t *PhaseTimer) Log(routeName string) {
	if t == nil {
		return
	}
	m := store.GetSettingMap()
	val := strings.ToLower(strings.TrimSpace(m["debug_request_timing"]))
	switch val {
	case "1", "true", "yes", "on":
		// proceed
	default:
		return
	}

	total := time.Since(t.startedAt).Round(time.Millisecond)
	parts := make([]string, 0, len(t.entries)+1)
	parts = append(parts, fmt.Sprintf("total=%s", total))
	for _, e := range t.entries {
		parts = append(parts, fmt.Sprintf("%s=%s", e.Name, e.Duration.Round(time.Millisecond)))
	}
	logger.Info("[timing] %s %s", routeName, strings.Join(parts, " "))
}

// InjectTimer creates a new PhaseTimer and stores it in the request context.
func InjectTimer(c fiber.Ctx) {
	c.Locals(phaseTimerLocalsKey, NewPhaseTimer())
}

// GetTimer retrieves the PhaseTimer stored by InjectTimer.
// Returns nil if none was injected; all PhaseTimer methods handle nil gracefully.
func GetTimer(c fiber.Ctx) *PhaseTimer {
	t, _ := c.Locals(phaseTimerLocalsKey).(*PhaseTimer)
	return t
}

// PhaseTimerMiddleware injects a PhaseTimer at the start of each request and
// logs the phase breakdown after the handler chain completes.
func PhaseTimerMiddleware() fiber.Handler {
	return func(c fiber.Ctx) error {
		InjectTimer(c)
		err := c.Next()
		routeName := ""
		if route := c.Route(); route != nil {
			routeName = strings.TrimSpace(route.Name)
		}
		GetTimer(c).Log(routeName)
		return err
	}
}
