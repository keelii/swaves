package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/pathutil"
	"sync"
	"syscall"
	"time"
)

type RuntimeInfo struct {
	PID        int      `json:"pid"`
	Executable string   `json:"executable"`
	Version    string   `json:"version"`
	Args       []string `json:"args,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`
	SQLiteFile string   `json:"sqlite_file,omitempty"`
	UpdatedAt  int64    `json:"updated_at"`
}

const (
	RuntimeCacheDir     = ".cache"
	RuntimeInfoName     = "master_runtime.json"
	UpgradeCacheDirName = "updater"

	RuntimeMasterPIDEnv        = "SWAVES_MASTER_PID"
	RuntimeMasterExecutableEnv = "SWAVES_MASTER_EXECUTABLE"
)

var DefaultBackupDir = filepath.Join(RuntimeCacheDir, "backups")

var (
	// ErrRuntimeInfoNotFound 表示 runtime info 文件不存在，通常意味着从未以 daemon-mode=1 启动过。
	ErrRuntimeInfoNotFound = errors.New("runtime info file not found")
	// ErrMasterNotRunning 表示 runtime info 文件存在但 master 进程已不在运行。
	ErrMasterNotRunning = errors.New("master process is not running")
	// ErrRuntimeCacheRootNotConfigured 表示运行时缓存根目录尚未由 SQLite 文件位置配置。
	ErrRuntimeCacheRootNotConfigured = errors.New("runtime cache root is not configured")
)

var (
	runtimeCacheRoot                                string
	runtimeSQLiteFile                               string
	runtimeCacheMu                                  sync.RWMutex
	runtimeExecutableVerificationUnsupportedLogOnce sync.Once
)

var runtimeProcessExecutablePath = processExecutablePath

func normalizeRuntimeInfo(info RuntimeInfo) (RuntimeInfo, error) {
	if info.PID <= 0 {
		return RuntimeInfo{}, fmt.Errorf("runtime pid is required")
	}
	info.Executable = strings.TrimSpace(info.Executable)
	if info.Executable == "" {
		return RuntimeInfo{}, fmt.Errorf("runtime executable is required")
	}
	if info.UpdatedAt <= 0 {
		info.UpdatedAt = time.Now().Unix()
	}
	info.WorkingDir = strings.TrimSpace(info.WorkingDir)
	info.SQLiteFile = strings.TrimSpace(info.SQLiteFile)
	return info, nil
}

func ConfigureRuntimeCacheRoot(sqliteFile string) error {
	root, err := pathutil.EnsureDatabaseCacheRoot(sqliteFile)
	if err != nil {
		return err
	}
	resolvedSQLiteFile, err := filepath.Abs(strings.TrimSpace(sqliteFile))
	if err != nil {
		return fmt.Errorf("resolve runtime sqlite file failed: %w", err)
	}
	runtimeCacheMu.Lock()
	runtimeCacheRoot = root
	runtimeSQLiteFile = filepath.Clean(resolvedSQLiteFile)
	runtimeCacheMu.Unlock()
	logger.Info("[update] runtime cache root configured: sqlite=%s cache_root=%s", strings.TrimSpace(sqliteFile), root)
	return nil
}

func ConfigureRuntimeCacheRootAt(root string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return fmt.Errorf("runtime cache root is required")
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve runtime cache root failed: %w", err)
	}
	absRoot = filepath.Clean(absRoot)
	if err := os.MkdirAll(absRoot, 0755); err != nil {
		return fmt.Errorf("create runtime cache root failed: %w", err)
	}

	runtimeCacheMu.Lock()
	runtimeCacheRoot = absRoot
	runtimeSQLiteFile = ""
	runtimeCacheMu.Unlock()
	logger.Info("[update] runtime cache root configured: cache_root=%s", absRoot)
	return nil
}

func RuntimeCacheRoot() (string, error) {
	runtimeCacheMu.RLock()
	root := strings.TrimSpace(runtimeCacheRoot)
	runtimeCacheMu.RUnlock()
	if root != "" {
		return root, nil
	}
	return "", fmt.Errorf("runtime cache root is not configured: call ConfigureRuntimeCacheRoot first: %w", ErrRuntimeCacheRootNotConfigured)
}

func RuntimeSQLiteFile() (string, error) {
	runtimeCacheMu.RLock()
	sqliteFile := strings.TrimSpace(runtimeSQLiteFile)
	runtimeCacheMu.RUnlock()
	if sqliteFile != "" {
		return sqliteFile, nil
	}
	return "", fmt.Errorf("runtime sqlite file is not configured: call ConfigureRuntimeCacheRoot first: %w", ErrRuntimeCacheRootNotConfigured)
}

func RuntimeCachePath() (string, error) {
	root, err := RuntimeCacheRoot()
	if err != nil {
		return "", err
	}
	return RuntimeInfoPathAtCacheRoot(root)
}

func RuntimeInfoPathAtCacheRoot(root string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", fmt.Errorf("runtime cache root is required")
	}
	return filepath.Join(root, RuntimeInfoName), nil
}

