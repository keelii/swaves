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

func defaultRestoreRequestPath() string {
	return filepath.Join(filepath.Dir(RuntimeInfoPath()), "restore_request.json")
}

func defaultRestoreStatusPath() string {
	return filepath.Join(filepath.Dir(RuntimeInfoPath()), "restore_status.json")
}

func WriteRestoreRequest(request RestoreRequest) error {
	return WriteRestoreRequestAtPath(defaultRestoreRequestPath(), request)
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
	return ReadRestoreRequestAtPath(defaultRestoreRequestPath())
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
	return RemoveRestoreRequestAtPath(defaultRestoreRequestPath())
}

func RemoveRestoreRequestAtPath(path string) error {
	return removeRestoreFile(path)
}

func WriteRestoreStatus(status RestoreStatus) error {
	return WriteRestoreStatusAtPath(defaultRestoreStatusPath(), status)
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
	return ReadRestoreStatusAtPath(defaultRestoreStatusPath())
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
