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

func TestResolveDatabaseCacheRoot(t *testing.T) {
	base := t.TempDir()
	dbPath := filepath.Join(base, "data.sqlite")

	got, err := ResolveDatabaseCacheRoot(dbPath)
	if err != nil {
		t.Fatalf("ResolveDatabaseCacheRoot failed: %v", err)
	}
	want := filepath.Join(base, ".cache")
	if got != want {
		t.Fatalf("ResolveDatabaseCacheRoot = %q, want %q", got, want)
	}
}

func TestResolveDatabaseCachePathUsesAbsoluteSQLiteDirectory(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	got, err := ResolveDatabaseCachePath("data.sqlite", "updater", "master_runtime.json")
	if err != nil {
		t.Fatalf("ResolveDatabaseCachePath failed: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	want := filepath.Join(wd, ".cache", "updater", "master_runtime.json")
	if got != want {
		t.Fatalf("ResolveDatabaseCachePath = %q, want %q", got, want)
	}
}

func TestEnsureDatabaseCacheRootCreatesDirectory(t *testing.T) {
	base := t.TempDir()
	dbPath := filepath.Join(base, "nested", "data.sqlite")

	got, err := EnsureDatabaseCacheRoot(dbPath)
	if err != nil {
		t.Fatalf("EnsureDatabaseCacheRoot failed: %v", err)
	}
	want := filepath.Join(base, "nested", ".cache")
	if got != want {
		t.Fatalf("EnsureDatabaseCacheRoot = %q, want %q", got, want)
	}
	if info, err := os.Stat(want); err != nil || !info.IsDir() {
		t.Fatalf("cache root missing or not dir: info=%v err=%v", info, err)
	}
}
