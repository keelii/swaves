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

	adminGroup.Get("/", handler.GetHome).Name("admin.home")
	adminGroup.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	}).Name("admin.panic")
	adminGroup.Get("/login", handler.GetLoginHandler).Name("admin.login.show")
	adminGroup.Post("/login", handler.PostLoginHandler).Name("admin.login.submit")
	adminGroup.Get("/logout", handler.GetLogoutHandler).Name("admin.logout")

	adminGroup.Get("/posts", handler.GetPostListHandler).Name("admin.posts.list")
	adminGroup.Get("/posts/new", handler.GetPostNewHandler).Name("admin.posts.new")
	adminGroup.Post("/posts/new", handler.PostCreatePostHandler).Name("admin.posts.create")
	adminGroup.Get("/posts/:id/edit", handler.GetPostEditHandler).Name("admin.posts.edit")
	adminGroup.Post("/posts/:id/edit", handler.PostUpdatePostHandler).Name("admin.posts.update")
	adminGroup.Post("/posts/:id/delete", handler.PostDeletePostHandler).Name("admin.posts.delete")

	adminGroup.Get("/media", handler.GetMediaListHandler).Name("admin.media.list")
	adminGroup.Get("/api/media/assets", handler.GetMediaAssetsAPIHandler).Name("admin.media.assets.list")
	adminGroup.Post("/api/media/assets", handler.PostMediaUploadAPIHandler).Name("admin.media.assets.upload")
	adminGroup.Delete("/api/media/assets/:id", handler.DeleteMediaAssetAPIHandler).Name("admin.media.assets.delete")

	adminGroup.Get("/comments", handler.GetCommentListHandler).Name("admin.comments.list")
	adminGroup.Post("/comments/:id/approve", handler.PostApproveCommentHandler).Name("admin.comments.approve")
	adminGroup.Post("/comments/:id/pending", handler.PostPendingCommentHandler).Name("admin.comments.pending")
	adminGroup.Post("/comments/:id/spam", handler.PostSpamCommentHandler).Name("admin.comments.spam")
	adminGroup.Post("/comments/:id/delete", handler.PostDeleteCommentHandler).Name("admin.comments.delete")

	adminGroup.Get("/tags", handler.GetTagListHandler).Name("admin.tags.list")
	adminGroup.Get("/tags/new", handler.GetTagNewHandler).Name("admin.tags.new")
	adminGroup.Post("/tags/new", handler.PostCreateTagHandler).Name("admin.tags.create")
	adminGroup.Get("/tags/:id/edit", handler.GetTagEditHandler).Name("admin.tags.edit")
	adminGroup.Post("/tags/:id/edit", handler.PostUpdateTagHandler).Name("admin.tags.update")
	adminGroup.Post("/tags/:id/delete", handler.PostDeleteTagHandler).Name("admin.tags.delete")

	adminGroup.Get("/categories", handler.GetCategoryListHandler).Name("admin.categories.list")
	adminGroup.Get("/categories/tree", handler.GetCategoryTreeHandler).Name("admin.categories.tree")
	adminGroup.Post("/categories/:id/parent", handler.PostUpdateCategoryParentHandler).Name("admin.categories.parent.update")
	adminGroup.Get("/categories/new", handler.GetCategoryNewHandler).Name("admin.categories.new")
	adminGroup.Post("/categories/new", handler.PostCreateCategoryHandler).Name("admin.categories.create")
	adminGroup.Get("/categories/:id/edit", handler.GetCategoryEditHandler).Name("admin.categories.edit")
	adminGroup.Post("/categories/:id/edit", handler.PostUpdateCategoryHandler).Name("admin.categories.update")
	adminGroup.Post("/categories/:id/delete", handler.PostDeleteCategoryHandler).Name("admin.categories.delete")

	adminGroup.Get("/redirects", handler.GetRedirectListHandler).Name("admin.redirects.list")
	adminGroup.Get("/redirects/new", handler.GetRedirectNewHandler).Name("admin.redirects.new")
	adminGroup.Post("/redirects/new", handler.PostCreateRedirectHandler).Name("admin.redirects.create")
	adminGroup.Get("/redirects/:id/edit", handler.GetRedirectEditHandler).Name("admin.redirects.edit")
	adminGroup.Post("/redirects/:id/edit", handler.PostUpdateRedirectHandler).Name("admin.redirects.update")
	adminGroup.Post("/redirects/:id/delete", handler.PostDeleteRedirectHandler).Name("admin.redirects.delete")

	adminGroup.Get("/encrypted-posts", handler.GetEncryptedPostListHandler).Name("admin.encrypted_posts.list")
	adminGroup.Get("/encrypted-posts/new", handler.GetEncryptedPostNewHandler).Name("admin.encrypted_posts.new")
	adminGroup.Post("/encrypted-posts/new", handler.PostCreateEncryptedPostHandler).Name("admin.encrypted_posts.create")
	adminGroup.Get("/encrypted-posts/:id/edit", handler.GetEncryptedPostEditHandler).Name("admin.encrypted_posts.edit")
	adminGroup.Post("/encrypted-posts/:id/edit", handler.PostUpdateEncryptedPostHandler).Name("admin.encrypted_posts.update")
	adminGroup.Post("/encrypted-posts/:id/delete", handler.PostDeleteEncryptedPostHandler).Name("admin.encrypted_posts.delete")

	adminGroup.Get("/settings", handler.GetSettingsHandler).Name("admin.settings.list")
	adminGroup.Get("/settings/all", handler.GetSettingsAllHandler).Name("admin.settings.all")
	adminGroup.Post("/settings/all", handler.PostUpdateSettingsAllHandler).Name("admin.settings.all.update")
	adminGroup.Get("/settings/new", handler.GetSettingNewHandler).Name("admin.settings.new")
	adminGroup.Post("/settings/new", handler.PostCreateSettingHandler).Name("admin.settings.create")
	adminGroup.Get("/settings/:id/edit", handler.GetSettingEditHandler).Name("admin.settings.edit")
	adminGroup.Post("/settings/:id/edit", handler.PostUpdateSettingHandler).Name("admin.settings.update")
	adminGroup.Post("/settings/:id/delete", handler.PostDeleteSettingHandler).Name("admin.settings.delete")

	adminGroup.Get("/trash", handler.GetTrashHandler).Name("admin.trash.list")
	adminGroup.Post("/trash/posts/:id/restore", handler.PostRestorePostHandler).Name("admin.trash.posts.restore")
	adminGroup.Post("/trash/posts/:id/delete", handler.PostHardDeletePostHandler).Name("admin.trash.posts.delete")
	adminGroup.Post("/trash/encrypted-posts/:id/restore", handler.PostRestoreEncryptedPostHandler).Name("admin.trash.encrypted_posts.restore")
	adminGroup.Post("/trash/encrypted-posts/:id/delete", handler.PostHardDeleteEncryptedPostHandler).Name("admin.trash.encrypted_posts.delete")
	adminGroup.Post("/trash/tags/:id/restore", handler.PostRestoreTagHandler).Name("admin.trash.tags.restore")
	adminGroup.Post("/trash/tags/:id/delete", handler.PostHardDeleteTagHandler).Name("admin.trash.tags.delete")
	adminGroup.Post("/trash/categories/:id/restore", handler.PostRestoreCategoryHandler).Name("admin.trash.categories.restore")
	adminGroup.Post("/trash/categories/:id/delete", handler.PostHardDeleteCategoryHandler).Name("admin.trash.categories.delete")
	adminGroup.Post("/trash/redirects/:id/restore", handler.PostRestoreRedirectHandler).Name("admin.trash.redirects.restore")
	adminGroup.Post("/trash/redirects/:id/delete", handler.PostHardDeleteRedirectHandler).Name("admin.trash.redirects.delete")

	adminGroup.Get("/http-error-logs", handler.GetHttpErrorLogListHandler).Name("admin.http_error_logs.list")
	adminGroup.Post("/http-error-logs/:id/delete", handler.PostDeleteHttpErrorLogHandler).Name("admin.http_error_logs.delete")

	adminGroup.Get("/tasks", handler.GetTaskListHandler).Name("admin.tasks.list")
	adminGroup.Get("/tasks/new", handler.GetTaskNewHandler).Name("admin.tasks.new")
	adminGroup.Post("/tasks/new", handler.PostCreateTaskHandler).Name("admin.tasks.create")
	adminGroup.Get("/tasks/:id/edit", handler.GetTaskEditHandler).Name("admin.tasks.edit")
	adminGroup.Post("/tasks/:id/edit", handler.PostUpdateTaskHandler).Name("admin.tasks.update")
	adminGroup.Post("/tasks/:id/delete", handler.PostDeleteTaskHandler).Name("admin.tasks.delete")
	adminGroup.Post("/tasks/:code/trigger", handler.PostTriggerTaskHandler).Name("admin.tasks.trigger")
	adminGroup.Get("/tasks/:code/runs", handler.GetTaskRunListHandler).Name("admin.tasks.runs")

	adminGroup.Get("/import", handler.GetImportHandler).Name("admin.import.show")
	adminGroup.Post("/import", handler.PostImportHandler).Name("admin.import.submit")
	adminGroup.Post("/import/parse-item", handler.PostImportParseItemHandler).Name("admin.import.parse_item")
	adminGroup.Post("/import/confirm-item", handler.PostImportConfirmItemHandler).Name("admin.import.confirm_item")
	adminGroup.Post("/import/cancel", handler.PostImportCancelHandler).Name("admin.import.cancel")

	adminGroup.Get("/export", handler.GetExportHandler).Name("admin.export.show")
	adminGroup.Get("/export/download", handler.GetExportDownloadHandler).Name("admin.export.download")

	adminGroup.Get("/metrics", handler.GetMetricsHandler).Name("admin.metrics")

	adminGroup.Get("/dev/ui-components", handler.GetDevUIComponentsHandler).Name("admin.dev.ui_components")
}
