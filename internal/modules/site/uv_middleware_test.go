package site

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

func testVisitorID(seed byte) string {
	return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, db.UVVisitorIDBytes))
}

func mustCountUVUnique(t *testing.T, dbx *db.DB, entityType db.UVEntityType, entityID int64) int {
	t.Helper()

	count, err := db.CountUVUnique(dbx, entityType, entityID)
	if err != nil {
		t.Fatalf("count uv failed: %v", err)
	}
	return count
}

func mustCountUVRowsForEntityType(t *testing.T, dbx *db.DB, entityType db.UVEntityType) int {
	t.Helper()

	var count int
	if err := dbx.QueryRow(
		`SELECT COUNT(*) FROM `+string(db.TableUVUnique)+` WHERE entity_type = ?`,
		entityType,
	).Scan(&count); err != nil {
		t.Fatalf("count uv rows failed: %v", err)
	}
	return count
}

func performSiteTestRequest(t *testing.T, app *fiber.App, path string, visitorID string) {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if visitorID != "" {
		req.AddCookie(&http.Cookie{Name: middleware.DefaultVisitorIDCookieName, Value: visitorID})
	}

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request %s failed: %v", path, err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
}

func TestTrackSiteUVMiddlewareTracksNamedSitePagesOnly(t *testing.T) {
	dbx := newSiteTestDB(t)
	handler := Handler{Model: dbx}

	app := fiber.New()
	uiGroup := app.Group("/")
	uiGroup.Use(middleware.EnsureVisitorID(""))
	uiGroup.Use(handler.trackSiteUVMiddleware())

	uiGroup.Get("/", func(c fiber.Ctx) error {
		return c.SendString("home")
	}).Name("site.home")
	uiGroup.Get("/categories", func(c fiber.Ctx) error {
		return c.SendString("categories")
	}).Name("site.categories")
	uiGroup.Get("/tags", func(c fiber.Ctx) error {
		return c.SendString("tags")
	}).Name("site.tags")
	uiGroup.Get("/404", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusNotFound).SendString("not found")
	}).Name("site.not_found")
	uiGroup.Get("/robots.txt", func(c fiber.Ctx) error {
		return c.SendString("robots")
	}).Name("site.robots")

	visitorA := testVisitorID(1)
	visitorB := testVisitorID(2)
	visitorC := testVisitorID(3)

	performSiteTestRequest(t, app, "/", visitorA)
	if got := mustCountUVUnique(t, dbx, db.UVEntitySite, 0); got != 1 {
		t.Fatalf("site uv after home = %d, want 1", got)
	}

	performSiteTestRequest(t, app, "/categories", visitorA)
	performSiteTestRequest(t, app, "/", visitorA)
	if got := mustCountUVUnique(t, dbx, db.UVEntitySite, 0); got != 1 {
		t.Fatalf("site uv after duplicate site page visits = %d, want 1", got)
	}

	performSiteTestRequest(t, app, "/tags", visitorB)
	if got := mustCountUVUnique(t, dbx, db.UVEntitySite, 0); got != 2 {
		t.Fatalf("site uv after second visitor = %d, want 2", got)
	}

	performSiteTestRequest(t, app, "/404", visitorC)
	performSiteTestRequest(t, app, "/robots.txt", visitorC)
	if got := mustCountUVUnique(t, dbx, db.UVEntitySite, 0); got != 2 {
		t.Fatalf("site uv after excluded routes = %d, want 2", got)
	}
}

func TestTrackSiteUVMiddlewareTracksHandlerDeclaredEntitiesOnlyOnSuccess(t *testing.T) {
	dbx := newSiteTestDB(t)
	handler := Handler{Model: dbx}

	app := fiber.New()
	uiGroup := app.Group("/")
	uiGroup.Use(middleware.EnsureVisitorID(""))
	uiGroup.Use(handler.trackSiteUVMiddleware())

	uiGroup.Get("/posts/:id", func(c fiber.Ctx) error {
		declareTrackUVEntity(c, db.UVEntityPost, 101)
		return c.SendString("post")
	}).Name("site.post.detail")
	uiGroup.Get("/categories/:slug", func(c fiber.Ctx) error {
		declareTrackUVEntity(c, db.UVEntityCategory, 201)
		return c.SendString("category")
	}).Name("site.category.detail")
	uiGroup.Get("/tags/:slug", func(c fiber.Ctx) error {
		declareTrackUVEntity(c, db.UVEntityTag, 301)
		return c.Status(fiber.StatusNotFound).SendString("missing")
	}).Name("site.tag.detail")

	visitorA := testVisitorID(11)
	visitorB := testVisitorID(12)
	visitorC := testVisitorID(13)

	performSiteTestRequest(t, app, "/posts/101", visitorA)
	performSiteTestRequest(t, app, "/posts/101", visitorA)
	if got := mustCountUVUnique(t, dbx, db.UVEntityPost, 101); got != 1 {
		t.Fatalf("post uv after duplicate visitor = %d, want 1", got)
	}
	if got := mustCountUVRowsForEntityType(t, dbx, db.UVEntitySite); got != 0 {
		t.Fatalf("site entity rows for detail-only requests = %d, want 0", got)
	}

	performSiteTestRequest(t, app, "/categories/go", visitorB)
	if got := mustCountUVUnique(t, dbx, db.UVEntityCategory, 201); got != 1 {
		t.Fatalf("category uv = %d, want 1", got)
	}

	performSiteTestRequest(t, app, "/tags/missing", visitorC)
	if got := mustCountUVUnique(t, dbx, db.UVEntityTag, 301); got != 0 {
		t.Fatalf("tag uv after not found response = %d, want 0", got)
	}
}
