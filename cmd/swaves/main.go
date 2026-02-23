package main

import (
	"os"
	"os/exec"
	"swaves/internal/app"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/types"
	"time"

	"github.com/gofiber/fiber/v3"
)

func watchParent() {
	ppid := os.Getppid()

	go func() {
		for {
			time.Sleep(time.Second)
			pid := os.Getppid()
			if pid != ppid {
				logger.Warn("parent process exited, exiting worker: pid=%d old_ppid=%d", pid, ppid)
				os.Exit(1)
			}
		}
	}()
}
func worker() {
	watchParent()

	swv := app.NewApp(types.AppConfig{
		SqliteFile:   "data.sqlite",
		ListenAddr:   ":3000",
		AppName:      "swaves",
		EnableSQLLog: true,
	})
	swv.Listen(fiber.ListenConfig{
		DisableStartupMessage: true,
	})

	defer swv.Shutdown()
}

func launcher() {
	for {
		cmd := exec.Command(os.Args[0])
		cmd.Env = append(os.Environ(), "WORKER=1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		logger.Info("[launcher] start worker")
		err := cmd.Start()
		if err != nil {
			logger.Error("[launcher] failed to start worker: %v", err)
			time.Sleep(time.Second)
			continue
		}

		// 阻塞等待 worker 退出
		err = cmd.Wait()
		if err != nil {
			logger.Warn("[launcher] worker exited: %v", err)
		} else {
			logger.Info("[launcher] worker exited")
		}

		time.Sleep(200 * time.Millisecond)
	}
}

func main() {
	swv := app.NewApp(types.AppConfig{
		SqliteFile:   "data.sqlite",
		BackupDir:    "backups",
		ListenAddr:   ":3000",
		AppName:      "swaves",
		EnableSQLLog: false,
	})
	swv.Listen(fiber.ListenConfig{
		DisableStartupMessage: true,
	})

	defer swv.Shutdown()
	//
	//if os.Getenv("WORKER") == "1" {
	//	worker()
	//} else {
	//	launcher()
	//}
}
