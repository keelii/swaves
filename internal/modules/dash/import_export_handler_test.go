package dash

import (
	"database/sql"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

func withDashTestSettings(t *testing.T, settings map[string]string) {
	t.Helper()

	restore := map[string]string{}
	if current, ok := store.Settings.Load().(map[string]string); ok {
		for key, value := range current {
			restore[key] = value
		}
	}
	t.Cleanup(func() {
		store.Settings.Store(restore)
	})

	next := make(map[string]string, len(settings))
	for key, value := range settings {
		next[key] = value
	}
	store.Settings.Store(next)
}

func TestCleanupFileStreamCloseRunsCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "export.sqlite")
	if err := os.WriteFile(filePath, []byte("sqlite-data"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	cleanupCalled := false
	stream, err := openCleanupFileStream(filePath, func() {
		cleanupCalled = true
	})
	if err != nil {
		t.Fatalf("openCleanupFileStream failed: %v", err)
	}

	if err := stream.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if !cleanupCalled {
		t.Fatal("expected cleanup to be called")
	}
}

func TestListLocalRestoreBackups(t *testing.T) {
	tmpDir := t.TempDir()
	withDashTestSettings(t, map[string]string{"backup_local_dir": tmpDir})

	oldPath := filepath.Join(tmpDir, "2026-04-01_old.sqlite")
	newPath := filepath.Join(tmpDir, "2026-04-02_new.sqlite")
	if err := os.WriteFile(oldPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("WriteFile old failed: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFile new failed: %v", err)
	}
	oldTime := time.Date(2026, 4, 1, 10, 0, 0, 0, time.Local)
	newTime := time.Date(2026, 4, 2, 10, 0, 0, 0, time.Local)
	if err := os.Chtimes(oldPath, oldTime, oldTime); err != nil {
		t.Fatalf("Chtimes old failed: %v", err)
	}
	if err := os.Chtimes(newPath, newTime, newTime); err != nil {
		t.Fatalf("Chtimes new failed: %v", err)
	}

	backups, dir, err := listLocalRestoreBackups()
	if err != nil {
		t.Fatalf("listLocalRestoreBackups failed: %v", err)
	}
	if dir != tmpDir {
		t.Fatalf("unexpected backup dir=%q", dir)
	}
	if len(backups) != 2 {
		t.Fatalf("unexpected backup count=%d", len(backups))
	}
	if backups[0].Name != "2026-04-02_new.sqlite" {
		t.Fatalf("expected newest backup first, got %q", backups[0].Name)
	}
}

func TestGetBackupRestoreDownloadHandler(t *testing.T) {
	tmpDir := t.TempDir()
	withDashTestSettings(t, map[string]string{"backup_local_dir": tmpDir})

	backupName := "2026-04-10.sqlite"
	backupBody := "sqlite-backup-data"
	backupPath := filepath.Join(tmpDir, backupName)
	if err := os.WriteFile(backupPath, []byte(backupBody), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	app := fiber.New()
	handler := &Handler{Model: openDashTestDB(t)}
	app.Get("/download", handler.GetBackupRestoreDownloadHandler)

	req := httptest.NewRequest("GET", "/download?backup_file="+backupName, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/x-sqlite3" {
		t.Fatalf("Content-Type = %q, want %q", got, "application/x-sqlite3")
	}
	if got := resp.Header.Get("Content-Disposition"); !strings.Contains(got, `filename="2026-04-10.sqlite"`) {
		t.Fatalf("Content-Disposition = %q", got)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if string(body) != backupBody {
		t.Fatalf("body = %q, want %q", string(body), backupBody)
	}
}

func TestDeleteLocalRestoreBackup(t *testing.T) {
	tmpDir := t.TempDir()
	withDashTestSettings(t, map[string]string{"backup_local_dir": tmpDir})

	targetPath := filepath.Join(tmpDir, "2026-04-10.sqlite")
	if err := os.WriteFile(targetPath, []byte("backup"), 0o644); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	if err := deleteLocalRestoreBackup("2026-04-10.sqlite"); err != nil {
		t.Fatalf("deleteLocalRestoreBackup failed: %v", err)
	}
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		t.Fatalf("expected backup file removed, stat err=%v", err)
	}
}

func TestValidateRestoreSQLiteFileAcceptsPrefixedTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restore.sqlite")

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	requiredTables := []string{
		string(db.TablePosts),
		string(db.TableSettings),
		string(db.TableCategories),
		string(db.TableTags),
		string(db.TableComments),
		string(db.TableAssets),
		string(db.TableSessions),
	}
	for _, table := range requiredTables {
		createSQL := "CREATE TABLE " + table + " (id INTEGER PRIMARY KEY)"
		if table == string(db.TableSettings) {
			createSQL = "CREATE TABLE " + table + " (id INTEGER PRIMARY KEY, code TEXT)"
		}
		if _, err := database.Exec(createSQL); err != nil {
			t.Fatalf("create table %s failed: %v", table, err)
		}
	}
	if _, err := database.Exec("INSERT INTO " + string(db.TableSettings) + " (id, code) VALUES (1, 'dash_password')"); err != nil {
		t.Fatalf("insert settings row failed: %v", err)
	}

	if err := validateRestoreSQLiteFile(dbPath); err != nil {
		t.Fatalf("validateRestoreSQLiteFile failed: %v", err)
	}
}

func TestValidateRestoreSQLiteFileReportsLogicalTableName(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "restore-missing.sqlite")

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	t.Cleanup(func() {
		_ = database.Close()
	})

	requiredTables := []string{
		string(db.TableSettings),
		string(db.TableCategories),
		string(db.TableTags),
		string(db.TableComments),
		string(db.TableAssets),
		string(db.TableSessions),
	}
	for _, table := range requiredTables {
		if _, err := database.Exec("CREATE TABLE " + table + " (id INTEGER PRIMARY KEY)"); err != nil {
			t.Fatalf("create table %s failed: %v", table, err)
		}
	}

	err = validateRestoreSQLiteFile(dbPath)
	if err == nil {
		t.Fatal("expected validateRestoreSQLiteFile to fail when posts table is missing")
	}
	if !strings.Contains(err.Error(), "posts") {
		t.Fatalf("expected logical missing table name in error, got %v", err)
	}
}

func TestPaginateRestoreBackups(t *testing.T) {
	backups := []restoreBackupFile{
		{Name: "a.sqlite"},
		{Name: "b.sqlite"},
		{Name: "c.sqlite"},
	}
	pager := types.Pagination{Page: 2, PageSize: 2}

	got := paginateRestoreBackups(backups, &pager)

	if pager.Total != 3 || pager.Num != 2 || pager.Page != 2 {
		t.Fatalf("unexpected pager after paginate: %#v", pager)
	}
	if len(got) != 1 || got[0].Name != "c.sqlite" {
		t.Fatalf("unexpected paged backups: %#v", got)
	}
}

func TestOpenReadOnlySQLiteAcceptsRelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "relative.sqlite")

	database, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	if _, err := database.Exec("CREATE TABLE t_settings (id INTEGER PRIMARY KEY, code TEXT)"); err != nil {
		t.Fatalf("create table failed: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	readOnlyDB, err := openReadOnlySQLite("relative.sqlite")
	if err != nil {
		t.Fatalf("openReadOnlySQLite failed: %v", err)
	}
	defer func() { _ = readOnlyDB.Close() }()

	var count int
	if err := readOnlyDB.QueryRow("SELECT COUNT(1) FROM t_settings").Scan(&count); err != nil {
		t.Fatalf("QueryRow failed: %v", err)
	}
}
