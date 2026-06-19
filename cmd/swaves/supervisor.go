package main

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/supervisor"
	"swaves/internal/platform/updater"
	"time"
)

const (
	daemonModeConfigEnv           = "SWAVES_DAEMON_MODE"
	workerModeEnv                 = "SWAVES_RUN_MODE"
	workerProcessFlag             = "--worker-process"
	workerGracefulShutdownTimeout = 8 * time.Second
	defaultWorkerStopTimeout      = workerGracefulShutdownTimeout + 4*time.Second
	defaultWorkerReadyTimeout     = 8 * time.Second
	defaultWorkerDrainTimeout     = 5 * time.Second
)

type supervisorConfig struct {
	DaemonMode      bool
	ListenAddr      string
	SqliteFile      string
	MaxFailures     int
	ReadyTimeout    time.Duration
	ShutdownTimeout time.Duration
	DrainTimeout    time.Duration
	ExecutablePath  string
	Args            []string
	Worker          func() error
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

	workingDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("resolve working directory failed: %w", err)
	}
	sqliteFile := resolveRuntimeSQLiteFile(cfg.SqliteFile, workingDir)
	if err := updater.WriteRuntimeInfo(updater.RuntimeInfo{
		PID:        os.Getpid(),
		Executable: execPath,
		Args:       append([]string{execPath}, cfg.Args...),
		WorkingDir: workingDir,
		SQLiteFile: sqliteFile,
	}); err != nil {
		return err
	}
	cfg.SqliteFile = sqliteFile
	defer func() { _ = updater.RemoveRuntimeInfo() }()

	return supervisor.Run(supervisor.Config{
		ListenAddr:      cfg.ListenAddr,
		Worker:          cfg.Worker,
		WorkerModeEnv:   workerModeEnv,
		ExecutablePath:  execPath,
		Args:            workerArgs(cfg.Args),
		ExtraEnv:        workerExtraEnv(execPath),
		MaxFailures:     cfg.MaxFailures,
		ReadyTimeout:    cfg.ReadyTimeout,
		ShutdownTimeout: cfg.ShutdownTimeout,
		DrainTimeout:    cfg.DrainTimeout,
		OnSIGHUP: func(ln net.Listener, active *supervisor.Process, spawn supervisor.SpawnFn) (*supervisor.Process, error) {
			restoreRequest, restoreErr := updater.ReadRestoreRequest()
			switch {
			case restoreErr == nil:
				logger.Info("[master] restart entering restore flow: worker_pid=%d source=%s", active.PID(), strings.TrimSpace(restoreRequest.Source))
				next, err := restoreWorkerProcess(ln, active, cfg, spawn, restoreRequest)
				if err != nil {
					logger.Error("[master] restore worker failed: %v", err)
					return nil, err
				}
				logger.Info("[master] restore flow switched worker: previous_worker_pid=%d next_worker_pid=%d", active.PID(), next.PID())
				return next, nil
			case errors.Is(restoreErr, updater.ErrRestoreRequestNotFound):
				logger.Info("[master] restart spawning replacement worker: worker_pid=%d", active.PID())
				next, err := spawn(ln)
				if err != nil {
					logger.Error("[master] restart worker failed: %v", err)
					return nil, err
				}
				logger.Info("[master] replacement worker ready: previous_worker_pid=%d next_worker_pid=%d", active.PID(), next.PID())
				supervisor.DrainProcess(active, cfg.DrainTimeout)
				if err := supervisor.StopProcess(active, cfg.ShutdownTimeout); err != nil {
					logger.Error("[master] stop previous worker failed: %v", err)
				}
				return next, nil
			default:
				logger.Error("[master] read restore request failed: %v", restoreErr)
				return nil, restoreErr
			}
		},
		Log: func(level, format string, args ...any) {
			switch level {
			case "info":
				logger.Info(format, args...)
			case "warn":
				logger.Warn(format, args...)
			case "error":
				logger.Error(format, args...)
			}
		},
	})
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
	if cfg.DrainTimeout <= 0 {
		cfg.DrainTimeout = defaultWorkerDrainTimeout
	}
}

func resolveSupervisorExecutablePath(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path != "" {
		return path, nil
	}
	return os.Executable()
}

func workerExtraEnv(execPath string) []string {
	return []string{
		updater.RuntimeMasterPIDEnv + "=" + fmt.Sprintf("%d", os.Getpid()),
		updater.RuntimeMasterExecutableEnv + "=" + strings.TrimSpace(execPath),
	}
}

