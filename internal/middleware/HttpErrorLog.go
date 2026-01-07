package middleware

import (
	"log"
	"strings"
	"swaves/internal/db"
	"time"

	"github.com/gofiber/fiber/v2"
)

const MaxMessageLength = 512

func HttpErrorLogMiddleware(dbx *db.DB) fiber.Handler {
	return func(c *fiber.Ctx) error {
		err := c.Next()

		status := c.Response().StatusCode()
		if status == 200 {
			return err
		}

		reqID, _ := c.Locals("reqId").(string)

		logItem := &db.HttpErrorLog{
			ReqID:       reqID,
			ClientIP:    c.IP(),
			Method:      c.Method(),
			Path:        c.Path(),
			Status:      status,
			UserAgent:   c.Get("User-Agent"),
			QueryParams: truncate(c.Context().URI().QueryString(), MaxMessageLength),
			BodyParams:  truncate(c.Body(), MaxMessageLength),
			CreatedAt:   time.Now().Unix(),
		}

		go func(l *db.HttpErrorLog) {
			if err := db.CreateHttpErrorLog(dbx, l); err != nil {
				log.Printf("insert http_error_log failed: %v", err)
			}
		}(logItem)

		return err
	}
}

func truncate(b []byte, max int) string {
	if len(b) > max {
		return string(b[:max])
	}
	return strings.TrimSpace(string(b))
}
