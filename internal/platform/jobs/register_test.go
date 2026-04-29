package job

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
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

func mustGetTaskByCode(t *testing.T, dbx *db.DB, code string) db.Task {
	t.Helper()

	task, err := db.GetTaskByCode(dbx, code)
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	return *task
}

func mustListTaskRuns(t *testing.T, dbx *db.DB, code string) []db.TaskRun {
	t.Helper()

	runs, err := db.ListTaskRuns(dbx, code, "", 100)
	if err != nil {
		t.Fatalf("ListTaskRuns failed: %v", err)
	}
	return runs
}

func withTaskSettings(t *testing.T, settings map[string]string) {
	t.Helper()
	prev := store.GetSettingMap()
	clonedPrev := make(map[string]string, len(prev))
	for key, value := range prev {
		clonedPrev[key] = value
	}
	next := make(map[string]string, len(settings))
	for key, value := range settings {
		next[key] = value
	}
	store.Settings.Store(next)
	t.Cleanup(func() {
		store.Settings.Store(clonedPrev)
	})
}

func withWorkingDir(t *testing.T, dir string) {
	t.Helper()

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory failed: %v", err)
		}
	})
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

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt != nil {
		t.Fatalf("LastRunAt should stay nil for no-op, got %v", *gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "" {
		t.Fatalf("LastStatus should stay empty for no-op, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
	if len(runs) != 0 {
		t.Fatalf("expected no task runs for no-op, got %d", len(runs))
	}
}

func TestExecuteTaskDeleteExpiredEncryptedPostsNoOpDoesNotUpdateTaskStatus(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustCreateTask(t, dbx, "task_clear_encrypted_posts_noop", db.TaskInternal)

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: DeleteExpiredEncryptedPostsJob,
			},
		},
		DB: dbx,
	})

	ExecuteTask(dbx, task)

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt != nil {
		t.Fatalf("LastRunAt should stay nil for no-op, got %v", *gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "" {
		t.Fatalf("LastStatus should stay empty for no-op, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
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

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt == nil || *gotTask.LastRunAt <= 0 {
		t.Fatalf("LastRunAt should be updated, got %v", gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "success" {
		t.Fatalf("LastStatus should be success, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
	if len(runs) != 1 {
		t.Fatalf("expected 1 task run, got %d", len(runs))
	}
	if runs[0].Status != "success" {
		t.Fatalf("task run status should be success, got %q", runs[0].Status)
	}
}

func TestExecuteInternalTaskAlsoCreatesTaskRun(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustCreateTask(t, dbx, "internal_task", db.TaskInternal)

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: func(_ *Registry) (*string, error) {
					msg := "internal done"
					return &msg, nil
				},
			},
		},
		DB: dbx,
	})

	ExecuteTask(dbx, task)

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt == nil || *gotTask.LastRunAt <= 0 {
		t.Fatalf("LastRunAt should be updated, got %v", gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "success" {
		t.Fatalf("LastStatus should be success, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
	if len(runs) != 1 {
		t.Fatalf("expected 1 task run for internal task, got %d", len(runs))
	}
	if runs[0].Status != "success" {
		t.Fatalf("task run status should be success, got %q", runs[0].Status)
	}
}

func TestExecuteTaskDisabledRemoteBackupDoesNotRecordRun(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustGetTaskByCode(t, dbx, "remote_backup_data")
	withTaskSettings(t, map[string]string{"sync_push_enabled": "0"})

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: PushSystemDataJob,
			},
		},
		DB: dbx,
	})

	ExecuteTask(dbx, task)

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt != nil {
		t.Fatalf("LastRunAt should stay nil for disabled remote backup no-op, got %v", *gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "" {
		t.Fatalf("LastStatus should stay empty for disabled remote backup no-op, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
	if len(runs) != 0 {
		t.Fatalf("expected no task runs for disabled remote backup no-op, got %d", len(runs))
	}
}

func TestRunLocalBackupNowBypassesInterval(t *testing.T) {
	dbx := openJobTestDB(t)
	backupDir := t.TempDir()
	withTaskSettings(t, map[string]string{
		"backup_local_dir":          backupDir,
		"backup_local_interval_min": "1440",
		"backup_local_max_count":    "30",
	})

	first, err := RunLocalBackupNow(dbx)
	if err != nil {
		t.Fatalf("RunLocalBackupNow first failed: %v", err)
	}
	if first == nil || strings.TrimSpace(*first) == "" {
		t.Fatal("RunLocalBackupNow first should return message")
	}

	if _, err := db.CreateTag(dbx, &db.Tag{Name: "tag-1", Slug: "tag-1"}); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	scheduled, err := DatabaseBackupJob(&Registry{DB: dbx})
	if err != nil {
		t.Fatalf("DatabaseBackupJob failed: %v", err)
	}
	if scheduled != nil {
		t.Fatalf("expected scheduled backup interval skip to be no-op, got %v", scheduled)
	}

	second, err := RunLocalBackupNow(dbx)
	if err != nil {
		t.Fatalf("RunLocalBackupNow second failed: %v", err)
	}
	if second == nil || strings.Contains(*second, "skip local backup: interval=") {
		t.Fatalf("RunLocalBackupNow second should bypass interval, got %v", second)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	sqliteCount := 0
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".sqlite") {
			sqliteCount++
		}
	}
	if sqliteCount != 2 {
		t.Fatalf("expected 2 sqlite backups, got %d", sqliteCount)
	}
}

func TestResolveLocalBackupDirForSQLiteUsesDatabaseCacheRoot(t *testing.T) {
	base := t.TempDir()
	got := ResolveLocalBackupDirForSQLite(db.DefaultBackupDir, filepath.Join(base, "data.sqlite"))
	want := filepath.Join(base, ".cache", "backups")
	if got != want {
		t.Fatalf("ResolveLocalBackupDirForSQLite = %q, want %q", got, want)
	}
}

func TestResolveLocalBackupDirForSQLiteKeepsWritesInsideCacheRoot(t *testing.T) {
	base := t.TempDir()
	got := ResolveLocalBackupDirForSQLite("../outside", filepath.Join(base, "data.sqlite"))
	want := filepath.Join(base, ".cache", "backups")
	if got != want {
		t.Fatalf("ResolveLocalBackupDirForSQLite = %q, want %q", got, want)
	}
}

func TestRunRemoteBackupNowDisabledReturnsMessage(t *testing.T) {
	dbx := openJobTestDB(t)
	withTaskSettings(t, map[string]string{"sync_push_enabled": "0"})
	withTestRegistry(t, &Registry{DB: dbx})

	msg, err := RunRemoteBackupNow(dbx)
	if err != nil {
		t.Fatalf("RunRemoteBackupNow failed: %v", err)
	}
	if msg == nil || !strings.Contains(*msg, "未启用") {
		t.Fatalf("expected disabled remote backup message, got %v", msg)
	}
}

func TestCheckAppUpdateJobCreatesNotification(t *testing.T) {
	dbx := openJobTestDB(t)
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "test"
	buildinfo.BuildTime = "2026-04-05T00:00:00Z"
	defer func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildTime = oldBuildTime
	}()

	msg, err := CheckAppUpdateJob(&Registry{
		DB: dbx,
		appUpdate: appUpdateDeps{
			checkLatestRelease: func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
				return updater.CheckResult{
					CurrentVersion:       currentVersion,
					CurrentVersionStable: true,
					LatestVersion:        "v1.2.4",
					LatestReleaseURL:     updater.ReleaseTagURL("v1.2.4"),
					HasUpgrade:           true,
					Target: &updater.ReleaseTarget{
						Archive: updater.ReleaseAsset{Name: "swaves_v1.2.4_linux_amd64.tar.gz"},
					},
				}, nil
			},
		},
	})
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

func TestExecuteTaskCheckAppUpdateNoUpgradeDoesNotRecordRun(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustGetTaskByCode(t, dbx, "check_app_update")
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "test"
	buildinfo.BuildTime = "2026-04-05T00:00:00Z"
	defer func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildTime = oldBuildTime
	}()

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: CheckAppUpdateJob,
			},
		},
		DB: dbx,
		appUpdate: appUpdateDeps{
			checkLatestRelease: func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
				return updater.CheckResult{
					CurrentVersion:       currentVersion,
					CurrentVersionStable: true,
					LatestVersion:        currentVersion,
					HasUpgrade:           false,
					Target:               nil,
				}, nil
			},
		},
	})

	ExecuteTask(dbx, task)

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt != nil {
		t.Fatalf("LastRunAt should stay nil for no-upgrade no-op, got %v", *gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "" {
		t.Fatalf("LastStatus should stay empty for no-upgrade no-op, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
	if len(runs) != 0 {
		t.Fatalf("expected no task runs for no-upgrade no-op, got %d", len(runs))
	}
}

func TestExecuteTaskClearExpiredNotificationsNoOpDoesNotRecordRun(t *testing.T) {
	dbx := openJobTestDB(t)
	task := mustCreateTask(t, dbx, "task_clear_notifications_noop", db.TaskInternal)

	withTestRegistry(t, &Registry{
		jobs: map[string]JobItem{
			task.Code: {
				Kind: task.Kind,
				Func: ClearExpiredNotificationsJob,
			},
		},
		DB: dbx,
	})

	ExecuteTask(dbx, task)

	gotTask := mustGetTaskByCode(t, dbx, task.Code)
	if gotTask.LastRunAt != nil {
		t.Fatalf("LastRunAt should stay nil for clear notifications no-op, got %v", *gotTask.LastRunAt)
	}
	if gotTask.LastStatus != "" {
		t.Fatalf("LastStatus should stay empty for clear notifications no-op, got %q", gotTask.LastStatus)
	}

	runs := mustListTaskRuns(t, dbx, task.Code)
	if len(runs) != 0 {
		t.Fatalf("expected no task runs for clear notifications no-op, got %d", len(runs))
	}
}

func TestMigrateBackupDir(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	oldDir := filepath.Join(base, db.LegacyBackupDir)
	newDir := filepath.Join(base, ".cache", "backups")

	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("MkdirAll old dir failed: %v", err)
	}

	backupFiles := []string{"backup-a.sqlite", "backup-b.sqlite"}

	// Write two fake backup files in the old dir.
	for _, name := range backupFiles {
		if err := os.WriteFile(filepath.Join(oldDir, name), []byte("data"), 0644); err != nil {
			t.Fatalf("WriteFile %s failed: %v", name, err)
		}
	}

	// No custom backup_local_dir setting → migration should run.
	withTaskSettings(t, map[string]string{})
	migrateBackupDir()

	for _, name := range backupFiles {
		if _, err := os.Stat(filepath.Join(newDir, name)); err != nil {
			t.Errorf("expected %s in new dir, got: %v", name, err)
		}
		if _, err := os.Stat(filepath.Join(oldDir, name)); err == nil {
			t.Errorf("expected %s to be removed from old dir after migration", name)
		}
	}
}

func TestMigrateBackupDirSkipsWhenCustomDirSet(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	oldDir := filepath.Join(base, db.LegacyBackupDir)
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("MkdirAll old dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "backup-x.sqlite"), []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Custom backup_local_dir is set → migration must not run.
	withTaskSettings(t, map[string]string{settingBackupLocalDir: "/custom/path"})
	migrateBackupDir()

	// File should still be in old dir.
	//if _, err := os.Stat(filepath.Join(oldDir, "backup-x.sqlite")); err != nil {
	//	t.Errorf("file should still be in old dir when custom dir is set: %v", err)
	//}
}

func TestMigrateBackupDirSkipsExistingDestination(t *testing.T) {
	base := t.TempDir()
	withWorkingDir(t, base)

	oldDir := filepath.Join(base, db.LegacyBackupDir)
	newDir := filepath.Join(base, ".cache", "backups")

	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatalf("MkdirAll old dir failed: %v", err)
	}
	if err := os.MkdirAll(newDir, 0755); err != nil {
		t.Fatalf("MkdirAll new dir failed: %v", err)
	}

	existingContent := []byte("new-version")
	if err := os.WriteFile(filepath.Join(newDir, "backup-dup.sqlite"), existingContent, 0644); err != nil {
		t.Fatalf("WriteFile new dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "backup-dup.sqlite"), []byte("old-version"), 0644); err != nil {
		t.Fatalf("WriteFile old dir failed: %v", err)
	}

	withTaskSettings(t, map[string]string{})
	migrateBackupDir()

	// Destination file must not be overwritten.
	got, err := os.ReadFile(filepath.Join(newDir, "backup-dup.sqlite"))
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(got) != string(existingContent) {
		t.Errorf("destination file was overwritten: got %q, want %q", got, existingContent)
	}
}
