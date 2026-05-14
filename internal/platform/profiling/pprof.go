package profiling

import (
	"net/http"
	_ "net/http/pprof"
	"swaves/internal/platform/logger"
)

// StartPprofServer starts a net/http pprof server on addr in a background goroutine.
// It is a no-op when addr is empty.
// Typical use: set SWAVES_PPROF_ADDR=127.0.0.1:6060 before starting swaves, then
// collect a CPU profile with:
//
//	curl -s "http://127.0.0.1:6060/debug/pprof/profile?seconds=30" -o cpu.pprof
func StartPprofServer(addr string) {
	if addr == "" {
		return
	}
	go func() {
		logger.Info("[pprof] listening on %s", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			logger.Error("[pprof] server stopped: %v", err)
		}
	}()
}