func UpgradeCacheDir() (string, error) {
	root, err := RuntimeCacheRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, UpgradeCacheDirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create updater cache dir failed: %w", err)
	}
	return dir, nil
}

func CreateUpgradeTempDir(pattern string) (string, error) {
	root, err := UpgradeCacheDir()
	if err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(root, pattern)
	if err != nil {
		return "", fmt.Errorf("create updater temp dir failed: %w", err)
	}
	return dir, nil
}

func RuntimeInfoPath() (string, error) {
	return RuntimeCachePath()
}

func readRuntimeInfoAtPath(path string) (RuntimeInfo, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeInfo{}, err
		}
		logger.Error("[update] read runtime info failed: path=%s err=%v", path, err)
		return RuntimeInfo{}, fmt.Errorf("read runtime info failed: %w", err)
	}

	var info RuntimeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		logger.Error("[update] parse runtime info failed: path=%s err=%v", path, err)
		return RuntimeInfo{}, fmt.Errorf("parse runtime info failed: %w", err)
	}
	if info.PID <= 0 {
		logger.Error("[update] runtime info invalid pid: path=%s master_pid=%d", path, info.PID)
		return RuntimeInfo{}, fmt.Errorf("runtime info pid is invalid")
	}
	info.Executable = strings.TrimSpace(info.Executable)
	if info.Executable == "" {
		logger.Error("[update] runtime info invalid executable: path=%s", path)
		return RuntimeInfo{}, fmt.Errorf("runtime info executable is invalid")
	}
	info.WorkingDir = strings.TrimSpace(info.WorkingDir)
	info.SQLiteFile = strings.TrimSpace(info.SQLiteFile)
	return info, nil
}

func WriteRuntimeInfo(info RuntimeInfo) error {
	info, err := normalizeRuntimeInfo(info)
	if err != nil {
		return err
	}

	path, err := RuntimeInfoPath()
	if err != nil {
		logger.Error("[update] write runtime info path failed: err=%v", err)
		return err
	}
	return writeRuntimeInfoAtPath(path, info)
}

func writeRuntimeInfoAtPath(path string, info RuntimeInfo) error {
	info, err := normalizeRuntimeInfo(info)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		logger.Error("[update] write runtime info create dir failed: path=%s err=%v", path, err)
		return fmt.Errorf("create runtime dir failed: %w", err)
	}

	data, err := json.Marshal(info)
	if err != nil {
		logger.Error("[update] write runtime info marshal failed: master_pid=%d path=%s err=%v", info.PID, path, err)
		return fmt.Errorf("marshal runtime info failed: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		logger.Error("[update] write runtime info failed: master_pid=%d path=%s err=%v", info.PID, path, err)
		return fmt.Errorf("write runtime info failed: %w", err)
	}
	return nil
}

func ReadRuntimeInfo() (RuntimeInfo, error) {
	path, err := RuntimeInfoPath()
	if err != nil {
		logger.Error("[update] read runtime info path failed: err=%v", err)
		return RuntimeInfo{}, err
	}
	info, err := readRuntimeInfoAtPath(path)
	if err == nil {
		return info, nil
	}
	if !os.IsNotExist(err) {
		return RuntimeInfo{}, err
	}
	info, repairErr := RepairRuntimeInfoFromEnv()
	if repairErr == nil {
		return info, nil
	}
	if !errors.Is(repairErr, ErrRuntimeInfoNotFound) {
		return RuntimeInfo{}, repairErr
	}
	logger.Warn("[update] runtime info file missing: path=%s", path)
	return RuntimeInfo{}, fmt.Errorf("runtime info file not found: path=%s: %w", path, ErrRuntimeInfoNotFound)
}

func ReadRuntimeInfoAtCacheRoot(root string) (RuntimeInfo, error) {
	path, err := RuntimeInfoPathAtCacheRoot(root)
	if err != nil {
		return RuntimeInfo{}, err
	}
	return readRuntimeInfoAtPath(path)
}

func ReadRuntimeInfoFromEnv() (RuntimeInfo, error) {
	rawPID := strings.TrimSpace(os.Getenv(RuntimeMasterPIDEnv))
	if rawPID == "" {
		return RuntimeInfo{}, fmt.Errorf("runtime env is not configured: %w", ErrRuntimeInfoNotFound)
	}
	pid, err := strconv.Atoi(rawPID)
	if err != nil {
		return RuntimeInfo{}, fmt.Errorf("runtime env pid is invalid: %w", err)
	}

	return normalizeRuntimeInfo(RuntimeInfo{
		PID:        pid,
		Executable: os.Getenv(RuntimeMasterExecutableEnv),
	})
}

