package dash

import (
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

func TestCloneInstallDefaultSettingsIncludesCrawlerBlockSetting(t *testing.T) {
	settings := cloneInstallDefaultSettingsWithSiteURL("")

	for _, setting := range settings {
		if setting.Code != db.SettingCodeBlockSearchEngineCrawlers {
			continue
		}
		if setting.Type != "checkbox" {
			t.Fatalf("expected crawler block setting type checkbox, got %q", setting.Type)
		}
		if strings.TrimSpace(setting.Value) != "" {
			t.Fatalf("expected crawler block setting default empty, got %q", setting.Value)
		}
		return
	}

	t.Fatalf("expected install settings to include %q", db.SettingCodeBlockSearchEngineCrawlers)
}

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

func TestApplyInstallFormValuesStoresCrawlerBlockCheckbox(t *testing.T) {
	app := fiber.New()
	handler := func(c fiber.Ctx) error {
		settings := applyInstallFormValues(c, cloneInstallDefaultSettingsWithSiteURL("https://example.com"))
		got := installSettingValue(settings, db.SettingCodeBlockSearchEngineCrawlers)
		if got != "1" {
			t.Fatalf("expected crawler block setting value 1, got %q", got)
		}
		return c.SendStatus(fiber.StatusOK)
	}
	app.Post("/install", handler)

	form := url.Values{}
	form.Add("setting_"+db.SettingCodeBlockSearchEngineCrawlers, "1")
	req := httptest.NewRequest(fiber.MethodPost, "/install", strings.NewReader(form.Encode()))
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}
