package dash

import (
	"fmt"
	"net/url"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

func buildRouteParams(params map[string]string) fiber.Map {
	routeParams := fiber.Map{}
	for key, value := range params {
		if strings.TrimSpace(key) == "" {
			continue
		}
		routeParams[key] = value
	}
	return routeParams
}

func (h *Handler) dashRoutePath(c fiber.Ctx, name string, params map[string]string) string {
	path, err := c.GetRouteURL(name, buildRouteParams(params))
	if err != nil {
		logger.Warn("[dash] route resolve failed: name=%s params=%v err=%v", name, params, err)
		return ""
	}
	return path
}

func normalizePathForCompare(path string) string {
	normalized := strings.TrimSpace(path)
	if normalized == "" {
		return ""
	}
	if normalized == "/" {
		return normalized
	}
	normalized = strings.TrimRight(normalized, "/")
	if normalized == "" {
		return "/"
	}
	return normalized
}

func mergeQuery(dst map[string]string, src map[string]string) map[string]string {
	if dst == nil {
		dst = map[string]string{}
	}
	for key, value := range src {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		dst[k] = value
	}
	return dst
}

func queryFromRefererForPath(c fiber.Ctx, targetPath string) map[string]string {
	referer := strings.TrimSpace(c.Get("Referer"))
	if referer == "" {
		return nil
	}
	parsed, err := url.Parse(referer)
	if err != nil {
		logger.Warn("[dash] parse referer failed: referer=%s err=%v", referer, err)
		return nil
	}
	if normalizePathForCompare(parsed.Path) != normalizePathForCompare(targetPath) {
		return nil
	}
	values, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		logger.Warn("[dash] parse referer query failed: referer=%s err=%v", referer, err)
		return nil
	}
	query := map[string]string{}
	for key, item := range values {
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		if len(item) == 0 {
			query[k] = ""
			continue
		}
		query[k] = item[len(item)-1]
	}
	return query
}

func (h *Handler) dashRouteURL(c fiber.Ctx, name string, params map[string]string, query map[string]string) string {
	routeParams := buildRouteParams(params)
	path := h.dashRoutePath(c, name, params)
	if path == "" {
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
		return fmt.Errorf("redirect route resolve failed: name=%s", name)
	}
	return webutil.RedirectTo(c, path, status...)
}

func (h *Handler) redirectToDashRouteKeepQuery(c fiber.Ctx, name string, params map[string]string, query map[string]string, status ...int) error {
	mergedQuery := map[string]string{}
	mergedQuery = mergeQuery(mergedQuery, queryFromRefererForPath(c, h.dashRoutePath(c, name, params)))
	mergedQuery = mergeQuery(mergedQuery, c.Queries())
	mergedQuery = mergeQuery(mergedQuery, query)
	if len(mergedQuery) == 0 {
		return h.redirectToDashRoute(c, name, params, nil, status...)
	}
	return h.redirectToDashRoute(c, name, params, mergedQuery, status...)
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
