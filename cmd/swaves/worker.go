package main

import (
	"os"
	"os/signal"
	"sync"
	"swaves/internal/app"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/supervisor"
	"swaves/internal/shared/types"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
)

func runSwavesWorker(appCfg types.AppConfig) error {
	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	var shutdownOnce sync.Once
	installWorkerReadyHook(swv.App)
	installWorkerDrainHook(swv.App, swv.PauseJobs, &shutdownOnce)
	installAppShutdownHook(swv.App, swv.Tracker, swv.PauseJobs, &shutdownOnce)

	pid := os.Getpid()
	listener, err := supervisor.InheritedListener()
	if err != nil {
		return err
	}
	listenCfg := fiber.ListenConfig{DisableStartupMessage: true}
	if listener != nil {
		logger.Info("%s serving inherited listener on %s", swv.Config.AppName, swv.Config.ListenAddr)
		err = swv.Serve(listener, listenCfg)
		if err != nil {
			logger.Error("[worker] serve returned error: pid=%d inherited_listener=true err=%v", pid, err)
			return err
		}
		return nil
	}

	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, listenCfg); err != nil {
		logger.Error("[worker] listen returned error: pid=%d inherited_listener=false err=%v", pid, err)
		return err
	}
	return nil
}

func runSwavesApp(appCfg types.AppConfig) error {
	swv := app.NewApp(appCfg)
	defer swv.Shutdown()

	var shutdownOnce sync.Once
	installAppShutdownHook(swv.App, swv.Tracker, swv.PauseJobs, &shutdownOnce)
	listenCfg := fiber.ListenConfig{DisableStartupMessage: true}
	logger.Info("%s listening on %s", swv.Config.AppName, swv.Config.ListenAddr)
	if err := swv.App.Listen(swv.Config.ListenAddr, listenCfg); err != nil {
		return err
	}
	return nil
}

func installWorkerDrainHook(appInstance *fiber.App, pauseJobs func(), shutdownOnce *sync.Once) {
	if appInstance == nil {
		return
	}
	pid := os.Getpid()
	drainCh := make(chan os.Signal, 1)
	signal.Notify(drainCh, syscall.SIGUSR1)
	go func() {
		<-drainCh
		signal.Stop(drainCh)
		logger.Info("[app] drain requested by signal: pid=%d timeout=%s", pid, workerGracefulShutdownTimeout)
		if pauseJobs != nil {
			pauseJobs()
		}
		shutdownOnce.Do(func() {
			if err := appInstance.ShutdownWithTimeout(workerGracefulShutdownTimeout); err != nil {
				logger.Warn("[app] drain shutdown failed: pid=%d err=%v", pid, err)
			}
			logger.Info("[app] drain shutdown complete: pid=%d", pid)
		})
	}()
}

func installAppShutdownHook(appInstance *fiber.App, tracker *middleware.RequestTracker, pauseJobs func(), shutdownOnce *sync.Once) {
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

		triggered := false
		shutdownOnce.Do(func() {
			triggered = true
			if err := appInstance.ShutdownWithTimeout(workerGracefulShutdownTimeout); err != nil {
				close(done)
				activeCount = tracker.ActiveCount()
				logger.Error("[app] graceful shutdown failed: pid=%d signal=%s elapsed=%s active_requests=%d active_details=%s err=%v", pid, sig, time.Since(startAt), activeCount, middleware.FormatActiveRequests(tracker.Snapshot(5), time.Now()), err)
				return
			}
			close(done)
			logger.Info("[app] shutdown completed by signal: pid=%d signal=%s elapsed=%s", pid, sig, time.Since(startAt))
		})
		if !triggered {
			// Drain (SIGUSR1) already initiated shutdown; stop the wait-state log.
			close(done)
		}
	}()
}

func logShutdownWaitState(pid int, tracker *middleware.RequestTracker, startAt time.Time, done <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	idleWarned := false
	lastActiveWarnAt := time.Time{}
	for {
		select {
		case <-done:
			return
		case now := <-ticker.C:
			activeCount := tracker.ActiveCount()
			if activeCount == 0 {
				if !idleWarned {
					logger.Warn("[app] shutdown waiting without active requests: pid=%d elapsed=%s hint=likely idle keep-alive connections or abnormal client/proxy timeout settings; check reverse proxy keepalive/read timeout config", pid, now.Sub(startAt).Round(time.Millisecond))
					idleWarned = true
				}
				continue
			}
			if now.Sub(lastActiveWarnAt) < 2*time.Second {
				continue
			}
			lastActiveWarnAt = now
			logger.Warn("[app] shutdown waiting: pid=%d elapsed=%s active_requests=%d active_details=%s", pid, now.Sub(startAt).Round(time.Millisecond), activeCount, middleware.FormatActiveRequests(tracker.Snapshot(5), now))
		}
	}
}

func installWorkerReadyHook(appInstance *fiber.App) {
	if appInstance == nil {
		return
	}
	appInstance.Hooks().OnListen(func(_ fiber.ListenData) error {
		return supervisor.SignalReady()
	})
}


