package types

import (
	"log"
	"strings"
	"swaves/internal/consts"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

type SessionStore struct {
	*session.Store
}

const sessionCookieName = "session_id"

func isSessionDecodeErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "failed to decode session data")
}

func (s *SessionStore) AcquireSession(c fiber.Ctx) (*session.Session, error) {
	sess, err := s.Store.Get(c)
	if err == nil {
		return sess, nil
	}

	// Fiber v3 sessions use map[any]any, while legacy Fiber v2 records were map[string]any.
	// If decoding old session bytes fails, drop the stale record and create a fresh session.
	if !isSessionDecodeErr(err) {
		return nil, err
	}

	sessionID := strings.TrimSpace(c.Cookies(sessionCookieName))
	if sessionID != "" {
		if delErr := s.Store.Delete(c.Context(), sessionID); delErr != nil {
			log.Println("Error deleting legacy session:", delErr)
		}
	}

	return s.Store.Get(c)
}

func (s *SessionStore) SaveSession(c fiber.Ctx) bool {
	var err error
	var sess *session.Session

	sess, err = s.AcquireSession(c)
	if err != nil {
		log.Println("Error getting session:", err)
		return false
	}
	defer sess.Release()

	if err = sess.Regenerate(); err != nil {
		log.Println("Session regenerate error:", err)
		return false
	}

	sess.Set(consts.LoginAdminName, true)
	sess.SetIdleTimeout(consts.LoginSessionExpire)

	if err = sess.Save(); err != nil {
		log.Println("Session save error:", err)
		return false
	}
	log.Println("Session saved.")
	return true
}

func (s *SessionStore) ClearSession(c fiber.Ctx) bool {
	var err error
	var sess *session.Session
	sess, err = s.AcquireSession(c)
	if err != nil {
		log.Println("Error getting session:", err)
		return false
	}
	defer sess.Release()
	if err = sess.Destroy(); err != nil {
		log.Println("Error destroying session:", err)
		return false
	}
	log.Println("Session cleared")
	return true
}

func (s *SessionStore) IsLogin(c fiber.Ctx) bool {
	sess, err := s.AcquireSession(c)
	if err != nil {
		log.Println("Error getting session:", err)
		return false
	}
	defer sess.Release()
	isLogin := sess.Get(consts.LoginAdminName)
	//log.Println("isLogin:", isLogin)
	return isLogin == true
}

type AppConfig struct {
	SqliteFile   string
	BackupDir    string
	ListenAddr   string
	AppName      string
	EnableSQLLog bool
}
