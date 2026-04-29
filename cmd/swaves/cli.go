package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/updater"
	"swaves/internal/shared/types"

	"golang.org/x/crypto/bcrypt"
)

const (
	defaultBackupDir    = db.DefaultBackupDir
	defaultListenAddr   = ":4096"
	defaultAppName      = "swaves"
	defaultDaemonMode   = 1
	defaultMaxFailures  = 5
	flagBackupDirKey    = "backup-dir"
	flagListenAddrKey   = "listen-addr"
	flagAppNameKey      = "app-name"
	flagEnableSQLLogKey = "enable-sql-log"
	flagDaemonModeKey   = "daemon-mode"
	flagMaxFailuresKey  = "max-failures"
)

const (
	flagBackupDirUsage    = "backup directory"
	flagListenAddrUsage   = "listen address"
	flagAppNameUsage      = "app name"
	flagEnableSQLLogUsage = "enable sql log"
	flagDaemonModeUsage   = "1: run with master process, otherwise run worker directly"
	flagMaxFailuresUsage  = "max consecutive worker failures before master exits (<=0 means unlimited)"
)

var (
	flagBackupDir    = flag.String(flagBackupDirKey, defaultBackupDir, flagBackupDirUsage)
	flagListenAddr   = flag.String(flagListenAddrKey, defaultListenAddr, flagListenAddrUsage)
	flagAppName      = flag.String(flagAppNameKey, defaultAppName, flagAppNameUsage)
	flagEnableSQLLog = flag.Bool(flagEnableSQLLogKey, config.EnableSQLLog, flagEnableSQLLogUsage)
	flagDaemonMode   = flag.Int(flagDaemonModeKey, defaultDaemonMode, flagDaemonModeUsage)
	flagMaxFailures  = flag.Int(flagMaxFailuresKey, defaultMaxFailures, flagMaxFailuresUsage)
)

var checkLatestRelease = updater.CheckLatestRelease
var installLatestRelease = updater.InstallLatestReleaseCLI

type mainConfig struct {
	AppConfig   types.AppConfig
	DaemonMode  bool
	MaxFailures int
}

func runCLI(args []string, stdout io.Writer, stderr io.Writer) int {
	handled, exitCode := runUtilityCommand(args, stdout, stderr)
	if handled {
		return exitCode
	}

	cfg, err := parseMainConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(stdout, cliUsage())
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "%v\n\n%s", err, cliUsage())
		return 2
	}
	if err := validateRuntimeMode(cfg, runtime.GOOS); err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n", err)
		return 2
	}

	if !cfg.DaemonMode && os.Getenv(workerModeEnv) != "1" {
		if err := runSwavesApp(cfg.AppConfig); err != nil {
			logger.Fatal("%v", err)
		}
		return 0
	}

	if err := runSupervisor(supervisorConfig{
		DaemonMode:  cfg.DaemonMode,
		ListenAddr:  cfg.AppConfig.ListenAddr,
		SqliteFile:  cfg.AppConfig.SqliteFile,
		MaxFailures: cfg.MaxFailures,
		Args:        args,
		Worker: func() error {
			return runSwavesWorker(cfg.AppConfig)
		},
	}); err != nil {
		logger.Fatal("%v", err)
	}

	return 0
}

func validateRuntimeMode(cfg mainConfig, goos string) error {
	if cfg.DaemonMode && goos == "windows" {
		return fmt.Errorf("%s=1 is not supported on Windows", flagDaemonModeKey)
	}
	return nil
}

