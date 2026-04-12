package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
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

	installWorkerReadyHook(swv.App)
	installAppShutdownHook(swv.App)

	listener, err := inheritedListenerFromEnv()
	if err != nil {
		return err
	}
	listenCfg := fiber.ListenConfig{DisableStartupMessage: true}
	if listener != nil {
		logger.Info("%s serving inherited listener on %s", swv.Config.AppName, swv.Config.ListenAddr)
		return swv.Serve(listener, listenCfg)
	}

	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, listenCfg); err != nil {
		return err
	}
	return nil
}

func runSwavesApp(appCfg types.AppConfig) error {
	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	installAppShutdownHook(swv.App)
	listenCfg := fiber.ListenConfig{DisableStartupMessage: true}
	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, listenCfg); err != nil {
		return err
	}
	return nil
}

func installAppShutdownHook(appInstance *fiber.App) {
	if appInstance == nil {
		return
	}

	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-shutdownCh
		signal.Stop(shutdownCh)
		logger.Info("[app] shutdown requested by signal: %s", sig)
		if err := appInstance.ShutdownWithTimeout(8 * time.Second); err != nil {
			logger.Error("graceful shutdown failed: %v", err)
			return
		}
		logger.Info("[app] shutdown completed by signal: %s", sig)
	}()
}

func installWorkerReadyHook(appInstance *fiber.App) {
	if appInstance == nil {
		return
	}
	appInstance.Hooks().OnListen(func(_ fiber.ListenData) error {
		return signalWorkerReady()
	})
}

func inheritedListenerFromEnv() (net.Listener, error) {
	fd, ok, err := envFD(workerListenerFDEnv)
	if err != nil || !ok {
		return nil, err
	}

	file := os.NewFile(uintptr(fd), "swaves-listener")
	if file == nil {
		return nil, fmt.Errorf("restore listener file failed")
	}
	defer func() { _ = file.Close() }()

	listener, err := net.FileListener(file)
	if err != nil {
		return nil, fmt.Errorf("restore listener failed: %w", err)
	}
	return listener, nil
}

func signalWorkerReady() error {
	fd, ok, err := envFD(workerReadyFDEnv)
	if err != nil || !ok {
		return err
	}

	file := os.NewFile(uintptr(fd), "swaves-ready")
	if file == nil {
		return fmt.Errorf("restore ready pipe failed")
	}
	defer func() { _ = file.Close() }()

	if _, err := file.WriteString(workerReadyMessage + "\n"); err != nil {
		return fmt.Errorf("signal worker ready failed: %w", err)
	}
	return nil
}

func envFD(name string) (int, bool, error) {
	raw, ok := os.LookupEnv(name)
	if !ok || raw == "" {
		return 0, false, nil
	}
	fd, err := strconv.Atoi(raw)
	if err != nil || fd < 0 {
		return 0, false, fmt.Errorf("invalid %s: %q", name, raw)
	}
	return fd, true, nil
}
