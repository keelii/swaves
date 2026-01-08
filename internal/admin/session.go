package admin

import (
	"swaves/internal/db"
	"time"

	"github.com/gofiber/fiber/v2/middleware/session"
	"github.com/gofiber/storage/sqlite3/v2"
)

type SessionStore struct {
	*session.Store
}

func NewSessionStore(dbInstance *db.DB) *SessionStore {
	storage := sqlite3.New(sqlite3.Config{
		Database:   "./data.sqlite",
		Table:      "session_storage",
		Reset:      false,
		GCInterval: 1 * time.Minute, // 每10分钟清理一次过期 session
	})
	store := session.New(session.Config{
		Storage:        storage,
		Expiration:     24 * time.Hour, // Session 有效期
		CookieHTTPOnly: true,
	})

	return &SessionStore{store}
}
