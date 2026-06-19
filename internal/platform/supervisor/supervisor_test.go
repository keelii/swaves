package supervisor

import (
	"errors"
	"net"
	"os"
	"testing"
)

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

func TestReadWorkerReadyAcceptsValidMessage(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}

	go func() {
		_, _ = writer.WriteString(readyMessage + "\n")
		_ = writer.Close()
	}()

	if err := readWorkerReady(reader); err != nil {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestReadWorkerReadyHandlesClosedPipe(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe failed: %v", err)
	}
	_ = writer.Close()

	err = readWorkerReady(reader)
	if err == nil {
		t.Fatal("expected error for closed pipe")
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

func TestEnvFD(t *testing.T) {
	t.Setenv("TEST_SUPERVISOR_FD", "7")
	fd, ok, err := EnvFD("TEST_SUPERVISOR_FD")
	if err != nil || !ok || fd != 7 {
		t.Fatalf("unexpected result fd=%d ok=%v err=%v", fd, ok, err)
	}
}

func TestEnvFDRejectsInvalidValue(t *testing.T) {
	t.Setenv("TEST_SUPERVISOR_FD", "nope")
	_, _, err := EnvFD("TEST_SUPERVISOR_FD")
	if err == nil {
		t.Fatal("expected invalid fd error")
	}
}

func TestEnvFDMissingValue(t *testing.T) {
	fd, ok, err := EnvFD("TEST_SUPERVISOR_FD_MISSING")
	if err != nil || ok || fd != 0 {
		t.Fatalf("unexpected result fd=%d ok=%v err=%v", fd, ok, err)
	}
}

func TestResolveExecutablePathUsesConfiguredValue(t *testing.T) {
	got, err := resolveExecutablePath(" /tmp/swaves ")
	if err != nil {
		t.Fatalf("resolveExecutablePath failed: %v", err)
	}
	if got != "/tmp/swaves" {
		t.Fatalf("resolveExecutablePath = %q, want %q", got, "/tmp/swaves")
	}
}

func TestResolveExecutablePathFallsBackToCurrentExecutable(t *testing.T) {
	want, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}

	got, err := resolveExecutablePath("")
	if err != nil {
		t.Fatalf("resolveExecutablePath failed: %v", err)
	}
	if got != want {
		t.Fatalf("resolveExecutablePath = %q, want %q", got, want)
	}
}

func TestNormalizeConfigAppliesDefaults(t *testing.T) {
	cfg := Config{}
	cfg.normalize()

	if cfg.WorkerModeEnv != DefaultWorkerModeEnv {
		t.Fatalf("unexpected worker_mode_env=%q", cfg.WorkerModeEnv)
	}
	if cfg.ReadyTimeout != DefaultReadyTimeout {
		t.Fatalf("unexpected ready_timeout=%s", cfg.ReadyTimeout)
	}
	if cfg.ShutdownTimeout != DefaultShutdownTimeout {
		t.Fatalf("unexpected shutdown_timeout=%s", cfg.ShutdownTimeout)
	}
	if cfg.DrainTimeout != DefaultDrainTimeout {
		t.Fatalf("unexpected drain_timeout=%s", cfg.DrainTimeout)
	}
}

func TestRunRequiresWorkerCallback(t *testing.T) {
	err := Run(Config{})
	if err == nil || err.Error() != "worker callback is required" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRunWorkerModeCallsWorkerDirectly(t *testing.T) {
	const testEnv = "TEST_SUPERVISOR_WORKER_MODE"
	t.Setenv(testEnv, "1")
	called := false
	err := Run(Config{
		WorkerModeEnv: testEnv,
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

func TestRunMasterRequiresListenAddr(t *testing.T) {
	const testEnv = "TEST_SUPERVISOR_WORKER_MODE_ADDR"
	err := Run(Config{
		WorkerModeEnv: testEnv,
		Worker:        func() error { return nil },
	})
	if err == nil || err.Error() != "listen addr is required in master mode" {
		t.Fatalf("unexpected err=%v", err)
	}
}
