package main

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
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

func TestReadWorkerReadyReturnsUnexpectedMessage(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}

	go func() {
		_, _ = writer.WriteString("NOPE\n")
		_ = writer.Close()
	}()

	err = readWorkerReady(reader)
	if err == nil || err.Error() != `unexpected worker ready message: "NOPE"` {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestEnvFD(t *testing.T) {
	t.Setenv("TEST_FD", "7")
	fd, ok, err := envFD("TEST_FD")
	if err != nil || !ok || fd != 7 {
		t.Fatalf("unexpected result fd=%d ok=%v err=%v", fd, ok, err)
	}
}

func TestEnvFDRejectsInvalidValue(t *testing.T) {
	t.Setenv("TEST_FD", "nope")
	_, _, err := envFD("TEST_FD")
	if err == nil {
		t.Fatal("expected invalid fd error")
	}
}

func TestEnvFDMissingValue(t *testing.T) {
	fd, ok, err := envFD("TEST_FD_MISSING")
	if err != nil || ok || fd != 0 {
		t.Fatalf("unexpected result fd=%d ok=%v err=%v", fd, ok, err)
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

func TestListenerFileRejectsUnsupportedListener(t *testing.T) {
	_, err := listenerFile(fakeListener{})
	if err == nil {
		t.Fatal("expected unsupported listener error")
	}
}

type fakeListener struct{}

func (fakeListener) Accept() (net.Conn, error) { return nil, errors.New("not implemented") }
func (fakeListener) Close() error              { return nil }
func (fakeListener) Addr() net.Addr            { return &net.TCPAddr{} }

func TestReplaceSQLiteDatabaseReplacesTargetAndCleansRuntimeFiles(t *testing.T) {
	tmpDir := t.TempDir()
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
