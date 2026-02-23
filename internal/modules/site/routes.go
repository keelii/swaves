package site

import (
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/store"
	"swaves/internal/shared/share"

	"github.com/gofiber/fiber/v3"
)

func RegisterRoutes(app *fiber.App, gStore *store.GlobalStore) {
	handler := NewHandler(
		gStore,
		NewService(gStore.Model),
	)

	uiGroup := app.Group(share.GetBasePath())
	uiGroup.Use(middleware.EnsureVisitorID(""))

	uiGroup.Post("/_action/like/:postID", handler.PostEntityLike).Name("site.action.like")
	uiGroup.Post("/_action/comment/:postID", commentRateLimitMiddleware(), handler.PostComment).Name("site.action.comment")

	uiGroup.Get("/", handler.GetHome).Name("site.home")
	uiGroup.Get("/404", handler.GetNotFound).Name("site.not_found")
	uiGroup.Get("/error", handler.GetError).Name("site.error")
	uiGroup.Get("/raw/*.md", handler.GetRaw).Name("site.raw")

	uiGroup.Get(share.GetRSSRoute(), handler.GetRSS).Name("site.rss")

	uiGroup.Get(share.GetCategoryRoute(), handler.GetCategoryIndex).Name("site.categories")
	uiGroup.Get(share.GetCategoryRoute()+"/:categorySlug", handler.GetCategoryDetail).Name("site.category.detail")

	uiGroup.Get(share.GetTagRoute(), handler.GetTagIndex).Name("site.tags")
	uiGroup.Get(share.GetTagRoute()+"/:tagSlug", handler.GetTagDetail).Name("site.tag.detail")

	uiGroup.Get(share.GetPageRoute("/:ist"), handler.GetPostByPage).Name("site.page.detail")

	postURLRoute := share.GetPostRoute()
	switch postURLRoute {
	case "/":
		uiGroup.Get("/:ist", handler.GetPostByArticle).Name("site.post.detail")
	case "/{datetime}":
		uiGroup.Get("/:year/:month/:day/:ist", handler.GetPostByDateAndSlug).Name("site.post.detail")
	default:
		uiGroup.Get(postURLRoute+"/:ist", handler.GetPostByDefault).Name("site.post.detail")
	}
}
