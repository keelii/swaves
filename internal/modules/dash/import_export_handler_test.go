package dash

import (
	"os"
	"path/filepath"
	"testing"
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
