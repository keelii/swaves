package sui

import "github.com/gofiber/fiber/v3"

/* ---------- GET /admin/login ---------- */

func (h *Handler) GetLoginHandler(c fiber.Ctx) error {
	return RenderSUIView(c, "sui/admin_login.html", fiber.Map{
		"Title":     "Admin Login",
		"ReturnUrl": c.Query("returnUrl"),
	}, "base")
}

/* ---------- POST /admin/login ---------- */

func (h *Handler) PostLoginHandler(c fiber.Ctx) error {
	returnUrl := c.FormValue("returnUrl")
	password := c.FormValue("password")
	if password == "" {
		return RenderSUIView(c, "sui/admin_login.html", fiber.Map{
			"Title":     "Admin Login",
			"Error":     "password is empty",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	if err := h.Service.CheckPassword(password); err != nil {
		return RenderSUIView(c, "sui/admin_login.html", fiber.Map{
			"Title":     "Admin Login",
			"Error":     "Invalid password",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	succ := h.Session.SaveSession(c)

	if succ {
		return h.redirectAfterLogin(c)
	}

	return RenderSUIView(c, "sui/admin_login.html", fiber.Map{
		"Title":     "Admin Login",
		"Error":     "Invalid Error",
		"ReturnUrl": returnUrl,
	}, "base")
}
