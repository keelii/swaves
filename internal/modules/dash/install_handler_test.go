package dash

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/platform/updater"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

func withInstallSettingsOptions(t *testing.T) {
	t.Helper()

	original := append([]InstallSettingsOption(nil), installSettings...)
	t.Cleanup(func() {
		installSettings = original
	})
	installSettings = append([]InstallSettingsOption(nil), installSettings...)
}

func TestBuildInstallSettingViewsAppliesPlaceholderOverride(t *testing.T) {
	withInstallSettingsOptions(t)
	for i := range installSettings {
		if installSettings[i].Code != "site_desc" {
			continue
		}
		installSettings[i].Placeholder = "一句话描述你的站点"
	}

	views := buildInstallSettingViews(cloneInstallDefaultSettingsWithSiteURL(""))

	for _, view := range views {
		if view.Code != "site_desc" {
			continue
		}
		if view.Placeholder != "一句话描述你的站点" {
			t.Fatalf("expected site_desc placeholder override, got %q", view.Placeholder)
		}
		return
	}

	t.Fatal("expected site_desc install view")
}

func TestCloneInstallDefaultSettingsWithSiteURLAppliesPresentationDefaultValue(t *testing.T) {
	withInstallSettingsOptions(t)
	for i := range installSettings {
		switch installSettings[i].Code {
		case "site_desc":
			installSettings[i].DefaultValue = "A default site description"
		case "site_url":
			installSettings[i].DefaultValue = "https://override.example"
		}
	}

	settings := cloneInstallDefaultSettingsWithSiteURL("https://current.example")

	if got := installSettingValue(settings, "site_desc"); got != "A default site description" {
		t.Fatalf("expected site_desc default override, got %q", got)
	}
	if got := installSettingValue(settings, "site_url"); got != "https://current.example" {
		t.Fatalf("expected current page site_url to win over default override, got %q", got)
	}
}

func TestPostInstallHandlerRestartsAndRedirectsToConfiguredDashPath(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "install-restart.sqlite")})
	t.Cleanup(func() { _ = dbx.Close() })

	sessionStore := &types.SessionStore{Store: session.NewStore()}
	handler := &Handler{Model: dbx, Session: sessionStore}

	restartCalls := 0
	currentExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	originalRead := readActiveRuntimeInfo
	originalRestart := restartActiveRuntime
	readActiveRuntimeInfo = func() (updater.RuntimeInfo, error) {
		return updater.RuntimeInfo{PID: 4321, Executable: currentExecutable}, nil
	}
	restartActiveRuntime = func() (int, error) {
		restartCalls++
		return 4321, nil
	}
	t.Cleanup(func() {
		readActiveRuntimeInfo = originalRead
		restartActiveRuntime = originalRestart
	})

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		fiber.Locals(c, "DashSessionStore", sessionStore)
		return c.Next()
	})
	app.Get("/dash", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }).Name("dash.home")
	app.Get("/dash/login", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }).Name("dash.login.show")
	app.Post("/install", handler.PostInstallHandler)

	form := url.Values{}
	form.Set("setting_dash_password", "install-secret")
	form.Set("setting_dash_path", "/console")
	req := httptest.NewRequest(fiber.MethodPost, "/install", strings.NewReader(form.Encode()))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected redirect status, got %d", resp.StatusCode)
	}
	if location := strings.TrimSpace(resp.Header.Get("Location")); location != "/console" {
		t.Fatalf("unexpected redirect location: got %q want %q", location, "/console")
	}
	if restartCalls != 1 {
		t.Fatalf("restartActiveRuntime calls = %d, want 1", restartCalls)
	}
	if cookie := strings.TrimSpace(resp.Header.Get("Set-Cookie")); cookie == "" {
		t.Fatal("expected install response to set session cookie")
	}
}

func TestPostInstallHandlerFallsBackToCurrentDashRouteWhenRestartUnavailable(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "install-restart-missing.sqlite")})
	t.Cleanup(func() { _ = dbx.Close() })

	sessionStore := &types.SessionStore{Store: session.NewStore()}
	handler := &Handler{Model: dbx, Session: sessionStore}

	restartCalls := 0
	currentExecutable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	originalRead := readActiveRuntimeInfo
	originalRestart := restartActiveRuntime
	readActiveRuntimeInfo = func() (updater.RuntimeInfo, error) {
		return updater.RuntimeInfo{PID: 4321, Executable: currentExecutable}, nil
	}
	restartActiveRuntime = func() (int, error) {
		restartCalls++
		return 0, fiber.ErrServiceUnavailable
	}
	t.Cleanup(func() {
		readActiveRuntimeInfo = originalRead
		restartActiveRuntime = originalRestart
	})

	app := fiber.New()
	app.Use(func(c fiber.Ctx) error {
		fiber.Locals(c, "DashSessionStore", sessionStore)
		return c.Next()
	})
	app.Get("/dash", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }).Name("dash.home")
	app.Get("/dash/login", func(c fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }).Name("dash.login.show")
	app.Post("/install", handler.PostInstallHandler)

	form := url.Values{}
	form.Set("setting_dash_password", "install-secret")
	form.Set("setting_dash_path", "/console")
	req := httptest.NewRequest(fiber.MethodPost, "/install", strings.NewReader(form.Encode()))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected redirect status, got %d", resp.StatusCode)
	}
	if location := strings.TrimSpace(resp.Header.Get("Location")); location != "/dash" {
		t.Fatalf("unexpected redirect location: got %q want %q", location, "/dash")
	}
	if restartCalls != 1 {
		t.Fatalf("restartActiveRuntime calls = %d, want 1", restartCalls)
	}
}
