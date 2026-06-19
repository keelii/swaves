package supervisor

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
	"syscall"
	"time"
)

// Environment variable names and fd numbers for the master↔worker IPC protocol.
// These are used internally by both spawnWorker (master side) and the worker-side
// helpers InheritedListener / SignalReady. Callers do not need these directly.
const (
	listenerFDEnv = "SUPERVISOR_LISTENER_FD"
	readyFDEnv    = "SUPERVISOR_READY_FD"
	listenerFD    = 3
	readyFD       = 4
	readyMessage  = "READY"
)

// Default timeouts applied when the corresponding Config field is zero.
const (
	DefaultWorkerModeEnv   = "SUPERVISOR_WORKER_MODE"
	DefaultReadyTimeout    = 8 * time.Second
	DefaultShutdownTimeout = 12 * time.Second
	DefaultDrainTimeout    = 5 * time.Second
)

// Process represents a running worker process managed by the supervisor.
type Process struct {
	cmd     *exec.Cmd
	done    chan struct{}
	exitErr error
}

// PID returns the OS process ID, or 0 if the process is nil.
func (p *Process) PID() int {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return 0
	}
	return p.cmd.Process.Pid
}

// Done returns a channel that is closed when the process exits.
func (p *Process) Done() <-chan struct{} {
	if p == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return p.done
}

// ExitErr returns the error reported by the process on exit, if any.
func (p *Process) ExitErr() error {
	if p == nil {
		return nil
	}
	return p.exitErr
}

// SpawnFn spawns a new worker process on the given listener using the
// supervisor's current configuration.
type SpawnFn func(ln net.Listener) (*Process, error)

// Config holds the configuration for the supervisor.
type Config struct {
	// ListenAddr is the TCP address to listen on. Required in master mode.
	ListenAddr string

	// Worker is called directly when the process is in worker mode.
	// Required (even in master mode, validated up-front).
	Worker func() error

	// WorkerModeEnv is the env var name that signals worker mode.
	// Defaults to DefaultWorkerModeEnv.
	WorkerModeEnv string

	// ExecutablePath overrides the worker executable. Defaults to os.Executable().
	ExecutablePath string

	// Args are the CLI args forwarded to worker processes verbatim.
	Args []string

	// ExtraEnv holds additional env vars appended to os.Environ() for workers.
	ExtraEnv []string

	// MaxFailures is the max consecutive worker failures before the supervisor
	// exits with an error. Zero or negative means unlimited restarts.
	MaxFailures int

	// ReadyTimeout is how long to wait for a worker to signal ready.
	// Defaults to DefaultReadyTimeout.
	ReadyTimeout time.Duration

	// ShutdownTimeout is how long to wait for a worker to stop before killing it.
	// Defaults to DefaultShutdownTimeout.
	ShutdownTimeout time.Duration

	// DrainTimeout is how long to wait after sending SIGUSR1 before proceeding.
	// Defaults to DefaultDrainTimeout.
	DrainTimeout time.Duration

	// OnSIGHUP is called when the master receives SIGHUP. It receives the active
	// listener, the current worker process, and a function to spawn a replacement.
	// Return the new active process (may be the same as active on partial failure).
	// Return a non-nil error to skip switching the active worker this cycle.
	// If nil, a default hot-reload is performed: spawn → drain old → stop old.
	OnSIGHUP func(ln net.Listener, active *Process, spawn SpawnFn) (*Process, error)

	// Log is an optional structured log function. level is "info", "warn", or "error".
	Log func(level, format string, args ...any)
}

func (cfg *Config) normalize() {
	if cfg.WorkerModeEnv == "" {
		cfg.WorkerModeEnv = DefaultWorkerModeEnv
	}
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = DefaultReadyTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = DefaultShutdownTimeout
	}
	if cfg.DrainTimeout <= 0 {
		cfg.DrainTimeout = DefaultDrainTimeout
	}
}

func (cfg *Config) logf(level, format string, args ...any) {
	if cfg.Log != nil {
		cfg.Log(level, format, args...)
	}
}