func runUtilityCommand(args []string, stdout io.Writer, stderr io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}

	switch strings.TrimSpace(args[0]) {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cliUsage())
		return true, 0
	case "-v", "--version", "version":
		if len(args) > 1 {
			_, _ = fmt.Fprintf(stderr, "unexpected extra arguments for version: %s\n\n%s", strings.Join(args[1:], " "), versionUsage())
			return true, 2
		}
		_, _ = fmt.Fprint(stdout, buildinfo.Summary())
		return true, 0
	case "upgrade":
		return true, runUpgradeCommand(args[1:], stdout, stderr)
	case "hash-password":
		if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
			_, _ = fmt.Fprint(stdout, hashPasswordUsage())
			return true, 0
		}
		if len(args) < 2 {
			_, _ = fmt.Fprintf(stderr, "password is required for hash-password\n\n%s", hashPasswordUsage())
			return true, 2
		}
		if len(args) > 2 {
			_, _ = fmt.Fprintf(stderr, "unexpected extra arguments for hash-password: %s\n\n%s", strings.Join(args[2:], " "), hashPasswordUsage())
			return true, 2
		}

		hashed, err := hashPassword(args[1])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%v\n", err)
			return true, 2
		}
		_, _ = fmt.Fprintln(stdout, hashed)
		return true, 0
	case "set-admin-password":
		if len(args) == 2 && (args[1] == "-h" || args[1] == "--help") {
			_, _ = fmt.Fprint(stdout, setAdminPasswordUsage())
			return true, 0
		}
		if len(args) < 2 {
			_, _ = fmt.Fprintf(stderr, "sqlite file is required for set-admin-password\n\n%s", setAdminPasswordUsage())
			return true, 2
		}
		if len(args) < 3 {
			_, _ = fmt.Fprintf(stderr, "password is required for set-admin-password\n\n%s", setAdminPasswordUsage())
			return true, 2
		}
		if len(args) > 3 {
			_, _ = fmt.Fprintf(stderr, "unexpected extra arguments for set-admin-password: %s\n\n%s", strings.Join(args[3:], " "), setAdminPasswordUsage())
			return true, 2
		}

		hashed, err := setAdminPassword(args[1], args[2])
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "%v\n", err)
			return true, 2
		}

		_, _ = fmt.Fprintln(stdout, hashed)
		_, _ = fmt.Fprintf(
			stderr,
			"updated settings.dash_password in %s\n",
			args[1],
		)
		return true, 0
	default:
		return false, 0
	}
}

func runUpgradeCommand(args []string, stdout io.Writer, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = fmt.Fprint(stdout, upgradeUsage())
		return 0
	}

	var checkOnly bool
	fs := flag.NewFlagSet("swaves upgrade", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.BoolVar(&checkOnly, "check", false, "check latest stable release only")
	if err := fs.Parse(args); err != nil {
		_, _ = fmt.Fprintf(stderr, "%v\n\n%s", err, upgradeUsage())
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "unexpected extra arguments for upgrade: %s\n\n%s", strings.Join(fs.Args(), " "), upgradeUsage())
		return 2
	}
	if !checkOnly {
		result, err := installLatestRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "upgrade failed: %v\n", err)
			return 2
		}
		_, _ = fmt.Fprintf(stdout, "current: %s\n", fallbackVersionLabel(result.CurrentVersion))
		_, _ = fmt.Fprintf(stdout, "latest:  %s\n", fallbackVersionLabel(result.LatestVersion))
		if result.ArchiveName != "" {
			_, _ = fmt.Fprintf(stdout, "asset:   %s\n", result.ArchiveName)
		}
		if result.Installed {
			_, _ = fmt.Fprintf(stdout, "status:  upgraded\n")
			if result.RestartedPID > 0 {
				_, _ = fmt.Fprintf(stdout, "master:  %d\n", result.RestartedPID)
			}
		} else {
			_, _ = fmt.Fprintf(stdout, "status:  no-op\n")
		}
		if result.Reason != "" {
			_, _ = fmt.Fprintf(stdout, "reason:  %s\n", result.Reason)
		}
		if result.ReleaseURL != "" {
			_, _ = fmt.Fprintf(stdout, "release: %s\n", result.ReleaseURL)
		}
		return 0
	}

	result, err := checkLatestRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "check latest release failed: %v\n", err)
		return 2
	}

	_, _ = fmt.Fprintf(stdout, "current: %s\n", fallbackVersionLabel(result.CurrentVersion))
	_, _ = fmt.Fprintf(stdout, "latest:  %s\n", fallbackVersionLabel(result.LatestVersion))
	if result.Target != nil {
		_, _ = fmt.Fprintf(stdout, "asset:   %s\n", result.Target.Archive.Name)
	}

	status := "up-to-date"
	switch {
	case result.Target == nil:
		status = "unsupported"
	case result.HasUpgrade:
		status = "upgrade available"
	case !result.CurrentVersionStable:
		status = "non-release build"
	}
	_, _ = fmt.Fprintf(stdout, "status:  %s\n", status)
	if strings.TrimSpace(result.Reason) != "" {
		_, _ = fmt.Fprintf(stdout, "reason:  %s\n", result.Reason)
	}
	if strings.TrimSpace(result.LatestReleaseURL) != "" {
		_, _ = fmt.Fprintf(stdout, "release: %s\n", result.LatestReleaseURL)
	}
	return 0
}

func fallbackVersionLabel(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	return version
}

func hashPassword(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("password is required")
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("hash password failed: %w", err)
	}
	return string(hashed), nil
}

