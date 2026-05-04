package updater

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
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
	originalSQLiteFile := runtimeSQLiteFile
	runtimeCacheRoot = ""
	runtimeSQLiteFile = ""
	runtimeExecutableVerificationUnsupportedLogOnce = sync.Once{}
	runtimeCacheMu.Unlock()
	t.Cleanup(func() {
		runtimeCacheMu.Lock()
		runtimeCacheRoot = original
		runtimeSQLiteFile = originalSQLiteFile
		runtimeExecutableVerificationUnsupportedLogOnce = sync.Once{}
		runtimeCacheMu.Unlock()
	})
}

func configureTestRuntimeCacheRoot(t *testing.T, base string) {
	t.Helper()
	configureTestRuntimeCacheRootWithSQLite(t, filepath.Join(base, "data.sqlite"))
}

func configureTestRuntimeCacheRootWithSQLite(t *testing.T, sqliteFile string) {
	t.Helper()
	if err := ConfigureRuntimeCacheRoot(sqliteFile); err != nil {
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

func TestConfigureRuntimeCacheRootAtUsesExplicitDirectory(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	root := filepath.Join(base, RuntimeCacheDir)
	if err := ConfigureRuntimeCacheRootAt(root); err != nil {
		t.Fatalf("ConfigureRuntimeCacheRootAt failed: %v", err)
	}

	got, err := RuntimeInfoPath()
	if err != nil {
		t.Fatalf("RuntimeInfoPath failed: %v", err)
	}
	want := filepath.Join(root, RuntimeInfoName)
	if got != want {
		t.Fatalf("RuntimeInfoPath = %q, want %q", got, want)
	}
	if _, err := RuntimeSQLiteFile(); !errors.Is(err, ErrRuntimeCacheRootNotConfigured) {
		t.Fatalf("RuntimeSQLiteFile error = %v, want ErrRuntimeCacheRootNotConfigured", err)
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

func TestWriteRuntimeInfoPersistsRuntimeLaunchDetails(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)

	configureTestRuntimeCacheRoot(t, base)

	want := RuntimeInfo{
		PID:        1234,
		Executable: filepath.Join(base, "swaves"),
		Args:       []string{filepath.Join(base, "swaves"), "data.sqlite"},
		WorkingDir: base,
		SQLiteFile: "data.sqlite",
	}
	if err := WriteRuntimeInfo(want); err != nil {
		t.Fatalf("WriteRuntimeInfo failed: %v", err)
	}

	got, err := ReadRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadRuntimeInfo failed: %v", err)
	}
	if got.PID != want.PID || got.Executable != want.Executable || got.WorkingDir != want.WorkingDir || got.SQLiteFile != want.SQLiteFile {
		t.Fatalf("ReadRuntimeInfo = %#v, want %#v", got, want)
	}
	if len(got.Args) != len(want.Args) {
		t.Fatalf("runtime args length = %d, want %d", len(got.Args), len(want.Args))
	}
	for i := range want.Args {
		if got.Args[i] != want.Args[i] {
			t.Fatalf("runtime args[%d] = %q, want %q", i, got.Args[i], want.Args[i])
		}
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

func TestReadActiveRuntimeInfoAllowsDeletedUpgradeBackupExecutable(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)
	configureTestRuntimeCacheRoot(t, base)

	actual := filepath.Join(base, RuntimeCacheDir, UpgradeCacheDirName, ".swaves-upgrade-123", ".swaves-executable-backup") + " (deleted)"
	stubRuntimeProcessExecutablePath(t, func(pid int) (string, bool, error) {
		return actual, true, nil
	})

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

func TestReadActiveRuntimeInfoFallsBackToEnvRuntimeInfo(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)
	configureTestRuntimeCacheRoot(t, base)

	t.Setenv(RuntimeMasterPIDEnv, strconv.Itoa(os.Getpid()))
	t.Setenv(RuntimeMasterExecutableEnv, filepath.Join(base, "swaves"))
	stubRuntimeProcessExecutablePath(t, func(pid int) (string, bool, error) {
		return filepath.Join(base, "swaves"), true, nil
	})

	got, err := ReadActiveRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadActiveRuntimeInfo failed: %v", err)
	}
	if got.PID != os.Getpid() || got.Executable != filepath.Join(base, "swaves") || got.SQLiteFile != filepath.Join(base, "data.sqlite") {
		t.Fatalf("ReadActiveRuntimeInfo = %#v", got)
	}
}

func TestReadRuntimeInfoRepairsMissingFileFromEnv(t *testing.T) {
	base := t.TempDir()
	resetRuntimeCacheRoot(t)
	configureTestRuntimeCacheRoot(t, base)

	t.Setenv(RuntimeMasterPIDEnv, strconv.Itoa(os.Getpid()))
	t.Setenv(RuntimeMasterExecutableEnv, filepath.Join(base, "swaves"))

	got, err := ReadRuntimeInfo()
	if err != nil {
		t.Fatalf("ReadRuntimeInfo failed: %v", err)
	}
	if got.PID != os.Getpid() || got.Executable != filepath.Join(base, "swaves") || got.SQLiteFile != filepath.Join(base, "data.sqlite") {
		t.Fatalf("ReadRuntimeInfo = %#v", got)
	}

	path, err := RuntimeInfoPath()
	if err != nil {
		t.Fatalf("RuntimeInfoPath failed: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected repaired runtime info file: %v", err)
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
