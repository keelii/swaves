package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/platform/logger"
	"syscall"
	"time"
)

type RuntimeInfo struct {
	PID        int    `json:"pid"`
	Executable string `json:"executable"`
	Version    string `json:"version"`
	UpdatedAt  int64  `json:"updated_at"`
}

var runtimeInfoPath = defaultRuntimeInfoPath

var (
	processExists     = defaultProcessExists
	currentExecutable = os.Executable
)

func RuntimeInfoPath() string {
	return runtimeInfoPath()
}

func defaultRuntimeInfoPath() string {
	cacheDir, err := os.UserCacheDir()
	if err == nil && strings.TrimSpace(cacheDir) != "" {
		return filepath.Join(cacheDir, "swaves", "master_runtime.json")
	}
	return filepath.Join(os.TempDir(), "swaves_master_runtime.json")
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
	logger.Info("[update] write runtime info start: master_pid=%d executable=%s path=%s", info.PID, info.Executable, path)
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
	logger.Info("[update] write runtime info success: master_pid=%d path=%s", info.PID, path)
	return nil
}

func ReadRuntimeInfo() (RuntimeInfo, error) {
	path := RuntimeInfoPath()
	logger.Info("[update] read runtime info start: path=%s", path)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Warn("[update] read runtime info missing: path=%s", path)
			return RuntimeInfo{}, fmt.Errorf("daemon mode is not active")
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
	logger.Info("[update] read runtime info success: master_pid=%d executable=%s path=%s", info.PID, info.Executable, path)
	return info, nil
}

func ReadActiveRuntimeInfo() (RuntimeInfo, error) {
	info, err := ReadRuntimeInfo()
	if err != nil {
		return RuntimeInfo{}, err
	}
	if !processExists(info.PID) {
		logger.Warn("[update] active runtime missing process: master_pid=%d executable=%s", info.PID, info.Executable)
		return RuntimeInfo{}, fmt.Errorf("master process is not running: pid=%d", info.PID)
	}
	logger.Info("[update] active runtime verified: master_pid=%d executable=%s", info.PID, info.Executable)
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
	logger.Info("[update] remove runtime info start: path=%s", path)
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		logger.Error("[update] remove runtime info failed: path=%s err=%v", path, err)
		return fmt.Errorf("remove runtime info failed: %w", err)
	}
	if err == nil {
		logger.Info("[update] remove runtime info success: path=%s", path)
		return nil
	}
	logger.Info("[update] remove runtime info skipped: path=%s reason=not_found", path)
	return nil
}
