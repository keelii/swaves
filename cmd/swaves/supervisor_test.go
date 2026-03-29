package main

import (
	"errors"
	"testing"
)

func TestRunSupervisorRequiresWorkerCallback(t *testing.T) {
	err := runSupervisor(supervisorConfig{})
	if err == nil || err.Error() != "worker callback is required" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRunSupervisorWorkerModeUsesWorkerDirectly(t *testing.T) {
	t.Setenv("TEST_WORKER_MODE", "1")
	called := false
	err := runSupervisor(supervisorConfig{
		WorkerModeEnv: "TEST_WORKER_MODE",
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
