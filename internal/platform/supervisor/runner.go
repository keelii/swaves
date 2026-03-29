package supervisor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"swaves/internal/platform/logger"
	"time"
)

const defaultWorkerModeEnv = "SWAVES_RUN_MODE"

type Config struct {
	DaemonMode    bool
	MaxFailures   int
	RestartDelay  time.Duration
	WorkerModeEnv string
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
		return cfg.Worker()
	}
	if !cfg.DaemonMode {
		return cfg.Worker()
	}

	restartDelay := cfg.RestartDelay
	if restartDelay <= 0 {
		restartDelay = 300 * time.Millisecond
	}

	consecutiveFailures := 0
	for {
		cmd, err := startWorkerProcess(workerModeEnv, cfg.Args)
		if err != nil {
			return err
		}

		err = cmd.Wait()
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

func startWorkerProcess(workerModeEnv string, args []string) (*exec.Cmd, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable failed: %w", err)
	}

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), workerModeEnv+"=1")
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker failed: %w", err)
	}

	logger.Info("[master] worker started pid=%d", cmd.Process.Pid)
	return cmd, nil
}
