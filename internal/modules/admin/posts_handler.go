package admin

import (
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/helper"
	"swaves/internal/shared/share"

	"github.com/gofiber/fiber/v3"
)

// parseTagsFromCommaSeparated 解析 "标签1, 标签2" 为 tagIDs，不存在的标签会创建
func parseTagsFromCommaSeparated(dbx *db.DB, s string) []int64 {
	var tagIDs []int64
	for _, part := range strings.Split(s, ",") {
		name := strings.TrimSpace(part)
		if name == "" {
			continue
		}
		if tag, err := CreateTagByName(dbx, name, 0); err == nil {
			tagIDs = append(tagIDs, tag.ID)
		}
	}
	return tagIDs
}

func (h *Handler) GetRecordListHandler(c fiber.Ctx) error {
	return RenderAdminView(c, "dash/records_index.html", fiber.Map{
		"Title": "Records",
	}, "")
}

// Posts
func (h *Handler) GetPostListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	// kind: 0=文章(post), 1=页面(page)，默认 0
	kindVal := c.Query("kind", "0")
	var kind db.PostKind
	if kindVal == "1" {
		kind = db.PostKindPage
	} else {
		kind = db.PostKindPost
	}
	var kindPtr *db.PostKind
	kindPtr = &kind

	countPost, countPage, countEncryptedPost := CountPost(h.Model)

	searchQuery := c.Query("q")
	var tagID, categoryID *int64
	if v := c.Query("tag"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			tagID = &id
		}
	}
	if v := c.Query("category"); v != "" {
		if id, err := strconv.ParseInt(v, 10, 64); err == nil {
			categoryID = &id
		}
	}

	opts := &db.PostQueryOptions{Kind: kindPtr, Pager: &pager}
	if tagID != nil {
		opts.TagID = *tagID
	}
	if categoryID != nil {
		opts.CategoryID = *categoryID
	}
	var posts []db.PostWithRelation
	var err error
	if searchQuery != "" {
		posts, err = db.ListPostsBySearch(h.Model, opts, searchQuery)
	} else {
		posts, err = db.ListPosts(h.Model, opts)
	}
	if err != nil {
		return err
	}

	postIDs := make([]int64, 0, len(posts))
	for _, item := range posts {
		if item.Post != nil && item.Post.ID > 0 {
			postIDs = append(postIDs, item.Post.ID)
		}
	}
	postUVMap, err := db.CountPostUVByIDs(h.Model, postIDs)
	if err != nil {
		return err
	}

	kindQuery := "0"
	if kind == db.PostKindPage {
		kindQuery = "1"
	}
	searchQueryEscaped := ""
	if searchQuery != "" {
		searchQueryEscaped = url.QueryEscape(searchQuery)
	}

	var filterTagName, filterCategoryName string
	var filterTagIDStr, filterCategoryIDStr string
	if tagID != nil {
		filterTagIDStr = strconv.FormatInt(*tagID, 10)
		if t, err := db.GetTagByID(h.Model, *tagID); err == nil {
			filterTagName = t.Name
		}
	}
	if categoryID != nil {
		filterCategoryIDStr = strconv.FormatInt(*categoryID, 10)
		if cat, err := db.GetCategoryByID(h.Model, *categoryID); err == nil {
			filterCategoryName = cat.Name
		}
	}

	// 清除单项筛选的 URL（供筛选区 tag 组件的 RemoveHref 使用）
	filterTagRemoveQuery := map[string]string{"kind": kindQuery}
	if searchQuery != "" {
		filterTagRemoveQuery["q"] = searchQuery
	}
	if filterCategoryIDStr != "" {
		filterTagRemoveQuery["category"] = filterCategoryIDStr
	}
	filterTagRemoveURL := h.adminRouteURL(c, "admin.posts.list", nil, filterTagRemoveQuery)
	if filterTagRemoveURL == "" {
		filterTagRemoveURL = share.BuildAdminPath("/posts")
	}

	filterCategoryRemoveQuery := map[string]string{"kind": kindQuery}
	if searchQuery != "" {
		filterCategoryRemoveQuery["q"] = searchQuery
	}
	if filterTagIDStr != "" {
		filterCategoryRemoveQuery["tag"] = filterTagIDStr
	}
	filterCategoryRemoveURL := h.adminRouteURL(c, "admin.posts.list", nil, filterCategoryRemoveQuery)
	if filterCategoryRemoveURL == "" {
		filterCategoryRemoveURL = share.BuildAdminPath("/posts")
	}

	return RenderAdminView(c, "dash/posts_index.html", fiber.Map{
		"Title":                   "Posts",
		"Posts":                   posts,
		"Pager":                   pager,
		"Kind":                    kind,
		"KindQuery":               kindQuery,
		"CountPost":               countPost,
		"CountPage":               countPage,
		"CountEncryptedPost":      countEncryptedPost,
		"SearchQuery":             searchQuery,
		"SearchQueryEscaped":      searchQueryEscaped,
		"FilterTagIDStr":          filterTagIDStr,
		"FilterCategoryIDStr":     filterCategoryIDStr,
		"FilterTagName":           filterTagName,
		"FilterCategoryName":      filterCategoryName,
		"FilterTagRemoveURL":      filterTagRemoveURL,
		"FilterCategoryRemoveURL": filterCategoryRemoveURL,
		"PostUVMap":               postUVMap,
	}, "")
}
func (h *Handler) GetPostNewHandler(c fiber.Ctx) error {
	return h.renderPostNew(c, nil)
}

