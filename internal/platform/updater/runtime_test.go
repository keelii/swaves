package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
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
	runtimeExecutableVerificationUnsupportedLogOnce = sync.Once{}
	runtimeCacheMu.Unlock()
	t.Cleanup(func() {
		runtimeCacheMu.Lock()
		runtimeCacheRoot = original
		runtimeExecutableVerificationUnsupportedLogOnce = sync.Once{}
		runtimeCacheMu.Unlock()
	})
}

func configureTestRuntimeCacheRoot(t *testing.T, base string) {
	t.Helper()
	if err := ConfigureRuntimeCacheRoot(filepath.Join(base, "data.sqlite")); err != nil {
		t.Fatalf("ConfigureRuntimeCacheRoot failed: %v", err)
	}
}

func stubRuntimeProcessExecutablePath(t *testing.T, fn func(pid int) (string, bool, error)) {
	t.Helper()

	original := runtimeProcessExecutablePath
	runtimeProcessExecutablePath = fn
	t.Cleanup(func() {
		runtimeProcessExecutablePath = original
	})
}

func TestRuntimeCacheRootRequiresConfiguration(t *testing.T) {
	resetRuntimeCacheRoot(t)

	if _, err := RuntimeCacheRoot(); !errors.Is(err, ErrRuntimeCacheRootNotConfigured) {
		t.Fatalf("RuntimeCacheRoot error = %v, want ErrRuntimeCacheRootNotConfigured", err)
	}
	if _, err := RuntimeInfoPath(); !errors.Is(err, ErrRuntimeCacheRootNotConfigured) {
		t.Fatalf("RuntimeInfoPath error = %v, want ErrRuntimeCacheRootNotConfigured", err)
	}
}

func TestWriteRuntimeInfoRequiresConfiguredCacheRoot(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	err := WriteRuntimeInfo(RuntimeInfo{PID: 4321, Executable: filepath.Join(base, "swaves")})
	if !errors.Is(err, ErrRuntimeCacheRootNotConfigured) {
		t.Fatalf("WriteRuntimeInfo error = %v, want ErrRuntimeCacheRootNotConfigured", err)
	}
}

func TestConfigureRuntimeCacheRootUsesSQLiteDirectory(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	configureTestRuntimeCacheRoot(t, base)

	got, err := RuntimeInfoPath()
	if err != nil {
		t.Fatalf("RuntimeInfoPath failed: %v", err)
	}
	want := filepath.Join(base, RuntimeCacheDir, RuntimeInfoName)
	if got != want {
		t.Fatalf("RuntimeInfoPath = %q, want %q", got, want)
	}
	if info, err := os.Stat(filepath.Join(base, ".cache")); err != nil || !info.IsDir() {
		t.Fatalf("cache root missing or not dir: info=%v err=%v", info, err)
	}
}

func TestCreateUpgradeTempDirUsesUpdaterCacheRoot(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	configureTestRuntimeCacheRoot(t, base)

	got, err := CreateUpgradeTempDir(".swaves-upgrade-")
	if err != nil {
		t.Fatalf("CreateUpgradeTempDir failed: %v", err)
	}

	wantParent := filepath.Join(base, ".cache", UpgradeCacheDirName)
	if filepath.Dir(got) != wantParent {
		t.Fatalf("upgrade temp dir parent = %q, want %q", filepath.Dir(got), wantParent)
	}
	if info, err := os.Stat(got); err != nil || !info.IsDir() {
		t.Fatalf("upgrade temp dir missing or not dir: info=%v err=%v", info, err)
	}
}

func TestReadRuntimeInfoReturnsMissingFileError(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	configureTestRuntimeCacheRoot(t, base)

	_, err := ReadRuntimeInfo()
	if err == nil {
		t.Fatal("ReadRuntimeInfo error = nil, want missing file error")
	}

	want := "runtime info file not found: path=" + filepath.Join(base, RuntimeCacheDir, RuntimeInfoName)
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("ReadRuntimeInfo error = %q, want substring %q", err.Error(), want)
	}
}

func TestReadActiveRuntimeInfoRejectsExecutableMismatch(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)
	stubRuntimeProcessExecutablePath(t, func(pid int) (string, bool, error) {
		return filepath.Join(base, "other"), true, nil
	})

	configureTestRuntimeCacheRoot(t, base)

	if err := WriteRuntimeInfo(RuntimeInfo{PID: os.Getpid(), Executable: filepath.Join(base, "swaves")}); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	_, err := ReadActiveRuntimeInfo()
	if err == nil {
		t.Fatal("ReadActiveRuntimeInfo error = nil, want executable mismatch")
	}
	if !strings.Contains(err.Error(), "master process executable mismatch") {
		t.Fatalf("ReadActiveRuntimeInfo error = %q, want executable mismatch", err.Error())
	}
}

func TestReadActiveRuntimeInfoAllowsUnsupportedExecutableVerification(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)
	stubRuntimeProcessExecutablePath(t, func(pid int) (string, bool, error) {
		return "", false, nil
	})

	if err := ConfigureRuntimeCacheRoot(filepath.Join(base, "data.sqlite")); err != nil {
		t.Fatalf("ConfigureRuntimeCacheRoot failed: %v", err)
	}

	want := RuntimeInfo{PID: os.Getpid(), Executable: filepath.Join(base, "swaves")}
	if err := WriteRuntimeInfo(want); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	got, err := ReadActiveRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadActiveRuntimeInfo failed: %v", err)
	}
	if got.PID != want.PID || got.Executable != want.Executable {
		t.Fatalf("ReadActiveRuntimeInfo = %#v, want %#v", got, want)
	}
}

func TestProcessExecutablePermissionErrorIsUnsupported(t *testing.T) {
	for _, err := range []error{syscall.EACCES, syscall.EPERM} {
		if !isProcessExecutablePermissionError(err) {
			t.Fatalf("isProcessExecutablePermissionError(%v) = false, want true", err)
		}
	}
	if isProcessExecutablePermissionError(os.ErrNotExist) {
		t.Fatal("isProcessExecutablePermissionError(os.ErrNotExist) = true, want false")
	}
}
