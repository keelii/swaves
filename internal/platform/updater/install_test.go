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
	"testing"
)

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
	if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
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

func TestInstallLocalReleaseArchiveWithoutActiveMasterInstallsCurrentExecutable(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target failed: %v", err)
	}

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	writeArchiveFile(t, archivePath, expectedReleaseBinaryName(archiveName), []byte("new-binary"))

	withInstallTestHooks(t, installTestHooks{
		currentExecutable: targetPath,
		activeRuntimeErr:  fmt.Errorf("daemon mode is not active"),
	})

	result, err := InstallLocalReleaseArchive(archiveName, archivePath, "v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLocalReleaseArchive failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected install result to be marked installed")
	}
	if result.RestartedPID != 0 {
		t.Fatalf("RestartedPID = %d, want 0", result.RestartedPID)
	}
	if result.LatestVersion != "v1.2.4" {
		t.Fatalf("LatestVersion = %q, want %q", result.LatestVersion, "v1.2.4")
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target failed: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("installed executable = %q, want %q", string(got), "new-binary")
	}
}

func TestInstallLatestReleaseCLIReusesCoreFlowAndRestartsMatchingMaster(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target failed: %v", err)
	}

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{{
		name:    expectedReleaseBinaryName(archiveName),
		content: []byte("new-binary"),
	}})
	checksum := sha256.Sum256(archiveData)
	checksumBody := hex.EncodeToString(checksum[:]) + "  " + archiveName + "\n"
	client := newRemoteInstallTestClient(t, archiveName, archiveData, checksumBody)

	signaledPID := 0
	withInstallTestHooks(t, installTestHooks{
		currentExecutable: targetPath,
		activeRuntime:     RuntimeInfo{PID: 4321, Executable: targetPath},
		signalProcess: func(pid int) error {
			signaledPID = pid
			return nil
		},
	})

	result, err := client.InstallLatestReleaseCLI("v1.2.3", "linux", "amd64")
	if err != nil {
		t.Fatalf("InstallLatestReleaseCLI failed: %v", err)
	}
	if !result.Installed {
		t.Fatal("expected install result to be marked installed")
	}
	if result.RestartedPID != 4321 {
		t.Fatalf("RestartedPID = %d, want 4321", result.RestartedPID)
	}
	if signaledPID != 4321 {
		t.Fatalf("signaled PID = %d, want 4321", signaledPID)
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target failed: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("installed executable = %q, want %q", string(got), "new-binary")
	}
}

func TestInstallLatestReleaseRequiresActiveMaster(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target failed: %v", err)
	}

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{{
		name:    expectedReleaseBinaryName(archiveName),
		content: []byte("new-binary"),
	}})
	checksum := sha256.Sum256(archiveData)
	checksumBody := hex.EncodeToString(checksum[:]) + "  " + archiveName + "\n"
	client := newRemoteInstallTestClient(t, archiveName, archiveData, checksumBody)

	withInstallTestHooks(t, installTestHooks{
		currentExecutable: targetPath,
		activeRuntimeErr:  fmt.Errorf("daemon mode is not active"),
	})

	_, err := client.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("expected InstallLatestRelease to require an active master")
	}
	if !strings.Contains(err.Error(), "automatic upgrade requires daemon-mode=1 and an active master process") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInstallLocalReleaseArchiveRollsBackWhenRestartFails(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(targetPath, []byte("old-binary"), 0o755); err != nil {
		t.Fatalf("WriteFile target failed: %v", err)
	}

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	writeArchiveFile(t, archivePath, expectedReleaseBinaryName(archiveName), []byte("new-binary"))

	withInstallTestHooks(t, installTestHooks{
		currentExecutable: targetPath,
		activeRuntime:     RuntimeInfo{PID: 4321, Executable: targetPath},
		signalProcess: func(pid int) error {
			return fmt.Errorf("boom")
		},
	})

	if _, err := InstallLocalReleaseArchive(archiveName, archivePath, "v1.2.3", "linux", "amd64"); err == nil {
		t.Fatal("expected restart failure to be returned")
	}
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target failed: %v", err)
	}
	if string(got) != "old-binary" {
		t.Fatalf("installed executable after rollback = %q, want %q", string(got), "old-binary")
	}
}

type installTestHooks struct {
	currentExecutable string
	activeRuntime     RuntimeInfo
	activeRuntimeErr  error
	signalProcess     func(pid int) error
}

func withInstallTestHooks(t *testing.T, hooks installTestHooks) {
	t.Helper()

	oldCurrentExecutable := currentInstallExecutableFunc
	oldReadActiveRuntime := readActiveRuntimeInfoFunc
	oldSignalProcess := signalProcessFunc

	currentInstallExecutableFunc = func() (string, error) {
		return hooks.currentExecutable, nil
	}
	readActiveRuntimeInfoFunc = func() (RuntimeInfo, error) {
		if hooks.activeRuntimeErr != nil {
			return RuntimeInfo{}, hooks.activeRuntimeErr
		}
		return hooks.activeRuntime, nil
	}
	signalProcessFunc = func(pid int) error {
		if hooks.signalProcess != nil {
			return hooks.signalProcess(pid)
		}
		return nil
	}

	t.Cleanup(func() {
		currentInstallExecutableFunc = oldCurrentExecutable
		readActiveRuntimeInfoFunc = oldReadActiveRuntime
		signalProcessFunc = oldSignalProcess
	})
}

func writeArchiveFile(t *testing.T, archivePath string, binaryName string, binary []byte) {
	t.Helper()

	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{{
		name:    binaryName,
		content: binary,
	}})
	if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
		t.Fatalf("WriteFile archive failed: %v", err)
	}
}

func newRemoteInstallTestClient(t *testing.T, archiveName string, archiveData []byte, checksumBody string) Client {
	t.Helper()

	baseURL := "https://example.test"
	archiveURL := baseURL + "/" + archiveName
	checksumURL := archiveURL + ".sha256"
	latestURL := baseURL + "/latest"
	return Client{
		BaseURL: latestURL,
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.String() {
			case latestURL:
				body := fmt.Sprintf(`{
"tag_name":"v1.2.4",
"html_url":"%s",
"published_at":"2026-04-05T00:00:00Z",
"draft":false,
"prerelease":false,
"assets":[
{"name":"%s","browser_download_url":"%s"},
{"name":"%s.sha256","browser_download_url":"%s"}
]
}`,
					ReleaseTagURL("v1.2.4"),
					archiveName,
					archiveURL,
					archiveName,
					checksumURL,
				)
				return newHTTPBytesResponse(http.StatusOK, []byte(body)), nil
			case archiveURL:
				return newHTTPBytesResponse(http.StatusOK, archiveData), nil
			case checksumURL:
				return newHTTPBytesResponse(http.StatusOK, []byte(checksumBody)), nil
			default:
				return nil, fmt.Errorf("unexpected request url: %s", req.URL.String())
			}
		})},
	}
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
		if err := tarWriter.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o755, Size: int64(len(entry.content))}); err != nil {
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

func newHTTPBytesResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}
