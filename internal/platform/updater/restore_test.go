package updater

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestRestoreRequestRoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	restoreRequestPath = func() string { return filepath.Join(tmpDir, "restore_request.json") }
	t.Cleanup(func() {
		restoreRequestPath = defaultRestoreRequestPath
	})

	want := RestoreRequest{Source: "/tmp/restore.sqlite", RequestedAt: 123}
	if err := WriteRestoreRequest(want); err != nil {
		t.Fatalf("WriteRestoreRequest failed: %v", err)
	}

	got, err := ReadRestoreRequest()
	if err != nil {
		t.Fatalf("ReadRestoreRequest failed: %v", err)
	}
	if got != want {
		t.Fatalf("ReadRestoreRequest = %#v, want %#v", got, want)
	}

	if err := RemoveRestoreRequest(); err != nil {
		t.Fatalf("RemoveRestoreRequest failed: %v", err)
	}
	if _, err := ReadRestoreRequest(); !errors.Is(err, ErrRestoreRequestNotFound) {
		t.Fatalf("expected ErrRestoreRequestNotFound, got %v", err)
	}
}

func TestRestoreStatusDefaultsToIdle(t *testing.T) {
	tmpDir := t.TempDir()
	restoreStatusPath = func() string { return filepath.Join(tmpDir, "restore_status.json") }
	t.Cleanup(func() {
		restoreStatusPath = defaultRestoreStatusPath
	})

	status, err := ReadRestoreStatus()
	if err != nil {
		t.Fatalf("ReadRestoreStatus failed: %v", err)
	}
	if status.State != RestoreStatusIdle {
		t.Fatalf("unexpected default status=%q", status.State)
	}
}
