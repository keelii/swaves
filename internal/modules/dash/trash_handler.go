package dash

import (
	"strconv"

	"github.com/gofiber/fiber/v3"
)

func getTrashTabCounts(h *Handler) (map[string]int, error) {
	postCount, err := CountTrashPosts(h.Model)
	if err != nil {
		return nil, err
	}
	encryptedPostCount, err := CountTrashEncryptedPosts(h.Model)
	if err != nil {
		return nil, err
	}
	tagCount, err := CountTrashTags(h.Model)
	if err != nil {
		return nil, err
	}
	categoryCount, err := CountTrashCategories(h.Model)
	if err != nil {
		return nil, err
	}
	redirectCount, err := CountTrashRedirects(h.Model)
	if err != nil {
		return nil, err
	}

	return map[string]int{
		"posts":           postCount,
		"encrypted-posts": encryptedPostCount,
		"tags":            tagCount,
		"categories":      categoryCount,
		"redirects":       redirectCount,
	}, nil
}

// Trash
func (h *Handler) GetTrashHandler(c fiber.Ctx) error {
	// 获取当前选中的类型，默认为 posts
	modelType := c.Query("type", "posts")

	var data interface{}
	var err error

	switch modelType {
	case "posts":
		data, err = GetTrashPosts(h.Model)
	case "encrypted-posts":
		data, err = GetTrashEncryptedPosts(h.Model)
	case "tags":
		data, err = GetTrashTags(h.Model)
	case "categories":
		data, err = GetTrashCategories(h.Model)
	case "redirects":
		data, err = GetTrashRedirects(h.Model)
	default:
		data, err = GetTrashPosts(h.Model)
		modelType = "posts"
	}

	if err != nil {
		return err
	}

	tabCounts, err := getTrashTabCounts(h)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/trash_index.html", fiber.Map{
		"Title":          "Trash",
		"Data":           data,
		"ModelType":      modelType,
		"TrashTabCounts": tabCounts,
	}, "")
}

func (h *Handler) PostRestorePostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestorePostService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostHardDeletePostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeletePostService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostRestoreEncryptedPostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreEncryptedPostService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostHardDeleteEncryptedPostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteEncryptedPostService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostRestoreTagHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreTagService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostHardDeleteTagHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteTagService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostRestoreRedirectHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreRedirectService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostHardDeleteRedirectHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteRedirectService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostRestoreCategoryHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreCategoryService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}

func (h *Handler) PostHardDeleteCategoryHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteCategoryService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.trash.list", nil, nil)
}
