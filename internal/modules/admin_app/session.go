package admin_app

import (
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/shared/types"
	"time"

	"github.com/gofiber/fiber/v3/middleware/session"
	"github.com/gofiber/storage/sqlite3/v2"
)

func NewSessionStore() *types.SessionStore {
	storage := sqlite3.New(sqlite3.Config{
		Database:   "./data.sqlite",
		Table:      string(db.TableSessions),
		Reset:      false,
		GCInterval: 1 * time.Minute, // 每1分钟清理一次过期 session
	})
	store := session.NewStore(session.Config{
		Storage:        storage,
		IdleTimeout:    24 * time.Hour, // Session 有效期
		CookieHTTPOnly: true,
		CookieSecure:   config.SessionCookieSecure,
		CookieSameSite: config.SessionCookieSameSite,
	})

	return &types.SessionStore{Store: store}
}
