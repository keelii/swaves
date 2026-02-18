package site

import (
	"swaves/internal/middleware"
	"swaves/internal/store"

	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, gStore *store.GlobalStore) {
	handler := NewHandler(
		gStore,
		NewService(gStore.Model),
	)

	uiGroup := app.Group(store.GetSetting("base_path"))
	uiGroup.Use(middleware.EnsureVisitorID(""))
	uiGroup.Post("/_action/like/:postID", handler.PostEntityLike)

	uiGroup.Get("/", handler.GetHome)
	// RSS
	uiGroup.Get(store.GetSetting("rss_path"), handler.GetRSS)
	// Categories and Tags
	uiGroup.Get(store.GetSetting("category_url_prefix"), handler.GetCategoryIndex)
	uiGroup.Get(store.GetSetting("tag_url_prefix"), handler.GetTagIndex)
	uiGroup.Get(store.GetSetting("category_url_prefix")+"/:categorySlug", handler.GetCategoryDetail)
	uiGroup.Get(store.GetSetting("tag_url_prefix")+"/:tagSlug", handler.GetTagDetail)

	// Pages
	uiGroup.Get("/:slug", handler.GetPostBySlug)
	// Posts
	postUrlPrefix := store.GetSetting("post_url_prefix")

	switch postUrlPrefix {
	case "/":
		uiGroup.Get("/:slug", handler.GetPostBySlug)
	case "/{datetime}":
		uiGroup.Get("/:year/:month/:day/:slug", handler.GetPostByDateAndSlug)
	default:
		uiGroup.Get(postUrlPrefix+"/:slug", handler.GetPostBySlug)
	}
	//uiGroup.Get(store.GetSetting("post_url_prefix"), ha)
	//uiGroup.Get("/posts/:date<regex(\\d{4}/\\d{2}/\\d{2})>", handler.GetDate)
}
