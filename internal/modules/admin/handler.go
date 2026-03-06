package admin

import (
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

type Handler struct {
	Model   *db.DB
	Session *types.SessionStore
	Service *Service
	Monitor *MonitorStore
}

func NewHandler(
	gStore *store.GlobalStore,
	adminService *Service,
	monitorStore *MonitorStore,
) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: adminService,
		Monitor: monitorStore,
	}
}

func (h *Handler) RenderAdminView(c fiber.Ctx, view string, data fiber.Map, layout string) error {
	_ = h
	return RenderAdminView(c, view, data, layout)
}