// Run is the main entry point for the supervisor pattern.
//
// Worker mode: if WorkerModeEnv is set to "1" in the environment, cfg.Worker()
// is called directly and Run returns its result.
//
// Master mode: Run listens on cfg.ListenAddr, spawns worker subprocesses, and
// manages their lifecycle. Signal handling:
//   - SIGHUP: hot-reload via cfg.OnSIGHUP (or default spawn→drain→stop).
//   - SIGINT/SIGTERM: graceful shutdown of the active worker.
//
// Worker processes receive the listener fd and a ready-pipe fd via ExtraFiles,
// and must call InheritedListener / SignalReady from this package.
func Run(cfg Config) error {
	if cfg.Worker == nil {
		return fmt.Errorf("worker callback is required")
	}
	cfg.normalize()

	if os.Getenv(cfg.WorkerModeEnv) == "1" {
		return cfg.Worker()
	}

	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen addr is required in master mode")
	}

	execPath, err := resolveExecutablePath(cfg.ExecutablePath)
	if err != nil {
		return fmt.Errorf("resolve executable failed: %w", err)
	}
	cfg.ExecutablePath = execPath

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	defer func() { _ = ln.Close() }()

	spawnFn := func(ln net.Listener) (*Process, error) {
		return spawnWorker(ln, cfg)
	}

	active, err := spawnFn(ln)
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
				cfg.logf("info", "[master] restart requested by signal: signal=%s worker_pid=%d", sig, active.PID())
				if cfg.OnSIGHUP != nil {
					next, err := cfg.OnSIGHUP(ln, active, spawnFn)
					if err != nil {
						// The callback is responsible for logging the error.
						continue
					}
					failures = 0
					active = next
				} else {
					next, err := spawnFn(ln)
					if err != nil {
						cfg.logf("error", "[master] restart worker failed: %v", err)
						continue
					}
					cfg.logf("info", "[master] replacement worker ready: previous_worker_pid=%d next_worker_pid=%d", active.PID(), next.PID())
					DrainProcess(active, cfg.DrainTimeout)
					if err := StopProcess(active, cfg.ShutdownTimeout); err != nil {
						cfg.logf("error", "[master] stop previous worker failed: %v", err)
					}
					failures = 0
					active = next
				}
			case syscall.SIGINT, syscall.SIGTERM:
				cfg.logf("info", "[master] shutdown requested by signal: %s", sig)
				return StopProcess(active, cfg.ShutdownTimeout)
			}
		case <-active.done:
			if active.exitErr != nil {
				failures++
				cfg.logf("error", "[master] worker exited: %v", active.exitErr)
				if cfg.MaxFailures > 0 && failures >= cfg.MaxFailures {
					return fmt.Errorf("worker failed %d times continuously, reached max-failures=%d", failures, cfg.MaxFailures)
				}
			} else {
				failures = 0
				cfg.logf("info", "[master] worker exited")
			}
			active, err = spawnFn(ln)
			if err != nil {
				return err
			}
		}
	}
}

// DrainProcess sends SIGUSR1 to the process and waits for it to exit within
// timeout. If the process does not exit in time, it is killed.
func DrainProcess(p *Process, timeout time.Duration) {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if err := p.cmd.Process.Signal(syscall.SIGUSR1); err != nil {
		if !errors.Is(err, os.ErrProcessDone) {
			return
		}
		return
	}
	select {
	case <-p.done:
	case <-time.After(timeout):
		_ = p.cmd.Process.Kill()
	}
}

// StopProcess sends SIGTERM to the process and waits for it to exit within
// timeout. If the process does not exit in time, it is killed.
func StopProcess(p *Process, timeout time.Duration) error {
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return nil
	}
	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal worker SIGTERM failed: %w", err)
	}
	select {
	case <-p.done:
		if p.exitErr != nil {
			return fmt.Errorf("worker exit after SIGTERM failed: %w", p.exitErr)
		}
		return nil
	case <-time.After(timeout):
		if err := p.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill worker after timeout failed: %w", err)
		}
		<-p.done
		return nil
	}
}

