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

func TestEnsureProcessCacheDir(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	got, err := EnsureProcessCacheDir("exports")
	if err != nil {
		t.Fatalf("EnsureProcessCacheDir failed: %v", err)
	}
	if err := ValidateProcessCachePath(got); err != nil {
		t.Fatalf("ValidateProcessCachePath failed: %v", err)
	}
	info, err := os.Stat(got)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected directory at %s", got)
	}
}

func TestCreateProcessCacheTempDir(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	got, err := CreateProcessCacheTempDir("export-", "exports")
	if err != nil {
		t.Fatalf("CreateProcessCacheTempDir failed: %v", err)
	}
	if err := ValidateProcessCachePath(got); err != nil {
		t.Fatalf("ValidateProcessCachePath failed: %v", err)
	}
	if _, err := os.Stat(got); err != nil {
		t.Fatalf("temp dir missing: %v", err)
	}
	gotParent, err := filepath.EvalSymlinks(filepath.Dir(got))
	if err != nil {
		t.Fatalf("EvalSymlinks got parent failed: %v", err)
	}
	wantParent, err := filepath.EvalSymlinks(filepath.Join(base, ".cache", "exports"))
	if err != nil {
		t.Fatalf("EvalSymlinks want parent failed: %v", err)
	}
	if gotParent != wantParent {
		t.Fatalf("temp dir parent = %q, want %q", gotParent, wantParent)
	}
}

func TestValidateProcessCachePathRejectsOutsidePath(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	outside := filepath.Join(t.TempDir(), "outside")
	if err := ValidateProcessCachePath(outside); err == nil {
		t.Fatal("ValidateProcessCachePath should reject paths outside cache root")
	}
}
