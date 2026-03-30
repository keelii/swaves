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
	MasterTitle     string
	WorkerTitle     string
	Args            []string
	Worker          func() error
}

type workerProcess struct {
	cmd      *exec.Cmd
	exitErr  error
	exitDone chan struct{}
	readyCh  <-chan error
}

func runSupervisor(cfg supervisorConfig) error {
	if cfg.Worker == nil {
		return fmt.Errorf("worker callback is required")
	}

	if os.Getenv(defaultWorkerModeEnv) == "1" {
		if cfg.WorkerTitle != "" {
			proctitle.Set(cfg.WorkerTitle)
		}
		return cfg.Worker()
	}
	if !cfg.DaemonMode {
		return cfg.Worker()
	}
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen addr is required in daemon mode")
	}

	normalizeSupervisorConfig(&cfg)

	if cfg.MasterTitle != "" {
		proctitle.Set(cfg.MasterTitle)
	}

	// master Listen port
	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	defer func() { _ = ln.Close() }()

	active, err := startReadyWorker(ln, cfg)
	if err != nil {
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
				next, err := restartWorker(active, ln, cfg)
				if err != nil {
					logger.Error("[master] restart worker failed: %v", err)
					continue
				}
				active = next
				consecutiveFailures = 0
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("[master] shutdown requested by signal: %s", sig)
				return stopWorkerProcess(active, cfg.ShutdownTimeout)
			}
		case <-active.exitDone:
			if active.exitErr != nil {
				consecutiveFailures++
				logger.Error("[master] worker exited: %v", active.exitErr)
				if cfg.MaxFailures > 0 && consecutiveFailures >= cfg.MaxFailures {
					return fmt.Errorf("worker failed %d times continuously, reached max-failures=%d", consecutiveFailures, cfg.MaxFailures)
				}
			} else {
				consecutiveFailures = 0
				logger.Info("[master] worker exited")
			}
			time.Sleep(cfg.RestartDelay)
			next, startErr := startReadyWorker(ln, cfg)
			if startErr != nil {
				return startErr
			}
			active = next
		}
	}
}

func normalizeSupervisorConfig(cfg *supervisorConfig) {
	if cfg == nil {
		return
	}
	if cfg.RestartDelay <= 0 {
		cfg.RestartDelay = 300 * time.Millisecond
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultWorkerReadyTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultWorkerStopTimeout
	}
}

func restartWorker(active *workerProcess, listener net.Listener, cfg supervisorConfig) (*workerProcess, error) {
	next, err := startReadyWorker(listener, cfg)
	if err != nil {
		return nil, err
	}
	if err := stopWorkerProcess(active, cfg.ShutdownTimeout); err != nil {
		logger.Error("[master] stop previous worker failed: %v", err)
	}
	return next, nil
}

func startReadyWorker(listener net.Listener, cfg supervisorConfig) (*workerProcess, error) {
	worker, err := startWorkerProcess(listener, cfg.Args)
	if err != nil {
		return nil, err
	}
	if err := waitWorkerReady(worker, cfg.ReadyTimeout); err != nil {
		_ = stopWorkerProcess(worker, cfg.ShutdownTimeout)
		return nil, err
	}
	return worker, nil
}

func startWorkerProcess(listener net.Listener, args []string) (*workerProcess, error) {
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

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable failed: %w", err)
	}

	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{listenerDup, readyWriter}
	cmd.Env = workerEnv()
	if err := cmd.Start(); err != nil {
		_ = readyReader.Close()
		return nil, fmt.Errorf("start worker failed: %w", err)
	}
	_ = readyWriter.Close()

	worker := &workerProcess{
		cmd:      cmd,
		exitDone: make(chan struct{}),
	}

	go func() {
		worker.exitErr = cmd.Wait()
		close(worker.exitDone)
	}()

	readyCh := make(chan error, 1)
	go func() {
		defer close(readyCh)
		readyCh <- readWorkerReady(readyReader)
	}()
	worker.readyCh = readyCh

	logger.Info("[master] worker started pid=%d", cmd.Process.Pid)
	return worker, nil
}

func workerEnv() []string {
	return append(os.Environ(),
		defaultWorkerModeEnv+"=1",
		workerListenerFDEnv+"="+strconv.Itoa(workerListenerFD),
		workerReadyFDEnv+"="+strconv.Itoa(workerReadyFD),
	)
}

func readWorkerReady(reader *os.File) error {
	defer func() { _ = reader.Close() }()

	message, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read worker ready failed: %w", err)
	}
	if strings.TrimSpace(message) != workerReadyMessage {
		return fmt.Errorf("unexpected worker ready message: %q", strings.TrimSpace(message))
	}
	return nil
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
	case <-worker.exitDone:
		if worker.exitErr != nil {
			return fmt.Errorf("worker exited before ready: %w", worker.exitErr)
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
	case <-worker.exitDone:
		if worker.exitErr != nil {
			return fmt.Errorf("worker exit after SIGTERM failed: %w", worker.exitErr)
		}
		logger.Info("[master] worker stopped gracefully")
		return nil
	case <-time.After(timeout):
		if err := worker.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill worker after timeout failed: %w", err)
		}
		<-worker.exitDone
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
