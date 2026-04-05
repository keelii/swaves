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
	"swaves/internal/platform/updater"
	"syscall"
	"time"
)

const (
	defaultWorkerModeEnv      = "SWAVES_RUN_MODE"
	defaultWorkerStopTimeout  = 8 * time.Second
	defaultWorkerReadyTimeout = 8 * time.Second
	defaultWorkerRestartDelay = 300 * time.Millisecond
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
	cmd     *exec.Cmd
	done    chan struct{}
	ready   chan error
	exitErr error
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
		return fmt.Errorf("daemon mode is required")
	}
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen addr is required in daemon mode")
	}

	normalizeSupervisorConfig(&cfg)
	if cfg.MasterTitle != "" {
		proctitle.Set(cfg.MasterTitle)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable failed: %w", err)
	}
	if err := updater.WriteRuntimeInfo(updater.RuntimeInfo{
		PID:        os.Getpid(),
		Executable: execPath,
	}); err != nil {
		return err
	}
	defer func() { _ = updater.RemoveRuntimeInfo() }()

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	defer func() { _ = ln.Close() }()

	active, err := spawnWorker(ln, cfg)
	if err != nil {
		return err
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	failures := 0
	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				logger.Info("[master] restart requested by signal: %s", sig)
				next, err := spawnWorker(ln, cfg)
				if err != nil {
					logger.Error("[master] restart worker failed: %v", err)
					continue
				}
				if err := stopWorkerProcess(active, cfg.ShutdownTimeout); err != nil {
					logger.Error("[master] stop previous worker failed: %v", err)
				}
				active = next
				failures = 0
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("[master] shutdown requested by signal: %s", sig)
				return stopWorkerProcess(active, cfg.ShutdownTimeout)
			}
		case <-active.done:
			if active.exitErr != nil {
				failures++
				logger.Error("[master] worker exited: %v", active.exitErr)
				if cfg.MaxFailures > 0 && failures >= cfg.MaxFailures {
					return fmt.Errorf("worker failed %d times continuously, reached max-failures=%d", failures, cfg.MaxFailures)
				}
			} else {
				failures = 0
				logger.Info("[master] worker exited")
			}

			time.Sleep(cfg.RestartDelay)
			active, err = spawnWorker(ln, cfg)
			if err != nil {
				return err
			}
		}
	}
}

func normalizeSupervisorConfig(cfg *supervisorConfig) {
	if cfg == nil {
		return
	}
	if cfg.RestartDelay <= 0 {
		cfg.RestartDelay = defaultWorkerRestartDelay
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultWorkerReadyTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultWorkerStopTimeout
	}
}

func spawnWorker(listener net.Listener, cfg supervisorConfig) (*workerProcess, error) {
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

	cmd := exec.Command(execPath, cfg.Args...)
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
		cmd:   cmd,
		done:  make(chan struct{}),
		ready: make(chan error, 1),
	}

	go func() {
		worker.exitErr = cmd.Wait()
		close(worker.done)
	}()

	go func() {
		defer close(worker.ready)
		worker.ready <- readWorkerReady(readyReader)
	}()

	logger.Info("[master] worker started pid=%d", cmd.Process.Pid)

	select {
	case err := <-worker.ready:
		if err != nil {
			_ = stopWorkerProcess(worker, cfg.ShutdownTimeout)
			return nil, err
		}
		logger.Info("[master] worker ready pid=%d", worker.cmd.Process.Pid)
		return worker, nil
	case <-worker.done:
		if worker.exitErr != nil {
			return nil, fmt.Errorf("worker exited before ready: %w", worker.exitErr)
		}
		return nil, fmt.Errorf("worker exited before ready")
	case <-time.After(cfg.ReadyTimeout):
		_ = stopWorkerProcess(worker, cfg.ShutdownTimeout)
		return nil, fmt.Errorf("worker ready timeout after %s", cfg.ReadyTimeout)
	}
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

func stopWorkerProcess(worker *workerProcess, timeout time.Duration) error {
	if worker == nil || worker.cmd == nil || worker.cmd.Process == nil {
		return nil
	}

	if err := worker.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal worker SIGTERM failed: %w", err)
	}

	select {
	case <-worker.done:
		if worker.exitErr != nil {
			return fmt.Errorf("worker exit after SIGTERM failed: %w", worker.exitErr)
		}
		logger.Info("[master] worker stopped gracefully")
		return nil
	case <-time.After(timeout):
		if err := worker.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill worker after timeout failed: %w", err)
		}
		<-worker.done
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
