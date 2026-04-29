package updater

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func withUpdaterWorkingDir(t *testing.T, dir string) {
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

func resetRuntimeCacheRoot(t *testing.T) {
	t.Helper()

	runtimeCacheMu.Lock()
	original := runtimeCacheRoot
	runtimeCacheRoot = ""
	runtimeCacheMu.Unlock()
	t.Cleanup(func() {
		runtimeCacheMu.Lock()
		runtimeCacheRoot = original
		runtimeCacheMu.Unlock()
	})
}

func TestDefaultRuntimeInfoPathUsesProcessCacheRoot(t *testing.T) {
	base := t.TempDir()
	withUpdaterWorkingDir(t, base)
	resetRuntimeCacheRoot(t)

	got := DefaultRuntimeInfoPath()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	want := filepath.Join(wd, RuntimeInfoFile)
	if got != want {
		t.Fatalf("DefaultRuntimeInfoPath = %q, want %q", got, want)
	}
}

func TestWriteRuntimeInfoUsesDefaultProcessCachePath(t *testing.T) {
	base := t.TempDir()
	withUpdaterWorkingDir(t, base)
	resetRuntimeCacheRoot(t)

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	want := filepath.Join(wd, RuntimeInfoFile)
	if err := WriteRuntimeInfo(RuntimeInfo{PID: 4321, Executable: filepath.Join(base, "swaves")}); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("runtime info file missing at %s: %v", want, err)
	}

	info, err := ReadRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadRuntimeInfo failed: %v", err)
	}
	if info.PID != 4321 {
		t.Fatalf("ReadRuntimeInfo pid = %d, want 4321", info.PID)
	}
	if info.Executable != filepath.Join(base, "swaves") {
		t.Fatalf("ReadRuntimeInfo executable = %q, want %q", info.Executable, filepath.Join(base, "swaves"))
	}

	if err := RemoveRuntimeInfo(); err != nil {
		t.Fatalf("RemoveRuntimeInfo failed: %v", err)
	}
	if _, err := os.Stat(want); !os.IsNotExist(err) {
		t.Fatalf("runtime info file should be removed, stat err=%v", err)
	}
}

func TestConfigureRuntimeCacheRootUsesSQLiteDirectory(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	if err := ConfigureRuntimeCacheRoot(filepath.Join(base, "data.sqlite")); err != nil {
		t.Fatalf("ConfigureRuntimeCacheRoot failed: %v", err)
	}

	got := DefaultRuntimeInfoPath()
	want := filepath.Join(base, RuntimeInfoFile)
	if got != want {
		t.Fatalf("DefaultRuntimeInfoPath = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(base, ".cache")); err != nil || !info.IsDir() {
		t.Fatalf("cache root missing or not dir: info=%v err=%v", info, err)
	}
}

func TestReadRuntimeInfoReturnsMissingFileError(t *testing.T) {
	base := t.TempDir()
	withUpdaterWorkingDir(t, base)
	resetRuntimeCacheRoot(t)

	_, err := ReadRuntimeInfo()
	if err == nil {
		t.Fatal("ReadRuntimeInfo error = nil, want missing file error")
	}

	wd, getwdErr := os.Getwd()
	if getwdErr != nil {
		t.Fatalf("Getwd failed: %v", getwdErr)
	}
	want := "runtime info file not found: path=" + filepath.Join(wd, RuntimeInfoFile)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("ReadRuntimeInfo error = %q, want substring %q", err.Error(), want)
	}
}
