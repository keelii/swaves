package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"swaves/internal/app"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/supervisor"
	"swaves/internal/shared/types"

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
		DaemonMode:  *flagDemonMode == 1,
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

	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		return err
	}
	return nil
}
