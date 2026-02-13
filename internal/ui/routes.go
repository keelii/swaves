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

	uiGroup := app.Group(store.GetSetting("base_path"))

	// RSS
	uiGroup.Get(store.GetSetting("rss_path"), handler.GetRSS)
	// Pages
	uiGroup.Get("/:pageSlug", handler.GetPage)
	// Posts
	uiGroup.Get("/*", handler.DispatchHandler)
	//uiGroup.Get(store.GetSetting("post_url_prefix"), ha)
	//uiGroup.Get("/posts/:date<regex(\\d{4}/\\d{2}/\\d{2})>", handler.GetDate)

	uiGroup.Get(store.GetSetting("category_index"), handler.GetCategoryIndex)
	uiGroup.Get(store.GetSetting("tag_index"), handler.GetTagIndex)
}
