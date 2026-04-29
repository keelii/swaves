package site

import (
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestInjectDefaultTitle(t *testing.T) {
	t.Run("injects route title when missing", func(t *testing.T) {
		data := fiber.Map{}
		injectDefaultTitle("site.categories", "list.html", data)
		if got := data["Title"]; got != buildPageTitle("Categories") {
			t.Fatalf("unexpected title: %#v", got)
		}
	})

	t.Run("preserves explicit title", func(t *testing.T) {
		data := fiber.Map{"Title": "Custom Title"}
		injectDefaultTitle("site.categories", "list.html", data)
		if got := data["Title"]; got != "Custom Title" {
			t.Fatalf("unexpected title: %#v", got)
		}
	})

	t.Run("ignores routes without default title", func(t *testing.T) {
		data := fiber.Map{}
		injectDefaultTitle("site.post.detail", "post.html", data)
		if _, exists := data["Title"]; exists {
			t.Fatalf("expected title to stay unset, got %#v", data["Title"])
		}
	})

	t.Run("falls back to view title for shared error pages", func(t *testing.T) {
		data := fiber.Map{}
		injectDefaultTitle("site.post.detail", "404.html", data)
		if got := data["Title"]; got != siteNotFoundTitle {
			t.Fatalf("unexpected fallback title: %#v", got)
		}
	})
}
