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

	uiGroup := app.Group("/ui")

	uiGroup.Get(store.GetSetting("base_path"), handler.GetHome)
	uiGroup.Get(store.GetSetting("rss_path"), handler.GetRSS)
	uiGroup.Get(store.GetSetting("category_index"), handler.GetCategoryIndex)
	uiGroup.Get(store.GetSetting("tag_index"), handler.GetTagIndex)
	uiGroup.Get("/:slug", handler.GetPost)

}
