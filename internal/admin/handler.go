package admin

import (
	"strconv"
	"strings"
	"swaves/internal/db"
	"swaves/internal/middleware"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	DB      *db.DB
	Service *Service
	Store   *SessionStore
}

func NewHandler(db *db.DB, adminService *Service, store *SessionStore) *Handler {
	return &Handler{
		DB:      db,
		Service: adminService,
		Store:   store,
	}
}

func (h *Handler) GetHome(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err != nil {
		return err
	}
	logined := sess.Get("admin")
	return c.Render("admin_home", fiber.Map{
		"Title":   "Admin Home",
		"IsLogin": logined,
	}, "admin_layout")
}

/* ---------- GET /admin/login ---------- */

func (h *Handler) GetLoginHandler(c *fiber.Ctx) error {
	return c.Render("admin_login", fiber.Map{
		"Title": "Admin Login",
		"Error": "",
	}, "admin_layout")
}

/* ---------- POST /admin/login ---------- */

func (h *Handler) PostLoginHandler(c *fiber.Ctx) error {
	password := c.FormValue("password")
	if password == "" {
		return c.Render("admin_login", fiber.Map{
			"Error": "password is empty",
		}, "admin_layout")
	}

	if err := h.Service.CheckPassword(password); err != nil {
		return c.Render("admin_login", fiber.Map{
			"Title": "Admin Login",
			"Error": "Invalid password",
		}, "admin_layout")
	}

	sess, err := h.Store.Get(c)
	if err != nil {
		return err
	}

	sess.Set("admin", true)

	if err := sess.Save(); err != nil {
		return err
	}

	return c.Redirect("/admin")
}

/* ---------- POST /admin/logout ---------- */

func (h *Handler) GetLogoutHandler(c *fiber.Ctx) error {
	sess, err := h.Store.Get(c)
	if err == nil {
		sess.Destroy()
	}

	return c.Redirect("/admin/login")
}

// Posts
func (h *Handler) GetPostListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	posts, err := ListPosts(h.DB, &pager)
	if err != nil {
		return err
	}

	return c.Render("posts_index", fiber.Map{
		"Posts": posts,
		"Pager": pager,
	}, "admin_layout")
}
func (h *Handler) GetPostNewHandler(c *fiber.Ctx) error {
	tags, err := GetAllTags(h.DB)
	if err != nil {
		return err
	}

	return c.Render("posts_new", fiber.Map{
		"Tags": tags,
	}, "admin_layout")
}

func (h *Handler) PostCreatePostHandler(c *fiber.Ctx) error {
	// 解析标签 ID（逗号分割）
	var tagIDs []int64
	tagsStr := c.FormValue("tags")
	if tagsStr != "" {
		// 按逗号分割
		tagIDStrs := strings.Split(tagsStr, ",")
		for _, tagIDStr := range tagIDStrs {
			tagIDStr = strings.TrimSpace(tagIDStr)
			if tagIDStr != "" {
				if tagID, err := strconv.ParseInt(tagIDStr, 10, 64); err == nil {
					tagIDs = append(tagIDs, tagID)
				}
			}
		}
	}

	// 处理新标签（逗号分割的标签名称）
	newTagsStr := c.FormValue("new_tags")
	if newTagsStr != "" {
		tagNames := strings.Split(newTagsStr, ",")
		for _, tagName := range tagNames {
			tagName = strings.TrimSpace(tagName)
			if tagName != "" {
				tag, err := CreateTagByName(h.DB, tagName)
				if err == nil {
					tagIDs = append(tagIDs, tag.ID)
				}
			}
		}
	}

	in := CreatePostInput{
		Title:   c.FormValue("title"),
		Slug:    c.FormValue("slug"),
		Content: c.FormValue("content"),
		Status:  c.FormValue("status"),
		TagIDs:  tagIDs,
	}

	if err := CreatePostService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

func (h *Handler) GetPostEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	postWithTags, err := GetPostForEdit(h.DB, id)
	if err != nil {
		return err
	}

	allTags, err := GetAllTags(h.DB)
	if err != nil {
		return err
	}

	// 创建已选标签的 map 以便在模板中快速查找
	selectedTagIDs := make(map[int64]bool)
	for _, tag := range postWithTags.Tags {
		selectedTagIDs[tag.ID] = true
	}

	return c.Render("posts_edit", fiber.Map{
		"Post":           postWithTags.Post,
		"Tags":           allTags,
		"SelectedTags":   postWithTags.Tags,
		"SelectedTagIDs": selectedTagIDs,
	}, "admin_layout")
}

