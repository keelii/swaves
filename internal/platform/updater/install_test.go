package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

type archiveEntry struct {
	name    string
	content []byte
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
