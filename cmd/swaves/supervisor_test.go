package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/updater"
)

func TestRunSupervisorRequiresWorkerCallback(t *testing.T) {
	err := runSupervisor(supervisorConfig{})
	if err == nil || err.Error() != "worker callback is required" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRunSupervisorWorkerModeUsesWorkerDirectly(t *testing.T) {
	t.Setenv(workerModeEnv, "1")
	called := false
	err := runSupervisor(supervisorConfig{
		Worker: func() error {
			called = true
			return nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected err=%v", err)
	}
	if !called {
		t.Fatal("expected worker callback to be called")
	}
}

func TestRunSupervisorRequiresDaemonMode(t *testing.T) {
	err := runSupervisor(supervisorConfig{
		DaemonMode: false,
		Worker: func() error {
			return nil
		},
	})
	if err == nil || err.Error() != "daemon mode is required" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRunSupervisorDaemonRequiresListenAddr(t *testing.T) {
	err := runSupervisor(supervisorConfig{
		DaemonMode: true,
		Worker: func() error {
			return nil
		},
	})
	if err == nil || err.Error() != "listen addr is required in daemon mode" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestNormalizeSupervisorConfigAppliesDefaults(t *testing.T) {
	cfg := supervisorConfig{}

	normalizeSupervisorConfig(&cfg)

	if cfg.ReadyTimeout != defaultWorkerReadyTimeout {
		t.Fatalf("unexpected ready timeout=%s", cfg.ReadyTimeout)
	}
	if cfg.ShutdownTimeout != defaultWorkerStopTimeout {
		t.Fatalf("unexpected shutdown timeout=%s", cfg.ShutdownTimeout)
	}
	if cfg.DrainTimeout != defaultWorkerDrainTimeout {
		t.Fatalf("unexpected drain timeout=%s", cfg.DrainTimeout)
	}
}

func TestResolveSupervisorExecutablePathUsesConfiguredValue(t *testing.T) {
	got, err := resolveSupervisorExecutablePath(" /tmp/swaves ")
	if err != nil {
		t.Fatalf("resolveSupervisorExecutablePath failed: %v", err)
	}
	if got != "/tmp/swaves" {
		t.Fatalf("resolveSupervisorExecutablePath = %q, want %q", got, "/tmp/swaves")
	}
}

func TestResolveSupervisorExecutablePathFallsBackToCurrentExecutable(t *testing.T) {
	want, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}

	got, err := resolveSupervisorExecutablePath("")
	if err != nil {
		t.Fatalf("resolveSupervisorExecutablePath failed: %v", err)
	}
	if got != want {
		t.Fatalf("resolveSupervisorExecutablePath = %q, want %q", got, want)
	}
}

func TestWorkerExtraEnvIncludesRuntimeMasterInfo(t *testing.T) {
	env := workerExtraEnv("/tmp/swaves")

	for _, want := range []string{
		updater.RuntimeMasterPIDEnv + "=",
		updater.RuntimeMasterExecutableEnv + "=/tmp/swaves",
	} {
		if !envHasPrefix(env, want) {
			t.Fatalf("expected env to contain %q, got %v", want, env)
		}
	}
}

func TestResolveRuntimeSQLiteFileMakesRelativePathAbsolute(t *testing.T) {
	got := resolveRuntimeSQLiteFile("data.sqlite", "/home/ubuntu")
	want := filepath.Join("/home/ubuntu", "data.sqlite")
	if got != want {
		t.Fatalf("resolveRuntimeSQLiteFile = %q, want %q", got, want)
	}
}

func TestWorkerArgsAppendsInternalWorkerFlag(t *testing.T) {
	args := workerArgs([]string{"data.sqlite", "--daemon-mode=1"})
	if len(args) != 3 {
		t.Fatalf("unexpected args len=%d args=%v", len(args), args)
	}
	if args[2] != workerProcessFlag {
		t.Fatalf("expected worker flag appended, got %v", args)
	}
}

func TestWorkerArgsDoesNotDuplicateInternalWorkerFlag(t *testing.T) {
	args := workerArgs([]string{"data.sqlite", workerProcessFlag})
	count := 0
	for _, arg := range args {
		if arg == workerProcessFlag {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one worker flag, got %d args=%v", count, args)
	}
}

func envHasPrefix(env []string, prefix string) bool {
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}


func TestReplaceSQLiteDatabaseReplacesTargetAndCleansRuntimeFiles(t *testing.T) {
	tmpDir := t.TempDir()
	configureSupervisorTestRuntimeCacheRoot(t, tmpDir)
	targetPath := filepath.Join(tmpDir, "data.sqlite")
	sourcePath := filepath.Join(tmpDir, "restore.sqlite")
	if err := os.WriteFile(targetPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile target failed: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile source failed: %v", err)
	}
	if err := os.WriteFile(targetPath+"-wal", []byte("wal"), 0o644); err != nil {
		t.Fatalf("WriteFile wal failed: %v", err)
	}
	if err := os.WriteFile(targetPath+"-shm", []byte("shm"), 0o644); err != nil {
		t.Fatalf("WriteFile shm failed: %v", err)
	}

	rollbackPath, err := replaceSQLiteDatabase(targetPath, sourcePath)
	if err != nil {
		t.Fatalf("replaceSQLiteDatabase failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target failed: %v", err)
	}
	if string(data) != "new" {
		t.Fatalf("unexpected target contents=%q", string(data))
	}
	if _, err := os.Stat(targetPath + "-wal"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected wal file removed, got err=%v", err)
	}
	if _, err := os.Stat(targetPath + "-shm"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected shm file removed, got err=%v", err)
	}
	if _, err := os.Stat(rollbackPath); err != nil {
		t.Fatalf("expected rollback path to exist: %v", err)
	}
	if filepath.Dir(rollbackPath) != filepath.Join(tmpDir, ".cache", updater.RestoreCacheDirName) {
		t.Fatalf("rollback dir=%q, want restore cache dir", filepath.Dir(rollbackPath))
	}
}

func TestRollbackSQLiteDatabaseRestoresOriginalFile(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "data.sqlite")
	rollbackPath := filepath.Join(tmpDir, "backup.sqlite")
	if err := os.WriteFile(targetPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile target failed: %v", err)
	}
	if err := os.WriteFile(rollbackPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile rollback failed: %v", err)
	}

	if err := rollbackSQLiteDatabase(targetPath, rollbackPath); err != nil {
		t.Fatalf("rollbackSQLiteDatabase failed: %v", err)
	}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("ReadFile target failed: %v", err)
	}
	if string(data) != "old" {
		t.Fatalf("unexpected target contents=%q", string(data))
	}
	if _, err := os.Stat(rollbackPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected rollback path removed, got err=%v", err)
	}
}

func configureSupervisorTestRuntimeCacheRoot(t *testing.T, base string) {
	t.Helper()
	if err := updater.ConfigureRuntimeCacheRoot(filepath.Join(base, "data.sqlite")); err != nil {
		t.Fatalf("ConfigureRuntimeCacheRoot failed: %v", err)
	}
}