// spawnWorker forks a worker subprocess, passes the listener and ready-pipe
// via ExtraFiles, and waits until the worker signals ready.
func spawnWorker(ln net.Listener, cfg Config) (*Process, error) {
	listenerDup, err := listenerFile(ln)
	if err != nil {
		cfg.logf("error", "[master] spawn worker duplicate listener failed: err=%v", err)
		return nil, err
	}
	defer func() { _ = listenerDup.Close() }()

	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		cfg.logf("error", "[master] spawn worker create ready pipe failed: err=%v", err)
		return nil, fmt.Errorf("create ready pipe failed: %w", err)
	}
	defer func() { _ = readyWriter.Close() }()

	execPath := strings.TrimSpace(cfg.ExecutablePath)
	if execPath == "" {
		cfg.logf("error", "[master] spawn worker rejected: executable path is required")
		return nil, fmt.Errorf("worker executable path is required")
	}

	cmd := exec.Command(execPath, cfg.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{listenerDup, readyWriter}
	cmd.Env = buildWorkerEnv(cfg)

	if err := cmd.Start(); err != nil {
		_ = readyReader.Close()
		cfg.logf("error", "[master] spawn worker start process failed: executable=%s err=%v", execPath, err)
		return nil, fmt.Errorf("start worker failed: %w", err)
	}
	_ = readyWriter.Close()

	p := &Process{
		cmd:  cmd,
		done: make(chan struct{}),
	}

	readyCh := make(chan error, 1)

	go func() {
		p.exitErr = cmd.Wait()
		close(p.done)
	}()

	go func() {
		readyCh <- readWorkerReady(readyReader)
	}()

	cfg.logf("info", "[master] worker started pid=%d", cmd.Process.Pid)

	select {
	case err := <-readyCh:
		if err != nil {
			_ = StopProcess(p, cfg.ShutdownTimeout)
			cfg.logf("error", "[master] spawn worker ready failed: pid=%d err=%v", p.PID(), err)
			return nil, err
		}
		cfg.logf("info", "[master] worker ready pid=%d", p.PID())
		return p, nil
	case <-p.done:
		if p.exitErr != nil {
			cfg.logf("error", "[master] spawn worker exited before ready: pid=%d err=%v", p.PID(), p.exitErr)
			return nil, fmt.Errorf("worker exited before ready: %w", p.exitErr)
		}
		cfg.logf("warn", "[master] spawn worker exited before ready: pid=%d", p.PID())
		return nil, fmt.Errorf("worker exited before ready")
	case <-time.After(cfg.ReadyTimeout):
		_ = StopProcess(p, cfg.ShutdownTimeout)
		cfg.logf("error", "[master] spawn worker ready timeout: pid=%d timeout=%s", p.PID(), cfg.ReadyTimeout)
		return nil, fmt.Errorf("worker ready timeout after %s", cfg.ReadyTimeout)
	}
}

// buildWorkerEnv constructs the env slice passed to a worker subprocess.
// It starts from os.Environ(), adds the IPC protocol vars, then appends ExtraEnv.
func buildWorkerEnv(cfg Config) []string {
	env := append(os.Environ(),
		cfg.WorkerModeEnv+"=1",
		listenerFDEnv+"="+strconv.Itoa(listenerFD),
		readyFDEnv+"="+strconv.Itoa(readyFD),
	)
	return append(env, cfg.ExtraEnv...)
}

// listenerFile duplicates the TCP listener's underlying file descriptor.
func listenerFile(ln net.Listener) (*os.File, error) {
	tcpLn, ok := ln.(*net.TCPListener)
	if !ok {
		return nil, fmt.Errorf("unsupported listener type %T", ln)
	}
	file, err := tcpLn.File()
	if err != nil {
		return nil, fmt.Errorf("duplicate listener fd failed: %w", err)
	}
	return file, nil
}

// readWorkerReady reads the ready message from the pipe written by the worker.
func readWorkerReady(reader *os.File) error {
	defer func() { _ = reader.Close() }()
	message, err := bufio.NewReader(reader).ReadString('\n')
	if err != nil {
		return fmt.Errorf("read worker ready failed: %w", err)
	}
	if strings.TrimSpace(message) != readyMessage {
		return fmt.Errorf("unexpected worker ready message: %q", strings.TrimSpace(message))
	}
	return nil
}

// resolveExecutablePath returns path if non-empty, otherwise os.Executable().
func resolveExecutablePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		return path, nil
	}
	return os.Executable()
}
