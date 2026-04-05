package dash

import (
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

const dashLoginRateLimitMessage = "今日登录访问次数已达上限，请明天再试"

/* ---------- GET /dash/login ---------- */

func (h *Handler) GetLoginHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/dash_login.html", fiber.Map{
		"Title":     "Dash Login",
		"ReturnUrl": c.Query("returnUrl"),
	}, "base")
}

func (h *Handler) RateLimitLoginHandler(c fiber.Ctx) error {
	returnURL := c.Query("returnUrl")
	if c.Method() == fiber.MethodPost {
		returnURL = c.FormValue("returnUrl")
	}

	c.Status(fiber.StatusTooManyRequests)
	return RenderDashView(c, "dash/dash_login.html", fiber.Map{
		"Title":     "Dash Login",
		"Error":     dashLoginRateLimitMessage,
		"ReturnUrl": returnURL,
	}, "base")
}

/* ---------- POST /dash/login ---------- */

func (h *Handler) PostLoginHandler(c fiber.Ctx) error {
	returnUrl := c.FormValue("returnUrl")
	password := c.FormValue("password")

	if middleware.DashLoginRateLimitExceeded(c) {
		middleware.DashLoginRateLimitLog(c)
		return h.RateLimitLoginHandler(c)
	}

	if password == "" {
		return RenderDashView(c, "dash/dash_login.html", fiber.Map{
			"Title":     "Dash Login",
			"Error":     "password is empty",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	if err := h.Service.CheckPassword(password); err != nil {
		if middleware.DashLoginRateLimitRecordFailure(c) {
			middleware.DashLoginRateLimitLog(c)
			return h.RateLimitLoginHandler(c)
		}
		return RenderDashView(c, "dash/dash_login.html", fiber.Map{
			"Title":     "Dash Login",
			"Error":     "Invalid password",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	middleware.DashLoginRateLimitReset(c)
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
