package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/gofiber/fiber/v3"
)

const envWorkerMode = "SWAVES_MASTER_WORKER_MODE"

var flagListenAddr = flag.String("listen-addr", ":3900", "listen address")
var flagDemonMode = flag.Int("demon-mode", 1, "1: run with master process, otherwise run worker directly")
var flagMaxFailures = flag.Int("max-failures", 5, "max consecutive worker failures before master exits (<=0 means unlimited)")

func main() {
	flag.Parse()

	if os.Getenv(envWorkerMode) == "1" {
		if err := runWorker(*flagListenAddr); err != nil {
			fmt.Fprintf(os.Stderr, "[worker] %v\n", err)
			os.Exit(1)
		}
		return
	}

	if *flagDemonMode == 1 {
		if err := runMaster(); err != nil {
			fmt.Fprintf(os.Stderr, "[master] %v\n", err)
			os.Exit(1)
		}
		return
	}

	if err := runWorker(*flagListenAddr); err != nil {
		fmt.Fprintf(os.Stderr, "[worker] %v\n", err)
		os.Exit(1)
	}
}

type workerProc struct {
	cmd *exec.Cmd
}

func runMaster() error {
	consecutiveFailures := 0

	for {
		worker, err := startWorker()
		if err != nil {
			return err
		}
		err = worker.cmd.Wait()
		if err != nil {
			consecutiveFailures++
			fmt.Fprintf(os.Stderr, "[master] worker exited: %v\n", err)
			if *flagMaxFailures > 0 && consecutiveFailures >= *flagMaxFailures {
				return fmt.Errorf("worker failed %d times continuously, reached max-failures=%d", consecutiveFailures, *flagMaxFailures)
			}
		} else {
			consecutiveFailures = 0
			fmt.Fprintf(os.Stderr, "[master] worker exited\n")
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func startWorker() (*workerProc, error) {
	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("resolve executable failed: %w", err)
	}

	cmd := exec.Command(execPath, os.Args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), envWorkerMode+"=1")

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start worker failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "[master] worker started pid=%d\n", cmd.Process.Pid)
	return &workerProc{cmd: cmd}, nil
}

func runWorker(addr string) error {
	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	fmt.Fprintf(os.Stderr, "[worker] listening on %s pid=%d\n", addr, os.Getpid())
	if err := app.Listen(addr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		return fmt.Errorf("listen failed: %w", err)
	}
	return nil
}
