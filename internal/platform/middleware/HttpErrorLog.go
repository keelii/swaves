package middleware

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

const httpErrorLogFieldMaxLen = 2000
const redactedValue = "[REDACTED]"

func HttpErrorLog(dbx *db.DB) fiber.Handler {
	return func(c fiber.Ctx) error {
		err := c.Next()

		status := c.Response().StatusCode()
		if err != nil {
			if e, ok := err.(*fiber.Error); ok {
				status = e.Code
			} else if status < fiber.StatusBadRequest {
				status = fiber.StatusInternalServerError
			}
		}

		if status < fiber.StatusBadRequest {
			return err
		}

		reqID := strings.TrimSpace(c.GetRespHeader("X-Req-Id"))
		if reqID == "" {
			reqID = strings.TrimSpace(requestid.FromContext(c))
		}
		if reqID == "" {
			reqID = "-"
		}

		queryParams := sanitizeQueryParams(string(c.RequestCtx().URI().QueryString()))
		bodyParams := ""
		method := strings.ToUpper(c.Method())
		if method != fiber.MethodGet && method != fiber.MethodHead {
			bodyParams = sanitizeBodyParams(string(c.Body()), c.Get("Content-Type"))
		}

		logItem := &db.HttpErrorLog{
			ReqID:       truncateForHttpErrorLog(reqID),
			ClientIP:    truncateForHttpErrorLog(strings.TrimSpace(c.IP())),
			Method:      truncateForHttpErrorLog(method),
			Path:        truncateForHttpErrorLog(strings.TrimSpace(c.Path())),
			Status:      status,
			UserAgent:   truncateForHttpErrorLog(strings.TrimSpace(c.Get("User-Agent"))),
			QueryParams: truncateForHttpErrorLog(queryParams),
			BodyParams:  truncateForHttpErrorLog(bodyParams),
		}
		if logItem.ClientIP == "" {
			logItem.ClientIP = "-"
		}
		if logItem.UserAgent == "" {
			logItem.UserAgent = "-"
		}

		if _, createErr := db.CreateHttpErrorLog(dbx, logItem); createErr != nil {
			logger.Error("http error log create failed: %v", createErr)
		}

		return err
	}
}

func truncateForHttpErrorLog(value string) string {
	if len(value) <= httpErrorLogFieldMaxLen {
		return value
	}
	return value[:httpErrorLogFieldMaxLen]
}

func sanitizeQueryParams(raw string) string {
	values, err := url.ParseQuery(raw)
	if err != nil {
		return ""
	}
	redactURLValues(values)
	return values.Encode()
}

func sanitizeBodyParams(raw string, contentType string) string {
	body := strings.TrimSpace(raw)
	if body == "" {
		return ""
	}

	normalizedType := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch normalizedType {
	case "application/x-www-form-urlencoded":
		values, err := url.ParseQuery(body)
		if err != nil {
			return "[invalid form body]"
		}
		redactURLValues(values)
		return values.Encode()
	case "application/json":
		sanitized, err := sanitizeJSONBody(body)
		if err != nil {
			return "[invalid json body]"
		}
		return sanitized
	case "multipart/form-data":
		return "[multipart body omitted]"
	default:
		return fmt.Sprintf("[body omitted content-type=%s]", normalizedType)
	}
}

func sanitizeJSONBody(raw string) (string, error) {
	var payload any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return "", err
	}
	sanitized := redactJSONPayload(payload)
	buf, err := json.Marshal(sanitized)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

func redactJSONPayload(payload any) any {
	switch typed := payload.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			if isSensitiveFieldName(key) {
				out[key] = redactedValue
				continue
			}
			out[key] = redactJSONPayload(value)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, redactJSONPayload(item))
		}
		return out
	default:
		return payload
	}
}

func redactURLValues(values url.Values) {
	for key, list := range values {
		if !isSensitiveFieldName(key) {
			continue
		}
		for idx := range list {
			list[idx] = redactedValue
		}
		values[key] = list
	}
}

func isSensitiveFieldName(name string) bool {
	normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(strings.ToLower(strings.TrimSpace(name)))
	if normalized == "" {
		return false
	}
	switch normalized {
	case "password", "passwd", "pwd", "secret", "token", "accesstoken", "refreshtoken",
		"apikey", "privatekey", "authorization", "cookie", "sessionid", "credential", "credentials":
		return true
	}
	return strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "privatekey") ||
		strings.Contains(normalized, "authorization")
}
