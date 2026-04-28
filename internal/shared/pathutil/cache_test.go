package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) failed: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore working directory failed: %v", err)
		}
	})
}

func TestResolveProcessCacheRoot(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	got, err := ResolveProcessCacheRoot()
	if err != nil {
		t.Fatalf("ResolveProcessCacheRoot failed: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	want := filepath.Join(wd, ".cache")
	if got != want {
		t.Fatalf("ResolveProcessCacheRoot = %q, want %q", got, want)
	}
}

func TestResolveProcessCachePath(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	got, err := ResolveProcessCachePath("updater", "master_runtime.json")
	if err != nil {
		t.Fatalf("ResolveProcessCachePath failed: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	want := filepath.Join(wd, ".cache", "updater", "master_runtime.json")
	if got != want {
		t.Fatalf("ResolveProcessCachePath = %q, want %q", got, want)
	}
}
