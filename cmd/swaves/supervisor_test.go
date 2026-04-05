package main

import (
	"errors"
	"net"
	"os"
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
