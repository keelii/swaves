package admin

import (
	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
)

func RegisterRoutes(app *fiber.App, conn *db.DB) {
	handler := NewHandler(
		conn,
		NewService(conn),
		NewSessionStore(conn),
	)

	adminGroup := app.Group("/admin")

	adminGroup.Get("/", handler.GetHome)
	adminGroup.Get("/login", handler.GetLoginHandler)
	adminGroup.Post("/login", handler.PostLoginHandler)
	adminGroup.Get("/logout", handler.GetLogoutHandler)

	adminGroup.Use(RequireAdmin(handler.Store))

	//store := NewSessionStore(deps.DB)
	//
	//// auth
	//app.Get("/admin/login", GetLoginHandler(deps))
	//app.Post("/admin/login", PostLoginHandler(deps))
	//app.Post("/admin/logout", PostLogoutHandler(store))
	//
	//// protected
	//admin := app.Group("/admin")
	//admin.Use(RequireAdmin(store))
	//
	adminGroup.Get("/posts", handler.GetPostListHandler)
	adminGroup.Get("/posts/new", handler.GetPostNewHandler)
	adminGroup.Post("/posts/new", handler.PostCreatePostHandler)
	adminGroup.Get("/posts/:id/edit", handler.GetPostEditHandler)
	adminGroup.Post("/posts/:id/edit", handler.PostUpdatePostHandler)
	adminGroup.Post("/posts/:id/delete", handler.PostDeletePostHandler)

	//admin.Get("/encrypted-posts", GetEncryptedPostListHandler(deps))
	//// ...
	//
	//admin.Get("/tags", GetTagListHandler(deps))
	//// ...
	//
	//admin.Get("/redirects", GetRedirectListHandler(deps))
	//// ...
	//
	//admin.Get("/configs", GetConfigsHandler(deps))
	//admin.Post("/configs", PostUpdateConfigsHandler(deps))
}
