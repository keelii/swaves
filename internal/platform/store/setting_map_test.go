package store

import (
	"testing"

	"swaves/internal/platform/db"
)

func TestGetSettingDoesNotFallbackWhenSettingsMapIsEmpty(t *testing.T) {
	if len(db.DefaultSettings) < 1 {
		t.Fatalf("expected default settings to exist")
	}
	code := db.DefaultSettings[0].Code
	if code == "" {
		t.Fatalf("expected first default setting code to be non-empty")
	}

	storeSettingsMap(map[string]string{})
	got := GetSetting(code)
	if got != "" {
		t.Fatalf("GetSetting(%q) = %q, want empty string when settings map is empty", code, got)
	}
}

func TestIsSettingMapEmptyTracksStoredMap(t *testing.T) {
	storeSettingsMap(map[string]string{})
	if !IsSettingEmpty() {
		t.Fatalf("expected settings map to be empty")
	}

	storeSettingsMap(map[string]string{"site_name": "swaves"})
	if IsSettingEmpty() {
		t.Fatalf("expected settings map to be non-empty")
	}
}
