package site

import (
	"swaves/internal/middleware"
	"swaves/internal/share"
	"swaves/internal/store"

	"github.com/gofiber/fiber/v3"
)

func RegisterRoutes(app *fiber.App, gStore *store.GlobalStore) {
	handler := NewHandler(
		gStore,
		NewService(gStore.Model),
	)

	uiGroup := app.Group(share.GetBasePath())
	uiGroup.Use(middleware.EnsureVisitorID(""))

	uiGroup.Post("/_action/like/:postID", handler.PostEntityLike)
	uiGroup.Post("/_action/comment/:postID", commentRateLimitMiddleware(), handler.PostComment)

	uiGroup.Get("/", handler.GetHome)
	uiGroup.Get("/404", handler.GetNotFound)
	uiGroup.Get("/error", handler.GetError)
	// RSS
	uiGroup.Get(share.GetRSSRoute(), handler.GetRSS)
	// Categories
	uiGroup.Get(share.GetCategoryRoute(), handler.GetCategoryIndex)
	uiGroup.Get(share.GetCategoryRoute()+"/:categorySlug", handler.GetCategoryDetail)
	// Tags
	uiGroup.Get(share.GetTagRoute(), handler.GetTagIndex)
	uiGroup.Get(share.GetTagRoute()+"/:tagSlug", handler.GetTagDetail)

	// IST stands for ID, Slug, or Title, which are the three ways to identify a post in the URL
	// Pages
	uiGroup.Get(share.GetPageRoute("/:ist"), handler.GetPostByPage)
	// Posts
	postUrlRoute := share.GetPostRoute()

	switch postUrlRoute {
	case "/":
		uiGroup.Get("/:ist", handler.GetPostByArticle)
	case "/{datetime}":
		uiGroup.Get("/:year/:month/:day/:ist", handler.GetPostByDateAndSlug)
	default:
		uiGroup.Get(postUrlRoute+"/:ist", handler.GetPostByDefault)
	}
}
