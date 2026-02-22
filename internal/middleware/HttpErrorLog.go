package middleware

import (
	"strings"
	"swaves/internal/db"
	"swaves/internal/logger"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

const httpErrorLogFieldMaxLen = 2000

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

		queryParams := string(c.RequestCtx().URI().QueryString())
		bodyParams := ""
		method := strings.ToUpper(c.Method())
		if method != fiber.MethodGet && method != fiber.MethodHead {
			bodyParams = string(c.Body())
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