func setAdminPassword(sqliteFile string, raw string) (string, error) {
	sqliteFile = strings.TrimSpace(sqliteFile)
	if sqliteFile == "" {
		return "", errors.New("sqlite file is required")
	}
	if raw == "" {
		return "", errors.New("password is required")
	}

	info, err := os.Stat(sqliteFile)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("sqlite file not found: %s", sqliteFile)
		}
		return "", fmt.Errorf("stat sqlite file failed: %w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("sqlite file path is a directory: %s", sqliteFile)
	}

	hashed, err := hashPassword(raw)
	if err != nil {
		return "", err
	}

	model, err := db.OpenWithError(db.Options{DSN: sqliteFile})
	if err != nil {
		return "", fmt.Errorf("open sqlite failed: %w", err)
	}
	defer func() { _ = model.Close() }()

	if err := db.UpdateSettingByCode(model, "dash_password", hashed); err != nil {
		return "", fmt.Errorf("update dash_password failed: %w", err)
	}
	setting, err := db.GetSettingByCode(model, "dash_password")
	if err != nil {
		return "", fmt.Errorf("load dash_password failed: %w", err)
	}
	if err := db.CheckPassword(model, raw); err != nil {
		return "", fmt.Errorf("verify dash_password failed: %w", err)
	}

	return setting.Value, nil
}

func parseAppConfig(args []string) (types.AppConfig, error) {
	cfg, err := parseMainConfig(args)
	return cfg.AppConfig, err
}

func parseMainConfig(args []string) (mainConfig, error) {
	cfg := mainConfig{
		AppConfig:   defaultAppConfig(),
		DaemonMode:  defaultDaemonMode == 1,
		MaxFailures: defaultMaxFailures,
	}

	if err := applyEnvAppConfig(&cfg.AppConfig); err != nil {
		return cfg, err
	}

	if len(args) > 0 {
		firstArg := strings.TrimSpace(args[0])
		if firstArg == "-h" || firstArg == "--help" || firstArg == "help" {
			return cfg, flag.ErrHelp
		}
	}

	flagArgs := consumeSQLitePositionalArg(&cfg.AppConfig, args)
	daemonMode := defaultDaemonMode
	internalWorker := false
	if !cfg.DaemonMode {
		daemonMode = 0
	}
	if raw, ok := lookupTrimmedEnv(daemonModeConfigEnv); ok {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			return cfg, fmt.Errorf("invalid %s: %w", daemonModeConfigEnv, err)
		}
		daemonMode = parsed
	}

	fs := newMainFlagSet(&cfg, &daemonMode, &internalWorker)
	if err := fs.Parse(flagArgs); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected extra arguments: %s", strings.Join(fs.Args(), " "))
	}
	if daemonMode != 0 && daemonMode != 1 {
		return cfg, fmt.Errorf("%s must be 0 or 1", flagDaemonModeKey)
	}

	normalizeAppConfig(&cfg.AppConfig)
	cfg.DaemonMode = daemonMode == 1
	if cfg.AppConfig.SqliteFile == "" {
		return cfg, errors.New("sqlite file is required")
	}
	if err := updater.ConfigureRuntimeCacheRoot(cfg.AppConfig.SqliteFile); err != nil {
		return cfg, err
	}

	return cfg, nil
}

func defaultAppConfig() types.AppConfig {
	return types.AppConfig{
		BackupDir:    defaultBackupDir,
		ListenAddr:   defaultListenAddr,
		AppName:      defaultAppName,
		EnableSQLLog: config.EnableSQLLog,
	}
}

func consumeSQLitePositionalArg(cfg *types.AppConfig, args []string) []string {
	if len(args) == 0 {
		return args
	}

	firstArg := strings.TrimSpace(args[0])
	if firstArg == "" || strings.HasPrefix(firstArg, "-") {
		return args
	}

	cfg.SqliteFile = firstArg
	return args[1:]
}

func newMainFlagSet(cfg *mainConfig, daemonMode *int, internalWorker *bool) *flag.FlagSet {
	fs := flag.NewFlagSet("swaves", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.AppConfig.BackupDir, flagBackupDirKey, cfg.AppConfig.BackupDir, flagBackupDirUsage)
	fs.StringVar(&cfg.AppConfig.ListenAddr, flagListenAddrKey, cfg.AppConfig.ListenAddr, flagListenAddrUsage)
	fs.StringVar(&cfg.AppConfig.AppName, flagAppNameKey, cfg.AppConfig.AppName, flagAppNameUsage)
	fs.BoolVar(&cfg.AppConfig.EnableSQLLog, flagEnableSQLLogKey, cfg.AppConfig.EnableSQLLog, flagEnableSQLLogUsage)
	fs.IntVar(daemonMode, flagDaemonModeKey, *daemonMode, flagDaemonModeUsage)
	fs.IntVar(&cfg.MaxFailures, flagMaxFailuresKey, cfg.MaxFailures, flagMaxFailuresUsage)
	fs.BoolVar(internalWorker, strings.TrimPrefix(workerProcessFlag, "--"), false, "")
	return fs
}

