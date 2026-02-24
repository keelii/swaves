package admin_app

import (
	"swaves/internal/platform/store"

	"github.com/gofiber/fiber/v3"
)

func RegisterRouter(
	app *fiber.App,
	gStore *store.GlobalStore,
) {
	monitorStore := NewMonitorStore()
	handler := NewHandler(
		gStore,
		NewService(gStore.Model),
		monitorStore,
	)

	adminGroup := app.Group("/admin_app")

	adminGroup.Get("/", handler.GetHome).Name("admin_app.home")
}
