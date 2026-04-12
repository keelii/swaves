package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/updater"
	"syscall"
	"time"
)

const (
	daemonModeConfigEnv           = "SWAVES_DAEMON_MODE"
	workerModeEnv                 = "SWAVES_RUN_MODE"
	workerProcessFlag             = "--worker-process"
	workerGracefulShutdownTimeout = 8 * time.Second
	defaultWorkerStopTimeout      = workerGracefulShutdownTimeout + 4*time.Second
	defaultWorkerReadyTimeout     = 8 * time.Second
	workerListenerFDEnv           = "SWAVES_LISTENER_FD"
	workerReadyFDEnv              = "SWAVES_READY_FD"
	workerReadyMessage            = "READY"
	workerListenerFD              = 3
	workerReadyFD                 = 4
)

type supervisorConfig struct {
	DaemonMode      bool
	ListenAddr      string
	SqliteFile      string
	MaxFailures     int
	ReadyTimeout    time.Duration
	ShutdownTimeout time.Duration
	ExecutablePath  string
	Args            []string
	Worker          func() error
}

type workerProcess struct {
	cmd     *exec.Cmd
	done    chan struct{}
	ready   chan error
	exitErr error
}

func workerPID(worker *workerProcess) int {
	if worker == nil || worker.cmd == nil || worker.cmd.Process == nil {
		return 0
	}
	return worker.cmd.Process.Pid
}

func runSupervisor(cfg supervisorConfig) error {
	if cfg.Worker == nil {
		return fmt.Errorf("worker callback is required")
	}
	if os.Getenv(workerModeEnv) == "1" {
		return cfg.Worker()
	}
	if !cfg.DaemonMode {
		return fmt.Errorf("daemon mode is required")
	}
	if cfg.ListenAddr == "" {
		return fmt.Errorf("listen addr is required in daemon mode")
	}

	normalizeSupervisorConfig(&cfg)

	execPath, err := resolveSupervisorExecutablePath(cfg.ExecutablePath)
	if err != nil {
		return fmt.Errorf("resolve executable failed: %w", err)
	}
	cfg.ExecutablePath = execPath
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
				logger.Info("[master] restart requested by signal: signal=%s worker_pid=%d", sig, workerPID(active))
				restoreRequest, restoreErr := updater.ReadRestoreRequest()
				switch {
				case restoreErr == nil:
					logger.Info("[master] restart entering restore flow: worker_pid=%d source=%s", workerPID(active), strings.TrimSpace(restoreRequest.Source))
					next, err := restoreWorkerProcess(ln, active, cfg, restoreRequest)
					if err != nil {
						logger.Error("[master] restore worker failed: %v", err)
						continue
					}
					logger.Info("[master] restore flow switched worker: previous_worker_pid=%d next_worker_pid=%d", workerPID(active), workerPID(next))
					active = next
					failures = 0
				case errors.Is(restoreErr, updater.ErrRestoreRequestNotFound):
					logger.Info("[master] restart spawning replacement worker: worker_pid=%d", workerPID(active))
					next, err := spawnWorker(ln, cfg)
					if err != nil {
						logger.Error("[master] restart worker failed: %v", err)
						continue
					}
					logger.Info("[master] replacement worker ready: previous_worker_pid=%d next_worker_pid=%d", workerPID(active), workerPID(next))
					if err := stopWorkerProcess(active, cfg.ShutdownTimeout); err != nil {
						logger.Error("[master] stop previous worker failed: %v", err)
					}
					active = next
					failures = 0
				default:
					logger.Error("[master] read restore request failed: %v", restoreErr)
				}
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
	if cfg.ReadyTimeout <= 0 {
		cfg.ReadyTimeout = defaultWorkerReadyTimeout
	}
	if cfg.ShutdownTimeout <= 0 {
		cfg.ShutdownTimeout = defaultWorkerStopTimeout
	}
}

