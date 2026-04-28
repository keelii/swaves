package dash

import (
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"

	"github.com/gofiber/fiber/v3"
)

type batchDeletePayload struct {
	IDs []int64 `json:"ids"`
}

type batchOperationConfig struct {
	action   string
	scope    string
	countKey string
	idsKey   string
	runByID  func(int64) error
}

func normalizeBatchDeleteIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	seen := make(map[int64]struct{}, len(ids))
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func batchDeleteErrorStatus(err error) int {
	if err == nil {
		return fiber.StatusOK
	}
	if db.IsErrNotFound(err) {
		return fiber.StatusNotFound
	}
	if db.IsErrInternalError(err) {
		return fiber.StatusInternalServerError
	}
	return fiber.StatusBadRequest
}

func (h *Handler) runBatchOperation(c fiber.Ctx, config batchOperationConfig) error {
	var payload batchDeletePayload
	if err := c.Bind().Body(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "invalid json",
		})
	}

	ids := normalizeBatchDeleteIDs(payload.IDs)
	if len(ids) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "ids is required",
		})
	}

	successIDs := make([]int64, 0, len(ids))
	failed := make([]fiber.Map, 0)

	for _, id := range ids {
		if err := config.runByID(id); err != nil {
			status := batchDeleteErrorStatus(err)
			logger.Warn("[batch-%s] scope=%s id=%d status=%d err=%v", config.action, config.scope, id, status, err)
			failed = append(failed, fiber.Map{
				"id":     id,
				"status": status,
				"error":  err.Error(),
			})
			continue
		}
		successIDs = append(successIDs, id)
	}

	response := fiber.Map{
		"ok":              len(failed) == 0,
		"requested_count": len(ids),
		"failed_count":    len(failed),
		"failed":          failed,
	}
	response[config.countKey] = len(successIDs)
	response[config.idsKey] = successIDs

	if len(failed) > 0 {
		return c.Status(fiber.StatusMultiStatus).JSON(response)
	}
	return c.JSON(response)
}

func (h *Handler) runBatchDelete(c fiber.Ctx, scope string, deleteByID func(int64) error) error {
	return h.runBatchOperation(c, batchOperationConfig{
		action:   "delete",
		scope:    scope,
		countKey: "deleted_count",
		idsKey:   "deleted_ids",
		runByID:  deleteByID,
	})
}

func (h *Handler) runBatchRestore(c fiber.Ctx, scope string, restoreByID func(int64) error) error {
	return h.runBatchOperation(c, batchOperationConfig{
		action:   "restore",
		scope:    scope,
		countKey: "restored_count",
		idsKey:   "restored_ids",
		runByID:  restoreByID,
	})
}

func (h *Handler) PostPostBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "posts", func(id int64) error {
		return DeletePostService(h.Model, id)
	})
}

func (h *Handler) PostCommentBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "comments", func(id int64) error {
		return DeleteCommentService(h.Model, id)
	})
}

func (h *Handler) PostTagBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "tags", func(id int64) error {
		return DeleteTagService(h.Model, id)
	})
}

func (h *Handler) PostCategoryBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "categories", func(id int64) error {
		return DeleteCategoryService(h.Model, id)
	})
}

func (h *Handler) PostRedirectBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "redirects", func(id int64) error {
		return DeleteRedirectService(h.Model, id)
	})
}

func (h *Handler) PostEncryptedPostBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "encrypted_posts", func(id int64) error {
		return DeleteEncryptedPostService(h.Model, id)
	})
}

func (h *Handler) PostSettingBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "settings", func(id int64) error {
		return DeleteSettingService(h.Model, id)
	})
}

func (h *Handler) PostTaskBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "tasks", func(id int64) error {
		return DeleteTaskService(h.Model, id)
	})
}

func (h *Handler) PostTaskRunBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "task_runs", func(id int64) error {
		return DeleteTaskRunService(h.Model, id)
	})
}

func (h *Handler) PostNotificationBatchDeleteAPIHandler(c fiber.Ctx) error {
	return h.runBatchDelete(c, "notifications", func(id int64) error {
		return DeleteNotificationService(h.Model, id, dashNotificationReceiver)
	})
}

func (h *Handler) PostTrashBatchDeleteAPIHandler(c fiber.Ctx) error {
	modelType := strings.TrimSpace(c.Params("type"))
	var deleteByID func(int64) error

	switch modelType {
	case "posts":
		deleteByID = func(id int64) error { return HardDeletePostService(h.Model, id) }
	case "encrypted-posts":
		deleteByID = func(id int64) error { return HardDeleteEncryptedPostService(h.Model, id) }
	case "tags":
		deleteByID = func(id int64) error { return HardDeleteTagService(h.Model, id) }
	case "categories":
		deleteByID = func(id int64) error { return HardDeleteCategoryService(h.Model, id) }
	case "redirects":
		deleteByID = func(id int64) error { return HardDeleteRedirectService(h.Model, id) }
	case "themes":
		deleteByID = func(id int64) error { return HardDeleteThemeService(h.Model, id) }
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "invalid trash type",
		})
	}

	return h.runBatchDelete(c, "trash."+modelType, deleteByID)
}

func (h *Handler) PostTrashBatchRestoreAPIHandler(c fiber.Ctx) error {
	modelType := strings.TrimSpace(c.Params("type"))
	var restoreByID func(int64) error

	switch modelType {
	case "posts":
		restoreByID = func(id int64) error { return RestorePostService(h.Model, id) }
	case "encrypted-posts":
		restoreByID = func(id int64) error { return RestoreEncryptedPostService(h.Model, id) }
	case "tags":
		restoreByID = func(id int64) error { return RestoreTagService(h.Model, id) }
	case "categories":
		restoreByID = func(id int64) error { return RestoreCategoryService(h.Model, id) }
	case "redirects":
		restoreByID = func(id int64) error { return RestoreRedirectService(h.Model, id) }
	case "themes":
		restoreByID = func(id int64) error { return RestoreThemeService(h.Model, id) }
	default:
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "invalid trash type",
		})
	}

	return h.runBatchRestore(c, "trash.restore."+modelType, restoreByID)
}
