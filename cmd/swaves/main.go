package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"swaves/internal/app"

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

	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	swv.Listen(fiber.ListenConfig{
		DisableStartupMessage: true,
	})
}
