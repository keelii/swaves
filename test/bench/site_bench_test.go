package bench

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"swaves/internal/app"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

func prepareDB(b *testing.B, dbPath string) {
	b.Helper()

	model := db.Open(db.Options{DSN: dbPath})
	if err := db.EnsureDefaultSettings(model); err != nil {
		_ = model.Close()
		b.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	if err := model.Close(); err != nil {
		b.Fatalf("close prepared db failed: %v", err)
	}
}

func newBenchApp(b *testing.B) app.SwavesApp {
	b.Helper()

	var dbPath string
	if envDB := os.Getenv("SWAVES_BENCH_DB"); envDB != "" {
		absDB, err := filepath.Abs(envDB)
		if err != nil {
			b.Fatalf("SWAVES_BENCH_DB invalid path: %v", err)
		}
		if _, err := os.Stat(absDB); err != nil {
			b.Fatalf("SWAVES_BENCH_DB file not found: %v", err)
		}
		dbPath = absDB
	} else {
		dbPath = filepath.Join(b.TempDir(), "bench.sqlite")
		prepareDB(b, dbPath)
	}
	middleware.DashLoginRateLimitResetAll()
	return app.NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-bench",
	})
}

func newBenchRequest(method, path string) *http.Request {
	return httptest.NewRequest(method, path, nil)
}

func BenchmarkSiteHome(b *testing.B) {
	swv := newBenchApp(b)
	defer swv.Shutdown()

	homePath := share.GetBasePath()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := newBenchRequest(fiber.MethodGet, homePath)
		resp, err := swv.App.Test(req)
		if err != nil {
			b.Fatalf("home request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != fiber.StatusOK {
			b.Fatalf("unexpected home status: %d", resp.StatusCode)
		}
	}
}
