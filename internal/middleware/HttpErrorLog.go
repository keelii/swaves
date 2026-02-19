package middleware

import (
	"log"
	"strings"
	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
)

const httpErrorLogFieldMaxLen = 2000

func HttpErrorLog(dbx *db.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
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
			if localReqID, ok := c.Locals("reqId").(string); ok {
				reqID = strings.TrimSpace(localReqID)
			}
		}
		if reqID == "" {
			reqID = "-"
		}

		queryParams := string(c.Context().URI().QueryString())
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
			log.Printf("http error log create failed: %v", createErr)
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
