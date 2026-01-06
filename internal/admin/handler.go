package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
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
		"Title": "Posts",
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
		"Title": "New Post",
		"Tags":  tags,
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
		"Title":          "Edit Post",
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
		"Title": "Tags",
		"Tags":  tags,
		"Pager": pager,
	}, "admin_layout")
}

func (h *Handler) GetTagNewHandler(c *fiber.Ctx) error {
	return c.Render("tags_new", fiber.Map{
		"Title": "New Tag",
	}, "admin_layout")
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
		"Title": "Edit Tag",
		"Tag":   tag,
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
		"Title":     "Redirects",
		"Redirects": redirects,
		"Pager":     pager,
	}, "admin_layout")
}

func (h *Handler) GetRedirectNewHandler(c *fiber.Ctx) error {
	return c.Render("redirects_new", fiber.Map{
		"Title": "New Redirect",
	}, "admin_layout")
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
		"Title":    "Edit Redirect",
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
		"Title": "Encrypted Posts",
		"Posts": posts,
		"Pager": pager,
	}, "admin_layout")
}

func (h *Handler) GetEncryptedPostNewHandler(c *fiber.Ctx) error {
	return c.Render("encrypted_posts_new", fiber.Map{
		"Title": "New Encrypted Post",
	}, "admin_layout")
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
		"Title": "Edit Encrypted Post",
		"Post":  post,
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

// SettingView 用于模板展示的设置视图
type SettingView struct {
	db.Setting
	OptionsParsed  []map[string]string    // 解析后的 options（用于 radio/checkbox）
	CheckboxValues map[string]bool        // checkbox 的选中状态
	AttrsParsed    map[string]interface{} // 解析后的 attrs（用于 HTML 属性）
}

// Settings
func (h *Handler) GetSettingsHandler(c *fiber.Ctx) error {
	// 获取所有 settings，以表格形式展示
	settings, err := ListAllSettings(h.DB)
	if err != nil {
		return err
	}

	return c.Render("settings_index", fiber.Map{
		"Title":    "Settings",
		"Settings": settings,
	}, "admin_layout")
}

func (h *Handler) GetSettingsAllHandler(c *fiber.Ctx) error {
	// 获取分类参数，如果没有则显示所有
	category := c.Query("category", "")

	var settings []db.Setting
	var err error
	if category == "" {
		settings, err = ListAllSettings(h.DB)
	} else {
		settings, err = ListSettingsByCategory(h.DB, category)
	}
	if err != nil {
		return err
	}

	// 转换为视图结构，解析 options 和 attrs，保持原有顺序
	settingsViews := make([]SettingView, 0, len(settings))
	for _, s := range settings {
		view := SettingView{Setting: s}

		// 解析 options（如果是 radio、checkbox 或 select）
		if (s.Type == "select" || s.Type == "radio" || s.Type == "checkbox") && s.Options != "" {
			var options []map[string]string
			err := json.Unmarshal([]byte(s.Options), &options)
			if err == nil {
				view.OptionsParsed = options
			} else {
				log.Println("Error parsing options for setting", s.Options, err)
			}
		}

		// 对于 checkbox，解析当前值并创建选中状态映射
		if s.Type == "checkbox" {
			view.CheckboxValues = make(map[string]bool)
			if s.Value != "" {
				values := strings.Split(s.Value, ",")
				for _, v := range values {
					view.CheckboxValues[strings.TrimSpace(v)] = true
				}
			}
		}

		// 解析 attrs（HTML 属性）
		view.AttrsParsed = make(map[string]interface{}) // 初始化为空 map，避免 nil
		if s.Attrs != "" {
			var attrs map[string]interface{}
			err := json.Unmarshal([]byte(s.Attrs), &attrs)
			if err == nil {
				view.AttrsParsed = attrs
			} else {
				log.Println("Error parsing attrs for setting", s.Attrs, err)
			}
		}

		settingsViews = append(settingsViews, view)
	}

	return c.Render("settings_all", fiber.Map{
		"Title":    "Settings - Edit All",
		"Settings": settingsViews,
		"Category": category,
	}, "admin_layout")
}

func (h *Handler) PostUpdateSettingsAllHandler(c *fiber.Ctx) error {
	// 获取所有配置项，然后更新每个的值
	settings, err := ListAllSettings(h.DB)
	if err != nil {
		return err
	}

	for _, setting := range settings {
		fieldName := "setting_" + setting.Code

		// checkbox 类型需要特殊处理，可能有多个值
		if setting.Type == "checkbox" {
			valuesBytes := c.Request().PostArgs().PeekMulti(fieldName)

			var values []string
			for _, v := range valuesBytes {
				values = append(values, string(v))
			}

			value := strings.Join(values, ",")
			if err := UpdateSettingValueService(h.DB, setting.Code, value); err != nil {
				return err
			}
		} else {
			// 其他类型直接获取单个值
			value := c.FormValue(fieldName)
			// 对于 password 类型，如果为空则不更新（保持原值）
			if setting.Type == "password" && value == "" {
				continue
			}
			if err := UpdateSettingValueService(h.DB, setting.Code, value); err != nil {
				return err
			}
		}
	}

	return c.Redirect("/admin/settings/all")
}

func (h *Handler) GetSettingEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	setting, err := GetSettingByID(h.DB, id)
	if err != nil {
		return err
	}

	// 转换为视图结构，解析 options 和 attrs
	view := SettingView{Setting: *setting}

	// 解析 options（如果是 radio、checkbox 或 select）
	if (setting.Type == "select" || setting.Type == "radio" || setting.Type == "checkbox") && setting.Options != "" {
		var options []map[string]string
		err := json.Unmarshal([]byte(setting.Options), &options)
		if err == nil {
			view.OptionsParsed = options
		} else {
			log.Println("Error parsing options for setting", setting.Options, err)
		}
	}

	// 解析 attrs（HTML 属性）
	view.AttrsParsed = make(map[string]interface{}) // 初始化为空 map，避免 nil
	if setting.Attrs != "" {
		var attrs map[string]interface{}
		err := json.Unmarshal([]byte(setting.Attrs), &attrs)
		if err == nil {
			view.AttrsParsed = attrs
		} else {
			log.Println("Error parsing attrs for setting", setting.Attrs, err)
		}
	}

	return c.Render("settings_edit", fiber.Map{
		"Title":   "Edit Setting",
		"Setting": view,
	}, "admin_layout")
}

