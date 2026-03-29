package supervisor

import (
	"errors"
	"testing"
)

func TestRunRequiresWorkerCallback(t *testing.T) {
	err := Run(Config{})
	if err == nil || err.Error() != "worker callback is required" {
		t.Fatalf("unexpected err=%v", err)
	}
}

func TestRunWorkerModeUsesWorkerDirectly(t *testing.T) {
	t.Setenv("TEST_WORKER_MODE", "1")
	called := false
	err := Run(Config{
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

func TestRunNonDaemonUsesWorkerDirectly(t *testing.T) {
	expected := errors.New("worker failed")
	err := Run(Config{
		DaemonMode: false,
		Worker: func() error {
			return expected
		},
	})
	if !errors.Is(err, expected) {
		t.Fatalf("unexpected err=%v", err)
	}
}
