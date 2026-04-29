package site

import (
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestInjectDefaultTitle(t *testing.T) {
	dbx := newSiteTestDB(t)
	if dbx == nil {
		t.Fatal("expected test database")
	}

	t.Run("injects route title when missing", func(t *testing.T) {
		data := fiber.Map{}
		injectDefaultTitle("site.categories", data)
		if got := data["Title"]; got != "Categories - Example Blog" {
			t.Fatalf("unexpected title: %#v", got)
		}
	})

	t.Run("preserves explicit title", func(t *testing.T) {
		data := fiber.Map{"Title": "Custom Title"}
		injectDefaultTitle("site.categories", data)
		if got := data["Title"]; got != "Custom Title" {
			t.Fatalf("unexpected title: %#v", got)
		}
	})

	t.Run("ignores routes without default title", func(t *testing.T) {
		data := fiber.Map{}
		injectDefaultTitle("site.post.detail", data)
		if _, exists := data["Title"]; exists {
			t.Fatalf("expected title to stay unset, got %#v", data["Title"])
		}
	})
}
