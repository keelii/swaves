package view

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/db"

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

func TestMiniJinjaViewLoadTemplatesFromFS(t *testing.T) {
	view, _ := NewViewEngineFS(os.DirFS(testTemplateRoot()), false)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates from fs failed: %v", err)
	}
}

func TestNewURLForResolver(t *testing.T) {
	app := fiber.New()
	app.Get("/settings/all", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("dash.settings.all")
	app.Get("/posts/:id/edit", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("dash.posts.edit")

	resolver := newURLForResolver(app)

	postEditURL, err := resolver("dash.posts.edit", map[string]string{
		"id":  "12",
		"tab": "comments",
	}, nil)
	if err != nil {
		t.Fatalf("resolve dash.posts.edit failed: %v", err)
	}
	if postEditURL != "/posts/12/edit?tab=comments" {
		t.Fatalf("unexpected dash.posts.edit url: %s", postEditURL)
	}

	settingsURL, err := resolver("dash.settings.all", map[string]string{
		"area":    "backend",
		"section": "editor",
	}, nil)
	if err != nil {
		t.Fatalf("resolve dash.settings.all failed: %v", err)
	}
	if settingsURL != "/settings/all?area=backend&section=editor" {
		t.Fatalf("unexpected dash.settings.all url: %s", settingsURL)
	}

	if _, err := resolver("dash.posts.edit", map[string]string{}, nil); err == nil {
		t.Fatalf("expected missing route param error")
	}
	if _, err := resolver("dash.not_found", nil, nil); err == nil {
		t.Fatalf("expected route not found error")
	}
}

func TestMaterializeCurrentThemeCacheWritesFlatThemeFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "data.sqlite")
	templateRoot := t.TempDir()
	model := db.Open(db.Options{DSN: dbPath})
	t.Cleanup(func() {
		_ = model.Close()
	})
	if err := os.MkdirAll(filepath.Join(templateRoot, "include"), 0o755); err != nil {
		t.Fatalf("create include root failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, "include", "favicon.html"), []byte("favicon"), 0o644); err != nil {
		t.Fatalf("write favicon include failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateRoot, "include", "math.html"), []byte("math"), 0o644); err != nil {
		t.Fatalf("write math include failed: %v", err)
	}

	files, err := json.Marshal(map[string]string{
		"site/home.html":           "legacy-home",
		"site/include/footer.html": "footer",
		"site/layout/layout.html":  "layout",
		"site/macro/content.html":  "macro",
		"site/layout/archive.html": "archive-layout",
	})
	if err != nil {
		t.Fatalf("marshal theme files failed: %v", err)
	}

	theme := &db.Theme{
		Name:        "Current",
		Code:        "current",
		Author:      "tester",
		Files:       string(files),
		CurrentFile: "home.html",
		Status:      "published",
		Version:     1,
	}
	if _, err := db.CreateTheme(model, theme); err != nil {
		t.Fatalf("CreateTheme failed: %v", err)
	}
	if err := db.SetThemeCurrent(model, theme.ID); err != nil {
		t.Fatalf("SetThemeCurrent failed: %v", err)
	}

	cacheRoot, err := MaterializeCurrentThemeCache(model, dbPath, templateRoot, nil)
	if err != nil {
		t.Fatalf("MaterializeCurrentThemeCache failed: %v", err)
	}

	assertCachedThemeFile := func(name string, want string) {
		t.Helper()
		got, err := os.ReadFile(filepath.Join(cacheRoot, name))
		if err != nil {
			t.Fatalf("read cached theme file %s failed: %v", name, err)
		}
		if string(got) != want {
			t.Fatalf("cached theme file %s = %q, want %q", name, string(got), want)
		}
	}

	assertCachedThemeFile("home.html", "legacy-home")
	assertCachedThemeFile("inc_footer.html", "footer")
	assertCachedThemeFile("layout_main.html", "layout")
	assertCachedThemeFile("macro_content.html", "macro")
	assertCachedThemeFile("layout_archive.html", "archive-layout")
	assertCachedThemeFile(filepath.Join("include", "favicon.html"), "favicon")
	assertCachedThemeFile(filepath.Join("include", "math.html"), "math")

	cacheBase, err := ResolveThemeCacheRoot(dbPath)
	if err != nil {
		t.Fatalf("ResolveThemeCacheRoot failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(cacheBase, "stale-theme"), 0o755); err != nil {
		t.Fatalf("create stale theme dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cacheBase, "stale-theme", "home.html"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale theme file failed: %v", err)
	}

	cacheRoot, err = MaterializeCurrentThemeCache(model, dbPath, templateRoot, nil)
	if err != nil {
		t.Fatalf("MaterializeCurrentThemeCache second pass failed: %v", err)
	}
	if cacheRoot == "" {
		t.Fatal("expected cache root on second materialize")
	}
	if _, err := os.Stat(filepath.Join(cacheBase, "stale-theme", "home.html")); !os.IsNotExist(err) {
		t.Fatalf("stale sibling cache should be removed on rematerialize, got err=%v", err)
	}
}

func TestResetThemeCacheRootClearsSiblingThemes(t *testing.T) {
	cacheRoot := filepath.Join(t.TempDir(), ".cache", "themes")
	targetRoot := filepath.Join(cacheRoot, "current")
	siblingRoot := filepath.Join(cacheRoot, "builtin")
	if err := os.MkdirAll(targetRoot, 0o755); err != nil {
		t.Fatalf("create target root failed: %v", err)
	}
	if err := os.MkdirAll(siblingRoot, 0o755); err != nil {
		t.Fatalf("create sibling root failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetRoot, "home.html"), []byte("old"), 0o644); err != nil {
		t.Fatalf("write target file failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(siblingRoot, "home.html"), []byte("sibling"), 0o644); err != nil {
		t.Fatalf("write sibling file failed: %v", err)
	}

	if err := resetThemeCacheRoot(cacheRoot, targetRoot); err != nil {
		t.Fatalf("resetThemeCacheRoot failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(siblingRoot, "home.html")); !os.IsNotExist(err) {
		t.Fatalf("sibling cache should be cleared, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetRoot, "home.html")); !os.IsNotExist(err) {
		t.Fatalf("target cache should be cleared, got err=%v", err)
	}
}

func TestViewEngineWithSharedLoadsThemeAndIncludeTemplates(t *testing.T) {
	root := t.TempDir()
	themeRoot := filepath.Join(root, "themes", "tuft")
	includeRoot := filepath.Join(root, "include")
	if err := os.MkdirAll(themeRoot, 0o755); err != nil {
		t.Fatalf("create theme root failed: %v", err)
	}
	if err := os.MkdirAll(includeRoot, 0o755); err != nil {
		t.Fatalf("create include root failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themeRoot, "layout_main.html"), []byte(`<!doctype html><html><body>{% include "include/test.html" %}{% block content %}{% endblock %}</body></html>`), 0o644); err != nil {
		t.Fatalf("write layout failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themeRoot, "home.html"), []byte(`{% extends "layout_main.html" %}{% block content %}<h1>{{ Title }}</h1>{% endblock %}`), 0o644); err != nil {
		t.Fatalf("write home failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeRoot, "test.html"), []byte(`<span>shared</span>`), 0o644); err != nil {
		t.Fatalf("write shared include failed: %v", err)
	}

	views, _ := NewViewEngineWithShared(themeRoot, root, true)
	viewEngine, ok := views.(*FiberView)
	if !ok {
		t.Fatal("expected FiberView")
	}
	var out bytes.Buffer
	if err := viewEngine.Render(&out, "home.html", fiber.Map{"Title": "hello"}); err != nil {
		t.Fatalf("render home failed: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "<span>shared</span>") {
		t.Fatalf("expected shared include render, got: %s", rendered)
	}
	if !strings.Contains(rendered, "<h1>hello</h1>") {
		t.Fatalf("expected theme template render, got: %s", rendered)
	}
}

func TestThemeDBViewEngineWithSharedLoadsCurrentThemeAndIncludeTemplates(t *testing.T) {
	root := t.TempDir()
	includeRoot := filepath.Join(root, "include")
	if err := os.MkdirAll(includeRoot, 0o755); err != nil {
		t.Fatalf("create include root failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeRoot, "test.html"), []byte(`<span>shared</span>`), 0o644); err != nil {
		t.Fatalf("write shared include failed: %v", err)
	}

	dbPath := filepath.Join(t.TempDir(), "theme-db.sqlite")
	model := db.Open(db.Options{DSN: dbPath})
	t.Cleanup(func() {
		_ = model.Close()
	})

	files, err := json.Marshal(map[string]string{
		"layout_main.html": `<!doctype html><html><body>{% include "include/test.html" %}{% block content %}{% endblock %}</body></html>`,
		"home.html":        `{% extends "layout_main.html" %}{% block content %}<h1>{{ Title }}</h1>{% endblock %}`,
	})
	if err != nil {
		t.Fatalf("marshal theme files failed: %v", err)
	}
	if _, err := db.CreateTheme(model, &db.Theme{
		Name:        "Current",
		Code:        "current",
		Author:      "tester",
		Files:       string(files),
		CurrentFile: "home.html",
		Status:      "published",
		Version:     1,
	}); err != nil {
		t.Fatalf("CreateTheme failed: %v", err)
	}
	theme, err := db.GetThemeByCode(model, "current")
	if err != nil {
		t.Fatalf("GetThemeByCode(current) failed: %v", err)
	}
	if err := db.SetThemeCurrent(model, theme.ID); err != nil {
		t.Fatalf("SetThemeCurrent(current) failed: %v", err)
	}

	views, _ := NewThemeDBViewEngineWithShared(model, root, true)
	viewEngine, ok := views.(*FiberView)
	if !ok {
		t.Fatal("expected FiberView")
	}

	var out bytes.Buffer
	if err := viewEngine.Render(&out, "home.html", fiber.Map{"Title": "hello"}); err != nil {
		t.Fatalf("render home failed: %v", err)
	}
	rendered := out.String()
	if !strings.Contains(rendered, "<span>shared</span>") {
		t.Fatalf("expected shared include render, got: %s", rendered)
	}
	if !strings.Contains(rendered, "<h1>hello</h1>") {
		t.Fatalf("expected db theme template render, got: %s", rendered)
	}
}
