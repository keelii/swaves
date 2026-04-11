package dash

import (
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/store"
	"swaves/internal/shared/share"

	"github.com/gofiber/fiber/v3"
)

func RegisterRouter(app *fiber.App, gStore *store.GlobalStore) {
	dashBasePath := share.GetDashUrl()

	monitorStore := NewMonitorStore()
	handler := NewHandler(
		gStore,
		NewService(gStore.Model),
		monitorStore,
	)

	if store.IsSettingEmpty() {
		app.Get("/install", middleware.DashCSRF(gStore.Session), handler.GetInstallHandler).Name("install.show")
		app.Post("/install", middleware.DashCSRF(gStore.Session), handler.PostInstallHandler).Name("install.submit")
	}

	dashGroup := app.Group(dashBasePath)
	dashGroup.Use(middleware.DashCSRF(gStore.Session))
	dashGroup.Use(middleware.RequireDash(gStore.Session, share.BuildDashPath("/login")))
	dashAPIPathPrefix := share.BuildDashPath("/api/")
	dashGroup.Use(func(c fiber.Ctx) error {
		if strings.HasPrefix(c.Path(), dashAPIPathPrefix) {
			return c.Next()
		}
		unreadCount, err := CountUnreadNotificationsService(handler.Model, dashNotificationReceiver)
		if err != nil {
			logger.Warn("[notify] preload unread count failed: path=%s err=%v", c.Path(), err)
			c.Locals("DashNotificationUnreadCount", 0)
			return c.Next()
		}
		c.Locals("DashNotificationUnreadCount", unreadCount)
		return c.Next()
	})

	if config.IsDevelopment {
		dashGroup.Get("/test", handler.TestRouter).Name("dash.test")
	}

	dashGroup.Get("/monitor", handler.GetMonitorHandler).Name("dash.monitor")
	dashGroup.Get("/metrics", handler.GetMetricsAPIHandler).Name("dash.metrics.api")
	dashGroup.Get("/api/monitor", handler.GetMonitorDataAPIHandler).Name("dash.monitor.data")

	dashGroup.Get("/", handler.GetHome).Name("dash.home")
	dashGroup.Get("/panic", func(c fiber.Ctx) error {
		panic("test panic")
	}).Name("dash.panic")
	dashGroup.Get("/login", handler.GetLoginHandler).Name("dash.login.show")
	dashGroup.Post("/login", handler.PostLoginHandler).Name("dash.login.submit")
	dashGroup.Get("/logout", handler.GetLogoutHandler).Name("dash.logout")

	dashGroup.Get("/records", handler.GetRecordListHandler).Name("dash.records.list")
	dashGroup.Get("/posts", handler.GetPostListHandler).Name("dash.posts.list")
	dashGroup.Get("/posts/new", handler.GetPostNewHandler).Name("dash.posts.new")
	dashGroup.Post("/posts/new", handler.PostCreatePostHandler).Name("dash.posts.create")
	dashGroup.Get("/posts/:id/edit", handler.GetPostEditHandler).Name("dash.posts.edit")
	dashGroup.Post("/posts/:id/edit", handler.PostUpdatePostHandler).Name("dash.posts.update")
	dashGroup.Post("/posts/:id/delete", handler.PostDeletePostHandler).Name("dash.posts.delete")
	dashGroup.Post("/api/posts/batch-delete", handler.PostPostBatchDeleteAPIHandler).Name("dash.posts.api.batch_delete")

	dashGroup.Get("/assets", handler.GetAssetListHandler).Name("dash.assets.list")
	dashGroup.Get("/api/assets", handler.GetAssetListAPIHandler).Name("dash.assets.api.list")
	dashGroup.Post("/api/assets", handler.PostAssetUploadAPIHandler).Name("dash.assets.api.upload")
	dashGroup.Post("/api/assets/batch-delete", handler.PostAssetBatchDeleteAPIHandler).Name("dash.assets.api.batch_delete")
	dashGroup.Delete("/api/assets/:id", handler.DeleteAssetAPIHandler).Name("dash.assets.api.delete")

	dashGroup.Get("/comments", handler.GetCommentListHandler).Name("dash.comments.list")
	dashGroup.Post("/comments/:id/approve", handler.PostApproveCommentHandler).Name("dash.comments.approve")
	dashGroup.Post("/comments/:id/pending", handler.PostPendingCommentHandler).Name("dash.comments.pending")
	dashGroup.Post("/comments/:id/spam", handler.PostSpamCommentHandler).Name("dash.comments.spam")
	dashGroup.Post("/comments/:id/delete", handler.PostDeleteCommentHandler).Name("dash.comments.delete")
	dashGroup.Post("/api/comments/batch-delete", handler.PostCommentBatchDeleteAPIHandler).Name("dash.comments.api.batch_delete")

	dashGroup.Get("/notifications", handler.GetNotificationListHandler).Name("dash.notifications.list")
	dashGroup.Get("/api/notifications", handler.GetNotificationListAPIHandler).Name("dash.notifications.api.list")
	dashGroup.Get("/api/notifications/unread_count", handler.GetNotificationUnreadCountAPIHandler).Name("dash.notifications.api.unread_count")
	dashGroup.Post("/api/notifications/read", handler.PostNotificationReadAPIHandler).Name("dash.notifications.api.read")
	dashGroup.Post("/api/notifications/read_all", handler.PostNotificationReadAllAPIHandler).Name("dash.notifications.api.read_all")
	dashGroup.Post("/api/notifications/delete", handler.PostNotificationDeleteAPIHandler).Name("dash.notifications.api.delete")
	dashGroup.Post("/api/notifications/batch-delete", handler.PostNotificationBatchDeleteAPIHandler).Name("dash.notifications.api.batch_delete")
	dashGroup.Get("/settings/system-update", handler.GetSettingsSystemUpdateHandler).Name("dash.settings.system_update")
	dashGroup.Post("/settings/system-update/auto", handler.PostSettingsSystemAutoUpdateHandler).Name("dash.settings.system_update.auto")
	dashGroup.Post("/settings/system-update/manual", handler.PostSettingsSystemManualUpdateHandler).Name("dash.settings.system_update.manual")
	dashGroup.Post("/settings/system-update/restart", handler.PostSettingsSystemRestartHandler).Name("dash.settings.system_update.restart")

	dashGroup.Get("/tags", handler.GetTagListHandler).Name("dash.tags.list")
	dashGroup.Get("/tags/new", handler.GetTagNewHandler).Name("dash.tags.new")
	dashGroup.Post("/tags/new", handler.PostCreateTagHandler).Name("dash.tags.create")
	dashGroup.Get("/tags/:id/edit", handler.GetTagEditHandler).Name("dash.tags.edit")
	dashGroup.Post("/tags/:id/edit", handler.PostUpdateTagHandler).Name("dash.tags.update")
	dashGroup.Post("/tags/:id/delete", handler.PostDeleteTagHandler).Name("dash.tags.delete")
	dashGroup.Post("/api/tags/batch-delete", handler.PostTagBatchDeleteAPIHandler).Name("dash.tags.api.batch_delete")

	dashGroup.Get("/categories", handler.GetCategoryListHandler).Name("dash.categories.list")
	dashGroup.Get("/categories/tree", handler.GetCategoryTreeHandler).Name("dash.categories.tree")
	dashGroup.Post("/categories/:id/parent", handler.PostUpdateCategoryParentHandler).Name("dash.categories.parent.update")
	dashGroup.Get("/categories/new", handler.GetCategoryNewHandler).Name("dash.categories.new")
	dashGroup.Post("/categories/new", handler.PostCreateCategoryHandler).Name("dash.categories.create")
	dashGroup.Get("/categories/:id/edit", handler.GetCategoryEditHandler).Name("dash.categories.edit")
	dashGroup.Post("/categories/:id/edit", handler.PostUpdateCategoryHandler).Name("dash.categories.update")
	dashGroup.Post("/categories/:id/delete", handler.PostDeleteCategoryHandler).Name("dash.categories.delete")
	dashGroup.Post("/api/categories/batch-delete", handler.PostCategoryBatchDeleteAPIHandler).Name("dash.categories.api.batch_delete")

	dashGroup.Get("/redirects", handler.GetRedirectListHandler).Name("dash.redirects.list")
	dashGroup.Get("/redirects/new", handler.GetRedirectNewHandler).Name("dash.redirects.new")
	dashGroup.Post("/redirects/new", handler.PostCreateRedirectHandler).Name("dash.redirects.create")
	dashGroup.Post("/redirects/import", handler.PostImportRedirectHandler).Name("dash.redirects.import")
	dashGroup.Get("/redirects/:id/edit", handler.GetRedirectEditHandler).Name("dash.redirects.edit")
	dashGroup.Post("/redirects/:id/edit", handler.PostUpdateRedirectHandler).Name("dash.redirects.update")
	dashGroup.Post("/redirects/:id/delete", handler.PostDeleteRedirectHandler).Name("dash.redirects.delete")
	dashGroup.Post("/api/redirects/batch-delete", handler.PostRedirectBatchDeleteAPIHandler).Name("dash.redirects.api.batch_delete")

	dashGroup.Get("/encrypted-posts", handler.GetEncryptedPostListHandler).Name("dash.encrypted_posts.list")
	dashGroup.Get("/encrypted-posts/new", handler.GetEncryptedPostNewHandler).Name("dash.encrypted_posts.new")
	dashGroup.Post("/encrypted-posts/new", handler.PostCreateEncryptedPostHandler).Name("dash.encrypted_posts.create")
	dashGroup.Get("/encrypted-posts/:id/edit", handler.GetEncryptedPostEditHandler).Name("dash.encrypted_posts.edit")
	dashGroup.Post("/encrypted-posts/:id/edit", handler.PostUpdateEncryptedPostHandler).Name("dash.encrypted_posts.update")
	dashGroup.Post("/encrypted-posts/:id/delete", handler.PostDeleteEncryptedPostHandler).Name("dash.encrypted_posts.delete")
	dashGroup.Post("/api/encrypted-posts/batch-delete", handler.PostEncryptedPostBatchDeleteAPIHandler).Name("dash.encrypted_posts.api.batch_delete")

	dashGroup.Get("/settings", handler.GetSettingsHandler).Name("dash.settings.list")
	dashGroup.Get("/settings/all", handler.GetSettingsAllHandler).Name("dash.settings.all")
	dashGroup.Post("/settings/all", handler.PostUpdateSettingsAllHandler).Name("dash.settings.all.update")
	dashGroup.Post("/api/settings/dash-ui", handler.PostUpdateDashUISettingAPIHandler).Name("dash.settings.api.ui_state.update")
	dashGroup.Get("/settings/new", handler.GetSettingNewHandler).Name("dash.settings.new")
	dashGroup.Post("/settings/new", handler.PostCreateSettingHandler).Name("dash.settings.create")
	dashGroup.Get("/settings/:id/edit", handler.GetSettingEditHandler).Name("dash.settings.edit")
	dashGroup.Post("/settings/:id/edit", handler.PostUpdateSettingHandler).Name("dash.settings.update")
	dashGroup.Post("/settings/:id/delete", handler.PostDeleteSettingHandler).Name("dash.settings.delete")
	dashGroup.Post("/api/settings/batch-delete", handler.PostSettingBatchDeleteAPIHandler).Name("dash.settings.api.batch_delete")

	dashGroup.Get("/trash", handler.GetTrashHandler).Name("dash.trash.list")
	dashGroup.Post("/trash/posts/:id/restore", handler.PostRestorePostHandler).Name("dash.trash.posts.restore")
	dashGroup.Post("/trash/posts/:id/delete", handler.PostHardDeletePostHandler).Name("dash.trash.posts.delete")
	dashGroup.Post("/trash/encrypted-posts/:id/restore", handler.PostRestoreEncryptedPostHandler).Name("dash.trash.encrypted_posts.restore")
	dashGroup.Post("/trash/encrypted-posts/:id/delete", handler.PostHardDeleteEncryptedPostHandler).Name("dash.trash.encrypted_posts.delete")
	dashGroup.Post("/trash/tags/:id/restore", handler.PostRestoreTagHandler).Name("dash.trash.tags.restore")
	dashGroup.Post("/trash/tags/:id/delete", handler.PostHardDeleteTagHandler).Name("dash.trash.tags.delete")
	dashGroup.Post("/trash/categories/:id/restore", handler.PostRestoreCategoryHandler).Name("dash.trash.categories.restore")
	dashGroup.Post("/trash/categories/:id/delete", handler.PostHardDeleteCategoryHandler).Name("dash.trash.categories.delete")
	dashGroup.Post("/trash/redirects/:id/restore", handler.PostRestoreRedirectHandler).Name("dash.trash.redirects.restore")
	dashGroup.Post("/trash/redirects/:id/delete", handler.PostHardDeleteRedirectHandler).Name("dash.trash.redirects.delete")
	dashGroup.Post("/api/trash/:type/batch-delete", handler.PostTrashBatchDeleteAPIHandler).Name("dash.trash.api.batch_delete")

	dashGroup.Get("/http-error-logs", handler.GetHttpErrorLogListHandler).Name("dash.http_error_logs.list")
	dashGroup.Post("/http-error-logs/:id/delete", handler.PostDeleteHttpErrorLogHandler).Name("dash.http_error_logs.delete")
	dashGroup.Post("/api/http-error-logs/batch-delete", handler.PostHttpErrorLogBatchDeleteAPIHandler).Name("dash.http_error_logs.api.batch_delete")

	dashGroup.Get("/tasks", handler.GetTaskListHandler).Name("dash.tasks.list")
	dashGroup.Get("/tasks/new", handler.GetTaskNewHandler).Name("dash.tasks.new")
	dashGroup.Post("/tasks/new", handler.PostCreateTaskHandler).Name("dash.tasks.create")
	dashGroup.Get("/tasks/:id/edit", handler.GetTaskEditHandler).Name("dash.tasks.edit")
	dashGroup.Post("/tasks/:id/edit", handler.PostUpdateTaskHandler).Name("dash.tasks.update")
	dashGroup.Post("/tasks/:id/delete", handler.PostDeleteTaskHandler).Name("dash.tasks.delete")
	dashGroup.Post("/api/tasks/batch-delete", handler.PostTaskBatchDeleteAPIHandler).Name("dash.tasks.api.batch_delete")
	dashGroup.Post("/tasks/:code/trigger", handler.PostTriggerTaskHandler).Name("dash.tasks.trigger")
	dashGroup.Get("/tasks/:code/runs", handler.GetTaskRunListHandler).Name("dash.tasks.runs")

	dashGroup.Get("/import", handler.GetImportHandler).Name("dash.import.show")
	dashGroup.Post("/import", handler.PostImportHandler).Name("dash.import.submit")
	dashGroup.Get("/import/parse-item", func(c fiber.Ctx) error {
		return c.Status(fiber.StatusMethodNotAllowed).JSON(fiber.Map{
			"ok":    false,
			"error": "method not allowed: use POST multipart/form-data",
		})
	})
	dashGroup.Post("/import/parse-item", handler.PostImportParseItemHandler).Name("dash.import.parse_item")
	dashGroup.Post("/import/save-item", handler.PostImportSaveItemHandler).Name("dash.import.save_item")
	dashGroup.Post("/import/confirm-item", handler.PostImportConfirmItemHandler).Name("dash.import.confirm_item")
	dashGroup.Post("/import/confirm-all", handler.PostImportConfirmAllHandler).Name("dash.import.confirm_all")
	dashGroup.Post("/import/cancel", handler.PostImportCancelHandler).Name("dash.import.cancel")

	dashGroup.Get("/export", handler.GetExportHandler).Name("dash.export.show")
	dashGroup.Get("/export/download", handler.GetExportDownloadHandler).Name("dash.export.download")
	dashGroup.Get("/backup-restore", handler.GetBackupRestoreHandler).Name("dash.backup_restore.show")
	dashGroup.Get("/backup-restore/status", handler.GetBackupRestoreStatusHandler).Name("dash.backup_restore.status")
	dashGroup.Post("/backup-restore/local", handler.PostExportRestoreLocalHandler).Name("dash.backup_restore.local")
	dashGroup.Post("/backup-restore/upload", handler.PostExportRestoreUploadHandler).Name("dash.backup_restore.upload")
	dashGroup.Post("/backup-restore/delete", handler.PostBackupRestoreDeleteHandler).Name("dash.backup_restore.delete")

	dashGroup.Get("/dev/ui-components", handler.GetDevUIComponentsHandler).Name("dash.dev.ui_components")
}
