package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"swaves/internal/app"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/supervisor"
	"swaves/internal/shared/types"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
)

func main() {
	args := os.Args[1:]

	handled, exitCode := runUtilityCommand(args, os.Stdout, os.Stderr)
	if handled {
		if exitCode != 0 {
			os.Exit(exitCode)
		}
		return
	}

	appCfg, err := parseMainAppConfig(args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, _ = fmt.Fprint(os.Stdout, cliUsage())
			return
		}
		_, _ = fmt.Fprintf(os.Stderr, "%v\n\n%s", err, cliUsage())
		os.Exit(2)
	}

	if err := supervisor.Run(supervisor.Config{
		DaemonMode:  *flagDaemonMode == 1,
		MaxFailures: *flagMaxFailures,
		MasterTitle: "swaves: master process",
		WorkerTitle: "swaves: worker process",
		Args:        args,
		Worker: func() error {
			return runSwavesWorker(appCfg)
		},
	}); err != nil {
		logger.Fatal("%v", err)
	}
}

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
