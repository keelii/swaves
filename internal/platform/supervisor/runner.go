package supervisor

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/proctitle"
	"syscall"
	"time"
)

const defaultWorkerModeEnv = "SWAVES_RUN_MODE"
const defaultWorkerStopTimeout = 8 * time.Second

type Config struct {
	DaemonMode    bool
	MaxFailures   int
	RestartDelay  time.Duration
	WorkerModeEnv string
	MasterTitle   string
	WorkerTitle   string
	Args          []string
	Worker        func() error
}

func Run(cfg Config) error {
	if cfg.Worker == nil {
		return fmt.Errorf("worker callback is required")
	}

	workerModeEnv := strings.TrimSpace(cfg.WorkerModeEnv)
	if workerModeEnv == "" {
		workerModeEnv = defaultWorkerModeEnv
	}

	if os.Getenv(workerModeEnv) == "1" {
		if strings.TrimSpace(cfg.WorkerTitle) != "" {
			proctitle.Set(cfg.WorkerTitle)
		}
		return cfg.Worker()
	}
	if !cfg.DaemonMode {
		return cfg.Worker()
	}
	if strings.TrimSpace(cfg.MasterTitle) != "" {
		proctitle.Set(cfg.MasterTitle)
	}

	restartDelay := cfg.RestartDelay
	if restartDelay <= 0 {
		restartDelay = 300 * time.Millisecond
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	consecutiveFailures := 0
	for {
		cmd, waitCh, err := startWorkerProcess(workerModeEnv, cfg.Args)
		if err != nil {
			return err
		}

		select {
		case sig := <-sigCh:
			logger.Info("[master] shutdown requested by signal: %s", sig)
			return stopWorkerProcess(cmd, waitCh, defaultWorkerStopTimeout)
		case err := <-waitCh:
			if err != nil {
				consecutiveFailures++
				logger.Error("[master] worker exited: %v", err)
				if cfg.MaxFailures > 0 && consecutiveFailures >= cfg.MaxFailures {
					return fmt.Errorf("worker failed %d times continuously, reached max-failures=%d", consecutiveFailures, cfg.MaxFailures)
				}
			} else {
				consecutiveFailures = 0
				logger.Info("[master] worker exited")
			}
			time.Sleep(restartDelay)
		}
	}
}

func startWorkerProcess(workerModeEnv string, args []string) (*exec.Cmd, <-chan error, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, nil, fmt.Errorf("resolve executable failed: %w", err)
	}

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), workerModeEnv+"=1")
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("start worker failed: %w", err)
	}

	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	logger.Info("[master] worker started pid=%d", cmd.Process.Pid)
	return cmd, waitCh, nil
}

func stopWorkerProcess(cmd *exec.Cmd, waitCh <-chan error, timeout time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal worker SIGTERM failed: %w", err)
	}

	select {
	case err := <-waitCh:
		if err != nil {
			return fmt.Errorf("worker exit after SIGTERM failed: %w", err)
		}
		logger.Info("[master] worker stopped gracefully")
		return nil
	case <-time.After(timeout):
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill worker after timeout failed: %w", err)
		}
		<-waitCh
		logger.Warn("[master] worker killed after timeout")
		return nil
	}
}
