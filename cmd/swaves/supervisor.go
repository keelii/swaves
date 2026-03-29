package main

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/proctitle"
	"syscall"
	"time"
)

const (
	defaultWorkerModeEnv      = "SWAVES_RUN_MODE"
	defaultWorkerStopTimeout  = 8 * time.Second
	defaultWorkerReadyTimeout = 8 * time.Second
	workerListenerFDEnv       = "SWAVES_LISTENER_FD"
	workerReadyFDEnv          = "SWAVES_READY_FD"
	workerReadyMessage        = "READY"
	workerListenerFD          = 3
	workerReadyFD             = 4
)

type supervisorConfig struct {
	DaemonMode      bool
	ListenAddr      string
	MaxFailures     int
	RestartDelay    time.Duration
	ReadyTimeout    time.Duration
	ShutdownTimeout time.Duration
	WorkerModeEnv   string
	MasterTitle     string
	WorkerTitle     string
	Args            []string
	Worker          func() error
}

type workerProcess struct {
	cmd     *exec.Cmd
	waitErr error
	waitCh  chan struct{}
	readyCh <-chan error
}

func runSupervisor(cfg supervisorConfig) error {
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
	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return fmt.Errorf("listen addr is required in daemon mode")
	}
	if strings.TrimSpace(cfg.MasterTitle) != "" {
		proctitle.Set(cfg.MasterTitle)
	}

	restartDelay := cfg.RestartDelay
	if restartDelay <= 0 {
		restartDelay = 300 * time.Millisecond
	}
	readyTimeout := cfg.ReadyTimeout
	if readyTimeout <= 0 {
		readyTimeout = defaultWorkerReadyTimeout
	}
	shutdownTimeout := cfg.ShutdownTimeout
	if shutdownTimeout <= 0 {
		shutdownTimeout = defaultWorkerStopTimeout
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	defer func() { _ = ln.Close() }()

	active, err := startWorkerProcess(ln, workerModeEnv, cfg.Args)
	if err != nil {
		return err
	}
	if err := waitWorkerReady(active, readyTimeout); err != nil {
		_ = stopWorkerProcess(active, shutdownTimeout)
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	consecutiveFailures := 0
	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				logger.Info("[master] restart requested by signal: %s", sig)
				next, err := startWorkerProcess(ln, workerModeEnv, cfg.Args)
				if err != nil {
					logger.Error("[master] start replacement worker failed: %v", err)
					continue
				}
				if err := waitWorkerReady(next, readyTimeout); err != nil {
					logger.Error("[master] replacement worker not ready: %v", err)
					_ = stopWorkerProcess(next, shutdownTimeout)
					continue
				}
				if err := stopWorkerProcess(active, shutdownTimeout); err != nil {
					logger.Error("[master] stop previous worker failed: %v", err)
				}
				active = next
				consecutiveFailures = 0
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("[master] shutdown requested by signal: %s", sig)
				return stopWorkerProcess(active, shutdownTimeout)
			}
		case <-active.waitCh:
			if active.waitErr != nil {
				consecutiveFailures++
				logger.Error("[master] worker exited: %v", active.waitErr)
				if cfg.MaxFailures > 0 && consecutiveFailures >= cfg.MaxFailures {
					return fmt.Errorf("worker failed %d times continuously, reached max-failures=%d", consecutiveFailures, cfg.MaxFailures)
				}
			} else {
				consecutiveFailures = 0
				logger.Info("[master] worker exited")
			}
			time.Sleep(restartDelay)
			next, startErr := startWorkerProcess(ln, workerModeEnv, cfg.Args)
			if startErr != nil {
				return startErr
			}
			if readyErr := waitWorkerReady(next, readyTimeout); readyErr != nil {
				_ = stopWorkerProcess(next, shutdownTimeout)
				return readyErr
			}
			active = next
		}
	}
}

func startWorkerProcess(listener net.Listener, workerModeEnv string, args []string) (*workerProcess, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable failed: %w", err)
	}

	listenerDup, err := listenerFile(listener)
	if err != nil {
		return nil, err
	}
	defer func() { _ = listenerDup.Close() }()

	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create ready pipe failed: %w", err)
	}
	defer func() { _ = readyWriter.Close() }()

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{listenerDup, readyWriter}
	cmd.Env = append(os.Environ(),
		workerModeEnv+"=1",
		workerListenerFDEnv+"="+strconv.Itoa(workerListenerFD),
		workerReadyFDEnv+"="+strconv.Itoa(workerReadyFD),
	)
	if err := cmd.Start(); err != nil {
		_ = readyReader.Close()
		return nil, fmt.Errorf("start worker failed: %w", err)
	}
	_ = readyWriter.Close()

	worker := &workerProcess{
		cmd:    cmd,
		waitCh: make(chan struct{}),
	}

	go func() {
		worker.waitErr = cmd.Wait()
		close(worker.waitCh)
	}()

	readyCh := make(chan error, 1)
	go func() {
		defer close(readyCh)
		defer func() { _ = readyReader.Close() }()

		message, err := bufio.NewReader(readyReader).ReadString('\n')
		if err != nil {
			readyCh <- fmt.Errorf("read worker ready failed: %w", err)
			return
		}
		if strings.TrimSpace(message) != workerReadyMessage {
			readyCh <- fmt.Errorf("unexpected worker ready message: %q", strings.TrimSpace(message))
			return
		}
		readyCh <- nil
	}()
	worker.readyCh = readyCh

	logger.Info("[master] worker started pid=%d", cmd.Process.Pid)
	return worker, nil
}

func waitWorkerReady(worker *workerProcess, timeout time.Duration) error {
	if worker == nil {
		return fmt.Errorf("worker is required")
	}
	if timeout <= 0 {
		timeout = defaultWorkerReadyTimeout
	}

	select {
	case err := <-worker.readyCh:
		if err != nil {
			return err
		}
		logger.Info("[master] worker ready pid=%d", worker.cmd.Process.Pid)
		return nil
	case <-worker.waitCh:
		if worker.waitErr != nil {
			return fmt.Errorf("worker exited before ready: %w", worker.waitErr)
		}
		return fmt.Errorf("worker exited before ready")
	case <-time.After(timeout):
		return fmt.Errorf("worker ready timeout after %s", timeout)
	}
}

func stopWorkerProcess(worker *workerProcess, timeout time.Duration) error {
	if worker == nil || worker.cmd == nil || worker.cmd.Process == nil {
		return nil
	}

	if err := worker.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal worker SIGTERM failed: %w", err)
	}

	select {
	case <-worker.waitCh:
		if worker.waitErr != nil {
			return fmt.Errorf("worker exit after SIGTERM failed: %w", worker.waitErr)
		}
		logger.Info("[master] worker stopped gracefully")
		return nil
	case <-time.After(timeout):
		if err := worker.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill worker after timeout failed: %w", err)
		}
		<-worker.waitCh
		logger.Warn("[master] worker killed after timeout")
		return nil
	}
}

func listenerFile(listener net.Listener) (*os.File, error) {
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		return nil, fmt.Errorf("unsupported listener type %T", listener)
	}
	file, err := tcpListener.File()
	if err != nil {
		return nil, fmt.Errorf("duplicate listener fd failed: %w", err)
	}
	return file, nil
}