func (h *Handler) PostUpdateSettingHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// 获取现有设置
	setting, err := GetSettingByID(h.DB, id)
	if err != nil {
		return err
	}

	// 更新字段
	setting.Category = c.FormValue("category")
	setting.Name = c.FormValue("name")
	setting.Code = c.FormValue("code")
	setting.Type = c.FormValue("type")
	setting.Options = c.FormValue("options")
	setting.Attrs = c.FormValue("attrs")
	setting.DefaultOptionValue = c.FormValue("default_option_value")
	setting.Description = c.FormValue("description")

	if sortStr := c.FormValue("sort"); sortStr != "" {
		if sort, err := strconv.ParseInt(sortStr, 10, 64); err == nil {
			setting.Sort = sort
		}
	}

	// 处理 value 字段
	value := c.FormValue("value")
	// 对于 password 类型，如果为空则不更新（保持原值）
	if setting.Type == "password" && value == "" {
		// 保持原值，不做更新
	} else {
		setting.Value = value
	}

	if err := UpdateSettingService(h.DB, setting); err != nil {
		// 转换为视图结构，解析 options 和 attrs
		view := SettingView{Setting: *setting}
		if (setting.Type == "select" || setting.Type == "radio" || setting.Type == "checkbox") && setting.Options != "" {
			var options []map[string]string
			if err2 := json.Unmarshal([]byte(setting.Options), &options); err2 == nil {
				view.OptionsParsed = options
			}
		}
		view.AttrsParsed = make(map[string]interface{}) // 初始化为空 map，避免 nil
		if setting.Attrs != "" {
			var attrs map[string]interface{}
			if err2 := json.Unmarshal([]byte(setting.Attrs), &attrs); err2 == nil {
				view.AttrsParsed = attrs
			}
		}

		return c.Render("settings_edit", fiber.Map{
			"Title":   "Edit Setting",
			"Error":   err.Error(),
			"Setting": view,
		}, "admin_layout")
	}

	return c.Redirect("/admin/settings")
}

func (h *Handler) PostDeleteSettingHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteSettingService(h.DB, id); err != nil {
		return err
	}

	return c.Redirect("/admin/settings")
}

