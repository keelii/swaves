package dash

import (
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/shared/types"
)

type Handler struct {
	Model   *db.DB
	Session *types.SessionStore
	Service *Service
	Monitor *MonitorStore
}

func NewHandler(
	gStore *store.GlobalStore,
	dashService *Service,
	monitorStore *MonitorStore,
) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: dashService,
		Monitor: monitorStore,
	}
}
