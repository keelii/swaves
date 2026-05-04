package dash

import (
	"fmt"
	"strings"
	"testing"

	"swaves/internal/platform/updater"
)

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
			latestInfo := loadLatestVersionInfo(func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
				return tt.checkResult, tt.checkErr
			},
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

func TestBuildSystemRuntimeDiagnosticDetectsVersionMismatch(t *testing.T) {
	got := buildSystemRuntimeDiagnostic(func(path string) (string, error) {
		if path != "/home/ubuntu/swaves" {
			t.Fatalf("unexpected executable path: %q", path)
		}
		return "v0.0.77", nil
	}, "/home/ubuntu/swaves", "v0.0.74")

	if !got.Mismatch {
		t.Fatal("expected mismatch")
	}
	if got.Level != "danger" {
		t.Fatalf("Level=%q, want danger", got.Level)
	}
	if !strings.Contains(got.Message, "可执行文件版本为 v0.0.77") {
		t.Fatalf("unexpected message: %q", got.Message)
	}
	if !strings.Contains(got.Message, "当前运行中的服务仍是 v0.0.74") {
		t.Fatalf("unexpected message: %q", got.Message)
	}
}

func TestBuildSystemRuntimeDiagnosticReportsProbeFailure(t *testing.T) {
	got := buildSystemRuntimeDiagnostic(func(path string) (string, error) {
		return "", fmt.Errorf("permission denied")
	}, "/home/ubuntu/swaves", "v0.0.74")

	if got.Mismatch {
		t.Fatal("probe failure should not be version mismatch")
	}
	if got.Level != "warning" {
		t.Fatalf("Level=%q, want warning", got.Level)
	}
	if !strings.Contains(got.Message, "无法读取运行中服务对应的可执行文件版本") {
		t.Fatalf("unexpected message: %q", got.Message)
	}
}

func TestParseSystemExecutableVersionOutput(t *testing.T) {
	got := parseSystemExecutableVersionOutput("swaves v0.0.77\ncommit: abc\n")
	if got != "v0.0.77" {
		t.Fatalf("version=%q, want v0.0.77", got)
	}
}

func TestSystemUpdateSupportStateWithoutDaemon(t *testing.T) {
	state := systemUpdateSupportState(func() (updater.RuntimeInfo, error) {
		return updater.RuntimeInfo{}, fmt.Errorf("daemon mode is not active")
	}, true)
	if !state.ManualUpdateEnabled {
		t.Fatal("expected manual update to be enabled without daemon mode")
	}
	if !state.AutoUpdateEnabled {
		t.Fatal("expected auto update to stay enabled without daemon mode")
	}
	if state.RestartEnabled {
		t.Fatal("expected restart to be disabled without daemon mode")
	}
	if state.RuntimeStatusMessage == "" {
		t.Fatal("expected runtime status message without daemon mode")
	}
	if !strings.Contains(state.RuntimeStatusMessage, "daemon mode is not active") {
		t.Fatalf("unexpected runtime status message: %q", state.RuntimeStatusMessage)
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
