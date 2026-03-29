package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/shared/types"

	"golang.org/x/crypto/bcrypt"
)

var (
	flagAdminPassword = flag.String("admin-password", "", "dash admin password bcrypt hash")
	flagBackupDir     = flag.String("backup-dir", "backups", "backup directory")
	flagListenAddr    = flag.String("listen-addr", ":3000", "listen address")
	flagAppName       = flag.String("app-name", "swaves", "app name")
	flagEnableSQLLog  = flag.Bool("enable-sql-log", config.EnableSQLLog, "enable sql log")
	flagDemonMode     = flag.Int("demon-mode", 1, "1: run with master process, otherwise run worker directly")
	flagMaxFailures   = flag.Int("max-failures", 5, "max consecutive worker failures before master exits (<=0 means unlimited)")
)

func runUtilityCommand(args []string, stdout io.Writer, stderr io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}

	switch strings.TrimSpace(args[0]) {
	case "-h", "--help", "help":
		_, _ = fmt.Fprint(stdout, cliUsage())
		return true, 0
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
			"updated settings.dash_password in %s\nnote: app startup will sync settings.dash_password from --admin-password / SWAVES_ADMIN_PASSWORD\n",
			args[1],
		)
		return true, 0
	default:
		return false, 0
	}
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
	cfg := types.AppConfig{
		BackupDir:    "backups",
		ListenAddr:   ":3000",
		AppName:      "swaves",
		EnableSQLLog: config.EnableSQLLog,
	}

	if err := applyEnvAppConfig(&cfg); err != nil {
		return cfg, err
	}

	if len(args) > 0 {
		firstArg := strings.TrimSpace(args[0])
		if firstArg == "-h" || firstArg == "--help" || firstArg == "help" {
			return cfg, flag.ErrHelp
		}
	}

	flagArgs := args
	if len(args) > 0 {
		firstArg := strings.TrimSpace(args[0])
		if firstArg != "" && !strings.HasPrefix(firstArg, "-") {
			cfg.SqliteFile = firstArg
			flagArgs = args[1:]
		}
	}

	fs := flag.NewFlagSet("swaves", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.AdminPassword, "admin-password", cfg.AdminPassword, "dash admin password bcrypt hash")
	fs.StringVar(&cfg.BackupDir, "backup-dir", cfg.BackupDir, "backup directory")
	fs.StringVar(&cfg.ListenAddr, "listen-addr", cfg.ListenAddr, "listen address")
	fs.StringVar(&cfg.AppName, "app-name", cfg.AppName, "app name")
	fs.BoolVar(&cfg.EnableSQLLog, "enable-sql-log", cfg.EnableSQLLog, "enable sql log")
	var ignoredDemonMode int
	var ignoredMaxFailures int
	fs.IntVar(&ignoredDemonMode, "demon-mode", 1, "1: run with master process, otherwise run worker directly")
	fs.IntVar(&ignoredMaxFailures, "max-failures", 5, "max consecutive worker failures before master exits (<=0 means unlimited)")

	if err := fs.Parse(flagArgs); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected extra arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(cfg.SqliteFile) == "" {
		return cfg, errors.New("sqlite file is required")
	}

	return cfg, nil
}

func parseMainAppConfig(args []string) (types.AppConfig, error) {
	cfg := types.AppConfig{
		BackupDir:    "backups",
		ListenAddr:   ":3000",
		AppName:      "swaves",
		EnableSQLLog: config.EnableSQLLog,
	}

	if err := applyEnvAppConfig(&cfg); err != nil {
		return cfg, err
	}

	*flagAdminPassword = cfg.AdminPassword
	*flagBackupDir = cfg.BackupDir
	*flagListenAddr = cfg.ListenAddr
	*flagAppName = cfg.AppName
	*flagEnableSQLLog = cfg.EnableSQLLog

	flagArgs := args
	if len(args) > 0 {
		firstArg := strings.TrimSpace(args[0])
		if firstArg != "" && !strings.HasPrefix(firstArg, "-") {
			cfg.SqliteFile = firstArg
			flagArgs = args[1:]
		}
	}

	flag.CommandLine.SetOutput(io.Discard)
	if err := flag.CommandLine.Parse(flagArgs); err != nil {
		return cfg, err
	}
	if flag.CommandLine.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected extra arguments: %s", strings.Join(flag.CommandLine.Args(), " "))
	}

	cfg.AdminPassword = strings.TrimSpace(*flagAdminPassword)
	cfg.BackupDir = strings.TrimSpace(*flagBackupDir)
	cfg.ListenAddr = strings.TrimSpace(*flagListenAddr)
	cfg.AppName = strings.TrimSpace(*flagAppName)
	cfg.EnableSQLLog = *flagEnableSQLLog

	if strings.TrimSpace(cfg.SqliteFile) == "" {
		return cfg, errors.New("sqlite file is required")
	}

	return cfg, nil
}

func applyEnvAppConfig(cfg *types.AppConfig) error {
	if cfg == nil {
		return nil
	}

	if value, ok := lookupTrimmedEnv("SWAVES_SQLITE_FILE"); ok {
		cfg.SqliteFile = value
	}
	if value, ok := lookupTrimmedEnv("SWAVES_ADMIN_PASSWORD"); ok {
		cfg.AdminPassword = value
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
  swaves hash-password <raw-password>
  swaves set-admin-password <sqlite-file> <raw-password>
  swaves <sqlite-file> --admin-password=<bcrypt-hash> [--backup-dir=<dir>] [--listen-addr=<addr>] [--app-name=<name>] [--enable-sql-log=<bool>] [--demon-mode=<0|1>] [--max-failures=<n>]

Environment:
  SWAVES_SQLITE_FILE
  SWAVES_ADMIN_PASSWORD
  SWAVES_BACKUP_DIR
  SWAVES_LISTEN_ADDR
  SWAVES_APP_NAME
  SWAVES_ENABLE_SQL_LOG
  SWAVES_ENSURE_DEFAULT_SETTINGS

Priority:
  command line > environment variables > defaults

Notes:
  set-admin-password updates settings.dash_password in the sqlite file and prints the stored bcrypt hash.
  App startup syncs settings.dash_password from --admin-password / SWAVES_ADMIN_PASSWORD.
  SWAVES_ENSURE_DEFAULT_SETTINGS=true only enables EnsureDefaultSettings when SWAVES_ENV=dev.

Examples:
  ./swaves hash-password admin
  ./swaves set-admin-password data.sqlite admin
  ./swaves data.sqlite --admin-password='$2a$10$exampleexampleexampleexampleexampleexampleexampleexample'
  SWAVES_SQLITE_FILE=data.sqlite SWAVES_ADMIN_PASSWORD='$2a$10$exampleexampleexampleexampleexampleexampleexampleexample' ./swaves
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
  Prints the stored bcrypt hash so you can reuse it for --admin-password or SWAVES_ADMIN_PASSWORD.
  App startup syncs settings.dash_password from --admin-password / SWAVES_ADMIN_PASSWORD.

Example:
  ./swaves set-admin-password data.sqlite admin
`) + "\n"
}
