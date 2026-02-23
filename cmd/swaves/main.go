package main

import (
	"swaves/internal/app"
	"swaves/internal/platform/config"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

func main() {
	swv := app.NewApp(types.AppConfig{
		SqliteFile:   "data.sqlite",
		BackupDir:    "backups",
		ListenAddr:   ":3000",
		AppName:      "swaves",
		EnableSQLLog: config.EnableSQLLog,
	})
	swv.Listen(fiber.ListenConfig{
		DisableStartupMessage: true,
	})

	defer swv.Shutdown()
}
