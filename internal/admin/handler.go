package admin

import (
	"swaves/internal/tpl"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	Auth  *Service
	Store *SessionStore
}

func NewHandler(auth *Service, store *SessionStore) *Handler {
	return &Handler{
		Auth:  auth,
		Store: store,
	}
}

/* ---------- GET /admin/login ---------- */

func (h *Handler) GetLoginHandler(c *fiber.Ctx) error {
	return tpl.RenderTemplate(c, "admin_login", map[string]string{
		"Title": "Admin Login",
		"Error": "",
	})
}

/* ---------- POST /admin/login ---------- */

func (h *Handler) PostLoginHandler(c *fiber.Ctx) error {
	password := c.FormValue("password")
	if password == "" {
		return tpl.RenderTemplate(c, "admin_login", map[string]string{
			"Error": "password is empty",
		})
	}

	if err := h.Auth.CheckPassword(password); err != nil {
		return tpl.RenderTemplate(c, "admin_login", map[string]string{
			"Title": "Admin Login",
			"Error": "Invalid password",
		})
	}

	sess, err := h.Store.Get(c)
	if err != nil {
		return err
	}

	sess.Set("admin", true)

	if err := sess.Save(); err != nil {
		return err
	}

	return c.Redirect("/admin")
}

/* ---------- POST /admin/logout ---------- */

func (h *Handler) GetLogoutHandler(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err == nil {
		sess.Destroy()
	}

	return c.Redirect("/admin/login")
}

func (h *Handler) GetHome(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err != nil {
		return err
	}
	logined := sess.Get("admin")
	return tpl.RenderTemplate(c, "admin_home", map[string]interface{}{
		"Title":   "Admin Home",
		"IsLogin": logined,
	})
}
