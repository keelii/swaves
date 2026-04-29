package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func withRuntimeCacheRoot(t *testing.T, root string) {
	t.Helper()

	runtimeCacheMu.Lock()
	oldRoot := runtimeCacheRoot
	runtimeCacheRoot = root
	runtimeCacheMu.Unlock()
	t.Cleanup(func() {
		runtimeCacheMu.Lock()
		runtimeCacheRoot = oldRoot
		runtimeCacheMu.Unlock()
	})
}

type updaterTestHooks struct {
	processExists     func(pid int) bool
	userCacheDir      func() (string, error)
	currentExecutable func() (string, error)
	signalProcess     func(pid int) error
}

func withUpdaterHooks(t *testing.T, hooks updaterTestHooks) {
	t.Helper()

	originalProcessExists := processExistsFunc
	originalUserCacheDir := userCacheDirFunc
	originalCurrentExecutable := currentExecutableFunc
	originalSignalProcess := signalProcessFunc

	if hooks.processExists != nil {
		processExistsFunc = hooks.processExists
	}
	if hooks.userCacheDir != nil {
		userCacheDirFunc = hooks.userCacheDir
	}
	if hooks.currentExecutable != nil {
		currentExecutableFunc = hooks.currentExecutable
	}
	if hooks.signalProcess != nil {
		signalProcessFunc = hooks.signalProcess
	}

	t.Cleanup(func() {
		processExistsFunc = originalProcessExists
		userCacheDirFunc = originalUserCacheDir
		currentExecutableFunc = originalCurrentExecutable
		signalProcessFunc = originalSignalProcess
	})
}

func mustWriteExecutable(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatalf("write executable failed: %v", err)
	}
}

func mustWriteRuntime(t *testing.T, info RuntimeInfo) {
	t.Helper()

	if err := WriteRuntimeInfo(info); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}
}

func TestInstallLatestReleaseReplacesExecutableAndSignalsMaster(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	var signaledPID atomic.Int64
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
		},
	})

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")

	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	result, err := client.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLatestRelease failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected Installed=true")
	}
	if result.RestartedPID != 4321 {
		t.Fatalf("unexpected restarted pid: %d", result.RestartedPID)
	}
	if signaledPID.Load() != 4321 {
		t.Fatalf("signal pid = %d, want 4321", signaledPID.Load())
	}

	gotBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable failed: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("unexpected executable contents: %q", string(gotBinary))
	}
}

func TestRestartActiveRuntimeSignalsMaster(t *testing.T) {
	tmpDir := t.TempDir()
	withRuntimeCacheRoot(t, tmpDir)
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: filepath.Join(tmpDir, "swaves")})

	var signaledPID atomic.Int64
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		signalProcess: func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
		},
	})

	pid, err := RestartActiveRuntime()
	if err != nil {
		t.Fatalf("RestartActiveRuntime failed: %v", err)
	}
	if pid != 4321 {
		t.Fatalf("RestartActiveRuntime pid = %d, want 4321", pid)
	}
	if signaledPID.Load() != 4321 {
		t.Fatalf("signal pid = %d, want 4321", signaledPID.Load())
	}
}

func TestInstallLatestReleaseNoOpWhenAlreadyLatest(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	mustWriteExecutable(t, executablePath, "same-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 1001, Executable: executablePath})
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 1001 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		t.Fatalf("signalProcess should not be called, got pid=%d", pid)
		return nil
		},
	})

	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://example.test/latest" {
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
			body := fmt.Sprintf(`{
				"tag_name":"v1.2.4",
				"html_url":"%s",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
				]
			}`, ReleaseTagURL("v1.2.4"))
			return newHTTPResponse(http.StatusOK, body), nil
		})},
	}

	result, err := client.InstallLatestRelease("v1.2.4", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLatestRelease failed: %v", err)
	}
	if result.Installed {
		t.Fatal("expected Installed=false")
	}
	if result.Reason != "already on latest version" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}

	gotBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable failed: %v", err)
	}
	if string(gotBinary) != "same-binary" {
		t.Fatalf("unexpected executable contents: %q", string(gotBinary))
	}
}