func (h *Handler) renderPostNew(c fiber.Ctx, data fiber.Map) error {
	tags, err := GetAllTags(h.Model)
	if err != nil {
		return err
	}

	categories, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}
	categoryOptions := BuildCategorySelectOptions(categories)

	if data == nil {
		data = fiber.Map{}
	}
	if _, ok := data["DraftTitle"]; !ok {
		data["DraftTitle"] = ""
	}
	if _, ok := data["DraftSlug"]; !ok {
		data["DraftSlug"] = ""
	}
	if _, ok := data["DraftContent"]; !ok {
		data["DraftContent"] = ""
	}
	if _, ok := data["DraftKind"]; !ok {
		data["DraftKind"] = "0"
	}
	if _, ok := data["DraftCategoryID"]; !ok {
		data["DraftCategoryID"] = int64(0)
	}
	if _, ok := data["DraftCommentEnabled"]; !ok {
		data["DraftCommentEnabled"] = true
	}
	if _, ok := data["SelectedTagNames"]; !ok {
		data["SelectedTagNames"] = ""
	}

	data["Title"] = "New Post"
	data["Tags"] = tags
	data["Categories"] = categories
	data["CategoryOptions"] = categoryOptions

	return RenderAdminView(c, "dash/posts_new.html", data, "")
}

func (h *Handler) renderPostNewWithDraft(c fiber.Ctx, err error, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	if err != nil {
		data["Error"] = err.Error()
	}
	return h.renderPostNew(c, data)
}

