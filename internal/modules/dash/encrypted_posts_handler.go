package dash

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/md"
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
	return h.renderEncryptedPostNew(c, nil)
}

func encryptedDraftExpiresAtUnix(raw string) int64 {
	if raw == "" {
		return 0
	}
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0
	}
	return value
}

func (h *Handler) renderEncryptedPostNew(c fiber.Ctx, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	if _, ok := data["DraftTitle"]; !ok {
		data["DraftTitle"] = ""
	}
	if _, ok := data["DraftContent"]; !ok {
		data["DraftContent"] = ""
	}
	if _, ok := data["DraftPassword"]; !ok {
		data["DraftPassword"] = ""
	}
	if _, ok := data["DraftExpiresAtUnix"]; !ok {
		data["DraftExpiresAtUnix"] = int64(0)
	}
	if _, ok := data["DraftTOCHTML"]; !ok {
		draftContent, _ := data["DraftContent"].(string)
		data["DraftTOCHTML"] = md.ParseMarkdownTOC(draftContent)
	}
	data["Title"] = "New Encrypted Post"
	data["SEditor"] = true
	return RenderDashView(c, "dash/encrypted_posts_new.html", data, "")
}

func (h *Handler) renderEncryptedPostNewWithError(c fiber.Ctx, err error, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	if err != nil {
		data["Error"] = err.Error()
	}
	return h.renderEncryptedPostNew(c, data)
}

func (h *Handler) renderEncryptedPostEdit(c fiber.Ctx, id int64, data fiber.Map) error {
	post, err := GetEncryptedPostForEdit(h.Model, id)
	if err != nil {
		return err
	}
	if data == nil {
		data = fiber.Map{}
	}
	if _, ok := data["Post"]; !ok {
		data["Post"] = post
	}
	if _, ok := data["DraftPassword"]; !ok {
		data["DraftPassword"] = ""
	}
	if _, ok := data["PostTOCHTML"]; !ok {
		if postValue, ok := data["Post"].(*db.EncryptedPost); ok && postValue != nil {
			data["PostTOCHTML"] = md.ParseMarkdownTOC(postValue.Content)
		} else {
			data["PostTOCHTML"] = md.ParseMarkdownTOC(post.Content)
		}
	}
	data["Title"] = "Edit Encrypted Post"
	data["SEditor"] = true
	return RenderDashView(c, "dash/encrypted_posts_edit.html", data, "")
}

func (h *Handler) renderEncryptedPostEditWithError(c fiber.Ctx, id int64, err error, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	if err != nil {
		data["Error"] = err.Error()
	}
	return h.renderEncryptedPostEdit(c, id, data)
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
		return h.renderEncryptedPostNewWithError(c, err, fiber.Map{
			"DraftTitle":         in.Title,
			"DraftContent":       in.Content,
			"DraftPassword":      in.Password,
			"DraftExpiresAtUnix": encryptedDraftExpiresAtUnix(expiresAtStr),
			"DraftTOCHTML":       md.ParseMarkdownTOC(in.Content),
		})
	}

	return h.redirectToDashRoute(c, "dash.encrypted_posts.list", nil, nil)
}

func (h *Handler) GetEncryptedPostEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	return h.renderEncryptedPostEdit(c, id, nil)
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
		currentPost, getErr := db.GetEncryptedPostByID(h.Model, id)
		if getErr != nil {
			return err
		}
		currentPost.Title = in.Title
		currentPost.Content = in.Content
		return h.renderEncryptedPostEditWithError(c, id, err, fiber.Map{
			"Post":               currentPost,
			"DraftPassword":      in.Password,
			"DraftExpiresAtUnix": encryptedDraftExpiresAtUnix(expiresAtStr),
			"PostTOCHTML":        md.ParseMarkdownTOC(in.Content),
		})
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
