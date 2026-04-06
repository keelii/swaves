package updater

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("create runtime dir failed: %w", err)
	}

	data, err := json.Marshal(info)
	if err != nil {
		return fmt.Errorf("marshal runtime info failed: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write runtime info failed: %w", err)
	}
	return nil
}

func ReadRuntimeInfo() (RuntimeInfo, error) {
	path := RuntimeInfoPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return RuntimeInfo{}, fmt.Errorf("daemon mode is not active")
		}
		return RuntimeInfo{}, fmt.Errorf("read runtime info failed: %w", err)
	}

	var info RuntimeInfo
	if err := json.Unmarshal(data, &info); err != nil {
		return RuntimeInfo{}, fmt.Errorf("parse runtime info failed: %w", err)
	}
	if info.PID <= 0 {
		return RuntimeInfo{}, fmt.Errorf("runtime info pid is invalid")
	}
	info.Executable = strings.TrimSpace(info.Executable)
	if info.Executable == "" {
		return RuntimeInfo{}, fmt.Errorf("runtime info executable is invalid")
	}
	return info, nil
}

func ReadActiveRuntimeInfo() (RuntimeInfo, error) {
	info, err := ReadRuntimeInfo()
	if err != nil {
		return RuntimeInfo{}, err
	}
	if !processExists(info.PID) {
		return RuntimeInfo{}, fmt.Errorf("master process is not running: pid=%d", info.PID)
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
		return fmt.Errorf("remove runtime info failed: %w", err)
	}
	return nil
}
