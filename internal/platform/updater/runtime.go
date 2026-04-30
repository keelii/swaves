package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/pathutil"
	"sync"
	"syscall"
	"time"
)

type RuntimeInfo struct {
	PID        int    `json:"pid"`
	Executable string `json:"executable"`
	Version    string `json:"version"`
	UpdatedAt  int64  `json:"updated_at"`
}

const (
	RuntimeCacheDir = ".cache"
	RuntimeInfoName = "master_runtime.json"
)

var DefaultBackupDir = filepath.Join(RuntimeCacheDir, "backups")

var (
	// ErrRuntimeInfoNotFound 表示 runtime info 文件不存在，通常意味着从未以 daemon-mode=1 启动过。
	ErrRuntimeInfoNotFound = errors.New("runtime info file not found")
	// ErrMasterNotRunning 表示 runtime info 文件存在但 master 进程已不在运行。
	ErrMasterNotRunning = errors.New("master process is not running")
)

var (
	runtimeCacheRoot string
	runtimeCacheMu   sync.RWMutex
)

func ConfigureRuntimeCacheRoot(sqliteFile string) error {
	root, err := pathutil.EnsureDatabaseCacheRoot(sqliteFile)
	if err != nil {
		return err
	}
	runtimeCacheMu.Lock()
	runtimeCacheRoot = root
	runtimeCacheMu.Unlock()
	logger.Info("[update] runtime cache root configured: sqlite=%s cache_root=%s", strings.TrimSpace(sqliteFile), root)
	return nil
}

func RuntimeCacheRoot() (string, error) {
	runtimeCacheMu.RLock()
	root := strings.TrimSpace(runtimeCacheRoot)
	runtimeCacheMu.RUnlock()
	if root != "" {
		return root, nil
	}
	root, err := pathutil.ResolveProcessCacheRoot()
	if err != nil {
		return "", err
	}
	return root, nil
}

func RuntimeCachePath() (string, error) {
	root, err := RuntimeCacheRoot()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, RuntimeInfoName), nil
}

func RuntimeInfoPath() string {
	path, err := RuntimeCachePath()
	if err == nil && strings.TrimSpace(path) != "" {
		return path
	}
	logger.Warn("[update] resolve runtime info path fallback: err=%v", err)
	return runtimeInfoRelativePath()
}

func runtimeInfoRelativePath() string {
	return filepath.Join(RuntimeCacheDir, RuntimeInfoName)
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
	return info, nil
}

func WriteRuntimeInfo(info RuntimeInfo) error {
	if info.PID <= 0 {
		return fmt.Errorf("runtime pid is required")
	}
	info.Executable = strings.TrimSpace(info.Executable)
	if info.Executable == "" {
		return fmt.Errorf("runtime executable is required")
	}
	if info.UpdatedAt <= 0 {
		info.UpdatedAt = time.Now().Unix()
	}

	path := RuntimeInfoPath()
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
	path := RuntimeInfoPath()
	info, err := readRuntimeInfoAtPath(path)
	if err == nil {
		return info, nil
	}
	if !os.IsNotExist(err) {
		return RuntimeInfo{}, err
	}
	logger.Warn("[update] runtime info file missing: path=%s", path)
	return RuntimeInfo{}, fmt.Errorf("runtime info file not found: path=%s: %w", path, ErrRuntimeInfoNotFound)
}

func ReadActiveRuntimeInfo() (RuntimeInfo, error) {
	info, err := ReadRuntimeInfo()
	if err != nil {
		return RuntimeInfo{}, err
	}
	if !defaultProcessExists(info.PID) {
		logger.Warn("[update] active runtime missing process: master_pid=%d executable=%s", info.PID, info.Executable)
		return RuntimeInfo{}, fmt.Errorf("master process is not running: pid=%d: %w", info.PID, ErrMasterNotRunning)
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

func RemoveRuntimeInfo() error {
	path := RuntimeInfoPath()
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		logger.Error("[update] remove runtime info failed: path=%s err=%v", path, err)
		return fmt.Errorf("remove runtime info failed: %w", err)
	}
	return nil
}
