package dash

import "testing"

func TestBuildInstallSettingViewsAppliesPlaceholderOverride(t *testing.T) {
	original := append([]installSettingPresentationOverride(nil), installSettingPresentationOverrides...)
	t.Cleanup(func() {
		installSettingPresentationOverrides = original
	})

	installSettingPresentationOverrides = append([]installSettingPresentationOverride(nil), installSettingPresentationOverrides...)
	for i := range installSettingPresentationOverrides {
		if installSettingPresentationOverrides[i].Code != "site_desc" {
			continue
		}
		installSettingPresentationOverrides[i].Placeholder = "一句话描述你的站点"
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
	original := append([]installSettingPresentationOverride(nil), installSettingPresentationOverrides...)
	t.Cleanup(func() {
		installSettingPresentationOverrides = original
	})

	installSettingPresentationOverrides = append([]installSettingPresentationOverride(nil), installSettingPresentationOverrides...)
	for i := range installSettingPresentationOverrides {
		switch installSettingPresentationOverrides[i].Code {
		case "site_desc":
			installSettingPresentationOverrides[i].DefaultValue = "A default site description"
		case "site_url":
			installSettingPresentationOverrides[i].DefaultValue = "https://override.example"
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
