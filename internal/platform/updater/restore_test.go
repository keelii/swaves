package updater

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRestoreRequestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	requestPath := filepath.Join(tmpDir, "restore_request.json")

	want := RestoreRequest{Source: "/tmp/restore.sqlite", RequestedAt: 123}
	if err := WriteRestoreRequestAtPath(requestPath, want); err != nil {
		t.Fatalf("WriteRestoreRequest failed: %v", err)
	}

	got, err := ReadRestoreRequestAtPath(requestPath)
	if err != nil {
		t.Fatalf("ReadRestoreRequest failed: %v", err)
	}
	if got != want {
		t.Fatalf("ReadRestoreRequest = %#v, want %#v", got, want)
	}

	if err := RemoveRestoreRequestAtPath(requestPath); err != nil {
		t.Fatalf("RemoveRestoreRequest failed: %v", err)
	}
	if _, err := ReadRestoreRequestAtPath(requestPath); !errors.Is(err, ErrRestoreRequestNotFound) {
		t.Fatalf("expected ErrRestoreRequestNotFound, got %v", err)
	}
}

func TestRestoreStatusDefaultsToIdle(t *testing.T) {
	tmpDir := t.TempDir()
	statusPath := filepath.Join(tmpDir, "restore_status.json")

	status, err := ReadRestoreStatusAtPath(statusPath)
	if err != nil {
		t.Fatalf("ReadRestoreStatus failed: %v", err)
	}
	if status.State != RestoreStatusIdle {
		t.Fatalf("unexpected default status=%q", status.State)
	}
}

func TestCreateRestoreTempFileUsesRestoreCacheRoot(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)
	configureTestRuntimeCacheRoot(t, base)

	file, err := CreateRestoreTempFile(".swaves-restore-upload-*.sqlite")
	if err != nil {
		t.Fatalf("CreateRestoreTempFile failed: %v", err)
	}
	path := file.Name()
	if err := file.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	defer func() { _ = os.Remove(path) }()

	wantDir := filepath.Join(base, ".cache", RestoreCacheDirName)
	if filepath.Dir(path) != wantDir {
		t.Fatalf("restore temp dir=%q, want %q", filepath.Dir(path), wantDir)
	}
}
