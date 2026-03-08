package dash

import (
	"fmt"
	"net/url"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/share"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

func (h *Handler) dashRouteURL(c fiber.Ctx, name string, params map[string]string, query map[string]string) string {
	routeParams := fiber.Map{}
	for key, value := range params {
		if strings.TrimSpace(key) == "" {
			continue
		}
		routeParams[key] = value
	}

	path, err := c.GetRouteURL(name, routeParams)
	if err != nil {
		logger.Warn("[dash] route resolve failed: name=%s params=%v err=%v", name, params, err)
		return ""
	}

	queryValues := url.Values{}
	for key, value := range query {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		queryValues.Set(k, value)
	}
	for key, raw := range routeParams {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if _, exists := queryValues[k]; exists {
			continue
		}
		queryValues.Set(k, strings.TrimSpace(fmt.Sprintf("%v", raw)))
	}
	encodedQuery := queryValues.Encode()
	if encodedQuery != "" {
		path += "?" + encodedQuery
	}
	return path
}

func (h *Handler) redirectToDashRoute(c fiber.Ctx, name string, params map[string]string, query map[string]string, status ...int) error {
	path := h.dashRouteURL(c, name, params, query)
	if path == "" {
		path = share.BuildDashPath("")
	}
	return webutil.RedirectTo(c, path, status...)
}

// redirectAfterLogin 从表单读取 returnUrl，校验后重定向，避免开放重定向
func (h *Handler) redirectAfterLogin(c fiber.Ctx) error {
	returnUrl := strings.TrimSpace(c.FormValue("returnUrl"))
	if returnUrl != "" && strings.HasPrefix(returnUrl, "/") && !strings.Contains(returnUrl, "//") {
		return webutil.RedirectTo(c, returnUrl)
	}
	return h.redirectToDashRoute(c, "dash.home", nil, nil)
}

/* ---------- POST /dash/logout ---------- */

func (h *Handler) GetLogoutHandler(c fiber.Ctx) error {
	h.Session.ClearSession(c)
	return h.redirectToDashRoute(c, "dash.login.show", nil, nil)
}