func TestInstallLocalReleaseArchiveReplacesExecutableAndSignalsMaster(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	var signaledPID atomic.Int64
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
		},
	})

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	archiveData := buildTarGzArchive(t, expectedReleaseBinaryName(archiveName), []byte("new-binary"))
	if err := os.WriteFile(archivePath, archiveData, 0644); err != nil {
		t.Fatalf("write local archive failed: %v", err)
	}

	result, err := InstallLocalReleaseArchive(archiveName, archivePath, "v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLocalReleaseArchive failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected Installed=true")
	}
	if result.LatestVersion != "v1.2.4" {
		t.Fatalf("LatestVersion = %q, want v1.2.4", result.LatestVersion)
	}
	if signaledPID.Load() != 4321 {
		t.Fatalf("signal pid = %d, want 4321", signaledPID.Load())
	}

	gotBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable failed: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("unexpected executable contents: %q", string(gotBinary))
	}
}

func TestInstallLocalReleaseArchiveRejectsWrongPlatform(t *testing.T) {
	if _, err := validateLocalArchiveName("swaves_v1.2.4_darwin_arm64.tar.gz", "linux", "amd64"); err == nil {
		t.Fatal("expected wrong platform archive to be rejected")
	}
}

func TestInstallLocalReleaseArchiveReplacesExecutableWithoutDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return false },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		t.Fatalf("signalProcess should not be called without daemon mode, got pid=%d", pid)
		return nil
		},
	})
	mustWriteExecutable(t, executablePath, "old-binary")

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	archiveData := buildTarGzArchive(t, expectedReleaseBinaryName(archiveName), []byte("new-binary"))
	if err := os.WriteFile(archivePath, archiveData, 0644); err != nil {
		t.Fatalf("write local archive failed: %v", err)
	}

	result, err := InstallLocalReleaseArchive(archiveName, archivePath, "v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLocalReleaseArchive failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected Installed=true")
	}
	if result.RestartedPID != 0 {
		t.Fatalf("RestartedPID = %d, want 0", result.RestartedPID)
	}
	if result.Reason != "installed v1.2.4, restart required" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}

	gotBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable failed: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("unexpected executable contents: %q", string(gotBinary))
	}
}

func TestInstallLatestReleaseCLIReplacesCurrentExecutableWithoutDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withUpdaterHooks(t, updaterTestHooks{
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
	})
	mustWriteExecutable(t, executablePath, "old-binary")

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	result, err := client.InstallLatestReleaseCLI("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLatestReleaseCLI failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected Installed=true")
	}
	if result.RestartedPID != 0 {
		t.Fatalf("RestartedPID = %d, want 0", result.RestartedPID)
	}
	if result.Reason != "installed v1.2.4 to current executable" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}

	gotBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable failed: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("unexpected executable contents: %q", string(gotBinary))
	}
}

func TestInstallLatestReleaseCLIRestartsMatchingActiveMaster(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	var signaledPID atomic.Int64
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
		},
	})

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	result, err := client.InstallLatestReleaseCLI("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLatestReleaseCLI failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected Installed=true")
	}
	if result.RestartedPID != 4321 {
		t.Fatalf("RestartedPID = %d, want 4321", result.RestartedPID)
	}
	if signaledPID.Load() != 4321 {
		t.Fatalf("signal pid = %d, want 4321", signaledPID.Load())
	}
	if result.Reason != "installed v1.2.4 to current executable and restarted master" {
		t.Fatalf("unexpected reason: %q", result.Reason)
	}

	gotBinary, err := os.ReadFile(executablePath)
	if err != nil {
		t.Fatalf("ReadFile executable failed: %v", err)
	}
	if string(gotBinary) != "new-binary" {
		t.Fatalf("unexpected executable contents: %q", string(gotBinary))
	}
}

func TestInstallLatestReleaseCLIRollsBackWhenRestartingMatchingActiveMasterFails(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		return errors.New("restart failed")
		},
	})

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	_, err := client.InstallLatestReleaseCLI("v1.2.3", "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "signal master restart failed") {
		t.Fatalf("expected signal failure, got %v", err)
	}

	gotBinary, readErr := os.ReadFile(executablePath)
	if readErr != nil {
		t.Fatalf("ReadFile executable failed: %v", readErr)
	}
	if string(gotBinary) != "old-binary" {
		t.Fatalf("unexpected executable contents after rollback: %q", string(gotBinary))
	}
}