func normalizeAppConfig(cfg *types.AppConfig) {
	cfg.SqliteFile = strings.TrimSpace(cfg.SqliteFile)
	cfg.BackupDir = strings.TrimSpace(cfg.BackupDir)
	cfg.ListenAddr = strings.TrimSpace(cfg.ListenAddr)
	cfg.AppName = strings.TrimSpace(cfg.AppName)
}

func applyEnvAppConfig(cfg *types.AppConfig) error {
	if cfg == nil {
		return nil
	}

	if value, ok := lookupTrimmedEnv("SWAVES_SQLITE_FILE"); ok {
		cfg.SqliteFile = value
	}
	if value, ok := lookupTrimmedEnv("SWAVES_BACKUP_DIR"); ok {
		cfg.BackupDir = value
	}
	if value, ok := lookupTrimmedEnv("SWAVES_LISTEN_ADDR"); ok {
		cfg.ListenAddr = value
	}
	if value, ok := lookupTrimmedEnv("SWAVES_APP_NAME"); ok {
		cfg.AppName = value
	}
	if raw, ok := lookupTrimmedEnv("SWAVES_ENABLE_SQL_LOG"); ok {
		parsed, err := strconv.ParseBool(raw)
		if err != nil {
			return fmt.Errorf("invalid SWAVES_ENABLE_SQL_LOG: %w", err)
		}
		cfg.EnableSQLLog = parsed
	}

	return nil
}

func lookupTrimmedEnv(name string) (string, bool) {
	value, ok := os.LookupEnv(name)
	if !ok {
		return "", false
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	return value, true
}

func cliUsage() string {
	return strings.TrimSpace(`
Usage:
  swaves --version
  swaves version
  swaves upgrade
  swaves upgrade --check
  swaves hash-password <raw-password>
  swaves set-admin-password <sqlite-file> <raw-password>
  swaves <sqlite-file> [--backup-dir=<dir>] [--listen-addr=<addr>] [--app-name=<name>] [--enable-sql-log=<bool>] [--daemon-mode=<0|1>] [--max-failures=<n>]

Environment:
  SWAVES_SQLITE_FILE
  SWAVES_BACKUP_DIR
  SWAVES_LISTEN_ADDR
  SWAVES_APP_NAME
  SWAVES_ENABLE_SQL_LOG
  SWAVES_DAEMON_MODE
  SWAVES_ENSURE_DEFAULT_SETTINGS

Priority:
  command line > environment variables > defaults

Notes:
  set-admin-password updates settings.dash_password in the sqlite file and prints the stored bcrypt hash.
  SWAVES_ENSURE_DEFAULT_SETTINGS=true only enables EnsureDefaultSettings when SWAVES_ENV=dev.

Examples:
  ./swaves --version
  ./swaves version
  ./swaves upgrade
  ./swaves upgrade --check
  ./swaves hash-password admin
  ./swaves set-admin-password data.sqlite admin
  ./swaves data.sqlite --listen-addr=:3000
  SWAVES_SQLITE_FILE=data.sqlite SWAVES_LISTEN_ADDR=:3000 ./swaves
`) + "\n"
}

func versionUsage() string {
	return strings.TrimSpace(`
Usage:
  swaves --version
  swaves version
`) + "\n"
}

func upgradeUsage() string {
	return strings.TrimSpace(`
Usage:
  swaves upgrade
  swaves upgrade --check

	Notes:
	  upgrade --check only checks the latest stable GitHub release for the current platform.
	  upgrade downloads the latest stable GitHub release for the current platform and replaces the current executable.
	`) + "\n"
}

func hashPasswordUsage() string {
	return strings.TrimSpace(`
Usage:
  swaves hash-password <raw-password>

Example:
  ./swaves hash-password admin
`) + "\n"
}

func setAdminPasswordUsage() string {
	return strings.TrimSpace(`
Usage:
  swaves set-admin-password <sqlite-file> <raw-password>

Notes:
  Updates settings.dash_password in the sqlite file.
  Prints the stored bcrypt hash after update.

Example:
  ./swaves set-admin-password data.sqlite admin
`) + "\n"
}
