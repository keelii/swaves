package dash

import "github.com/gofiber/fiber/v3"

/* ---------- GET /dash/login ---------- */

func (h *Handler) GetLoginHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/dash_login.html", fiber.Map{
		"Title":     "Dash Login",
		"ReturnUrl": c.Query("returnUrl"),
	}, "base")
}

/* ---------- POST /dash/login ---------- */

func (h *Handler) PostLoginHandler(c fiber.Ctx) error {
	returnUrl := c.FormValue("returnUrl")
	password := c.FormValue("password")
	if password == "" {
		return RenderDashView(c, "dash/dash_login.html", fiber.Map{
			"Title":     "Dash Login",
			"Error":     "password is empty",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	if err := h.Service.CheckPassword(password); err != nil {
		return RenderDashView(c, "dash/dash_login.html", fiber.Map{
			"Title":     "Dash Login",
			"Error":     "Invalid password",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	succ := h.Session.SaveSession(c)

	if succ {
		return h.redirectAfterLogin(c)
	}

	return RenderDashView(c, "dash/dash_login.html", fiber.Map{
		"Title":     "Dash Login",
		"Error":     "Invalid Error",
		"ReturnUrl": returnUrl,
	}, "base")
}
