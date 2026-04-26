package dash

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/share"

	"github.com/gofiber/fiber/v3"
)

const dashNotificationReceiver = db.NotificationReceiverDash

const (
	dashNotificationEventAll        = ""
	dashNotificationEventPostLike   = db.NotificationEventPostLike
	dashNotificationEventComment    = db.NotificationEventComment
	dashNotificationEventTaskResult = db.NotificationEventTaskResult
	dashNotificationEventAppUpdate  = db.NotificationEventAppUpdate
	dashCommentLinkKeyPrefix        = "comment_link:"
)

type NotificationListItemView struct {
	ID               int64
	EventType        string
	Title            string
	Body             string
	AggregateCount   int
	ReadAt           *int64
	UpdatedAt        int64
	CommentURL       string
	CommentInNewTab  bool
	AppUpdatePageURL string
}

func parseCommentURLFromAggregateKey(raw string) string {
	text := strings.TrimSpace(raw)
	if text == "" || !strings.HasPrefix(text, dashCommentLinkKeyPrefix) {
		return ""
	}

	payload := strings.TrimPrefix(text, dashCommentLinkKeyPrefix)
	splitAt := strings.Index(payload, ":")
	if splitAt <= 0 || splitAt >= len(payload)-1 {
		return ""
	}

	escapedURL := payload[splitAt+1:]
	commentURL, err := url.QueryUnescape(escapedURL)
	if err != nil {
		return ""
	}
	commentURL = strings.TrimSpace(commentURL)
	if commentURL == "" {
		return ""
	}
	parsedURL, err := url.Parse(commentURL)
	if err != nil {
		return ""
	}
	// Keep links inside current site and reject potentially dangerous schemes.
	if parsedURL.IsAbs() || strings.TrimSpace(parsedURL.Host) != "" {
		return ""
	}
	if !strings.HasPrefix(parsedURL.Path, "/") {
		return ""
	}
	return commentURL
}

func buildNotificationListItems(notifications []db.Notification, defaultCommentURL string, appUpdatePageURL string) []NotificationListItemView {
	items := make([]NotificationListItemView, 0, len(notifications))
	defaultCommentURL = strings.TrimSpace(defaultCommentURL)
	for _, n := range notifications {
		item := NotificationListItemView{
			ID:             n.ID,
			EventType:      n.EventType,
			Title:          n.Title,
			Body:           n.Body,
			AggregateCount: n.AggregateCount,
			ReadAt:         n.ReadAt,
			UpdatedAt:      n.UpdatedAt,
		}
		if n.EventType == dashNotificationEventComment {
			item.CommentURL = parseCommentURLFromAggregateKey(n.AggregateKey)
			if item.CommentURL == "" {
				item.CommentURL = defaultCommentURL
			}
			item.CommentInNewTab = isWebSideRelativePath(item.CommentURL)
		}
		if n.EventType == dashNotificationEventAppUpdate {
			item.AppUpdatePageURL = appUpdatePageURL
		}
		items = append(items, item)
	}
	return items
}

func isWebSideRelativePath(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return false
	}

	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return false
	}

	// Only classify same-origin relative paths; absolute URLs are rejected at parseCommentURLFromAggregateKey.
	if parsedURL.IsAbs() || strings.TrimSpace(parsedURL.Host) != "" {
		return false
	}
	path := strings.TrimSpace(parsedURL.Path)
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}

	dashBasePath := share.GetDashUrl()
	if path == dashBasePath || strings.HasPrefix(path, dashBasePath+"/") {
		return false
	}
	return true
}

func normalizeNotificationEventTypeFilter(raw string) string {
	switch strings.TrimSpace(raw) {
	case dashNotificationEventPostLike:
		return dashNotificationEventPostLike
	case dashNotificationEventComment:
		return dashNotificationEventComment
	case dashNotificationEventTaskResult:
		return dashNotificationEventTaskResult
	case dashNotificationEventAppUpdate:
		return dashNotificationEventAppUpdate
	default:
		return dashNotificationEventAll
	}
}

func getNotificationTabCounts(dbx *db.DB, receiver string) (map[string]int, error) {
	totalCount, err := CountNotificationsByEventTypeService(dbx, receiver, dashNotificationEventAll)
	if err != nil {
		return nil, err
	}
	postLikeCount, err := CountNotificationsByEventTypeService(dbx, receiver, dashNotificationEventPostLike)
	if err != nil {
		return nil, err
	}
	commentCount, err := CountNotificationsByEventTypeService(dbx, receiver, dashNotificationEventComment)
	if err != nil {
		return nil, err
	}
	taskResultCount, err := CountNotificationsByEventTypeService(dbx, receiver, dashNotificationEventTaskResult)
	if err != nil {
		return nil, err
	}
	appUpdateCount, err := CountNotificationsByEventTypeService(dbx, receiver, dashNotificationEventAppUpdate)
	if err != nil {
		return nil, err
	}

	return map[string]int{
		"all":         totalCount,
		"post_like":   postLikeCount,
		"comment":     commentCount,
		"task_result": taskResultCount,
		"app_update":  appUpdateCount,
	}, nil
}

