package notify

import (
	"fmt"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"time"
)

const (
	settingEnablePostLike     = "notify_enable_post_like"
	settingEnablePostComment  = "notify_enable_comment"
	settingEnableTaskSuccess  = "notify_enable_task_success"
	settingEnableTaskError    = "notify_enable_task_error"
	settingLikeAggregateMin   = "notify_like_aggregate_window_min"
	settingRetentionDays      = "notify_retention_days"
	taskNotifyMessageMaxLen   = 1024
	defaultLikeAggregateMin   = 30
	defaultNotificationRetain = 30
)

func IsPostLikeNotificationEnabled() bool {
	return store.GetSettingBool(settingEnablePostLike, true)
}

func IsCommentNotificationEnabled() bool {
	return store.GetSettingBool(settingEnablePostComment, true)
}

func IsTaskSuccessNotificationEnabled() bool {
	return store.GetSettingBool(settingEnableTaskSuccess, false)
}

func IsTaskErrorNotificationEnabled() bool {
	return store.GetSettingBool(settingEnableTaskError, true)
}

func NotificationRetentionDays() int {
	days := store.GetSettingInt(settingRetentionDays, defaultNotificationRetain)
	if days < 1 {
		days = 1
	}
	return days
}

func LikeAggregateWindowMinutes() int {
	windowMin := store.GetSettingInt(settingLikeAggregateMin, defaultLikeAggregateMin)
	if windowMin < 1 {
		windowMin = 1
	}
	if windowMin > 1440 {
		windowMin = 1440
	}
	return windowMin
}

func BuildPostLikeAggregateKey(postID int64, nowUnix int64) string {
	windowSeconds := int64(LikeAggregateWindowMinutes() * 60)
	if windowSeconds <= 0 {
		windowSeconds = int64(defaultLikeAggregateMin * 60)
	}
	bucketStart := nowUnix - (nowUnix % windowSeconds)
	return fmt.Sprintf("like:post:%d:bucket:%d", postID, bucketStart)
}

func CreatePostLikeNotification(dbx *db.DB, post db.Post, likeCount int, nowUnix int64) error {
	title := "文章收到新点赞"
	body := fmt.Sprintf("《%s》当前共有 %d 个赞。", normalizePostTitle(post), likeCount)
	if likeCount <= 1 {
		body = fmt.Sprintf("《%s》收到新的点赞。", normalizePostTitle(post))
	}

	n := &db.Notification{
		Receiver:     db.NotificationReceiverAdmin,
		EventType:    db.NotificationEventPostLike,
		Level:        db.NotificationLevelInfo,
		Title:        title,
		Body:         body,
		AggregateKey: BuildPostLikeAggregateKey(post.ID, nowUnix),
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}
	_, err := db.CreateOrBumpNotificationByAggregateKey(dbx, n)
	return err
}

func CreateCommentNotification(dbx *db.DB, post db.Post, comment db.Comment, nowUnix int64) error {
	title := "收到新留言"
	body := fmt.Sprintf("《%s》收到来自 %s 的留言。", normalizePostTitle(post), normalizeCommentAuthor(comment.Author))

	n := &db.Notification{
		Receiver:     db.NotificationReceiverAdmin,
		EventType:    db.NotificationEventComment,
		Level:        db.NotificationLevelInfo,
		Title:        title,
		Body:         body,
		AggregateKey: "",
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}
	_, err := db.CreateNotification(dbx, n)
	return err
}

func CreateTaskResultNotification(dbx *db.DB, task db.Task, status string, message string, nowUnix int64) error {
	taskName := strings.TrimSpace(task.Name)
	if taskName == "" {
		taskName = strings.TrimSpace(task.Code)
	}
	if taskName == "" {
		taskName = "未知任务"
	}

	trimmedMessage := strings.TrimSpace(message)
	if len(trimmedMessage) > taskNotifyMessageMaxLen {
		trimmedMessage = trimmedMessage[:taskNotifyMessageMaxLen] + "…"
	}

	level := db.NotificationLevelInfo
	title := fmt.Sprintf("任务执行成功：%s", taskName)
	body := fmt.Sprintf("任务 %s 执行成功。", taskName)
	if strings.EqualFold(strings.TrimSpace(status), "error") {
		level = db.NotificationLevelError
		title = fmt.Sprintf("任务执行失败：%s", taskName)
		body = fmt.Sprintf("任务 %s 执行失败。", taskName)
	}
	if trimmedMessage != "" {
		body += " " + trimmedMessage
	}

	n := &db.Notification{
		Receiver:  db.NotificationReceiverAdmin,
		EventType: db.NotificationEventTaskResult,
		Level:     level,
		Title:     title,
		Body:      body,
		CreatedAt: nowUnix,
		UpdatedAt: nowUnix,
	}
	_, err := db.CreateNotification(dbx, n)
	return err
}

func ExpiredBeforeUnix(now time.Time) int64 {
	days := NotificationRetentionDays()
	return now.Add(-time.Duration(days) * 24 * time.Hour).Unix()
}

func normalizePostTitle(post db.Post) string {
	title := strings.TrimSpace(post.Title)
	if title != "" {
		return title
	}
	slug := strings.TrimSpace(post.Slug)
	if slug != "" {
		return slug
	}
	return "未命名文章"
}

func normalizeCommentAuthor(author string) string {
	author = strings.TrimSpace(author)
	if author == "" {
		return "匿名用户"
	}
	return author
}
