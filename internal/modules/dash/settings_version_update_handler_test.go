package dash

import (
	"fmt"
	"testing"

	"swaves/internal/platform/updater"
)

func TestLoadLatestVersionInfoPrefersFreshReleaseCheck(t *testing.T) {
	original := checkLatestRelease
	checkLatestRelease = func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
		return updater.CheckResult{
			LatestVersion:    "v0.0.13",
			LatestReleaseURL: "https://github.com/keelii/swaves/releases/tag/v0.0.13",
		}, nil
	}
	defer func() {
		checkLatestRelease = original
	}()

	latestVersion, latestReleaseURL := loadLatestVersionInfo(
		"v0.0.12",
		"linux",
		"amd64",
		"v0.0.11",
		"https://github.com/keelii/swaves/releases/tag/v0.0.11",
	)

	if latestVersion != "v0.0.13" {
		t.Fatalf("latestVersion = %q, want %q", latestVersion, "v0.0.13")
	}
	if latestReleaseURL != "https://github.com/keelii/swaves/releases/tag/v0.0.13" {
		t.Fatalf("latestReleaseURL = %q", latestReleaseURL)
	}
}

func TestLoadLatestVersionInfoFallsBackWhenReleaseCheckFails(t *testing.T) {
	original := checkLatestRelease
	checkLatestRelease = func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
		return updater.CheckResult{}, fmt.Errorf("boom")
	}
	defer func() {
		checkLatestRelease = original
	}()

	latestVersion, latestReleaseURL := loadLatestVersionInfo(
		"v0.0.12",
		"linux",
		"amd64",
		"v0.0.11",
		"https://github.com/keelii/swaves/releases/tag/v0.0.11",
	)

	if latestVersion != "v0.0.11" {
		t.Fatalf("latestVersion = %q, want %q", latestVersion, "v0.0.11")
	}
	if latestReleaseURL != "https://github.com/keelii/swaves/releases/tag/v0.0.11" {
		t.Fatalf("latestReleaseURL = %q", latestReleaseURL)
	}
}
