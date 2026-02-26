package sui

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/share"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

func normalizeCommentStatus(raw string) db.CommentStatus {
	switch strings.TrimSpace(raw) {
	case string(db.CommentStatusPending):
		return db.CommentStatusPending
	case string(db.CommentStatusApproved):
		return db.CommentStatusApproved
	case string(db.CommentStatusSpam):
		return db.CommentStatusSpam
	default:
		return ""
	}
}

func (h *Handler) buildCommentListURL(c fiber.Ctx, status db.CommentStatus) string {
	query := map[string]string{}
	if status != "" {
		query["status"] = string(status)
	}

	commentURL := h.adminRouteURL(c, "admin.comments.list", nil, query)
	if commentURL == "" {
		return share.BuildAdminPath("/comments")
	}
	return commentURL
}

// Comments
func (h *Handler) GetCommentListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	status := normalizeCommentStatus(c.Query("status"))

	comments, err := ListCommentsService(h.Model, status, &pager)
	if err != nil {
		return err
	}

	return RenderSUIView(c, "comments_index", fiber.Map{
		"Title":        "评论管理",
		"Comments":     comments,
		"Pager":        pager,
		"StatusFilter": string(status),
	}, "")
}

func (h *Handler) PostApproveCommentHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = UpdateCommentStatusService(h.Model, id, db.CommentStatusApproved); err != nil {
		return err
	}

	return webutil.RedirectTo(c, h.buildCommentListURL(c, normalizeCommentStatus(c.FormValue("status_filter"))))
}

func (h *Handler) PostPendingCommentHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = UpdateCommentStatusService(h.Model, id, db.CommentStatusPending); err != nil {
		return err
	}

	return webutil.RedirectTo(c, h.buildCommentListURL(c, normalizeCommentStatus(c.FormValue("status_filter"))))
}

func (h *Handler) PostSpamCommentHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = UpdateCommentStatusService(h.Model, id, db.CommentStatusSpam); err != nil {
		return err
	}

	return webutil.RedirectTo(c, h.buildCommentListURL(c, normalizeCommentStatus(c.FormValue("status_filter"))))
}

func (h *Handler) PostDeleteCommentHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = DeleteCommentService(h.Model, id); err != nil {
		return err
	}

	return webutil.RedirectTo(c, h.buildCommentListURL(c, normalizeCommentStatus(c.FormValue("status_filter"))))
}
