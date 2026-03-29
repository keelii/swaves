package main

import (
	"errors"
	"net"
	"os/exec"
	"testing"
	"time"
)

func TestRunSupervisorRequiresWorkerCallback(t *testing.T) {
	err := runSupervisor(supervisorConfig{})
	if err == nil || err.Error() != "worker callback is required" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRunSupervisorWorkerModeUsesWorkerDirectly(t *testing.T) {
	t.Setenv(defaultWorkerModeEnv, "1")
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

func TestRunSupervisorNonDaemonUsesWorkerDirectly(t *testing.T) {
	expected := errors.New("worker failed")
	err := runSupervisor(supervisorConfig{
		DaemonMode: false,
		Worker: func() error {
			return expected
		},
	})
	if !errors.Is(err, expected) {
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

func TestWaitWorkerReadyReturnsExitError(t *testing.T) {
	worker := &workerProcess{
		cmd:      &exec.Cmd{},
		exitErr:  errors.New("boom"),
		exitDone: make(chan struct{}),
		readyCh:  make(chan error),
	}
	close(worker.exitDone)

	err := waitWorkerReady(worker, 50*time.Millisecond)
	if err == nil || err.Error() != "worker exited before ready: boom" {
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
