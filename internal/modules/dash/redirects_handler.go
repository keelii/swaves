package dash

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

// Redirects
func (h *Handler) GetRedirectListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	redirects, err := ListRedirects(h.Model, &pager)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/redirects_index.html", fiber.Map{
		"Title":     "Redirects",
		"Redirects": redirects,
		"Pager":     pager,
	}, "")
}

type redirectTargetOptionView struct {
	ID        int64
	Title     string
	URL       string
	KindLabel string
}

func parseRedirectStatus(raw string) int {
	status, _ := parseRedirectStatusStrict(raw)
	return status
}

func parseRedirectStatusStrict(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 301, nil
	}
	status, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || (status != 301 && status != 302) {
		return 0, fiber.ErrBadRequest
	}
	return status, nil
}

func (h *Handler) loadRedirectTargetOptions() []redirectTargetOptionView {
	pager := types.Pagination{
		Page:     1,
		PageSize: 200,
	}
	posts, err := db.ListPosts(h.Model, &db.PostQueryOptions{Pager: &pager})
	if err != nil {
		logger.Error("[redirect] list posts for target options failed: %v", err)
		return []redirectTargetOptionView{}
	}

	options := make([]redirectTargetOptionView, 0, len(posts))
	seenURL := make(map[string]bool, len(posts))
	for _, item := range posts {
		if item.Post == nil || item.Post.Status != "published" {
			continue
		}

		postURL := strings.TrimSpace(share.GetPostUrl(*item.Post))
		if postURL == "" || seenURL[postURL] {
			continue
		}
		seenURL[postURL] = true

		title := strings.TrimSpace(item.Post.Title)
		if title == "" {
			title = strings.TrimSpace(item.Post.Slug)
		}
		if title == "" {
			title = postURL
		}

		kindLabel := "文章"
		if item.Post.Kind == db.PostKindPage {
			kindLabel = "页面"
		}

		options = append(options, redirectTargetOptionView{
			ID:        item.Post.ID,
			Title:     title,
			URL:       postURL,
			KindLabel: kindLabel,
		})
	}
	return options
}

func (h *Handler) GetRedirectNewHandler(c fiber.Ctx) error {
	status, err := parseRedirectStatusStrict(c.Query("status"))
	if err != nil {
		return fiber.ErrBadRequest
	}
	draft := &db.Redirect{
		From:    strings.TrimSpace(c.Query("from")),
		To:      strings.TrimSpace(c.Query("to")),
		Status:  status,
		Enabled: 1,
	}

	return RenderDashView(c, "dash/redirects_new.html", fiber.Map{
		"Title":                 "New Redirect",
		"Redirect":              draft,
		"RedirectTargetOptions": h.loadRedirectTargetOptions(),
	}, "")
}

func (h *Handler) PostCreateRedirectHandler(c fiber.Ctx) error {
	status, err := parseRedirectStatusStrict(c.FormValue("status"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	in := CreateRedirectInput{
		From:    c.FormValue("from"),
		To:      c.FormValue("to"),
		Status:  status,
		Enabled: enabledInt,
	}

	if err := CreateRedirectService(h.Model, in); err != nil {
		draft := &db.Redirect{
			From:    in.From,
			To:      in.To,
			Status:  in.Status,
			Enabled: in.Enabled,
		}
		return RenderDashView(c, "dash/redirects_new.html", fiber.Map{
			"Title":                 "New Redirect",
			"Error":                 err.Error(),
			"Redirect":              draft,
			"RedirectTargetOptions": h.loadRedirectTargetOptions(),
		}, "")
	}

	return h.redirectToDashRoute(c, "dash.redirects.list", nil, nil)
}

func (h *Handler) GetRedirectEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	redirect, err := GetRedirectForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/redirects_edit.html", fiber.Map{
		"Title":    "Edit Redirect",
		"Redirect": redirect,
	}, "")
}

func (h *Handler) PostUpdateRedirectHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	status, err := parseRedirectStatusStrict(c.FormValue("status"))
	if err != nil {
		return fiber.ErrBadRequest
	}

	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	in := UpdateRedirectInput{
		From:    c.FormValue("from"),
		To:      c.FormValue("to"),
		Status:  status,
		Enabled: enabledInt,
	}

	if err := UpdateRedirectService(h.Model, id, in); err != nil {
		return err
	}

	return h.redirectToDashRoute(c, "dash.redirects.list", nil, nil)
}

func (h *Handler) PostDeleteRedirectHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteRedirectService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.redirects.list", nil, nil)
}