func spawnWorker(listener net.Listener, cfg supervisorConfig) (*workerProcess, error) {
	logger.Info("[master] spawn worker start: executable=%s args=%v", strings.TrimSpace(cfg.ExecutablePath), workerArgs(cfg.Args))
	listenerDup, err := listenerFile(listener)
	if err != nil {
		logger.Error("[master] spawn worker duplicate listener failed: err=%v", err)
		return nil, err
	}
	defer func() { _ = listenerDup.Close() }()

	readyReader, readyWriter, err := os.Pipe()
	if err != nil {
		logger.Error("[master] spawn worker create ready pipe failed: err=%v", err)
		return nil, fmt.Errorf("create ready pipe failed: %w", err)
	}
	defer func() { _ = readyWriter.Close() }()

	execPath := strings.TrimSpace(cfg.ExecutablePath)
	if execPath == "" {
		logger.Error("[master] spawn worker rejected: executable path is required")
		return nil, fmt.Errorf("worker executable path is required")
	}

	cmd := exec.Command(execPath, workerArgs(cfg.Args)...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{listenerDup, readyWriter}
	cmd.Env = workerEnv()
	if err := cmd.Start(); err != nil {
		_ = readyReader.Close()
		logger.Error("[master] spawn worker start process failed: executable=%s err=%v", execPath, err)
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
			logger.Error("[master] spawn worker ready failed: pid=%d err=%v", worker.cmd.Process.Pid, err)
			return nil, err
		}
		logger.Info("[master] worker ready pid=%d", worker.cmd.Process.Pid)
		return worker, nil
	case <-worker.done:
		if worker.exitErr != nil {
			logger.Error("[master] spawn worker exited before ready: pid=%d err=%v", worker.cmd.Process.Pid, worker.exitErr)
			return nil, fmt.Errorf("worker exited before ready: %w", worker.exitErr)
		}
		logger.Warn("[master] spawn worker exited before ready: pid=%d", worker.cmd.Process.Pid)
		return nil, fmt.Errorf("worker exited before ready")
	case <-time.After(cfg.ReadyTimeout):
		_ = stopWorkerProcess(worker, cfg.ShutdownTimeout)
		logger.Error("[master] spawn worker ready timeout: pid=%d timeout=%s", worker.cmd.Process.Pid, cfg.ReadyTimeout)
		return nil, fmt.Errorf("worker ready timeout after %s", cfg.ReadyTimeout)
	}
}

func resolveSupervisorExecutablePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		return path, nil
	}
	return os.Executable()
}

func workerEnv() []string {
	return append(os.Environ(),
		workerModeEnv+"=1",
		workerListenerFDEnv+"="+strconv.Itoa(workerListenerFD),
		workerReadyFDEnv+"="+strconv.Itoa(workerReadyFD),
	)
}

func workerArgs(args []string) []string {
	if len(args) == 0 {
		return []string{workerProcessFlag}
	}

	next := make([]string, 0, len(args)+1)
	for _, arg := range args {
		if strings.TrimSpace(arg) == workerProcessFlag {
			return append(next, args[len(next):]...)
		}
		next = append(next, arg)
	}
	return append(next, workerProcessFlag)
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
	startAt := time.Now()
	pid := worker.cmd.Process.Pid
	logger.Info("[master] stopping worker pid=%d timeout=%s", pid, timeout)

	if err := worker.cmd.Process.Signal(syscall.SIGTERM); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return fmt.Errorf("signal worker SIGTERM failed: %w", err)
	}
	logger.Info("[master] worker SIGTERM sent pid=%d", pid)

	select {
	case <-worker.done:
		if worker.exitErr != nil {
			return fmt.Errorf("worker exit after SIGTERM failed: %w", worker.exitErr)
		}
		logger.Info("[master] worker stopped gracefully pid=%d elapsed=%s", pid, time.Since(startAt))
		return nil
	case <-time.After(timeout):
		if err := worker.cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("kill worker after timeout failed: %w", err)
		}
		<-worker.done
		logger.Warn("[master] worker killed after timeout pid=%d timeout=%s elapsed=%s", pid, timeout, time.Since(startAt))
		return nil
	}
}

func listenerFile(listener net.Listener) (*os.File, error) {
	tcpListener, ok := listener.(*net.TCPListener)
	if !ok {
		logger.Error("[master] duplicate listener rejected: type=%T", listener)
		return nil, fmt.Errorf("unsupported listener type %T", listener)
	}
	file, err := tcpListener.File()
	if err != nil {
		logger.Error("[master] duplicate listener fd failed: err=%v", err)
		return nil, fmt.Errorf("duplicate listener fd failed: %w", err)
	}
	logger.Info("[master] duplicate listener fd success")
	return file, nil
}

