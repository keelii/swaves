package dash

import (
	"runtime"
	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/db"
	"swaves/internal/platform/updater"

	"github.com/gofiber/fiber/v3"
)

func (h *Handler) PostAppUpgradeAPIHandler(c fiber.Ctx) error {
	if runtime.GOOS == "windows" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "Windows 暂不支持 daemon-mode 自动更新",
		})
	}

	id, err := parseNotificationID(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "id 非法",
		})
	}

	result, err := updater.InstallLatestRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}
	if result.Installed {
		if err := MarkNotificationReadService(h.Model, id, dashNotificationReceiver); err != nil && !db.IsErrNotFound(err) {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"ok":    false,
				"error": "更新已开始，但标记通知已读失败",
			})
		}
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
			"installed":     result.Installed,
			"current":       result.CurrentVersion,
			"latest":        result.LatestVersion,
			"reason":        result.Reason,
			"release_url":   result.ReleaseURL,
			"restarted_pid": result.RestartedPID,
			"unread_count":  unreadCount,
		},
	})
}
