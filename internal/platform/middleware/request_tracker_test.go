package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

func TestRequestTrackerTracksActiveRequest(t *testing.T) {
	tracker := NewRequestTracker()
	app := fiber.New()
	started := make(chan struct{})
	release := make(chan struct{})

	app.Use(requestid.New())
	app.Use(tracker.Middleware())
	app.Get("/slow", func(c fiber.Ctx) error {
		close(started)
		<-release
		return c.SendStatus(fiber.StatusOK)
	})

	done := make(chan error, 1)
	go func() {
		req := httptest.NewRequest(fiber.MethodGet, "/slow", nil)
		_, err := app.Test(req, fiber.TestConfig{Timeout: 2 * time.Second})
		done <- err
	}()

	<-started
	if got := tracker.ActiveCount(); got != 1 {
		t.Fatalf("ActiveCount = %d, want 1", got)
	}

	snapshot := tracker.Snapshot(5)
	if len(snapshot) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snapshot))
	}
	if snapshot[0].Path != "/slow" {
		t.Fatalf("Snapshot path = %q, want %q", snapshot[0].Path, "/slow")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if got := tracker.ActiveCount(); got != 0 {
		t.Fatalf("ActiveCount = %d, want 0", got)
	}
}

func TestFormatActiveRequests(t *testing.T) {
	now := time.Date(2026, 4, 12, 11, 0, 5, 0, time.UTC)
	items := []ActiveRequest{
		{
			ID:        3,
			ReqID:     "req-1",
			Method:    fiber.MethodPost,
			Path:      "/dash/restart",
			IP:        "127.0.0.1",
			StartedAt: now.Add(-2 * time.Second),
		},
	}

	got := FormatActiveRequests(items, now)
	if !strings.Contains(got, "req_id=req-1") {
		t.Fatalf("expected req_id in output, got %q", got)
	}
	if !strings.Contains(got, "POST /dash/restart") {
		t.Fatalf("expected method/path in output, got %q", got)
	}
	if !strings.Contains(got, "age=2s") {
		t.Fatalf("expected age in output, got %q", got)
	}
}
