package main

import (
	"os"
	"os/signal"
	"swaves/internal/app"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/types"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
)

func runSwavesWorker(appCfg types.AppConfig) error {
	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(shutdownCh)
	go func() {
		<-shutdownCh
		if err := swv.App.ShutdownWithTimeout(8 * time.Second); err != nil {
			logger.Error("graceful shutdown failed: %v", err)
		}
	}()

	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		return err
	}
	return nil
}
