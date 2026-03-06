package admin

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

const adminNotificationReceiver = db.NotificationReceiverAdmin

type notificationReadRequest struct {
	ID int64 `json:"id"`
}

func parseNotificationID(c fiber.Ctx) (int64, error) {
	var req notificationReadRequest
	_ = c.Bind().Body(&req)
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
	pager := middleware.GetPagination(c)
	notifications, err := ListNotificationsService(h.Model, adminNotificationReceiver, &pager)
	if err != nil {
		return err
	}
	unreadCount, err := CountUnreadNotificationsService(h.Model, adminNotificationReceiver)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "dash/notifications_index.html", fiber.Map{
		"Title":         "通知中心",
		"Notifications": notifications,
		"UnreadCount":   unreadCount,
		"Pager":         pager,
	}, "")
}

func (h *Handler) GetNotificationListAPIHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	notifications, err := ListNotificationsService(h.Model, adminNotificationReceiver, &pager)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取通知列表失败",
		})
	}
	unreadCount, err := CountUnreadNotificationsService(h.Model, adminNotificationReceiver)
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
			"unread_count": unreadCount,
		},
	})
}

func (h *Handler) GetNotificationUnreadCountAPIHandler(c fiber.Ctx) error {
	unreadCount, err := CountUnreadNotificationsService(h.Model, adminNotificationReceiver)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "获取未读数量失败",
		})
	}

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

	if err := MarkNotificationReadService(h.Model, id, adminNotificationReceiver); err != nil {
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

	unreadCount, err := CountUnreadNotificationsService(h.Model, adminNotificationReceiver)
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
	updatedCount, err := MarkAllNotificationsReadService(h.Model, adminNotificationReceiver)
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

	if err := DeleteNotificationService(h.Model, id, adminNotificationReceiver); err != nil {
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

	unreadCount, err := CountUnreadNotificationsService(h.Model, adminNotificationReceiver)
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
