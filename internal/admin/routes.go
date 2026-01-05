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

	adminGroup.Get("/tags", handler.GetTagListHandler)
	adminGroup.Get("/tags/new", handler.GetTagNewHandler)
	adminGroup.Post("/tags/new", handler.PostCreateTagHandler)
	adminGroup.Get("/tags/:id/edit", handler.GetTagEditHandler)
	adminGroup.Post("/tags/:id/edit", handler.PostUpdateTagHandler)
	adminGroup.Post("/tags/:id/delete", handler.PostDeleteTagHandler)

	adminGroup.Get("/redirects", handler.GetRedirectListHandler)
	adminGroup.Get("/redirects/new", handler.GetRedirectNewHandler)
	adminGroup.Post("/redirects/new", handler.PostCreateRedirectHandler)
	adminGroup.Get("/redirects/:id/edit", handler.GetRedirectEditHandler)
	adminGroup.Post("/redirects/:id/edit", handler.PostUpdateRedirectHandler)
	adminGroup.Post("/redirects/:id/delete", handler.PostDeleteRedirectHandler)

	adminGroup.Get("/encrypted-posts", handler.GetEncryptedPostListHandler)
	adminGroup.Get("/encrypted-posts/new", handler.GetEncryptedPostNewHandler)
	adminGroup.Post("/encrypted-posts/new", handler.PostCreateEncryptedPostHandler)
	adminGroup.Get("/encrypted-posts/:id/edit", handler.GetEncryptedPostEditHandler)
	adminGroup.Post("/encrypted-posts/:id/edit", handler.PostUpdateEncryptedPostHandler)
	adminGroup.Post("/encrypted-posts/:id/delete", handler.PostDeleteEncryptedPostHandler)

	adminGroup.Get("/settings", handler.GetSettingsHandler)
	adminGroup.Post("/settings", handler.PostUpdateSettingsHandler)

	adminGroup.Get("/trash", handler.GetTrashHandler)
	adminGroup.Post("/trash/posts/:id/restore", handler.PostRestorePostHandler)
	adminGroup.Post("/trash/encrypted-posts/:id/restore", handler.PostRestoreEncryptedPostHandler)
	adminGroup.Post("/trash/tags/:id/restore", handler.PostRestoreTagHandler)
	adminGroup.Post("/trash/redirects/:id/restore", handler.PostRestoreRedirectHandler)

	adminGroup.Get("/http-error-logs", handler.GetHttpErrorLogListHandler)
	adminGroup.Post("/http-error-logs/:id/delete", handler.PostDeleteHttpErrorLogHandler)

	adminGroup.Get("/cron-jobs", handler.GetCronJobListHandler)
	adminGroup.Get("/cron-jobs/new", handler.GetCronJobNewHandler)
	adminGroup.Post("/cron-jobs/new", handler.PostCreateCronJobHandler)
	adminGroup.Get("/cron-jobs/:job_id/logs", handler.GetCronJobLogListHandler)

	adminGroup.Get("/import", handler.GetImportHandler)
	adminGroup.Post("/import", handler.PostImportHandler)
}
