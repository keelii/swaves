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
			LatestReleaseURL: updater.ReleaseTagURL("v0.0.13"),
		}, nil
	}
	defer func() {
		checkLatestRelease = original
	}()

	latestInfo := loadLatestVersionInfo(
		"v0.0.12",
		"linux",
		"amd64",
		"v0.0.11",
		updater.ReleaseTagURL("v0.0.11"),
	)

	if latestInfo.Version != "v0.0.13" {
		t.Fatalf("latestVersion = %q, want %q", latestInfo.Version, "v0.0.13")
	}
	if latestInfo.ReleaseURL != updater.ReleaseTagURL("v0.0.13") {
		t.Fatalf("latestReleaseURL = %q", latestInfo.ReleaseURL)
	}
	if !latestInfo.HasSystemUpdate {
		t.Fatal("expected system update to be detected when latest version is newer")
	}
	if !latestInfo.AutoUpdateEnabled {
		t.Fatal("expected auto update to stay enabled when latest version is newer")
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

	latestInfo := loadLatestVersionInfo(
		"v0.0.12",
		"linux",
		"amd64",
		"v0.0.11",
		updater.ReleaseTagURL("v0.0.11"),
	)

	if latestInfo.Version != "v0.0.11" {
		t.Fatalf("latestVersion = %q, want %q", latestInfo.Version, "v0.0.11")
	}
	if latestInfo.ReleaseURL != updater.ReleaseTagURL("v0.0.11") {
		t.Fatalf("latestReleaseURL = %q", latestInfo.ReleaseURL)
	}
	if !latestInfo.HasSystemUpdate {
		t.Fatal("expected system update to stay detected when fallback version differs from current version")
	}
	if !latestInfo.AutoUpdateEnabled {
		t.Fatal("expected auto update to stay enabled when fallback version is older")
	}
}

func TestLoadLatestVersionInfoDisablesAutoUpdateWhenAlreadyLatest(t *testing.T) {
	original := checkLatestRelease
	checkLatestRelease = func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
		return updater.CheckResult{
			LatestVersion:    "v0.0.15",
			LatestReleaseURL: updater.ReleaseTagURL("v0.0.15"),
		}, nil
	}
	defer func() {
		checkLatestRelease = original
	}()

	latestInfo := loadLatestVersionInfo(
		"v0.0.15",
		"linux",
		"amd64",
		"v0.0.14",
		updater.ReleaseTagURL("v0.0.14"),
	)

	if latestInfo.Version != "v0.0.15" {
		t.Fatalf("latestVersion = %q, want %q", latestInfo.Version, "v0.0.15")
	}
	if latestInfo.HasSystemUpdate {
		t.Fatal("expected system update to be disabled when current version already matches latest version")
	}
	if latestInfo.AutoUpdateEnabled {
		t.Fatal("expected auto update to be disabled when current version already matches latest version")
	}
}

func TestBuildSystemUpdateNoticeRequiresManualRestartWhenNoMasterRestart(t *testing.T) {
	got := buildSystemUpdateNotice(updater.InstallResult{
		Installed:     true,
		LatestVersion: "v0.0.17",
	})
	want := "已安装 v0.0.17，请手动重启服务后生效。"
	if got != want {
		t.Fatalf("buildSystemUpdateNotice() = %q, want %q", got, want)
	}
}

func TestSystemUpdateSupportStateDisablesManualUpdateWithoutDaemon(t *testing.T) {
	t.Setenv("XDG_CACHE_HOME", t.TempDir())

	state := systemUpdateSupportState(true)
	if state.ManualUpdateEnabled {
		t.Fatal("expected manual update to be disabled without daemon mode")
	}
	if state.AutoUpdateEnabled {
		t.Fatal("expected auto update to be disabled without daemon mode")
	}
	if state.RestartEnabled {
		t.Fatal("expected restart to be disabled without daemon mode")
	}
}

func TestParseRefreshDelaySeconds(t *testing.T) {
	tests := []struct {
		raw  string
		want int
	}{
		{raw: "", want: 0},
		{raw: "0", want: 0},
		{raw: "-1", want: 0},
		{raw: "1", want: 1},
		{raw: "5", want: 5},
		{raw: " 3 ", want: 3},
		{raw: "abc", want: 0},
	}

	for _, tt := range tests {
		if got := parseRefreshDelaySeconds(tt.raw); got != tt.want {
			t.Fatalf("parseRefreshDelaySeconds(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}
