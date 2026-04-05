package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestInstallLatestReleaseReplacesExecutableAndSignalsMaster(t *testing.T) {
	tmpDir := t.TempDir()
	runtimePath := filepath.Join(tmpDir, "runtime.json")
	oldRuntimePath := runtimeInfoPath
	oldCurrentExecutable := currentExecutable
	oldProcessExecutable := processExecutable
	runtimeInfoPath = func() string { return runtimePath }
	defer func() {
		runtimeInfoPath = oldRuntimePath
		currentExecutable = oldCurrentExecutable
		processExecutable = oldProcessExecutable
	}()

	executablePath := filepath.Join(tmpDir, "swaves")
	currentExecutable = func() (string, error) { return executablePath, nil }
	processExecutable = func(pid int) (string, error) { return executablePath, nil }
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("write old executable failed: %v", err)
	}
	if err := WriteRuntimeInfo(RuntimeInfo{PID: 4321, Executable: executablePath}); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	var signaledPID atomic.Int64
	oldProcessExists := processExists
	oldSignalProcess := signalProcess
	processExists = func(pid int) bool { return pid == 4321 }
	signalProcess = func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
	}
	defer func() {
		processExists = oldProcessExists
		signalProcess = oldSignalProcess
	}()

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
					"html_url":"https://github.com/keelii/swaves/releases/tag/v1.2.4",
					"published_at":"2026-04-05T00:00:00Z",
					"draft":false,
					"prerelease":false,
					"assets":[
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
						{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
					]
				}`)
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

func TestInstallLatestReleaseNoOpWhenAlreadyLatest(t *testing.T) {
	tmpDir := t.TempDir()
	runtimePath := filepath.Join(tmpDir, "runtime.json")
	oldRuntimePath := runtimeInfoPath
	oldCurrentExecutable := currentExecutable
	oldProcessExecutable := processExecutable
	runtimeInfoPath = func() string { return runtimePath }
	defer func() {
		runtimeInfoPath = oldRuntimePath
		currentExecutable = oldCurrentExecutable
		processExecutable = oldProcessExecutable
	}()

	executablePath := filepath.Join(tmpDir, "swaves")
	currentExecutable = func() (string, error) { return executablePath, nil }
	processExecutable = func(pid int) (string, error) { return executablePath, nil }
	if err := os.WriteFile(executablePath, []byte("same-binary"), 0755); err != nil {
		t.Fatalf("write executable failed: %v", err)
	}
	if err := WriteRuntimeInfo(RuntimeInfo{PID: 1001, Executable: executablePath}); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	oldProcessExists := processExists
	oldSignalProcess := signalProcess
	processExists = func(pid int) bool { return pid == 1001 }
	signalProcess = func(pid int) error {
		t.Fatalf("signalProcess should not be called, got pid=%d", pid)
		return nil
	}
	defer func() {
		processExists = oldProcessExists
		signalProcess = oldSignalProcess
	}()

	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.String() != "https://example.test/latest" {
				return newHTTPResponse(http.StatusNotFound, "not found"), nil
			}
			body := `{
				"tag_name":"v1.2.4",
				"html_url":"https://github.com/keelii/swaves/releases/tag/v1.2.4",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/archive"},
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/archive.sha256"}
				]
			}`
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
	runtimePath := filepath.Join(tmpDir, "runtime.json")
	oldRuntimePath := runtimeInfoPath
	oldCurrentExecutable := currentExecutable
	oldProcessExecutable := processExecutable
	runtimeInfoPath = func() string { return runtimePath }
	defer func() {
		runtimeInfoPath = oldRuntimePath
		currentExecutable = oldCurrentExecutable
		processExecutable = oldProcessExecutable
	}()

	executablePath := filepath.Join(tmpDir, "swaves")
	currentExecutable = func() (string, error) { return executablePath, nil }
	processExecutable = func(pid int) (string, error) { return executablePath, nil }
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("write old executable failed: %v", err)
	}
	if err := WriteRuntimeInfo(RuntimeInfo{PID: 4321, Executable: executablePath}); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	var signaledPID atomic.Int64
	oldProcessExists := processExists
	oldSignalProcess := signalProcess
	processExists = func(pid int) bool { return pid == 4321 }
	signalProcess = func(pid int) error {
		signaledPID.Store(int64(pid))
		return nil
	}
	defer func() {
		processExists = oldProcessExists
		signalProcess = oldSignalProcess
	}()

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

func TestInstallLatestReleaseRejectsStaleRuntimeInfo(t *testing.T) {
	tmpDir := t.TempDir()
	runtimePath := filepath.Join(tmpDir, "runtime.json")
	oldRuntimePath := runtimeInfoPath
	oldCurrentExecutable := currentExecutable
	oldProcessExecutable := processExecutable
	oldProcessExists := processExists
	runtimeInfoPath = func() string { return runtimePath }
	currentExecutable = func() (string, error) { return filepath.Join(tmpDir, "swaves"), nil }
	processExecutable = func(pid int) (string, error) { return filepath.Join(tmpDir, "other-binary"), nil }
	processExists = func(pid int) bool { return pid == 4321 }
	defer func() {
		runtimeInfoPath = oldRuntimePath
		currentExecutable = oldCurrentExecutable
		processExecutable = oldProcessExecutable
		processExists = oldProcessExists
	}()

	executablePath := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(executablePath, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("write executable failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "other-binary"), []byte("other"), 0755); err != nil {
		t.Fatalf("write other executable failed: %v", err)
	}
	if err := WriteRuntimeInfo(RuntimeInfo{PID: 4321, Executable: executablePath}); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	_, err := Client{}.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err == nil || !strings.Contains(err.Error(), "active master process") {
		t.Fatalf("expected active master process error, got %v", err)
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
