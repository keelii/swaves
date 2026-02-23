package view

import (
	"path/filepath"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func testTemplateRoot() string {
	return filepath.Clean(filepath.Join("..", "..", "..", "web", "templates"))
}

func TestMiniJinjaViewLoadTemplates(t *testing.T) {
	view, _ := NewViewEngine(testTemplateRoot(), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}
}

func TestNewURLForResolver(t *testing.T) {
	app := fiber.New()
	app.Get("/settings/all", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("admin.settings.all")
	app.Get("/posts/:id/edit", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("admin.posts.edit")

	resolver := newURLForResolver(app)

	postEditURL, err := resolver("admin.posts.edit", map[string]string{
		"id":  "12",
		"tab": "comments",
	}, nil)
	if err != nil {
		t.Fatalf("resolve admin.posts.edit failed: %v", err)
	}
	if postEditURL != "/posts/12/edit?tab=comments" {
		t.Fatalf("unexpected admin.posts.edit url: %s", postEditURL)
	}

	settingsURL, err := resolver("admin.settings.all", map[string]string{
		"kind": "third_party_services",
	}, nil)
	if err != nil {
		t.Fatalf("resolve admin.settings.all failed: %v", err)
	}
	if settingsURL != "/settings/all?kind=third_party_services" {
		t.Fatalf("unexpected admin.settings.all url: %s", settingsURL)
	}

	if _, err := resolver("admin.posts.edit", map[string]string{}, nil); err == nil {
		t.Fatalf("expected missing route param error")
	}
	if _, err := resolver("admin.not_found", nil, nil); err == nil {
		t.Fatalf("expected route not found error")
	}
}
