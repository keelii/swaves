package job

import (
	"path/filepath"
	"testing"

	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/db"
	"swaves/internal/platform/updater"
)

func openJobTestDB(t *testing.T) *db.DB {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "data.sqlite")
	dbx := db.Open(db.Options{DSN: dsn})
	t.Cleanup(func() {
		_ = dbx.Close()
	})
	return dbx
}

func withTestRegistry(t *testing.T, reg *Registry) {
	t.Helper()
	registryMu.Lock()
	prev := registry
	registry = reg
	registryMu.Unlock()
	t.Cleanup(func() {
		registryMu.Lock()
		registry = prev
		registryMu.Unlock()
	})
}

func mustCreateTask(t *testing.T, dbx *db.DB, code string, kind db.TaskKind) db.Task {
	t.Helper()
	task := db.Task{
		Code:        code,
		Name:        code,
		Description: "test task",
		Schedule:    "@daily",
		Enabled:     1,
		Kind:        kind,
	}
	if _, err := db.CreateTask(dbx, &task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	return task
}

func TestExecuteTaskNoOpDoesNotUpdateTaskStatus(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustCreateTask(t, dbx, "task_noop", db.TaskUser)

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: func(_ *Registry) (*string, error) {
					return nil, nil
				},
			},
		},
		DB: dbx,
	})

	ExecuteTask(dbx, task)

	gotTask, err := db.GetTaskByCode(dbx, task.Code)
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if gotTask.LastRunAt != nil {
		t.Fatalf("LastRunAt should stay nil for no-op, got %v", *gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "" {
		t.Fatalf("LastStatus should stay empty for no-op, got %q", gotTask.LastStatus)
	}

	runs, err := db.ListTaskRuns(dbx, task.Code, "", 100)
	if err != nil {
		t.Fatalf("ListTaskRuns failed: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected no task runs for no-op, got %d", len(runs))
	}
}

func TestExecuteTaskSuccessUpdatesStatusAndTaskRun(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustCreateTask(t, dbx, "task_success", db.TaskUser)

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: func(_ *Registry) (*string, error) {
					msg := "ok"
					return &msg, nil
				},
			},
		},
		DB: dbx,
	})

	ExecuteTask(dbx, task)

	gotTask, err := db.GetTaskByCode(dbx, task.Code)
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if gotTask.LastRunAt == nil || *gotTask.LastRunAt <= 0 {
		t.Fatalf("LastRunAt should be updated, got %v", gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "success" {
		t.Fatalf("LastStatus should be success, got %q", gotTask.LastStatus)
	}

	runs, err := db.ListTaskRuns(dbx, task.Code, "", 100)
	if err != nil {
		t.Fatalf("ListTaskRuns failed: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 task run, got %d", len(runs))
	}
	if runs[0].Status != "success" {
		t.Fatalf("task run status should be success, got %q", runs[0].Status)
	}
}

func TestCheckAppUpdateJobCreatesNotification(t *testing.T) {
	dbx := openJobTestDB(t)
	oldCheck := checkLatestAppRelease
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "test"
	buildinfo.BuildTime = "2026-04-05T00:00:00Z"
	checkLatestAppRelease = func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
		return updater.CheckResult{
			CurrentVersion:       currentVersion,
			CurrentVersionStable: true,
			LatestVersion:        "v1.2.4",
			LatestReleaseURL:     "https://github.com/keelii/swaves/releases/tag/v1.2.4",
			HasUpgrade:           true,
			Target: &updater.ReleaseTarget{
				Archive: updater.ReleaseAsset{Name: "swaves_v1.2.4_linux_amd64.tar.gz"},
			},
		}, nil
	}
	defer func() {
		checkLatestAppRelease = oldCheck
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildTime = oldBuildTime
	}()

	msg, err := CheckAppUpdateJob(&Registry{DB: dbx})
	if err != nil {
		t.Fatalf("CheckAppUpdateJob failed: %v", err)
	}
	if msg == nil || *msg == "" {
		t.Fatal("expected job message")
	}

	items, err := db.ListNotificationsByEventType(dbx, db.NotificationReceiverDash, db.NotificationEventAppUpdate, 10, 0)
	if err != nil {
		t.Fatalf("ListNotificationsByEventType failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("app update notification count = %d, want 1", len(items))
	}
}
