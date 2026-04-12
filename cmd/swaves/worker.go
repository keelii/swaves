package main

import (
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"swaves/internal/app"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/types"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
)

func runSwavesWorker(appCfg types.AppConfig) error {
	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	installWorkerReadyHook(swv.App)
	installAppShutdownHook(swv.App, swv.Tracker, swv.PauseJobs)

	pid := os.Getpid()
	listener, err := inheritedListenerFromEnv()
	if err != nil {
		return err
	}
	listenCfg := fiber.ListenConfig{DisableStartupMessage: true}
	if listener != nil {
		startAt := time.Now()
		logger.Info("%s serving inherited listener on %s", swv.Config.AppName, swv.Config.ListenAddr)
		logger.Info("[worker] serve start: pid=%d inherited_listener=true addr=%s", pid, swv.Config.ListenAddr)
		err = swv.Serve(listener, listenCfg)
		if err != nil {
			logger.Error("[worker] serve returned error: pid=%d inherited_listener=true elapsed=%s err=%v", pid, time.Since(startAt), err)
			return err
		}
		logger.Info("[worker] serve returned: pid=%d inherited_listener=true elapsed=%s", pid, time.Since(startAt))
		return nil
	}

	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	startAt := time.Now()
	logger.Info("[worker] listen start: pid=%d inherited_listener=false addr=%s", pid, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, listenCfg); err != nil {
		logger.Error("[worker] listen returned error: pid=%d inherited_listener=false elapsed=%s err=%v", pid, time.Since(startAt), err)
		return err
	}
	logger.Info("[worker] listen returned: pid=%d inherited_listener=false elapsed=%s", pid, time.Since(startAt))
	return nil
}

func runSwavesApp(appCfg types.AppConfig) error {
	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	installAppShutdownHook(swv.App, swv.Tracker, swv.PauseJobs)
	listenCfg := fiber.ListenConfig{DisableStartupMessage: true}
	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, listenCfg); err != nil {
		return err
	}
	return nil
}

func installAppShutdownHook(appInstance *fiber.App, tracker *middleware.RequestTracker, pauseJobs func()) {
	if appInstance == nil {
		return
	}

	pid := os.Getpid()
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-shutdownCh
		signal.Stop(shutdownCh)
		startAt := time.Now()
		activeCount := tracker.ActiveCount()
		logger.Info("[app] shutdown requested by signal: pid=%d signal=%s timeout=%s active_requests=%d active_details=%s", pid, sig, workerGracefulShutdownTimeout, activeCount, middleware.FormatActiveRequests(tracker.Snapshot(5), startAt))
		if pauseJobs != nil {
			pauseJobs()
		}

		done := make(chan struct{})
		go logShutdownWaitState(pid, tracker, startAt, done)

		if err := appInstance.ShutdownWithTimeout(workerGracefulShutdownTimeout); err != nil {
			close(done)
			logger.Error("[app] graceful shutdown failed: pid=%d signal=%s elapsed=%s err=%v", pid, sig, time.Since(startAt), err)
			return
		}
		close(done)
		logger.Info("[app] shutdown completed by signal: pid=%d signal=%s elapsed=%s", pid, sig, time.Since(startAt))
	}()
}

func logShutdownWaitState(pid int, tracker *middleware.RequestTracker, startAt time.Time, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			activeCount := tracker.ActiveCount()
			if activeCount == 0 {
				logger.Info("[app] shutdown waiting: pid=%d elapsed=%s active_requests=0", pid, now.Sub(startAt).Round(time.Millisecond))
				continue
			}
			logger.Warn("[app] shutdown waiting: pid=%d elapsed=%s active_requests=%d active_details=%s", pid, now.Sub(startAt).Round(time.Millisecond), activeCount, middleware.FormatActiveRequests(tracker.Snapshot(5), now))
		}
	}
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

	pid := os.Getpid()
	file := os.NewFile(uintptr(fd), "swaves-ready")
	if file == nil {
		return fmt.Errorf("restore ready pipe failed")
	}
	defer func() { _ = file.Close() }()

	logger.Info("[worker] signaling ready: pid=%d fd=%d", pid, fd)
	if _, err := file.WriteString(workerReadyMessage + "\n"); err != nil {
		return fmt.Errorf("signal worker ready failed: %w", err)
	}
	logger.Info("[worker] ready signaled: pid=%d", pid)
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
