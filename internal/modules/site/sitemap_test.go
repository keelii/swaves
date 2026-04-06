package site

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/platform/store"

	"github.com/gofiber/fiber/v3"
)

func newSiteTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "site.sqlite")
	dbx := db.Open(db.Options{DSN: dbPath})
	t.Cleanup(func() {
		_ = dbx.Close()
	})

	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("ensure default settings failed: %v", err)
	}
	if err := db.UpdateSettingByCode(dbx, "site_url", "https://example.com"); err != nil {
		t.Fatalf("update site_url failed: %v", err)
	}
	if err := db.UpdateSettingByCode(dbx, "site_title", "Example Blog"); err != nil {
		t.Fatalf("update site_title failed: %v", err)
	}
	if err := db.UpdateSettingByCode(dbx, "post_url_prefix", ""); err != nil {
		t.Fatalf("update post_url_prefix failed: %v", err)
	}
	if err := store.ReloadSettings(&store.GlobalStore{Model: dbx}); err != nil {
		t.Fatalf("reload settings failed: %v", err)
	}

	return dbx
}

func TestGetSitemapIncludesPublishedContentURLs(t *testing.T) {
	dbx := newSiteTestDB(t)

	post := &db.Post{
		Title:       "Hello World",
		Slug:        "hello-world",
		Content:     "This is a post.",
		Status:      "published",
		Kind:        db.PostKindPost,
		PublishedAt: 1735603200,
		UpdatedAt:   1735689600,
	}
	if _, err := db.CreatePost(dbx, post); err != nil {
		t.Fatalf("create post failed: %v", err)
	}

	page := &db.Post{
		Title:       "About",
		Slug:        "about",
		Content:     "About page.",
		Status:      "published",
		Kind:        db.PostKindPage,
		PublishedAt: 1735603200,
	}
	if _, err := db.CreatePost(dbx, page); err != nil {
		t.Fatalf("create page failed: %v", err)
	}

	if _, err := db.CreateCategory(dbx, &db.Category{Name: "Go", Slug: "go"}); err != nil {
		t.Fatalf("create category failed: %v", err)
	}
	if _, err := db.CreateTag(dbx, &db.Tag{Name: "Fiber", Slug: "fiber"}); err != nil {
		t.Fatalf("create tag failed: %v", err)
	}

	app := fiber.New()
	handler := Handler{Model: dbx}
	app.Get("/sitemap.xml", handler.GetSitemap)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/sitemap.xml", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request sitemap failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read sitemap failed: %v", err)
	}
	text := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, text)
	}
	for _, want := range []string{
		"<loc>https://example.com/</loc>",
		"<loc>https://example.com/about</loc>",
		"<loc>https://example.com/hello-world</loc>",
		"<loc>https://example.com/categories</loc>",
		"<loc>https://example.com/categories/go</loc>",
		"<loc>https://example.com/tags</loc>",
		"<loc>https://example.com/tags/fiber</loc>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected sitemap to contain %q, got %s", want, text)
		}
	}
	if !strings.Contains(text, "<lastmod>") {
		t.Fatalf("expected sitemap lastmod entries, got %s", text)
	}
}

func TestGetRobotsIncludesSitemapDirective(t *testing.T) {
	dbx := newSiteTestDB(t)

	app := fiber.New()
	handler := Handler{Model: dbx}
	app.Get("/robots.txt", handler.GetRobots)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/robots.txt", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request robots failed: %v", err)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read robots failed: %v", err)
	}
	text := string(body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d body=%s", resp.StatusCode, text)
	}
	if !strings.Contains(text, "User-agent: *") {
		t.Fatalf("expected robots user-agent directive, got %s", text)
	}
	if !strings.Contains(text, "Sitemap: https://example.com/sitemap.xml") {
		t.Fatalf("expected robots sitemap directive, got %s", text)
	}
}