func restoreWorkerProcess(listener net.Listener, active *workerProcess, cfg supervisorConfig, request updater.RestoreRequest) (*workerProcess, error) {
	defer cleanupRestoreSource(request.Source)
	logger.Info("[master] restore worker start: worker_pid=%d sqlite=%s source=%s", workerPID(active), strings.TrimSpace(cfg.SqliteFile), strings.TrimSpace(request.Source))

	if cfg.SqliteFile == "" {
		logger.Error("[master] restore worker rejected: sqlite file is required")
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusFailed,
			Message: "restore failed: sqlite file is required",
		})
		_ = updater.RemoveRestoreRequest()
		return nil, fmt.Errorf("sqlite file is required for restore")
	}

	_ = updater.WriteRestoreStatus(updater.RestoreStatus{
		State:   updater.RestoreStatusStoppingWorker,
		Message: "旧 worker 正在停止。",
	})
	logger.Info("[master] restore worker stopping active worker: worker_pid=%d", workerPID(active))
	if err := stopWorkerProcess(active, cfg.ShutdownTimeout); err != nil {
		logger.Error("[master] restore worker stop active worker failed: worker_pid=%d err=%v", workerPID(active), err)
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusFailed,
			Message: "停止旧 worker 失败: " + err.Error(),
		})
		_ = updater.RemoveRestoreRequest()
		return nil, err
	}

	_ = updater.WriteRestoreStatus(updater.RestoreStatus{
		State:   updater.RestoreStatusReplacingDB,
		Message: "正在替换数据库文件。",
	})
	logger.Info("[master] restore worker replacing sqlite database: target=%s source=%s", strings.TrimSpace(cfg.SqliteFile), strings.TrimSpace(request.Source))
	rollbackPath, err := replaceSQLiteDatabase(cfg.SqliteFile, request.Source)
	if err != nil {
		logger.Error("[master] restore worker replace sqlite database failed: err=%v", err)
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusFailed,
			Message: "替换数据库失败: " + err.Error(),
		})
		_ = updater.RemoveRestoreRequest()
		next, restartErr := spawnWorker(listener, cfg)
		if restartErr != nil {
			logger.Error("[master] restore worker restart old worker after replace failure failed: err=%v", restartErr)
			return nil, fmt.Errorf("replace database failed: %w (restart old worker failed: %v)", err, restartErr)
		}
		logger.Warn("[master] restore worker resumed previous runtime after replace failure: next_worker_pid=%d", workerPID(next))
		return next, nil
	}
	logger.Info("[master] restore worker sqlite database replaced: rollback=%s", rollbackPath)

	_ = updater.WriteRestoreStatus(updater.RestoreStatus{
		State:   updater.RestoreStatusStartingWorker,
		Message: "正在启动新 worker。",
	})
	logger.Info("[master] restore worker starting replacement worker")
	next, err := spawnWorker(listener, cfg)
	if err == nil {
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusSuccess,
			Message: "数据库恢复成功，服务已切换到新 worker。",
		})
		_ = updater.RemoveRestoreRequest()
		if rollbackPath != "" {
			_ = os.Remove(rollbackPath)
		}
		logger.Info("[master] restore worker success: next_worker_pid=%d rollback_removed=%t", workerPID(next), rollbackPath != "")
		return next, nil
	}
	logger.Error("[master] restore worker start replacement failed: err=%v", err)

	if rollbackErr := rollbackSQLiteDatabase(cfg.SqliteFile, rollbackPath); rollbackErr != nil {
		logger.Error("[master] restore worker rollback failed: rollback=%s err=%v", rollbackPath, rollbackErr)
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusFailed,
			Message: "恢复新数据库失败且回滚失败: " + rollbackErr.Error(),
		})
		_ = updater.RemoveRestoreRequest()
		return nil, fmt.Errorf("start restored worker failed: %w (rollback failed: %v)", err, rollbackErr)
	}
	logger.Warn("[master] restore worker rolled back sqlite database: rollback=%s", rollbackPath)

	fallback, restartErr := spawnWorker(listener, cfg)
	if restartErr != nil {
		logger.Error("[master] restore worker restart after rollback failed: err=%v", restartErr)
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusFailed,
			Message: "恢复新数据库失败，且回滚后的 worker 启动失败: " + restartErr.Error(),
		})
		_ = updater.RemoveRestoreRequest()
		return nil, fmt.Errorf("start restored worker failed: %w (restart rolled back worker failed: %v)", err, restartErr)
	}

	_ = updater.WriteRestoreStatus(updater.RestoreStatus{
		State:   updater.RestoreStatusRolledBack,
		Message: "新数据库启动失败，已回滚到旧数据库。",
	})
	_ = updater.RemoveRestoreRequest()
	logger.Warn("[master] restore worker fallback resumed previous database: next_worker_pid=%d", workerPID(fallback))
	return fallback, nil
}

func cleanupRestoreSource(sourcePath string) {
	base := filepath.Base(strings.TrimSpace(sourcePath))
	if !strings.HasPrefix(base, ".swaves-restore-") {
		if strings.TrimSpace(sourcePath) != "" {
			logger.Info("[master] cleanup restore source skipped: path=%s reason=not_managed_temp", sourcePath)
		}
		return
	}
	if err := os.Remove(sourcePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Warn("[master] cleanup restore source failed: path=%s err=%v", sourcePath, err)
		return
	}
	logger.Info("[master] cleanup restore source success: path=%s", sourcePath)
}

