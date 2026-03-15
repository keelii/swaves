package dash

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"time"

	"github.com/gofiber/fiber/v3"
)

func parseExpiresAtFromOption(option, customValue string) string {
	option = strings.TrimSpace(option)
	if option == "" || option == "none" {
		return ""
	}
	now := time.Now()
	switch option {
	case "1min":
		return strconv.FormatInt(now.Add(1*time.Minute).Unix(), 10)
	case "5min":
		return strconv.FormatInt(now.Add(5*time.Minute).Unix(), 10)
	case "1hour":
		return strconv.FormatInt(now.Add(time.Hour).Unix(), 10)
	case "1day":
		return strconv.FormatInt(now.Add(24*time.Hour).Unix(), 10)
	case "custom":
		customValue = strings.TrimSpace(customValue)
		if customValue == "" {
			return ""
		}
		var unix int64
		if t, err := time.ParseInLocation("2006-01-02T15:04", customValue, time.Local); err == nil {
			unix = t.Unix()
		} else if t, err := time.ParseInLocation("2006-01-02T15:04:05", customValue, time.Local); err == nil {
			unix = t.Unix()
		} else {
			return ""
		}
		if unix <= 0 {
			return ""
		}
		return strconv.FormatInt(unix, 10)
	default:
		return ""
	}
}

// Encrypted Posts
func (h *Handler) GetEncryptedPostListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	posts, err := ListEncryptedPosts(h.Model, &pager)
	if err != nil {
		return err
	}

	countPost, countPage, countEncryptedPost := CountPost(h.Model)

	return RenderDashView(c, "dash/encrypted_posts_index.html", fiber.Map{
		"Title":              "Encrypted Posts",
		"Posts":              posts,
		"Pager":              pager,
		"CountPost":          countPost,
		"CountPage":          countPage,
		"CountEncryptedPost": countEncryptedPost,
	}, "")
}

func (h *Handler) GetEncryptedPostNewHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/encrypted_posts_new.html", fiber.Map{
		"Title":   "New Encrypted Post",
		"SEditor": true,
	}, "")
}

func (h *Handler) PostCreateEncryptedPostHandler(c fiber.Ctx) error {
	expiresAtStr := parseExpiresAtFromOption(c.FormValue("expires_at_option"), c.FormValue("expires_at_custom"))

	in := CreateEncryptedPostInput{
		Title:     c.FormValue("title"),
		Content:   c.FormValue("content"),
		Password:  c.FormValue("password"),
		ExpiresAt: expiresAtStr,
	}

	if err := CreateEncryptedPostService(h.Model, in); err != nil {
		return err
	}

	return h.redirectToDashRoute(c, "dash.encrypted_posts.list", nil, nil)
}

func (h *Handler) GetEncryptedPostEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	post, err := GetEncryptedPostForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/encrypted_posts_edit.html", fiber.Map{
		"Title":   "Edit Encrypted Post",
		"SEditor": true,
		"Post":    post,
	}, "")
}

func (h *Handler) PostUpdateEncryptedPostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	expiresAtStr := parseExpiresAtFromOption(c.FormValue("expires_at_option"), c.FormValue("expires_at_custom"))
	if expiresAtStr == "" && c.FormValue("expires_at_option") == "" {
		// 编辑页未提交过期选项（只读展示），保留原有值
		post, err := db.GetEncryptedPostByID(h.Model, id)
		if err == nil && post.ExpiresAt != nil {
			expiresAtStr = strconv.FormatInt(*post.ExpiresAt, 10)
		}
	}

	in := UpdateEncryptedPostInput{
		Title:     c.FormValue("title"),
		Content:   c.FormValue("content"),
		Password:  c.FormValue("password"),
		ExpiresAt: expiresAtStr,
	}

	if err := UpdateEncryptedPostService(h.Model, id, in); err != nil {
		return err
	}

	return h.redirectToDashRoute(c, "dash.encrypted_posts.list", nil, nil)
}

func (h *Handler) PostDeleteEncryptedPostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteEncryptedPostService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToDashRouteKeepQuery(c, "dash.encrypted_posts.list", nil, nil)
}
