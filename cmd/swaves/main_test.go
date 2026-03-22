package main

import (
	"bytes"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
)

func TestRunUtilityCommandHashPassword(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handled, exitCode := runUtilityCommand([]string{"hash-password", "admin"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected hash-password to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", exitCode, stderr.String())
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}

	hashed := strings.TrimSpace(stdout.String())
	if hashed == "" {
		t.Fatal("expected hashed password output")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte("admin")); err != nil {
		t.Fatalf("output is not valid bcrypt hash for input password: %v", err)
	}
}

func TestRunUtilityCommandHashPasswordRequiresPassword(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handled, exitCode := runUtilityCommand([]string{"hash-password"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected hash-password to be handled")
	}
	if exitCode != 2 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "password is required for hash-password") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunUtilityCommandSetAdminPassword(t *testing.T) {
	sqliteFile := filepath.Join(t.TempDir(), "data.sqlite")
	model := db.Open(db.Options{DSN: sqliteFile})
	_ = model.Close()

	rawPassword := strings.Repeat("admin-password-", 4) + "adm"
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handled, exitCode := runUtilityCommand([]string{"set-admin-password", sqliteFile, rawPassword}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected set-admin-password to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", exitCode, stderr.String())
	}

	hashed := strings.TrimSpace(stdout.String())
	if hashed == "" {
		t.Fatal("expected stored hash output")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(hashed), []byte(rawPassword)); err != nil {
		t.Fatalf("output is not valid bcrypt hash for input password: %v", err)
	}
	if !strings.Contains(stderr.String(), "updated settings.dash_password") {
		t.Fatalf("expected success message, got stderr=%q", stderr.String())
	}

	verifyDB, err := db.OpenWithError(db.Options{DSN: sqliteFile})
	if err != nil {
		t.Fatalf("OpenWithError failed: %v", err)
	}
	defer func() { _ = verifyDB.Close() }()

	if err := db.CheckPassword(verifyDB, rawPassword); err != nil {
		t.Fatalf("CheckPassword should pass: %v", err)
	}
	setting, err := db.GetSettingByCode(verifyDB, "dash_password")
	if err != nil {
		t.Fatalf("GetSettingByCode(dash_password) failed: %v", err)
	}
	if setting.Value != hashed {
		t.Fatalf("stored dash_password mismatch: got=%q want=%q", setting.Value, hashed)
	}
}

func TestRunUtilityCommandSetAdminPasswordRequiresPassword(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handled, exitCode := runUtilityCommand([]string{"set-admin-password", "data.sqlite"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected set-admin-password to be handled")
	}
	if exitCode != 2 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stderr.String(), "password is required for set-admin-password") {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunUtilityCommandSetAdminPasswordHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handled, exitCode := runUtilityCommand([]string{"set-admin-password", "--help"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected set-admin-password help to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "swaves set-admin-password <sqlite-file> <raw-password>") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunUtilityCommandTopLevelHelp(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	handled, exitCode := runUtilityCommand([]string{"--help"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected help to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d", exitCode)
	}
	if !strings.Contains(stdout.String(), "swaves hash-password <raw-password>") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), "swaves set-admin-password <sqlite-file> <raw-password>") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestParseAppConfigSupportsPositionalSQLiteAndFlags(t *testing.T) {
	cfg, err := parseAppConfig([]string{
		"data.sqlite",
		"--admin-password=$2a$10$abcdefghijklmnopqrstuvabcdefghijklmnopqrstuvabcd",
		"--backup-dir=my-backups",
		"--listen-addr=:4321",
		"--app-name=swaves-local",
		"--enable-sql-log=true",
	})
	if err != nil {
		t.Fatalf("parseAppConfig failed: %v", err)
	}

	if cfg.SqliteFile != "data.sqlite" {
		t.Fatalf("unexpected sqlite file: %q", cfg.SqliteFile)
	}
	if cfg.AdminPassword != "$2a$10$abcdefghijklmnopqrstuvabcdefghijklmnopqrstuvabcd" {
		t.Fatalf("unexpected admin password: %q", cfg.AdminPassword)
	}
	if cfg.BackupDir != "my-backups" {
		t.Fatalf("unexpected backup dir: %q", cfg.BackupDir)
	}
	if cfg.ListenAddr != ":4321" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if cfg.AppName != "swaves-local" {
		t.Fatalf("unexpected app name: %q", cfg.AppName)
	}
	if !cfg.EnableSQLLog {
		t.Fatalf("expected sql log enabled")
	}
}

func TestParseAppConfigSupportsEnvironmentVariables(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv("SWAVES_ADMIN_PASSWORD", "$2a$10$envhashenvhashenvhashenvhashenvhashenvhashenvhash")
	t.Setenv("SWAVES_BACKUP_DIR", "env-backups")
	t.Setenv("SWAVES_LISTEN_ADDR", ":5678")
	t.Setenv("SWAVES_APP_NAME", "swaves-env")
	t.Setenv("SWAVES_ENABLE_SQL_LOG", "true")

	cfg, err := parseAppConfig(nil)
	if err != nil {
		t.Fatalf("parseAppConfig failed: %v", err)
	}

	if cfg.SqliteFile != "env.sqlite" {
		t.Fatalf("unexpected sqlite file: %q", cfg.SqliteFile)
	}
	if cfg.AdminPassword != "$2a$10$envhashenvhashenvhashenvhashenvhashenvhashenvhash" {
		t.Fatalf("unexpected admin password: %q", cfg.AdminPassword)
	}
	if cfg.BackupDir != "env-backups" {
		t.Fatalf("unexpected backup dir: %q", cfg.BackupDir)
	}
	if cfg.ListenAddr != ":5678" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if cfg.AppName != "swaves-env" {
		t.Fatalf("unexpected app name: %q", cfg.AppName)
	}
	if !cfg.EnableSQLLog {
		t.Fatalf("expected sql log enabled")
	}
}

func TestParseAppConfigFlagsOverrideEnvironmentVariables(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv("SWAVES_ADMIN_PASSWORD", "$2a$10$envhashenvhashenvhashenvhashenvhashenvhashenvhash")
	t.Setenv("SWAVES_BACKUP_DIR", "env-backups")
	t.Setenv("SWAVES_LISTEN_ADDR", ":5678")
	t.Setenv("SWAVES_APP_NAME", "swaves-env")
	t.Setenv("SWAVES_ENABLE_SQL_LOG", "false")

	cfg, err := parseAppConfig([]string{
		"cli.sqlite",
		"--admin-password=$2a$10$clihashclihashclihashclihashclihashclihashclihashc",
		"--backup-dir=cli-backups",
		"--listen-addr=:9999",
		"--app-name=swaves-cli",
		"--enable-sql-log=true",
	})
	if err != nil {
		t.Fatalf("parseAppConfig failed: %v", err)
	}

	if cfg.SqliteFile != "cli.sqlite" {
		t.Fatalf("unexpected sqlite file: %q", cfg.SqliteFile)
	}
	if cfg.AdminPassword != "$2a$10$clihashclihashclihashclihashclihashclihashclihashc" {
		t.Fatalf("unexpected admin password: %q", cfg.AdminPassword)
	}
	if cfg.BackupDir != "cli-backups" {
		t.Fatalf("unexpected backup dir: %q", cfg.BackupDir)
	}
	if cfg.ListenAddr != ":9999" {
		t.Fatalf("unexpected listen addr: %q", cfg.ListenAddr)
	}
	if cfg.AppName != "swaves-cli" {
		t.Fatalf("unexpected app name: %q", cfg.AppName)
	}
	if !cfg.EnableSQLLog {
		t.Fatalf("expected sql log enabled")
	}
}

func TestParseAppConfigUsesDefaultFlags(t *testing.T) {
	cfg, err := parseAppConfig([]string{
		"data.sqlite",
		"--admin-password=$2a$10$abcdefghijklmnopqrstuvabcdefghijklmnopqrstuvabcd",
	})
	if err != nil {
		t.Fatalf("parseAppConfig failed: %v", err)
	}

	if cfg.BackupDir != "backups" {
		t.Fatalf("unexpected default backup dir: %q", cfg.BackupDir)
	}
	if cfg.ListenAddr != ":3000" {
		t.Fatalf("unexpected default listen addr: %q", cfg.ListenAddr)
	}
	if cfg.AppName != "swaves" {
		t.Fatalf("unexpected default app name: %q", cfg.AppName)
	}
	if cfg.EnableSQLLog != config.EnableSQLLog {
		t.Fatalf("unexpected default sql log flag: got=%v want=%v", cfg.EnableSQLLog, config.EnableSQLLog)
	}
}

func TestParseAppConfigRequiresAdminPassword(t *testing.T) {
	_, err := parseAppConfig([]string{"data.sqlite"})
	if err == nil {
		t.Fatal("expected missing admin password error")
	}
	if err.Error() != "admin password is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAppConfigRequiresSQLitePositionalArgument(t *testing.T) {
	_, err := parseAppConfig([]string{"--admin-password=hash"})
	if err == nil {
		t.Fatal("expected missing sqlite file error")
	}
	if err.Error() != "sqlite file is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAppConfigReturnsHelp(t *testing.T) {
	_, err := parseAppConfig([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestParseAppConfigRejectsInvalidEnvBool(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv("SWAVES_ADMIN_PASSWORD", "$2a$10$envhashenvhashenvhashenvhashenvhashenvhashenvhash")
	t.Setenv("SWAVES_ENABLE_SQL_LOG", "not-bool")

	_, err := parseAppConfig(nil)
	if err == nil {
		t.Fatal("expected invalid env bool error")
	}
	if !strings.Contains(err.Error(), "invalid SWAVES_ENABLE_SQL_LOG") {
		t.Fatalf("unexpected error: %v", err)
	}
}