func (h *Handler) PostUpdatePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// 解析标签 ID（逗号分割）
	var tagIDs []int64
	tagsStr := c.FormValue("tags")
	if tagsStr != "" {
		// 按逗号分割
		tagIDStrs := strings.Split(tagsStr, ",")
		for _, tagIDStr := range tagIDStrs {
			tagIDStr = strings.TrimSpace(tagIDStr)
			if tagIDStr != "" {
				if tagID, err := strconv.ParseInt(tagIDStr, 10, 64); err == nil {
					tagIDs = append(tagIDs, tagID)
				}
			}
		}
	}

	in := UpdatePostInput{
		Title:   c.FormValue("title"),
		Content: c.FormValue("content"),
		Status:  c.FormValue("status"),
		TagIDs:  tagIDs,
	}

	if err := UpdatePostService(h.DB, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

func (h *Handler) PostDeletePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeletePostService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

// Tags
func (h *Handler) GetTagListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	tags, err := ListTags(h.DB, &pager)
	if err != nil {
		return err
	}

	return c.Render("tags_index", fiber.Map{
		"Tags":  tags,
		"Pager": pager,
	}, "admin_layout")
}

func (h *Handler) GetTagNewHandler(c *fiber.Ctx) error {
	return c.Render("tags_new", nil, "admin_layout")
}

func (h *Handler) PostCreateTagHandler(c *fiber.Ctx) error {
	in := CreateTagInput{
		Name: c.FormValue("name"),
		Slug: c.FormValue("slug"),
	}

	if err := CreateTagService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/tags")
}

func (h *Handler) GetTagEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	tag, err := GetTagForEdit(h.DB, id)
	if err != nil {
		return err
	}

	return c.Render("tags_edit", fiber.Map{
		"Tag": tag,
	}, "admin_layout")
}

func (h *Handler) PostUpdateTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	in := UpdateTagInput{
		Name: c.FormValue("name"),
		Slug: c.FormValue("slug"),
	}

	if err := UpdateTagService(h.DB, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/tags")
}

func (h *Handler) PostDeleteTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteTagService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/tags")
}

// Redirects
func (h *Handler) GetRedirectListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	redirects, err := ListRedirects(h.DB, &pager)
	if err != nil {
		return err
	}

	return c.Render("redirects_index", fiber.Map{
		"Redirects": redirects,
		"Pager":     pager,
	}, "admin_layout")
}

func (h *Handler) GetRedirectNewHandler(c *fiber.Ctx) error {
	return c.Render("redirects_new", nil, "admin_layout")
}

func (h *Handler) PostCreateRedirectHandler(c *fiber.Ctx) error {
	in := CreateRedirectInput{
		From: c.FormValue("from"),
		To:   c.FormValue("to"),
	}

	if err := CreateRedirectService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/redirects")
}

func (h *Handler) GetRedirectEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	redirect, err := GetRedirectForEdit(h.DB, id)
	if err != nil {
		return err
	}

	return c.Render("redirects_edit", fiber.Map{
		"Redirect": redirect,
	}, "admin_layout")
}

func (h *Handler) PostUpdateRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	in := UpdateRedirectInput{
		From: c.FormValue("from"),
		To:   c.FormValue("to"),
	}

	if err := UpdateRedirectService(h.DB, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/redirects")
}

func (h *Handler) PostDeleteRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteRedirectService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/redirects")
}

