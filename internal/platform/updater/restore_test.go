package updater

import (
	"errors"
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
