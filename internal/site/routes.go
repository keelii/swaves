package site

import (
	"swaves/internal/middleware"
	"swaves/internal/share"
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
	uiGroup.Post("/_action/comment/:postID", commentRateLimitMiddleware(), handler.PostComment)

	uiGroup.Get("/", handler.GetHome)
	uiGroup.Get("/404", handler.GetNotFound)
	uiGroup.Get("/error", handler.GetError)
	// RSS
	uiGroup.Get(share.GetRSSUrl(), handler.GetRSS)
	// Categories
	uiGroup.Get(share.GetCategoryIndex(), handler.GetCategoryIndex)
	uiGroup.Get(share.GetCategoryIndex()+"/:categorySlug", handler.GetCategoryDetail)
	// Tags
	uiGroup.Get(share.GetTagIndex(), handler.GetTagIndex)
	uiGroup.Get(share.GetTagIndex()+"/:tagSlug", handler.GetTagDetail)

	// IST stands for ID, Slug, or Title, which are the three ways to identify a post in the URL
	// Pages
	uiGroup.Get(share.GetPagePrefix()+"/:ist", handler.GetPostByIST)
	// Posts
	postUrlPrefix := store.GetSetting("post_url_prefix")

	switch postUrlPrefix {
	case "/":
		uiGroup.Get("/:ist", handler.GetPostByIST)
	case "/{datetime}":
		uiGroup.Get("/:year/:month/:day/:ist", handler.GetPostByDateAndSlug)
	default:
		uiGroup.Get(postUrlPrefix+"/:ist", handler.GetPostByIST)
	}
	//uiGroup.Get(store.GetSetting("post_url_prefix"), ha)
	//uiGroup.Get("/posts/:date<regex(\\d{4}/\\d{2}/\\d{2})>", handler.GetDate)
}