func (h *Handler) PostCreatePostHandler(c fiber.Ctx) error {
	// 草稿字段：创建失败后回填
	title := c.FormValue("title")
	slug := strings.TrimSpace(c.FormValue("slug"))
	content := c.FormValue("content")
	tagNames := c.FormValue("tags")

	// 解析分类 ID（单选）
	var categoryID int64
	categoriesStr := strings.TrimSpace(c.FormValue("categories"))
	if categoriesStr != "" {
		if id, err := strconv.ParseInt(categoriesStr, 10, 64); err == nil {
			categoryID = id
		}
	}

	draftKind := c.FormValue("kind")
	kind := db.PostKindPost
	if draftKind == "1" {
		kind = db.PostKindPage
	} else {
		draftKind = "0"
	}
	commentEnabled := c.FormValue("comment_enabled") == "1" || c.FormValue("comment_enabled") == "on" || c.FormValue("comment_enabled") == "true"

	draft := fiber.Map{
		"DraftTitle":          title,
		"DraftSlug":           slug,
		"DraftContent":        content,
		"DraftKind":           draftKind,
		"DraftCategoryID":     categoryID,
		"DraftCommentEnabled": commentEnabled,
		"SelectedTagNames":    tagNames,
	}

	if !helper.IsSlug(slug) {
		return h.renderPostNewWithDraft(c, errSlugInvalid("010", slug), draft)
	}

	// 解析标签：name="tags" 逗号分隔，每段为标签名（存在则关联，不存在则创建）
	tagIDs := parseTagsFromCommaSeparated(h.Model, tagNames)

	// 新建：action= publish 发布，否则保存为草稿
	status := "draft"
	if c.FormValue("action") == "publish" {
		status = "published"
	}

	in := CreatePostInput{
		Title:          title,
		Slug:           slug,
		Content:        content,
		Status:         status,
		Kind:           kind,
		TagIDs:         tagIDs,
		CategoryID:     categoryID,
		CommentEnabled: &commentEnabled,
	}

	postID, err := CreatePostService(h.Model, in)
	if err != nil {
		return h.renderPostNewWithDraft(c, err, draft)
	}

	return h.redirectToAdminRoute(c, "admin.posts.edit", map[string]string{
		"id": strconv.FormatInt(postID, 10),
	}, nil, fiber.StatusSeeOther)
}

func (h *Handler) GetPostEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	postWithTags, err := GetPostForEdit(h.Model, id)
	if err != nil {
		return err
	}

	allTags, err := GetAllTags(h.Model)
	if err != nil {
		return err
	}

	// 已选标签名逗号拼接，供 input 展示
	selectedTagNames := make([]string, 0, len(postWithTags.Tags))
	for _, tag := range postWithTags.Tags {
		selectedTagNames = append(selectedTagNames, tag.Name)
	}

	// 获取所有分类
	allCategories, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}
	categoryOptions := BuildCategorySelectOptions(allCategories)

	// 获取当前 post 的分类（单选）
	category, err := db.GetPostCategory(h.Model, id)
	if err != nil {
		return err
	}

	// 如果没有分类，使用空的 Category（ID 为 0）
	var emptyCategory db.Category
	if category == nil {
		category = &emptyCategory
	}

	return RenderAdminView(c, "dash/posts_edit.html", fiber.Map{
		"Title":            "Edit Post",
		"Post":             postWithTags.Post,
		"Tags":             allTags,
		"SelectedTags":     postWithTags.Tags,
		"SelectedTagNames": strings.Join(selectedTagNames, ", "),
		"Categories":       allCategories,
		"CategoryOptions":  categoryOptions,
		"Category":         *category,
	}, "")
}

func (h *Handler) PostUpdatePostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	tagIDs := parseTagsFromCommaSeparated(h.Model, c.FormValue("tags"))

	// 解析分类 ID（单选）
	var categoryID int64
	categoriesStr := c.FormValue("categories")
	if categoriesStr != "" {
		if id, err := strconv.ParseInt(categoriesStr, 10, 64); err == nil {
			categoryID = id
		}
	}

	kind := db.PostKindPost
	if c.FormValue("kind") == "1" {
		kind = db.PostKindPage
	}
	commentEnabled := c.FormValue("comment_enabled") == "1" || c.FormValue("comment_enabled") == "on" || c.FormValue("comment_enabled") == "true"

	actionStr := c.FormValue("action")
	action := UpdatePostActionSave
	switch actionStr {
	case "publish":
		action = UpdatePostActionPublish
	case "update":
		action = UpdatePostActionUpdate
	}

	in := UpdatePostInput{
		Title:          c.FormValue("title"),
		Content:        c.FormValue("content"),
		Status:         c.FormValue("status"),
		Kind:           kind,
		TagIDs:         tagIDs,
		CategoryID:     categoryID,
		CommentEnabled: &commentEnabled,
		Action:         action,
	}

	if err := UpdatePostService(h.Model, id, in); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.posts.edit", map[string]string{
		"id": strconv.FormatInt(id, 10),
	}, nil, fiber.StatusSeeOther)
}

func (h *Handler) PostDeletePostHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeletePostService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.posts.list", nil, nil)
}
