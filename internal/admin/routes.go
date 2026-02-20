package admin

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

	adminGroup := app.Group(share.GetAdminUrl())
	adminGroup.Use(middleware.RequireAdmin(gStore.Session, share.BuildAdminPath("/login")))

	adminGroup.Get("/", handler.GetHome)
	adminGroup.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	})
	adminGroup.Get("/login", handler.GetLoginHandler)
	adminGroup.Post("/login", handler.PostLoginHandler)
	adminGroup.Get("/logout", handler.GetLogoutHandler)

	//store := NewSessionStore(deps.Model)
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
	adminGroup.Get("/media", handler.GetMediaListHandler)
	adminGroup.Get("/api/media/assets", handler.GetMediaAssetsAPIHandler)
	adminGroup.Post("/api/media/assets", handler.PostMediaUploadAPIHandler)
	adminGroup.Delete("/api/media/assets/:id", handler.DeleteMediaAssetAPIHandler)
	adminGroup.Get("/comments", handler.GetCommentListHandler)
	adminGroup.Post("/comments/:id/approve", handler.PostApproveCommentHandler)
	adminGroup.Post("/comments/:id/pending", handler.PostPendingCommentHandler)
	adminGroup.Post("/comments/:id/spam", handler.PostSpamCommentHandler)
	adminGroup.Post("/comments/:id/delete", handler.PostDeleteCommentHandler)

	adminGroup.Get("/tags", handler.GetTagListHandler)
	adminGroup.Get("/tags/new", handler.GetTagNewHandler)
	adminGroup.Post("/tags/new", handler.PostCreateTagHandler)
	adminGroup.Get("/tags/:id/edit", handler.GetTagEditHandler)
	adminGroup.Post("/tags/:id/edit", handler.PostUpdateTagHandler)
	adminGroup.Post("/tags/:id/delete", handler.PostDeleteTagHandler)

	adminGroup.Get("/categories", handler.GetCategoryListHandler)
	adminGroup.Get("/categories/tree", handler.GetCategoryTreeHandler)
	adminGroup.Post("/categories/:id/parent", handler.PostUpdateCategoryParentHandler)
	adminGroup.Get("/categories/new", handler.GetCategoryNewHandler)
	adminGroup.Post("/categories/new", handler.PostCreateCategoryHandler)
	adminGroup.Get("/categories/:id/edit", handler.GetCategoryEditHandler)
	adminGroup.Post("/categories/:id/edit", handler.PostUpdateCategoryHandler)
	adminGroup.Post("/categories/:id/delete", handler.PostDeleteCategoryHandler)

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
	adminGroup.Get("/settings/all", handler.GetSettingsAllHandler)
	adminGroup.Post("/settings/all", handler.PostUpdateSettingsAllHandler)
	adminGroup.Get("/settings/new", handler.GetSettingNewHandler)
	adminGroup.Post("/settings/new", handler.PostCreateSettingHandler)
	adminGroup.Get("/settings/:id/edit", handler.GetSettingEditHandler)
	adminGroup.Post("/settings/:id/edit", handler.PostUpdateSettingHandler)
	adminGroup.Post("/settings/:id/delete", handler.PostDeleteSettingHandler)

	adminGroup.Get("/trash", handler.GetTrashHandler)
	adminGroup.Post("/trash/posts/:id/restore", handler.PostRestorePostHandler)
	adminGroup.Post("/trash/posts/:id/delete", handler.PostHardDeletePostHandler)
	adminGroup.Post("/trash/encrypted-posts/:id/restore", handler.PostRestoreEncryptedPostHandler)
	adminGroup.Post("/trash/encrypted-posts/:id/delete", handler.PostHardDeleteEncryptedPostHandler)
	adminGroup.Post("/trash/tags/:id/restore", handler.PostRestoreTagHandler)
	adminGroup.Post("/trash/tags/:id/delete", handler.PostHardDeleteTagHandler)
	adminGroup.Post("/trash/categories/:id/restore", handler.PostRestoreCategoryHandler)
	adminGroup.Post("/trash/categories/:id/delete", handler.PostHardDeleteCategoryHandler)
	adminGroup.Post("/trash/redirects/:id/restore", handler.PostRestoreRedirectHandler)
	adminGroup.Post("/trash/redirects/:id/delete", handler.PostHardDeleteRedirectHandler)

	adminGroup.Get("/http-error-logs", handler.GetHttpErrorLogListHandler)
	adminGroup.Post("/http-error-logs/:id/delete", handler.PostDeleteHttpErrorLogHandler)

	adminGroup.Get("/tasks", handler.GetTaskListHandler)
	adminGroup.Get("/tasks/new", handler.GetTaskNewHandler)
	adminGroup.Post("/tasks/new", handler.PostCreateTaskHandler)
	adminGroup.Get("/tasks/:id/edit", handler.GetTaskEditHandler)
	adminGroup.Post("/tasks/:id/edit", handler.PostUpdateTaskHandler)
	adminGroup.Post("/tasks/:id/delete", handler.PostDeleteTaskHandler)
	adminGroup.Post("/tasks/:code/trigger", handler.PostTriggerTaskHandler)
	adminGroup.Get("/tasks/:code/runs", handler.GetTaskRunListHandler)

	adminGroup.Get("/import", handler.GetImportHandler)
	adminGroup.Post("/import", handler.PostImportHandler)
	adminGroup.Post("/import/parse-item", handler.PostImportParseItemHandler)
	adminGroup.Post("/import/confirm-item", handler.PostImportConfirmItemHandler)

	adminGroup.Get("/export", handler.GetExportHandler)
	adminGroup.Get("/export/download", handler.GetExportDownloadHandler)

	adminGroup.Get("/metrics", handler.GetMetricsHandler)

	adminGroup.Get("/dev/ui-components", handler.GetDevUIComponentsHandler)
}
