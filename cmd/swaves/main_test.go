package main

import (
	"bytes"
	"errors"
	"flag"
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/updater"

	"golang.org/x/crypto/bcrypt"
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
	if err := db.EnsureDefaultSettings(model); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
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
	if strings.Contains(stdout.String(), "--demon-mode") {
		t.Fatalf("help should not mention removed demon-mode alias: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunUtilityCommandVersionFlag(t *testing.T) {
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v1.2.3"
	buildinfo.Commit = "abc1234"
	buildinfo.BuildTime = "2026-04-05T00:00:00Z"
	defer func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildTime = oldBuildTime
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handled, exitCode := runUtilityCommand([]string{"--version"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected --version to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "swaves v1.2.3") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunUtilityCommandVersionSubcommand(t *testing.T) {
	oldVersion := buildinfo.Version
	oldCommit := buildinfo.Commit
	oldBuildTime := buildinfo.BuildTime
	buildinfo.Version = "v2.0.0"
	buildinfo.Commit = "def5678"
	buildinfo.BuildTime = "2026-04-05T12:00:00Z"
	defer func() {
		buildinfo.Version = oldVersion
		buildinfo.Commit = oldCommit
		buildinfo.BuildTime = oldBuildTime
	}()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handled, exitCode := runUtilityCommand([]string{"version"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected version to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", exitCode, stderr.String())
	}
	if !strings.Contains(stdout.String(), "commit: def5678") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunUtilityCommandUpgradeCheck(t *testing.T) {
	oldCheck := checkLatestRelease
	checkLatestRelease = func(currentVersion string, goos string, goarch string) (updater.CheckResult, error) {
		return updater.CheckResult{
			CurrentVersion:   currentVersion,
			LatestVersion:    "v1.2.4",
			LatestReleaseURL: "https://github.com/keelii/swaves/releases/tag/v1.2.4",
			HasUpgrade:       true,
			Target: &updater.ReleaseTarget{
				Archive: updater.ReleaseAsset{Name: "swaves_v1.2.4_linux_amd64.tar.gz"},
			},
			Reason: "upgrade available: v1.2.3 -> v1.2.4",
		}, nil
	}
	defer func() { checkLatestRelease = oldCheck }()

	oldVersion := buildinfo.Version
	buildinfo.Version = "v1.2.3"
	defer func() { buildinfo.Version = oldVersion }()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handled, exitCode := runUtilityCommand([]string{"upgrade", "--check"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected upgrade --check to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", exitCode, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "status:  upgrade available") {
		t.Fatalf("unexpected stdout: %q", out)
	}
	if !strings.Contains(out, "asset:   swaves_v1.2.4_linux_amd64.tar.gz") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestRunUtilityCommandUpgradeInstallsCurrentExecutableWithoutMaster(t *testing.T) {
	oldInstall := installLatestRelease
	installLatestRelease = func(currentVersion string, goos string, goarch string) (updater.InstallResult, error) {
		return updater.InstallResult{
			CurrentVersion: currentVersion,
			LatestVersion:  "v1.2.4",
			ArchiveName:    "swaves_v1.2.4_linux_amd64.tar.gz",
			Installed:      true,
			Reason:         "installed v1.2.4 to current executable",
		}, nil
	}
	defer func() { installLatestRelease = oldInstall }()

	oldVersion := buildinfo.Version
	buildinfo.Version = "v1.2.3"
	defer func() { buildinfo.Version = oldVersion }()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	handled, exitCode := runUtilityCommand([]string{"upgrade"}, &stdout, &stderr)
	if !handled {
		t.Fatal("expected upgrade to be handled")
	}
	if exitCode != 0 {
		t.Fatalf("unexpected exit code: %d stderr=%q", exitCode, stderr.String())
	}

	out := stdout.String()
	if !strings.Contains(out, "status:  upgraded") {
		t.Fatalf("unexpected stdout: %q", out)
	}
	if strings.Contains(out, "master:") {
		t.Fatalf("unexpected master line in stdout: %q", out)
	}
	if !strings.Contains(out, "reason:  installed v1.2.4 to current executable") {
		t.Fatalf("unexpected stdout: %q", out)
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
	t.Setenv("SWAVES_BACKUP_DIR", "env-backups")
	t.Setenv("SWAVES_LISTEN_ADDR", ":5678")
	t.Setenv("SWAVES_APP_NAME", "swaves-env")
	t.Setenv("SWAVES_ENABLE_SQL_LOG", "false")

	cfg, err := parseAppConfig([]string{
		"cli.sqlite",
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
	t.Setenv("SWAVES_SQLITE_FILE", "")
	t.Setenv("SWAVES_BACKUP_DIR", "")
	t.Setenv("SWAVES_LISTEN_ADDR", "")
	t.Setenv("SWAVES_APP_NAME", "")
	t.Setenv("SWAVES_ENABLE_SQL_LOG", "")

	cfg, err := parseAppConfig([]string{
		"data.sqlite",
	})
	if err != nil {
		t.Fatalf("parseAppConfig failed: %v", err)
	}

	if cfg.BackupDir != updater.DefaultBackupDir {
		t.Fatalf("unexpected default backup dir: %q", cfg.BackupDir)
	}
	if cfg.ListenAddr != defaultListenAddr {
		t.Fatalf("unexpected default listen addr: %q", cfg.ListenAddr)
	}
	if cfg.AppName != defaultAppName {
		t.Fatalf("unexpected default app name: %q", cfg.AppName)
	}
	if cfg.EnableSQLLog != config.EnableSQLLog {
		t.Fatalf("unexpected default sql log flag: got=%v want=%v", cfg.EnableSQLLog, config.EnableSQLLog)
	}
}

func TestParseAppConfigRequiresSQLitePositionalArgument(t *testing.T) {
	_, err := parseAppConfig(nil)
	if err == nil {
		t.Fatal("expected missing sqlite file error")
	}
	if err.Error() != "sqlite file is required" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAppConfigRejectsAdminPasswordFlag(t *testing.T) {
	_, err := parseAppConfig([]string{
		"data.sqlite",
		"--admin-password=hash",
	})
	if err == nil {
		t.Fatal("expected unknown flag error")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -admin-password") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAppConfigReturnsHelp(t *testing.T) {
	_, err := parseAppConfig([]string{"--help"})
	if !errors.Is(err, flag.ErrHelp) {
		t.Fatalf("expected flag.ErrHelp, got %v", err)
	}
}

func TestParseMainConfigParsesSupervisorFlags(t *testing.T) {
	cfg, err := parseMainConfig([]string{
		"data.sqlite",
		"--daemon-mode=0",
		"--max-failures=9",
	})
	if err != nil {
		t.Fatalf("parseMainConfig failed: %v", err)
	}

	if cfg.DaemonMode {
		t.Fatal("expected daemon mode disabled")
	}
	if cfg.MaxFailures != 9 {
		t.Fatalf("unexpected max failures: %d", cfg.MaxFailures)
	}
}

func TestParseMainConfigRejectsRemovedDemonModeFlag(t *testing.T) {
	_, err := parseMainConfig([]string{
		"data.sqlite",
		"--demon-mode=0",
	})
	if err == nil {
		t.Fatal("expected removed demon-mode flag to be rejected")
	}
	if !strings.Contains(err.Error(), "flag provided but not defined: -demon-mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseMainConfigSupportsDaemonModeEnv(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv(daemonModeConfigEnv, "1")

	cfg, err := parseMainConfig(nil)
	if err != nil {
		t.Fatalf("parseMainConfig failed: %v", err)
	}
	if !cfg.DaemonMode {
		t.Fatal("expected daemon mode enabled from environment")
	}
}

func TestParseMainConfigAcceptsInternalWorkerFlag(t *testing.T) {
	cfg, err := parseMainConfig([]string{
		"data.sqlite",
		workerProcessFlag,
	})
	if err != nil {
		t.Fatalf("parseMainConfig failed: %v", err)
	}
	if cfg.DaemonMode != (defaultDaemonMode == 1) {
		t.Fatal("expected internal worker flag not to change daemon mode")
	}
}

func TestParseMainConfigIgnoresWorkerModeEnvForDaemonConfig(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv(workerModeEnv, "1")

	cfg, err := parseMainConfig(nil)
	if err != nil {
		t.Fatalf("parseMainConfig failed: %v", err)
	}
	if cfg.DaemonMode != (defaultDaemonMode == 1) {
		t.Fatal("expected worker mode env not to change daemon mode")
	}
}

func TestParseMainConfigRejectsInvalidDaemonMode(t *testing.T) {
	_, err := parseMainConfig([]string{
		"data.sqlite",
		"--daemon-mode=2",
	})
	if err == nil {
		t.Fatal("expected invalid daemon mode error")
	}
	if err.Error() != "daemon-mode must be 0 or 1" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseMainConfigRejectsInvalidDaemonModeEnv(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv(daemonModeConfigEnv, "nope")

	_, err := parseMainConfig(nil)
	if err == nil {
		t.Fatal("expected invalid daemon mode env error")
	}
	if !strings.Contains(err.Error(), "invalid SWAVES_DAEMON_MODE") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRuntimeModeRejectsWindowsDaemonMode(t *testing.T) {
	err := validateRuntimeMode(mainConfig{DaemonMode: true}, "windows")
	if err == nil {
		t.Fatal("expected windows daemon mode error")
	}
	if err.Error() != "daemon-mode=1 is not supported on Windows" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateRuntimeModeAllowsNonWindowsDaemonMode(t *testing.T) {
	err := validateRuntimeMode(mainConfig{DaemonMode: true}, "linux")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestParseAppConfigRejectsInvalidEnvBool(t *testing.T) {
	t.Setenv("SWAVES_SQLITE_FILE", "env.sqlite")
	t.Setenv("SWAVES_ENABLE_SQL_LOG", "not-bool")

	_, err := parseAppConfig(nil)
	if err == nil {
		t.Fatal("expected invalid env bool error")
	}
	if !strings.Contains(err.Error(), "invalid SWAVES_ENABLE_SQL_LOG") {
		t.Fatalf("unexpected error: %v", err)
	}
}
