package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"swaves/internal/app"
	"swaves/internal/platform/config"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	exitCode := run(os.Args[1:], os.Stdout, os.Stderr)
	if exitCode != 0 {
		os.Exit(exitCode)
	}
}

func run(args []string, stdout io.Writer, stderr io.Writer) int {
	handled, exitCode := runUtilityCommand(args, stdout, stderr)
	if handled {
		return exitCode
	}

	appCfg, err := parseAppConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(stdout, cliUsage())
			return 0
		}
		_, _ = fmt.Fprintf(stderr, "%v\n\n%s", err, cliUsage())
		return 2
	}

	swv := app.NewApp(appCfg)
	swv.Listen(fiber.ListenConfig{
		DisableStartupMessage: true,
	})
	defer swv.Shutdown()

	return 0
}

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

	if err := fs.Parse(flagArgs); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected extra arguments: %s", strings.Join(fs.Args(), " "))
	}
	if strings.TrimSpace(cfg.SqliteFile) == "" {
		return cfg, errors.New("sqlite file is required")
	}
	if strings.TrimSpace(cfg.AdminPassword) == "" {
		return cfg, errors.New("admin password is required")
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
  swaves <sqlite-file> --admin-password=<bcrypt-hash> [--backup-dir=<dir>] [--listen-addr=<addr>] [--app-name=<name>] [--enable-sql-log=<bool>]

Environment:
  SWAVES_SQLITE_FILE
  SWAVES_ADMIN_PASSWORD
  SWAVES_BACKUP_DIR
  SWAVES_LISTEN_ADDR
  SWAVES_APP_NAME
  SWAVES_ENABLE_SQL_LOG

Priority:
  command line > environment variables > defaults

Examples:
  ./swaves hash-password admin
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
