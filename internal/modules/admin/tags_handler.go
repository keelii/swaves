package admin

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/helper"

	"github.com/gofiber/fiber/v3"
)

// Tags
func (h *Handler) GetTagListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	tags, err := ListTags(h.Model, &pager)
	if err != nil {
		return err
	}

	// 统计每个标签的文章数量
	tagIDs := make([]int64, len(tags))
	for i, tag := range tags {
		tagIDs[i] = tag.ID
	}
	postCounts, err := db.CountPostsByTags(h.Model, tagIDs)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "dash/tags_index.html", fiber.Map{
		"Title":      "Tags",
		"Tags":       tags,
		"Pager":      pager,
		"PostCounts": postCounts,
	}, "")
}

func (h *Handler) GetTagNewHandler(c fiber.Ctx) error {
	return RenderAdminView(c, "dash/tags_new.html", fiber.Map{
		"Title": "New Tag",
	}, "")
}

func (h *Handler) PostCreateTagHandler(c fiber.Ctx) error {
	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		return RenderAdminView(c, "dash/tags_new.html", fiber.Map{
			"Title": "New Tag",
			"Error": errSlugInvalid("011", slug).Error(),
		}, "")
	}
	in := CreateTagInput{
		Name: c.FormValue("name"),
		Slug: slug,
	}

	if err := CreateTagService(h.Model, in); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.tags.list", nil, nil)
}

func (h *Handler) GetTagEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	tag, err := GetTagForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "dash/tags_edit.html", fiber.Map{
		"Title": "Edit Tag",
		"Tag":   tag,
	}, "")
}

func (h *Handler) PostUpdateTagHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		tag, _ := GetTagForEdit(h.Model, id)
		return RenderAdminView(c, "dash/tags_edit.html", fiber.Map{
			"Title": "Edit Tag",
			"Tag":   tag,
			"Error": errSlugInvalid("012", slug).Error(),
		}, "")
	}
	in := UpdateTagInput{
		Name: c.FormValue("name"),
		Slug: slug,
	}

	if err := UpdateTagService(h.Model, id, in); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.tags.list", nil, nil)
}

func (h *Handler) PostDeleteTagHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteTagService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.tags.list", nil, nil)
}