func RepairRuntimeInfoFromEnv() (RuntimeInfo, error) {
	info, err := ReadRuntimeInfoFromEnv()
	if err != nil {
		return RuntimeInfo{}, err
	}
	sqliteFile, err := RuntimeSQLiteFile()
	if err != nil {
		return RuntimeInfo{}, err
	}
	info.SQLiteFile = sqliteFile
	path, err := RuntimeInfoPath()
	if err != nil {
		return RuntimeInfo{}, err
	}
	if err := writeRuntimeInfoAtPath(path, info); err != nil {
		return RuntimeInfo{}, err
	}
	logger.Info("[update] runtime info repaired from worker env: path=%s master_pid=%d executable=%s", path, info.PID, info.Executable)
	return info, nil
}

func ReadActiveRuntimeInfo() (RuntimeInfo, error) {
	info, err := ReadRuntimeInfo()
	if err != nil {
		envInfo, envErr := ReadRuntimeInfoFromEnv()
		if envErr != nil {
			return RuntimeInfo{}, err
		}
		info = envInfo
	}
	if !defaultProcessExists(info.PID) {
		logger.Warn("[update] active runtime missing process: master_pid=%d executable=%s", info.PID, info.Executable)
		envInfo, envErr := ReadRuntimeInfoFromEnv()
		if envErr != nil || envInfo.PID == info.PID {
			return RuntimeInfo{}, fmt.Errorf("master process is not running: pid=%d: %w", info.PID, ErrMasterNotRunning)
		}
		info = envInfo
	}
	if err := verifyRuntimeExecutable(info); err != nil {
		logger.Warn("[update] active runtime executable verification failed: master_pid=%d executable=%s err=%v", info.PID, info.Executable, err)
		envInfo, envErr := ReadRuntimeInfoFromEnv()
		if envErr != nil || envInfo.PID == info.PID {
			return RuntimeInfo{}, err
		}
		info = envInfo
		if err := verifyRuntimeExecutable(info); err != nil {
			logger.Warn("[update] active runtime env executable verification failed: master_pid=%d executable=%s err=%v", info.PID, info.Executable, err)
			return RuntimeInfo{}, err
		}
	}
	return info, nil
}

func defaultProcessExists(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}

func verifyRuntimeExecutable(info RuntimeInfo) error {
	actual, supported, err := runtimeProcessExecutablePath(info.PID)
	if err != nil {
		return fmt.Errorf("verify master executable failed: pid=%d: %w", info.PID, err)
	}
	if !supported {
		runtimeExecutableVerificationUnsupportedLogOnce.Do(func() {
			logger.Info("[update] active runtime executable verification unavailable on %s", runtime.GOOS)
		})
		return nil
	}
	if !sameExecutablePath(actual, info.Executable) {
		if isDeletedUpgradeBackupExecutable(actual) {
			logger.Warn("[update] active runtime executable is deleted upgrade backup, accepting runtime info for restart: master_pid=%d actual=%s expected=%s", info.PID, actual, info.Executable)
			return nil
		}
		return fmt.Errorf("master process executable mismatch: pid=%d expected=%s actual=%s: %w", info.PID, info.Executable, actual, ErrMasterNotRunning)
	}
	return nil
}

func isDeletedUpgradeBackupExecutable(actual string) bool {
	actual = strings.TrimSpace(actual)
	if !strings.HasSuffix(actual, " (deleted)") {
		return false
	}
	actual = strings.TrimSuffix(actual, " (deleted)")
	actual = filepath.Clean(actual)
	parts := strings.Split(actual, string(os.PathSeparator))
	if len(parts) < 4 || parts[len(parts)-1] != ".swaves-executable-backup" {
		return false
	}
	upgradeDir := parts[len(parts)-2]
	if !strings.HasPrefix(upgradeDir, ".swaves-upgrade-") {
		return false
	}
	return parts[len(parts)-3] == UpgradeCacheDirName && parts[len(parts)-4] == RuntimeCacheDir
}

func processExecutablePath(pid int) (string, bool, error) {
	if runtime.GOOS != "linux" {
		return "", false, nil
	}
	path, err := os.Readlink(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		if os.IsNotExist(err) {
			return "", true, ErrMasterNotRunning
		}
		if isProcessExecutablePermissionError(err) {
			return "", false, nil
		}
		return "", true, err
	}
	return path, true, nil
}

func isProcessExecutablePermissionError(err error) bool {
	return errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM)
}

func sameExecutablePath(left string, right string) bool {
	return cleanExecutablePath(left) == cleanExecutablePath(right)
}

func cleanExecutablePath(path string) string {
	path = strings.TrimSpace(path)
	path = strings.TrimSuffix(path, " (deleted)")
	if path == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if absPath, err := filepath.Abs(path); err == nil {
		path = absPath
	}
	return filepath.Clean(path)
}

func RemoveRuntimeInfo() error {
	path, err := RuntimeInfoPath()
	if err != nil {
		logger.Error("[update] remove runtime info path failed: err=%v", err)
		return err
	}
	err = os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		logger.Error("[update] remove runtime info failed: path=%s err=%v", path, err)
		return fmt.Errorf("remove runtime info failed: %w", err)
	}
	return nil
}