func TestExtractReleaseBinarySkipsNonTargetFiles(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "swaves_v1.2.4_linux_amd64.tar.gz")
	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{
		{name: "README.md", content: []byte("readme")},
		{name: "swaves_v1.2.4_linux_amd64", content: []byte("new-binary")},
	})
	if err := os.WriteFile(archivePath, archiveData, 0644); err != nil {
		t.Fatalf("write archive failed: %v", err)
	}

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, "swaves_v1.2.4_linux_amd64")
	if err != nil {
		t.Fatalf("extractReleaseBinary failed: %v", err)
	}
	got, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("unexpected extracted contents: %q", string(got))
	}
}

func TestInstallLatestReleaseAllowsExecutableReplacementWhileMasterKeepsRunning(t *testing.T) {
	tmpDir := t.TempDir()
	withRuntimeCacheRoot(t, tmpDir)

	executablePath := filepath.Join(tmpDir, "swaves")
	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	var signaledPID atomic.Int64
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
		},
	})

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")

	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	result, err := client.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLatestRelease failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected Installed=true")
	}
	if signaledPID.Load() != 4321 {
		t.Fatalf("signal pid = %d, want 4321", signaledPID.Load())
	}
}

func TestInstallLatestReleaseRollsBackWhenSignalFails(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		return errors.New("restart failed")
		},
	})

	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	_, err := client.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "signal master restart failed") {
		t.Fatalf("expected signal failure, got %v", err)
	}

	gotBinary, readErr := os.ReadFile(executablePath)
	if readErr != nil {
		t.Fatalf("ReadFile executable failed: %v", readErr)
	}
	if string(gotBinary) != "old-binary" {
		t.Fatalf("unexpected executable contents after rollback: %q", string(gotBinary))
	}
}

func TestInstallLatestReleaseRejectsRuntimeChangeBeforeReplace(t *testing.T) {
	tmpDir := t.TempDir()
	executablePath := filepath.Join(tmpDir, "swaves")
	withRuntimeCacheRoot(t, tmpDir)
	signalCalled := false
	withUpdaterHooks(t, updaterTestHooks{
		processExists: func(pid int) bool { return pid == 4321 || pid == 9999 },
		currentExecutable: func() (string, error) {
			return executablePath, nil
		},
		signalProcess: func(pid int) error {
		signalCalled = true
		return nil
		},
	})

	mustWriteExecutable(t, executablePath, "old-binary")
	mustWriteRuntime(t, RuntimeInfo{PID: 4321, Executable: executablePath})

	archiveData := buildTarGzArchive(t, "swaves_v1.2.4_linux_amd64", []byte("new-binary"))
	hash := sha256.Sum256(archiveData)
	checksumData := []byte(hex.EncodeToString(hash[:]) + "  swaves_v1.2.4_linux_amd64.tar.gz\n")
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case "https://example.test/latest":
				body := fmt.Sprintf(`{
					"tag_name":"v1.2.4",
					"html_url":"%s",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`, ReleaseTagURL("v1.2.4"))
				return newHTTPResponse(http.StatusOK, body), nil
			case "https://example.test/archive":
				if err := WriteRuntimeInfo(RuntimeInfo{PID: 9999, Executable: executablePath}); err != nil {
					t.Fatalf("WriteRuntimeInfo during download failed: %v", err)
				}
				return newBinaryResponse(http.StatusOK, archiveData), nil
			case "https://example.test/archive.sha256":
				return newBinaryResponse(http.StatusOK, checksumData), nil
			default:
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
		})},
	}

	_, err := client.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "runtime pid changed") {
		t.Fatalf("expected runtime change error, got %v", err)
	}
	if signalCalled {
		t.Fatal("signalProcess should not be called when runtime changes")
	}

	gotBinary, readErr := os.ReadFile(executablePath)
	if readErr != nil {
		t.Fatalf("ReadFile executable failed: %v", readErr)
	}
	if string(gotBinary) != "old-binary" {
		t.Fatalf("unexpected executable contents after runtime change: %q", string(gotBinary))
	}
}

func buildTarGzArchive(t *testing.T, name string, content []byte) []byte {
	t.Helper()
	return buildTarGzArchiveEntries(t, []archiveEntry{{name: name, content: content}})
}

type archiveEntry struct {
	name    string
	content []byte
}

func buildTarGzArchiveEntries(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{Name: entry.name, Mode: 0755, Size: int64(len(entry.content))}); err != nil {
			t.Fatalf("WriteHeader failed: %v", err)
		}
		if _, err := tarWriter.Write(entry.content); err != nil {
			t.Fatalf("Write archive contents failed: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Close tar writer failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Close gzip writer failed: %v", err)
	}
	return buf.Bytes()
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func newBinaryResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