func (h *Handler) GetSettingNewHandler(c *fiber.Ctx) error {
	// 如果需要处理预填充的 options（例如从错误返回），可以在这里解析
	var optionsParsed []map[string]string
	var defaultOptionValue string

	// 从查询参数获取 options（用于错误回显）
	if optionsJSON := c.Query("options"); optionsJSON != "" {
		if err := json.Unmarshal([]byte(optionsJSON), &optionsParsed); err == nil {
			defaultOptionValue = c.Query("default_option_value", "")
		}
	}

	return c.Render("settings_new", fiber.Map{
		"Title":              "New Setting",
		"OptionsParsed":      optionsParsed,
		"DefaultOptionValue": defaultOptionValue,
	}, "admin_layout")
}

func (h *Handler) PostCreateSettingHandler(c *fiber.Ctx) error {
	s := &db.Setting{
		Category:           c.FormValue("category"),
		Name:               c.FormValue("name"),
		Code:               c.FormValue("code"),
		Type:               c.FormValue("type"),
		Options:            c.FormValue("options"),
		Attrs:              c.FormValue("attrs"),
		Value:              c.FormValue("value"),
		DefaultOptionValue: c.FormValue("default_option_value"),
		Description:        c.FormValue("description"),
	}

	if sortStr := c.FormValue("sort"); sortStr != "" {
		if sort, err := strconv.ParseInt(sortStr, 10, 64); err == nil {
			s.Sort = sort
		}
	}

	if s.Category == "" {
		s.Category = "default"
	}

	if err := CreateSettingService(h.DB, s); err != nil {
		// 解析 options 用于错误回显
		var optionsParsed []map[string]string
		if s.Options != "" {
			json.Unmarshal([]byte(s.Options), &optionsParsed)
		}

		return c.Render("settings_new", fiber.Map{
			"Title":              "New Setting",
			"Error":              err.Error(),
			"Setting":            s,
			"OptionsParsed":      optionsParsed,
			"DefaultOptionValue": s.DefaultOptionValue,
		}, "admin_layout")
	}

	return c.Redirect("/admin/settings")
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
		"Title":     "Trash",
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
		"Title": "Http Error Logs",
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

// CronJobs
func (h *Handler) GetCronJobListHandler(c *fiber.Ctx) error {
	jobs, err := ListCronJobs(h.DB)
	if err != nil {
		return err
	}

	return c.Render("cron_jobs_index", fiber.Map{
		"Title": "Cron Jobs",
		"Jobs":  jobs,
	}, "admin_layout")
}

func (h *Handler) GetCronJobNewHandler(c *fiber.Ctx) error {
	return c.Render("cron_jobs_new", fiber.Map{
		"Title": "New Cron Job",
	}, "admin_layout")
}

func (h *Handler) PostCreateCronJobHandler(c *fiber.Ctx) error {
	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"

	in := CreateCronJobInput{
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		Schedule:    c.FormValue("schedule"),
		Enabled:     enabled,
	}

	if err := CreateCronJobService(h.DB, in); err != nil {
		return err
	}

	return c.Redirect("/admin/cron-jobs")
}

// CronJobLogs
func (h *Handler) GetCronJobLogListHandler(c *fiber.Ctx) error {
	jobID, err := strconv.ParseInt(c.Params("job_id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// 获取 job 信息
	job, err := GetCronJobForEdit(h.DB, jobID)
	if err != nil {
		return err
	}

	// 获取日志列表，默认限制 100 条
	logs, err := ListCronJobLogs(h.DB, jobID, 100)
	if err != nil {
		return err
	}

	return c.Render("cron_job_logs_index", fiber.Map{
		"Title": "Cron Job Logs: " + job.Name,
		"Job":   job,
		"Logs":  logs,
	}, "admin_layout")
}

// Import/Export
func (h *Handler) GetImportHandler(c *fiber.Ctx) error {
	return c.Render("import", fiber.Map{
		"Title": "Import Markdown",
	}, "admin_layout")
}

func (h *Handler) PostImportHandler(c *fiber.Ctx) error {
	// 获取上传的文件
	form, err := c.MultipartForm()
	if err != nil {
		return c.Render("import", fiber.Map{
			"Title": "Import Markdown",
			"Error": "Failed to parse form: " + err.Error(),
		}, "admin_layout")
	}

	files := form.File["files"]
	if len(files) == 0 {
		return c.Render("import", fiber.Map{
			"Title": "Import Markdown",
			"Error": "Please select at least one file to import",
		}, "admin_layout")
	}

	// 获取 slug 来源选择
	slugSourceStr := c.FormValue("slug_source")
	slugSource := SlugFromTitle // 默认从 title 生成
	switch slugSourceStr {
	case "filename":
		slugSource = SlugFromFilename
	case "frontmatter":
		slugSource = SlugFromFrontmatter
	case "title":
		slugSource = SlugFromTitle
	}

	// 如果是从 frontmatter，获取字段名
	slugField := c.FormValue("slug_field")
	if slugField == "" {
		slugField = "slug"
	}

	// 读取所有文件
	var importFiles []ImportFile
	for _, fileHeader := range files {
		src, err := fileHeader.Open()
		if err != nil {
			return c.Render("import", fiber.Map{
				"Title": "Import Markdown",
				"Error": "Failed to open file " + fileHeader.Filename + ": " + err.Error(),
			}, "admin_layout")
		}

		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return c.Render("import", fiber.Map{
				"Title": "Import Markdown",
				"Error": "Failed to read file " + fileHeader.Filename + ": " + err.Error(),
			}, "admin_layout")
		}

		// 提取文件名（不含扩展名）
		filename := fileHeader.Filename
		if idx := strings.LastIndex(filename, "."); idx > 0 {
			filename = filename[:idx]
		}

		importFiles = append(importFiles, ImportFile{
			Filename: filename,
			Content:  string(content),
		})
	}

	// 解析文件但不入库，返回预览数据
	items, err := ParseImportFiles(importFiles, slugSource, slugField)
	if err != nil && len(items) == 0 {
		// 如果全部解析失败，返回错误
		return c.Render("import", fiber.Map{
			"Title": "Import Markdown",
			"Error": err.Error(),
		}, "admin_layout")
	}

	// 即使有部分错误，也显示预览页面（有警告信息）
	return c.Render("import_preview", fiber.Map{
		"Title": "Import Preview",
		"Items": items,
		"Error": err, // 如果有错误，显示警告信息
	}, "admin_layout")
}

func (h *Handler) PostImportPreviewHandler(c *fiber.Ctx) error {
	// 从表单中读取所有 items 数据
	// 表单字段格式：items[0][title], items[0][slug], items[0][content], etc.

	// 从 items_count 字段获取数量
	countStr := c.FormValue("items_count")
	itemCount := 0
	if countStr != "" {
		if count, err := strconv.Atoi(countStr); err == nil {
			itemCount = count
		}
	}

	if itemCount == 0 {
		return c.Render("import_preview", fiber.Map{
			"Title": "Import Preview",
			"Items": []PreviewPostItem{},
			"Error": "No items to import",
		}, "admin_layout")
	}

	var items []PreviewPostItem

	// 构建 items
	for i := 0; i < itemCount; i++ {
		item := PreviewPostItem{
			Index:     i,
			Title:     c.FormValue(fmt.Sprintf("items[%d][title]", i)),
			Slug:      c.FormValue(fmt.Sprintf("items[%d][slug]", i)),
			Content:   c.FormValue(fmt.Sprintf("items[%d][content]", i)),
			Status:    c.FormValue(fmt.Sprintf("items[%d][status]", i)),
			CreatedAt: c.FormValue(fmt.Sprintf("items[%d][created_at]", i)),
			Tags:      c.FormValue(fmt.Sprintf("items[%d][tags]", i)),
			Filename:  c.FormValue(fmt.Sprintf("items[%d][filename]", i)),
		}

		// 解析时间戳
		if createdAtStr := c.FormValue(fmt.Sprintf("items[%d][created_at_unix]", i)); createdAtStr != "" {
			if ts, err := strconv.ParseInt(createdAtStr, 10, 64); err == nil {
				item.CreatedAtUnix = ts
			}
		}

		items = append(items, item)
	}

	if len(items) == 0 {
		return c.Render("import_preview", fiber.Map{
			"Title": "Import Preview",
			"Items": items,
			"Error": "No items to import",
		}, "admin_layout")
	}

	// 调用 service 进行导入
	if err := ImportPreviewService(h.DB, items); err != nil {
		return c.Render("import_preview", fiber.Map{
			"Title": "Import Preview",
			"Items": items,
			"Error": err.Error(),
		}, "admin_layout")
	}

	return c.Redirect("/admin/posts")
}
