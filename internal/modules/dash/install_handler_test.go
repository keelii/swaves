package dash

import "testing"

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
