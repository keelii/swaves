package admin

import (
	"strconv"
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

// HttpErrorLogs
func (h *Handler) GetHttpErrorLogListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	logs, err := ListHttpErrorLogs(h.Model, &pager)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "admin/http_error_logs_index.html", fiber.Map{
		"Title": "Http Error Logs",
		"Logs":  logs,
		"Pager": pager,
	}, "")
}

func (h *Handler) PostDeleteHttpErrorLogHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteHttpErrorLogService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.http_error_logs.list", nil, nil)
}
