package dash

import (
	"fmt"
	"testing"

	"swaves/internal/platform/updater"
)

func withLatestReleaseCheck(t *testing.T, fn func(currentVersion string, goos string, goarch string) (updater.CheckResult, error)) {
	t.Helper()

	original := checkLatestRelease
	checkLatestRelease = fn
	t.Cleanup(func() {
		checkLatestRelease = original
	})
}

func TestLoadLatestVersionInfo(t *testing.T) {
	tests := []struct {
		name                  string
		currentVersion        string
		fallbackVersion       string
		checkResult           updater.CheckResult
		checkErr              error
		wantVersion           string
		wantReleaseURL        string
		wantHasSystemUpdate   bool
		wantAutoUpdateEnabled bool
	}{
		{
			name:            "prefers fresh release check",
			currentVersion:  "v0.0.12",
			fallbackVersion: "v0.0.11",
			checkResult: updater.CheckResult{
				LatestVersion:    "v0.0.13",
				LatestReleaseURL: updater.ReleaseTagURL("v0.0.13"),
			},
			wantVersion:           "v0.0.13",
			wantReleaseURL:        updater.ReleaseTagURL("v0.0.13"),
			wantHasSystemUpdate:   true,
			wantAutoUpdateEnabled: true,
		},
		{
			name:                  "falls back when release check fails",
			currentVersion:        "v0.0.12",
			fallbackVersion:       "v0.0.11",
			checkErr:              fmt.Errorf("boom"),
			wantVersion:           "v0.0.11",
			wantReleaseURL:        updater.ReleaseTagURL("v0.0.11"),
			wantHasSystemUpdate:   true,
			wantAutoUpdateEnabled: true,
		},
		{
			name:            "disables auto update when already latest",
			currentVersion:  "v0.0.15",
			fallbackVersion: "v0.0.14",
			checkResult: updater.CheckResult{
				LatestVersion:    "v0.0.15",
				LatestReleaseURL: updater.ReleaseTagURL("v0.0.15"),
			},
			wantVersion:           "v0.0.15",
			wantReleaseURL:        updater.ReleaseTagURL("v0.0.15"),
			wantHasSystemUpdate:   false,
			wantAutoUpdateEnabled: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withLatestReleaseCheck(t, func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
				return tt.checkResult, tt.checkErr
			})

			latestInfo := loadLatestVersionInfo(
				tt.currentVersion,
				"linux",
				"amd64",
				tt.fallbackVersion,
				updater.ReleaseTagURL(tt.fallbackVersion),
			)

			if latestInfo.Version != tt.wantVersion {
				t.Fatalf("latestVersion = %q, want %q", latestInfo.Version, tt.wantVersion)
			}
			if latestInfo.ReleaseURL != tt.wantReleaseURL {
				t.Fatalf("latestReleaseURL = %q, want %q", latestInfo.ReleaseURL, tt.wantReleaseURL)
			}
			if latestInfo.HasSystemUpdate != tt.wantHasSystemUpdate {
				t.Fatalf("HasSystemUpdate = %v, want %v", latestInfo.HasSystemUpdate, tt.wantHasSystemUpdate)
			}
			if latestInfo.AutoUpdateEnabled != tt.wantAutoUpdateEnabled {
				t.Fatalf("AutoUpdateEnabled = %v, want %v", latestInfo.AutoUpdateEnabled, tt.wantAutoUpdateEnabled)
			}
		})
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
	original := readActiveRuntimeInfo
	readActiveRuntimeInfo = func() (updater.RuntimeInfo, error) {
		return updater.RuntimeInfo{}, fmt.Errorf("daemon mode is not active")
	}
	t.Cleanup(func() {
		readActiveRuntimeInfo = original
	})

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
