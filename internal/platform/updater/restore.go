package updater

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	RestoreCacheDirName         = "restore"
	RestoreStatusIdle           = "idle"
	RestoreStatusPending        = "pending"
	RestoreStatusStoppingWorker = "stopping_worker"
	RestoreStatusReplacingDB    = "replacing_database"
	RestoreStatusStartingWorker = "starting_worker"
	RestoreStatusSuccess        = "success"
	RestoreStatusFailed         = "failed"
	RestoreStatusRolledBack     = "rolled_back"
)

var ErrRestoreRequestNotFound = errors.New("restore request not found")

type RestoreRequest struct {
	Source      string `json:"source"`
	RequestedAt int64  `json:"requested_at"`
}

type RestoreStatus struct {
	State     string `json:"status"`
	Message   string `json:"message"`
	UpdatedAt int64  `json:"updated_at"`
}

func RestoreCacheDir() (string, error) {
	root, err := RuntimeCacheRoot()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, RestoreCacheDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("create restore cache dir failed: %w", err)
	}
	return dir, nil
}

func CreateRestoreTempFile(pattern string) (*os.File, error) {
	dir, err := RestoreCacheDir()
	if err != nil {
		return nil, err
	}
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return nil, fmt.Errorf("create restore temp file failed: %w", err)
	}
	return file, nil
}

func defaultRestoreRequestPath() (string, error) {
	path, err := RuntimeInfoPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "restore_request.json"), nil
}

func defaultRestoreStatusPath() (string, error) {
	path, err := RuntimeInfoPath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(path), "restore_status.json"), nil
}

func WriteRestoreRequest(request RestoreRequest) error {
	path, err := defaultRestoreRequestPath()
	if err != nil {
		return err
	}
	return WriteRestoreRequestAtPath(path, request)
}

func WriteRestoreRequestAtPath(path string, request RestoreRequest) error {
	request.Source = strings.TrimSpace(request.Source)
	if request.Source == "" {
		return fmt.Errorf("restore source is required")
	}
	if request.RequestedAt <= 0 {
		request.RequestedAt = time.Now().Unix()
	}
	return writeRestoreJSON(path, request)
}

func ReadRestoreRequest() (RestoreRequest, error) {
	path, err := defaultRestoreRequestPath()
	if err != nil {
		return RestoreRequest{}, err
	}
	return ReadRestoreRequestAtPath(path)
}

func ReadRestoreRequestAtPath(path string) (RestoreRequest, error) {
	var request RestoreRequest
	if err := readRestoreJSON(path, &request); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RestoreRequest{}, ErrRestoreRequestNotFound
		}
		return RestoreRequest{}, err
	}
	request.Source = strings.TrimSpace(request.Source)
	if request.Source == "" {
		return RestoreRequest{}, fmt.Errorf("restore request source is invalid")
	}
	return request, nil
}

func RemoveRestoreRequest() error {
	path, err := defaultRestoreRequestPath()
	if err != nil {
		return err
	}
	return RemoveRestoreRequestAtPath(path)
}

func RemoveRestoreRequestAtPath(path string) error {
	return removeRestoreFile(path)
}

func WriteRestoreStatus(status RestoreStatus) error {
	path, err := defaultRestoreStatusPath()
	if err != nil {
		return err
	}
	return WriteRestoreStatusAtPath(path, status)
}

func WriteRestoreStatusAtPath(path string, status RestoreStatus) error {
	if strings.TrimSpace(status.State) == "" {
		status.State = RestoreStatusIdle
	}
	if status.UpdatedAt <= 0 {
		status.UpdatedAt = time.Now().Unix()
	}
	return writeRestoreJSON(path, status)
}

func ReadRestoreStatus() (RestoreStatus, error) {
	path, err := defaultRestoreStatusPath()
	if err != nil {
		return RestoreStatus{}, err
	}
	return ReadRestoreStatusAtPath(path)
}

func ReadRestoreStatusAtPath(path string) (RestoreStatus, error) {
	var status RestoreStatus
	if err := readRestoreJSON(path, &status); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return RestoreStatus{State: RestoreStatusIdle}, nil
		}
		return RestoreStatus{}, err
	}
	if strings.TrimSpace(status.State) == "" {
		status.State = RestoreStatusIdle
	}
	return status, nil
}

func writeRestoreJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create restore dir failed: %w", err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("marshal restore file failed: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write restore file failed: %w", err)
	}
	return nil
}

func readRestoreJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(data, value); err != nil {
		return fmt.Errorf("parse restore file failed: %w", err)
	}
	return nil
}

func removeRestoreFile(path string) error {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove restore file failed: %w", err)
	}
	return nil
}
