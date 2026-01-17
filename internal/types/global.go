package types

import (
	"log"
	"swaves/internal/consts"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/session"
)

type SessionStore struct {
	*session.Store
}

func (s *SessionStore) SaveSession(c *fiber.Ctx) bool {
	var err error
	var sess *session.Session

	sess, err = s.Store.Get(c)
	if err != nil {
		log.Println("Error getting session:", err)
		return false
	}

	sess.Set(consts.LoginAdminName, true)
	sess.SetExpiry(consts.LoginSessionExpire)

	if err = sess.Save(); err != nil {
		log.Println("Session save error:", err)
		return false
	}
	log.Println("Session saved.")
	return true
}

func (s *SessionStore) ClearSession(c *fiber.Ctx) bool {
	var err error
	var sess *session.Session
	sess, err = s.Store.Get(c)
	if err != nil {
		log.Println("Error getting session:", err)
		return false
	}
	if err = sess.Destroy(); err != nil {
		log.Println("Error destroying session:", err)
		return false
	}
	log.Println("Session cleared")
	return true
}

func (s *SessionStore) IsLogin(c *fiber.Ctx) bool {
	sess, err := s.Store.Get(c)
	if err != nil {
		log.Println("Error getting session:", err)
		return false
	}
	isLogin := sess.Get(consts.LoginAdminName)
	log.Println("isLogin:", isLogin)
	return isLogin == true
}