type notificationReadRequest struct {
	ID int64 `json:"id"`
}

func parseNotificationID(c fiber.Ctx) (int64, error) {
	var req notificationReadRequest
	if err := c.Bind().Body(&req); err != nil {
		return 0, fiber.ErrBadRequest
	}
	if req.ID > 0 {
		return req.ID, nil
	}
	rawID := strings.TrimSpace(c.FormValue("id"))
	if rawID == "" {
		return 0, fiber.ErrBadRequest
	}
	parsedID, err := strconv.ParseInt(rawID, 10, 64)
	if err != nil || parsedID <= 0 {
		return 0, fiber.ErrBadRequest
	}
	return parsedID, nil
}

func (h *Handler) GetNotificationListHandler(c fiber.Ctx) error {
	eventType := normalizeNotificationEventTypeFilter(c.Query("event_type"))
	pager := middleware.GetPagination(c)
	notifications, err := ListNotificationsService(h.Model, dashNotificationReceiver, eventType, &pager)
	if err != nil {
		return err
	}
	unreadCount, err := CountUnreadNotificationsService(h.Model, dashNotificationReceiver)
	if err != nil {
		return err
	}
	tabCounts, err := getNotificationTabCounts(h.Model, dashNotificationReceiver)
	if err != nil {
		return err
	}
	commentListURL := h.dashRouteURL(c, "dash.comments.list", nil, nil)
	if strings.TrimSpace(commentListURL) == "" {
		return fmt.Errorf("resolve route failed: dash.comments.list")
	}
	appUpdatePageURL := h.dashRouteURL(c, "dash.settings.system_update", nil, nil)
	if appUpdatePageURL == "" {
		return fmt.Errorf("resolve route failed: dash.settings.system_update")
	}
	notificationItems := buildNotificationListItems(notifications, commentListURL, appUpdatePageURL)

	return RenderDashView(c, "dash/notifications_index.html", fiber.Map{
		"Title":                 "通知中心",
		"Notifications":         notificationItems,
		"UnreadCount":           unreadCount,
		"NotificationEventType": eventType,
		"NotificationTabCounts": tabCounts,
		"Pager":                 pager,
	}, "")
}

func (h *Handler) GetNotificationListAPIHandler(c fiber.Ctx) error {
	eventType := normalizeNotificationEventTypeFilter(c.Query("event_type"))
	pager := middleware.GetPagination(c)
	notifications, err := ListNotificationsService(h.Model, dashNotificationReceiver, eventType, &pager)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取通知列表失败",
		})
	}
	unreadCount, err := CountUnreadNotificationsService(h.Model, dashNotificationReceiver)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取未读数量失败",
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"items": notifications,
			"pager": fiber.Map{
				"page":      pager.Page,
				"page_size": pager.PageSize,
				"total":     pager.Total,
				"num":       pager.Num,
			},
			"event_type":   eventType,
			"unread_count": unreadCount,
		},
	})
}

func (h *Handler) GetNotificationUnreadCountAPIHandler(c fiber.Ctx) error {
	unreadCount, err := CountUnreadNotificationsService(h.Model, dashNotificationReceiver)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取未读数量失败",
		})
	}

	c.Set(fiber.HeaderCacheControl, "no-store, no-cache, must-revalidate")
	c.Set(fiber.HeaderPragma, "no-cache")
	c.Set(fiber.HeaderExpires, "0")

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"unread_count": unreadCount,
		},
	})
}

func (h *Handler) PostNotificationReadAPIHandler(c fiber.Ctx) error {
	id, err := parseNotificationID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "id 非法",
		})
	}

	if err := MarkNotificationReadService(h.Model, id, dashNotificationReceiver); err != nil {
		if db.IsErrNotFound(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"ok":    false,
				"error": "通知不存在",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "标记已读失败",
		})
	}

	unreadCount, err := CountUnreadNotificationsService(h.Model, dashNotificationReceiver)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取未读数量失败",
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"id":           id,
			"unread_count": unreadCount,
		},
	})
}

func (h *Handler) PostNotificationReadAllAPIHandler(c fiber.Ctx) error {
	updatedCount, err := MarkAllNotificationsReadService(h.Model, dashNotificationReceiver)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "全部已读失败",
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"updated_count": updatedCount,
			"unread_count":  0,
		},
	})
}

func (h *Handler) PostNotificationDeleteAPIHandler(c fiber.Ctx) error {
	id, err := parseNotificationID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "id 非法",
		})
	}

	if err := DeleteNotificationService(h.Model, id, dashNotificationReceiver); err != nil {
		if db.IsErrNotFound(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"ok":    false,
				"error": "通知不存在或未读",
			})
		}
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "删除通知失败",
		})
	}

	unreadCount, err := CountUnreadNotificationsService(h.Model, dashNotificationReceiver)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取未读数量失败",
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"id":           id,
			"unread_count": unreadCount,
		},
	})
}
