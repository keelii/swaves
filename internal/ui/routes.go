package ui

import (
	"swaves/internal/store"

	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, gStore *store.GlobalStore) {
	handler := NewHandler(
		gStore,
		NewService(gStore.Model),
	)

	uiGroup := app.Group("/")

	uiGroup.Get(store.GetSetting("base_url"), handler.GetHome)
	uiGroup.Get(store.GetSetting("rss_url"), handler.GetRSS)

}
