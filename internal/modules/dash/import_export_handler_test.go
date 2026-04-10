package dash

import (
	"os"
	"path/filepath"
	"swaves/internal/platform/store"
	"testing"
	"time"
)

func TestCleanupFileStreamCloseRunsCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "export.sqlite")
	if err := os.WriteFile(filePath, []byte("sqlite-data"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cleanupCalled := false
	stream, err := openCleanupFileStream(filePath, func() {
		cleanupCalled = true
	})
	if err != nil {
		t.Fatalf("openCleanupFileStream failed: %v", err)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !cleanupCalled {
		t.Fatal("expected cleanup to be called")
	}
}

func TestListLocalRestoreBackups(t *testing.T) {
	tmpDir := t.TempDir()
	restore := map[string]string{}
	if current, ok := store.Settings.Load().(map[string]string); ok {
		for key, value := range current {
			restore[key] = value
		}
	}
	t.Cleanup(func() {
		store.Settings.Store(restore)
	})
	store.Settings.Store(map[string]string{"backup_local_dir": tmpDir})

	oldPath := filepath.Join(tmpDir, "2026-04-01_old.sqlite")
	newPath := filepath.Join(tmpDir, "2026-04-02_new.sqlite")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile old failed: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile new failed: %v", err)
	}
	oldTime := time.Date(2026, 4, 1, 10, 0, 0, 0, time.Local)
	newTime := time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes old failed: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("Chtimes new failed: %v", err)
	}

	backups, dir, err := listLocalRestoreBackups()
	if err != nil {
		t.Fatalf("listLocalRestoreBackups failed: %v", err)
	}
	if dir != tmpDir {
		t.Fatalf("unexpected backup dir=%q", dir)
	}
	if len(backups) != 2 {
		t.Fatalf("unexpected backup count=%d", len(backups))
	}
	if backups[0].Name != "2026-04-02_new.sqlite" {
		t.Fatalf("expected newest backup first, got %q", backups[0].Name)
	}
}