func resolveRuntimeSQLiteFile(sqliteFile string, workingDir string) string {
	sqliteFile = strings.TrimSpace(sqliteFile)
	if sqliteFile == "" || filepath.IsAbs(sqliteFile) {
		return filepath.Clean(sqliteFile)
	}
	workingDir = strings.TrimSpace(workingDir)
	if workingDir == "" {
		return sqliteFile
	}
	return filepath.Clean(filepath.Join(workingDir, sqliteFile))
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

func restoreWorkerProcess(listener net.Listener, active *supervisor.Process, cfg supervisorConfig, spawn supervisor.SpawnFn, request updater.RestoreRequest) (*supervisor.Process, error) {
	defer cleanupRestoreSource(request.Source)
	logger.Info("[master] restore worker start: worker_pid=%d sqlite=%s source=%s", active.PID(), strings.TrimSpace(cfg.SqliteFile), strings.TrimSpace(request.Source))

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
	logger.Info("[master] restore worker stopping active worker: worker_pid=%d", active.PID())
	if err := supervisor.StopProcess(active, cfg.ShutdownTimeout); err != nil {
		logger.Error("[master] restore worker stop active worker failed: worker_pid=%d err=%v", active.PID(), err)
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
		next, restartErr := spawn(listener)
		if restartErr != nil {
			logger.Error("[master] restore worker restart old worker after replace failure failed: err=%v", restartErr)
			return nil, fmt.Errorf("replace database failed: %w (restart old worker failed: %v)", err, restartErr)
		}
		logger.Warn("[master] restore worker resumed previous runtime after replace failure: next_worker_pid=%d", next.PID())
		return next, nil
	}
	logger.Info("[master] restore worker sqlite database replaced: rollback=%s", rollbackPath)

	_ = updater.WriteRestoreStatus(updater.RestoreStatus{
		State:   updater.RestoreStatusStartingWorker,
		Message: "正在启动新 worker。",
	})
	logger.Info("[master] restore worker starting replacement worker")
	next, err := spawn(listener)
	if err == nil {
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusSuccess,
			Message: "数据库恢复成功，服务已切换到新 worker。",
		})
		_ = updater.RemoveRestoreRequest()
		if rollbackPath != "" {
			_ = os.Remove(rollbackPath)
		}
		logger.Info("[master] restore worker success: next_worker_pid=%d rollback_removed=%t", next.PID(), rollbackPath != "")
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

	fallback, restartErr := spawn(listener)
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
	logger.Warn("[master] restore worker fallback resumed previous database: next_worker_pid=%d", fallback.PID())
	return fallback, nil
}

func cleanupRestoreSource(sourcePath string) {
	base := filepath.Base(strings.TrimSpace(sourcePath))
	if !strings.HasPrefix(base, ".swaves-restore-upload-") {
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

	stagedFile, err := updater.CreateRestoreTempFile(".swaves-restore-stage-*.sqlite")
	if err != nil {
		logger.Error("[master] replace sqlite database create staged file failed: err=%v", err)
		return "", err
	}
	stagedPath := stagedFile.Name()
	if err := stagedFile.Close(); err != nil {
		_ = os.Remove(stagedPath)
		logger.Error("[master] replace sqlite database close staged file failed: staged=%s err=%v", stagedPath, err)
		return "", err
	}
	if err := copyFile(sourcePath, stagedPath); err != nil {
		logger.Error("[master] replace sqlite database stage copy failed: source=%s staged=%s err=%v", sourcePath, stagedPath, err)
		return "", fmt.Errorf("stage restore database failed: %w", err)
	}

	rollbackFile, err := updater.CreateRestoreTempFile(".swaves-restore-backup-*.sqlite")
	if err != nil {
		_ = os.Remove(stagedPath)
		logger.Error("[master] replace sqlite database create rollback file failed: err=%v", err)
		return "", err
	}
	rollbackPath := rollbackFile.Name()
	if err := rollbackFile.Close(); err != nil {
		_ = os.Remove(stagedPath)
		_ = os.Remove(rollbackPath)
		logger.Error("[master] replace sqlite database close rollback file failed: rollback=%s err=%v", rollbackPath, err)
		return "", err
	}
	if err := os.Remove(rollbackPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		_ = os.Remove(stagedPath)
		logger.Error("[master] replace sqlite database remove rollback placeholder failed: rollback=%s err=%v", rollbackPath, err)
		return "", fmt.Errorf("prepare rollback database path failed: %w", err)
	}
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