// Encrypted Posts
func (h *Handler) GetEncryptedPostListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	posts, err := ListEncryptedPosts(h.DB, &pager)
	if err != nil {
		return err
	}

	return c.Render("encrypted_posts_index", fiber.Map{
		"Posts": posts,
		"Pager": pager,
	}, "admin_layout")
}

func (h *Handler) GetEncryptedPostNewHandler(c *fiber.Ctx) error {
	return c.Render("encrypted_posts_new", nil, "admin_layout")
}

func (h *Handler) PostCreateEncryptedPostHandler(c *fiber.Ctx) error {
	in := CreateEncryptedPostInput{
		Title:     c.FormValue("title"),
		Content:   c.FormValue("content"),
		Password:  c.FormValue("password"),
		ExpiresAt: c.FormValue("expires_at"),
	}

	if err := CreateEncryptedPostService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/encrypted-posts")
}

func (h *Handler) GetEncryptedPostEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	post, err := GetEncryptedPostForEdit(h.DB, id)
	if err != nil {
		return err
	}

	return c.Render("encrypted_posts_edit", fiber.Map{
		"Post": post,
	}, "admin_layout")
}

func (h *Handler) PostUpdateEncryptedPostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	in := UpdateEncryptedPostInput{
		Title:     c.FormValue("title"),
		Content:   c.FormValue("content"),
		Password:  c.FormValue("password"),
		ExpiresAt: c.FormValue("expires_at"),
	}

	if err := UpdateEncryptedPostService(h.DB, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/encrypted-posts")
}

func (h *Handler) PostDeleteEncryptedPostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteEncryptedPostService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/encrypted-posts")
}

// Configs
func (h *Handler) GetConfigsHandler(c *fiber.Ctx) error {
	config, err := GetConfigForEdit(h.DB)
	if err != nil {
		return err
	}

	return c.Render("configs_edit", fiber.Map{
		"Config": config,
	}, "admin_layout")
}

func (h *Handler) PostUpdateConfigsHandler(c *fiber.Ctx) error {
	in := UpdateConfigInput{
		Name:            c.FormValue("name"),
		Language:        c.FormValue("language"),
		Timezone:        c.FormValue("timezone"),
		PostSlugPattern: c.FormValue("post_slug_pattern"),
		TagSlugPattern:  c.FormValue("tag_slug_pattern"),
		TagsPattern:     c.FormValue("tags_pattern"),
		GiscusConfig:    c.FormValue("giscus_config"),
		GA4ID:           c.FormValue("ga4_id"),
		AdminPassword:   c.FormValue("admin_password"),
	}

	if err := UpdateConfigService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/configs")
}

// Trash
func (h *Handler) GetTrashHandler(c *fiber.Ctx) error {
	// 获取当前选中的类型，默认为 posts
	modelType := c.Query("type", "posts")

	var data interface{}
	var err error

	switch modelType {
	case "posts":
		data, err = GetTrashPosts(h.DB)
	case "encrypted-posts":
		data, err = GetTrashEncryptedPosts(h.DB)
	case "tags":
		data, err = GetTrashTags(h.DB)
	case "redirects":
		data, err = GetTrashRedirects(h.DB)
	default:
		data, err = GetTrashPosts(h.DB)
		modelType = "posts"
	}

	if err != nil {
		return err
	}

	return c.Render("trash_index", fiber.Map{
		"Data":      data,
		"ModelType": modelType,
	}, "admin_layout")
}

func (h *Handler) PostRestorePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestorePostService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreEncryptedPostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreEncryptedPostService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreTagService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreRedirectService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

// HttpErrorLogs
func (h *Handler) GetHttpErrorLogListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	logs, err := ListHttpErrorLogs(h.DB, &pager)
	if err != nil {
		return err
	}

	return c.Render("http_error_logs_index", fiber.Map{
		"Logs":  logs,
		"Pager": pager,
	}, "admin_layout")
}

func (h *Handler) PostDeleteHttpErrorLogHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteHttpErrorLogService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/http-error-logs")
}
