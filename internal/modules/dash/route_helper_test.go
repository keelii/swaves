package dash

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestRedirectToDashRouteReturnsErrorWhenRouteMissing(t *testing.T) {
	app := fiber.New()
	h := &Handler{}

	app.Get("/test", func(c fiber.Ctx) error {
		return h.redirectToDashRoute(c, "dash.route.not.exists", nil, nil)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusInternalServerError)
	}
}
