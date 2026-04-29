package updater

import (
	"os"
	"path/filepath"
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
	want := filepath.Join(wd, ".cache", "updater", "master_runtime.json")
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
	want := filepath.Join(wd, ".cache", "updater", "master_runtime.json")
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
	want := filepath.Join(base, ".cache", "updater", "master_runtime.json")
	if got != want {
		t.Fatalf("DefaultRuntimeInfoPath = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(base, ".cache")); err != nil || !info.IsDir() {
		t.Fatalf("cache root missing or not dir: info=%v err=%v", info, err)
	}
}

func TestReadRuntimeInfoFallsBackToLegacyUserCachePath(t *testing.T) {
	base := t.TempDir()
	withUpdaterWorkingDir(t, base)
	resetRuntimeCacheRoot(t)

	legacyBase := t.TempDir()
	oldUserCacheDir := osUserCacheDir
	osUserCacheDir = func() (string, error) {
		return legacyBase, nil
	}
	t.Cleanup(func() {
		osUserCacheDir = oldUserCacheDir
	})

	legacyPath := filepath.Join(legacyBase, "swaves", "master_runtime.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"pid":1234,"executable":"/tmp/swaves"}`), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, err := ReadRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadRuntimeInfo failed: %v", err)
	}
	if info.PID != 1234 {
		t.Fatalf("ReadRuntimeInfo pid = %d, want 1234", info.PID)
	}
	if info.Executable != "/tmp/swaves" {
		t.Fatalf("ReadRuntimeInfo executable = %q, want %q", info.Executable, "/tmp/swaves")
	}
}

func TestReadRuntimeInfoFallsBackToLegacyProcessCachePath(t *testing.T) {
	base := t.TempDir()
	withUpdaterWorkingDir(t, base)
	resetRuntimeCacheRoot(t)

	oldUserCacheDir := osUserCacheDir
	osUserCacheDir = func() (string, error) {
		return filepath.Join(base, "non-existent-user-cache"), nil
	}
	t.Cleanup(func() {
		osUserCacheDir = oldUserCacheDir
	})

	legacyPath := filepath.Join(base, ".cache", "swaves", "master_runtime.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"pid":2345,"executable":"/root/swaves"}`), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, err := ReadRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadRuntimeInfo failed: %v", err)
	}
	if info.PID != 2345 {
		t.Fatalf("ReadRuntimeInfo pid = %d, want 2345", info.PID)
	}
	if info.Executable != "/root/swaves" {
		t.Fatalf("ReadRuntimeInfo executable = %q, want %q", info.Executable, "/root/swaves")
	}
}

func TestReadRuntimeInfoFallsBackToLegacyConfiguredCachePath(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	if err := ConfigureRuntimeCacheRoot(filepath.Join(base, "data.sqlite")); err != nil {
		t.Fatalf("ConfigureRuntimeCacheRoot failed: %v", err)
	}

	legacyPath := filepath.Join(base, ".cache", "swaves", "master_runtime.json")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("MkdirAll failed: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`{"pid":3456,"executable":"/root/swaves"}`), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	info, err := ReadRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadRuntimeInfo failed: %v", err)
	}
	if info.PID != 3456 {
		t.Fatalf("ReadRuntimeInfo pid = %d, want 3456", info.PID)
	}
	if info.Executable != "/root/swaves" {
		t.Fatalf("ReadRuntimeInfo executable = %q, want %q", info.Executable, "/root/swaves")
	}
}

func TestReadRuntimeInfoReturnsInactiveWhenCurrentAndLegacyPathsAreMissing(t *testing.T) {
	base := t.TempDir()
	withUpdaterWorkingDir(t, base)
	resetRuntimeCacheRoot(t)

	legacyBase := t.TempDir()
	oldUserCacheDir := osUserCacheDir
	osUserCacheDir = func() (string, error) {
		return legacyBase, nil
	}
	t.Cleanup(func() {
		osUserCacheDir = oldUserCacheDir
	})

	if _, err := ReadRuntimeInfo(); err == nil || err.Error() != "daemon mode is not active" {
		t.Fatalf("ReadRuntimeInfo err = %v, want daemon mode is not active", err)
	}
}