func replaceSQLiteDatabase(targetPath string, sourcePath string) (string, error) {
	targetPath = strings.TrimSpace(targetPath)
	sourcePath = strings.TrimSpace(sourcePath)
	if targetPath == "" {
		return "", fmt.Errorf("target database path is required")
	}
	if sourcePath == "" {
		return "", fmt.Errorf("restore source path is required")
	}
	logger.Info("[master] replace sqlite database start: target=%s source=%s", targetPath, sourcePath)

	if err := removeSQLiteRuntimeFiles(targetPath); err != nil {
		logger.Error("[master] replace sqlite database remove runtime files failed: target=%s err=%v", targetPath, err)
		return "", err
	}

	targetDir := filepath.Dir(targetPath)
	if targetDir == "" {
		targetDir = "."
	}
	stagedPath := filepath.Join(targetDir, fmt.Sprintf(".swaves-restore-stage-%d.sqlite", time.Now().UnixNano()))
	if err := copyFile(sourcePath, stagedPath); err != nil {
		logger.Error("[master] replace sqlite database stage copy failed: source=%s staged=%s err=%v", sourcePath, stagedPath, err)
		return "", fmt.Errorf("stage restore database failed: %w", err)
	}

	rollbackPath := filepath.Join(targetDir, fmt.Sprintf(".swaves-restore-backup-%d.sqlite", time.Now().UnixNano()))
	renamedOld := false
	defer func() {
		if !renamedOld {
			_ = os.Remove(stagedPath)
		}
	}()

	if err := os.Rename(targetPath, rollbackPath); err != nil {
		logger.Error("[master] replace sqlite database backup current failed: target=%s rollback=%s err=%v", targetPath, rollbackPath, err)
		return "", fmt.Errorf("backup current database failed: %w", err)
	}
	renamedOld = true
	if err := os.Rename(stagedPath, targetPath); err != nil {
		_ = os.Rename(rollbackPath, targetPath)
		logger.Error("[master] replace sqlite database activate staged failed: staged=%s target=%s err=%v", stagedPath, targetPath, err)
		return "", fmt.Errorf("activate restored database failed: %w", err)
	}
	logger.Info("[master] replace sqlite database success: target=%s rollback=%s", targetPath, rollbackPath)
	return rollbackPath, nil
}

func rollbackSQLiteDatabase(targetPath string, rollbackPath string) error {
	if rollbackPath == "" {
		return fmt.Errorf("rollback database path is required")
	}
	logger.Info("[master] rollback sqlite database start: target=%s rollback=%s", targetPath, rollbackPath)
	if err := removeSQLiteRuntimeFiles(targetPath); err != nil {
		logger.Error("[master] rollback sqlite database remove runtime files failed: target=%s err=%v", targetPath, err)
		return err
	}
	if err := os.Remove(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		logger.Error("[master] rollback sqlite database remove current failed: target=%s err=%v", targetPath, err)
		return fmt.Errorf("remove failed restore database failed: %w", err)
	}
	if err := os.Rename(rollbackPath, targetPath); err != nil {
		logger.Error("[master] rollback sqlite database restore backup failed: rollback=%s target=%s err=%v", rollbackPath, targetPath, err)
		return fmt.Errorf("restore original database failed: %w", err)
	}
	logger.Info("[master] rollback sqlite database success: target=%s rollback=%s", targetPath, rollbackPath)
	return nil
}

func removeSQLiteRuntimeFiles(targetPath string) error {
	for _, suffix := range []string{"-wal", "-shm"} {
		path := targetPath + suffix
		err := os.Remove(path)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			logger.Error("[master] remove sqlite runtime file failed: path=%s err=%v", path, err)
			return fmt.Errorf("remove sqlite runtime file failed: %w", err)
		}
		if err == nil {
			logger.Info("[master] remove sqlite runtime file success: path=%s", path)
		}
	}
	return nil
}

func copyFile(srcPath string, dstPath string) error {
	logger.Info("[master] copy file start: src=%s dst=%s", srcPath, dstPath)
	src, err := os.Open(srcPath)
	if err != nil {
		logger.Error("[master] copy file open source failed: src=%s err=%v", srcPath, err)
		return err
	}
	defer func() { _ = src.Close() }()

	info, err := src.Stat()
	if err != nil {
		logger.Error("[master] copy file stat source failed: src=%s err=%v", srcPath, err)
		return err
	}

	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		logger.Error("[master] copy file open destination failed: dst=%s err=%v", dstPath, err)
		return err
	}
	defer func() { _ = dst.Close() }()

	if _, err := io.Copy(dst, src); err != nil {
		logger.Error("[master] copy file write failed: src=%s dst=%s err=%v", srcPath, dstPath, err)
		return err
	}
	if err := dst.Close(); err != nil {
		logger.Error("[master] copy file close destination failed: dst=%s err=%v", dstPath, err)
		return err
	}
	logger.Info("[master] copy file success: src=%s dst=%s", srcPath, dstPath)
	return nil
}
