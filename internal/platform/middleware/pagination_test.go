package middleware

import (
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestPaginationMiddlewareUsesFrontendPageSizeByDefault(t *testing.T) {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("settings.page_size", "11")
		c.Locals("settings.dash_page_size", "33")
		return c.Next()
	})
	app.Use(PaginationMiddleware())
	app.Get("/", func(c fiber.Ctx) error {
		p := GetPagination(c)
		if p.PageSize != 11 {
			t.Fatalf("PageSize = %d, want %d", p.PageSize, 11)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

func TestPaginationMiddlewareUsesDashPageSizeForDashRoutes(t *testing.T) {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("settings.page_size", "11")
		c.Locals("settings.dash_page_size", "33")
		return c.Next()
	})
	app.Use(PaginationMiddleware())
	app.Get("/dash/posts", func(c fiber.Ctx) error {
		p := GetPagination(c)
		if p.PageSize != 33 {
			t.Fatalf("PageSize = %d, want %d", p.PageSize, 33)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/dash/posts", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}

func TestPaginationMiddlewareFallsBackToFrontendPageSizeWhenDashSettingMissing(t *testing.T) {
	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		c.Locals("settings.page_size", "11")
		return c.Next()
	})
	app.Use(PaginationMiddleware())
	app.Get("/dash/posts", func(c fiber.Ctx) error {
		p := GetPagination(c)
		if p.PageSize != 11 {
			t.Fatalf("PageSize = %d, want %d", p.PageSize, 11)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest(fiber.MethodGet, "/dash/posts", nil)
	if _, err := app.Test(req); err != nil {
		t.Fatalf("request failed: %v", err)
	}
}
