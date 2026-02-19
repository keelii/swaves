package admin

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"swaves/helper"
	"swaves/internal/db"
	job "swaves/internal/jobs"
	"swaves/internal/middleware"
	"swaves/internal/store"
	"swaves/internal/types"
	"time"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	Model   *db.DB
	Session *types.SessionStore
	Service *Service
}

type dashboardUVRangeConfig struct {
	Key              string
	Label            string
	BucketSeconds    int64
	BucketCount      int
	PointLabelLayout string
	RangeLabelLayout string
}

type dashboardUVChartResult struct {
	SVG        string
	Total      int
	StartLabel string
	EndLabel   string
}

type dashboardUVTabData struct {
	Key        string
	Label      string
	Active     bool
	SVG        string
	Total      int
	StartLabel string
	EndLabel   string
}

const dashboardActiveUsersWindowSeconds int64 = 30 * 60
const dashboardActiveUsersWindowLabel = "30分钟"

var dashboardUVRanges = []dashboardUVRangeConfig{
	{
		Key:              "24h",
		Label:            "24 小时",
		BucketSeconds:    60 * 60,
		BucketCount:      24,
		PointLabelLayout: "15:00",
		RangeLabelLayout: "01-02 15:04",
	},
	{
		Key:              "7d",
		Label:            "7 天",
		BucketSeconds:    24 * 60 * 60,
		BucketCount:      7,
		PointLabelLayout: "01-02",
		RangeLabelLayout: "01-02",
	},
	{
		Key:              "30d",
		Label:            "30 天",
		BucketSeconds:    24 * 60 * 60,
		BucketCount:      30,
		PointLabelLayout: "01-02",
		RangeLabelLayout: "01-02",
	},
}

func NewHandler(gStore *store.GlobalStore, adminService *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: adminService,
	}
}

// parseExpiresAtFromOption 根据 expires_at_option 与 expires_at_custom 解析出 Unix 时间戳字符串并入库用。
// option: "none"|"5min"|"1hour"|"1day"|"custom"；为 "custom" 时用 customValue 解析为本地时间。
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

func RenderAdminView(c *fiber.Ctx, view string, data fiber.Map, layout string) error {
	if layout == "" {
		layout = "admin_layout"
	}
	if data == nil {
		data = fiber.Map{}
	}

	data["UrlPath"] = c.Path()
	data["Query"] = c.Queries()

	// 注入 Locals
	c.Context().VisitUserValues(func(k []byte, v interface{}) {
		//log.Println("Injecting local:", string(k))
		data[string(k)] = v
	})

	return c.Render(view, data, layout)
}

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

func resolveDashboardUVRange(raw string) dashboardUVRangeConfig {
	raw = strings.TrimSpace(raw)
	for _, item := range dashboardUVRanges {
		if item.Key == raw {
			return item
		}
	}
	return dashboardUVRanges[len(dashboardUVRanges)-1]
}

func localDayStart(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, t.Location())
}

func localHourStart(t time.Time) time.Time {
	year, month, day := t.Date()
	return time.Date(year, month, day, t.Hour(), 0, 0, 0, t.Location())
}

func buildDashboardUVChart(model *db.DB, config dashboardUVRangeConfig, now time.Time) (dashboardUVChartResult, error) {
	var startTime time.Time
	var endTime time.Time
	switch config.Key {
	case "24h":
		endTime = localHourStart(now).Add(time.Hour)
		startTime = endTime.Add(-time.Duration(config.BucketCount) * time.Duration(config.BucketSeconds) * time.Second)
	default:
		endTime = localDayStart(now).Add(24 * time.Hour)
		startTime = endTime.Add(-time.Duration(config.BucketCount) * time.Duration(config.BucketSeconds) * time.Second)
	}

	startAt := startTime.Unix()
	endAt := endTime.Unix()

	buckets, err := db.ListDistinctVisitorsByBucket(model, startAt, endAt, config.BucketSeconds)
	if err != nil {
		return dashboardUVChartResult{}, err
	}

	uvByBucketIndex := make(map[int]int, len(buckets))
	for _, bucket := range buckets {
		if bucket.BucketIndex < 0 || bucket.BucketIndex >= config.BucketCount {
			continue
		}
		uvByBucketIndex[bucket.BucketIndex] = bucket.UV
	}

	points := make([]UVChartPoint, 0, config.BucketCount)
	for i := 0; i < config.BucketCount; i++ {
		bucketStart := startTime.Add(time.Duration(i) * time.Duration(config.BucketSeconds) * time.Second)
		points = append(points, UVChartPoint{
			Label:     bucketStart.Format(config.PointLabelLayout),
			UV:        uvByBucketIndex[i],
			Timestamp: bucketStart.Unix(),
		})
	}

	svg, err := BuildUVChartSVG(UVChartUIData{
		Points:              points,
		ClassName:           "dashboard-uv-svg",
		GridStrokeWidth:     0.4,
		LineStrokeWidth:     0.6,
		Width:               240,
		Height:              32,
		PreserveAspectRatio: "xMinYMid meet",
	})
	if err != nil {
		return dashboardUVChartResult{}, err
	}

	total, err := db.CountDistinctVisitorsBetween(model, startAt, endAt)
	if err != nil {
		return dashboardUVChartResult{}, err
	}

	return dashboardUVChartResult{
		SVG:        svg,
		Total:      total,
		StartLabel: startTime.Format(config.RangeLabelLayout),
		EndLabel:   endTime.Add(-time.Second).Format(config.RangeLabelLayout),
	}, nil
}

func (h *Handler) GetHome(c *fiber.Ctx) error {
	totalUV, err := db.CountUVUnique(h.Model, db.UVEntitySite, 0)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	activeUsers, err := db.CountActiveVisitors(h.Model, dashboardActiveUsersWindowSeconds)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	totalLikes, err := db.CountTotalLikes(h.Model)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	postCount, err := db.CountPostsByKind(h.Model, db.PostKindPost)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	pageCount, err := db.CountPostsByKind(h.Model, db.PostKindPage)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	categoryCount, err := db.CountCategories(h.Model)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	tagCount, err := db.CountTags(h.Model)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	topUVContents, err := db.ListTopUVContents(h.Model, 10)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	topLikedContents, err := db.ListTopLikedContents(h.Model, 10)
	if err != nil {
		return RenderAdminView(c, "admin_home", fiber.Map{
			"Title": "工作台",
			"Error": err.Error(),
		}, "")
	}

	activeRange := resolveDashboardUVRange(c.Query("uv_range")).Key
	chartTabs := make([]dashboardUVTabData, 0, len(dashboardUVRanges))
	nowTime := time.Now()
	for _, rangeConfig := range dashboardUVRanges {
		chartData, chartErr := buildDashboardUVChart(h.Model, rangeConfig, nowTime)
		if chartErr != nil {
			return RenderAdminView(c, "admin_home", fiber.Map{
				"Title": "工作台",
				"Error": chartErr.Error(),
			}, "")
		}
		chartTabs = append(chartTabs, dashboardUVTabData{
			Key:        rangeConfig.Key,
			Label:      rangeConfig.Label,
			Active:     rangeConfig.Key == activeRange,
			SVG:        chartData.SVG,
			Total:      chartData.Total,
			StartLabel: chartData.StartLabel,
			EndLabel:   chartData.EndLabel,
		})
	}

	return RenderAdminView(c, "admin_home", fiber.Map{
		"Title":                  "工作台",
		"ActiveUsers":            activeUsers,
		"ActiveUsersWindowLabel": dashboardActiveUsersWindowLabel,
		"TotalUV":                totalUV,
		"TotalLikes":             totalLikes,
		"PostCount":              postCount + pageCount,
		"CategoryCount":          categoryCount,
		"TagCount":               tagCount,
		"TopUVContents":          topUVContents,
		"TopLikedContents":       topLikedContents,
		"UVChartTabs":            chartTabs,
	}, "")
}

/* ---------- GET /admin/login ---------- */

func (h *Handler) GetLoginHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "admin_login", fiber.Map{
		"Title":     "Admin Login",
		"ReturnUrl": c.Query("returnUrl"),
	}, "base")
}

/* ---------- POST /admin/login ---------- */

func (h *Handler) PostLoginHandler(c *fiber.Ctx) error {
	returnUrl := c.FormValue("returnUrl")
	password := c.FormValue("password")
	if password == "" {
		return RenderAdminView(c, "admin_login", fiber.Map{
			"Title":     "Admin Login",
			"Error":     "password is empty",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	if err := h.Service.CheckPassword(password); err != nil {
		return RenderAdminView(c, "admin_login", fiber.Map{
			"Title":     "Admin Login",
			"Error":     "Invalid password",
			"ReturnUrl": returnUrl,
		}, "base")
	}

	succ := h.Session.SaveSession(c)

	if succ {
		return redirectAfterLogin(c)
	}

	return RenderAdminView(c, "admin_login", fiber.Map{
		"Title":     "Admin Login",
		"Error":     "Invalid Error",
		"ReturnUrl": returnUrl,
	}, "base")
}

// redirectAfterLogin 从表单读取 returnUrl，校验后重定向，避免开放重定向
func redirectAfterLogin(c *fiber.Ctx) error {
	returnUrl := strings.TrimSpace(c.FormValue("returnUrl"))
	if returnUrl != "" && strings.HasPrefix(returnUrl, "/") && !strings.Contains(returnUrl, "//") {
		return c.Redirect(returnUrl)
	}
	return c.Redirect("/admin")
}

/* ---------- POST /admin/logout ---------- */

func (h *Handler) GetLogoutHandler(c *fiber.Ctx) error {
	h.Session.ClearSession(c)
	return c.Redirect("/admin/login")
}

// Posts
func (h *Handler) GetPostListHandler(c *fiber.Ctx) error {
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

	countPost, _ := db.CountPostsByKind(h.Model, db.PostKindPost)
	countPage, _ := db.CountPostsByKind(h.Model, db.PostKindPage)

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
	filterTagRemoveURL := "/admin/posts?kind=" + kindQuery
	if searchQueryEscaped != "" {
		filterTagRemoveURL += "&q=" + searchQueryEscaped
	}
	if filterCategoryIDStr != "" {
		filterTagRemoveURL += "&category=" + filterCategoryIDStr
	}
	filterCategoryRemoveURL := "/admin/posts?kind=" + kindQuery
	if searchQueryEscaped != "" {
		filterCategoryRemoveURL += "&q=" + searchQueryEscaped
	}
	if filterTagIDStr != "" {
		filterCategoryRemoveURL += "&tag=" + filterTagIDStr
	}

	return RenderAdminView(c, "posts_index", fiber.Map{
		"Title":                   "Posts",
		"Posts":                   posts,
		"Pager":                   pager,
		"Kind":                    kind,
		"KindQuery":               kindQuery,
		"CountPost":               countPost,
		"CountPage":               countPage,
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
func (h *Handler) GetPostNewHandler(c *fiber.Ctx) error {
	return h.renderPostNew(c, nil)
}

func (h *Handler) renderPostNew(c *fiber.Ctx, data fiber.Map) error {
	tags, err := GetAllTags(h.Model)
	if err != nil {
		return err
	}

	categories, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}

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
	if _, ok := data["SelectedTagNames"]; !ok {
		data["SelectedTagNames"] = ""
	}

	data["Title"] = "New Post"
	data["Tags"] = tags
	data["Categories"] = categories

	return RenderAdminView(c, "posts_new", data, "")
}

func (h *Handler) renderPostNewWithDraft(c *fiber.Ctx, err error, data fiber.Map) error {
	if data == nil {
		data = fiber.Map{}
	}
	if err != nil {
		data["Error"] = err.Error()
	}
	return h.renderPostNew(c, data)
}

func (h *Handler) PostCreatePostHandler(c *fiber.Ctx) error {
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

	draft := fiber.Map{
		"DraftTitle":       title,
		"DraftSlug":        slug,
		"DraftContent":     content,
		"DraftKind":        draftKind,
		"DraftCategoryID":  categoryID,
		"SelectedTagNames": tagNames,
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
		Title:      title,
		Slug:       slug,
		Content:    content,
		Status:     status,
		Kind:       kind,
		TagIDs:     tagIDs,
		CategoryID: categoryID,
	}

	if err := CreatePostService(h.Model, in); err != nil {
		return h.renderPostNewWithDraft(c, err, draft)
	}

	return c.Redirect("/admin/posts")
}

func (h *Handler) GetPostEditHandler(c *fiber.Ctx) error {
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

	return RenderAdminView(c, "posts_edit", fiber.Map{
		"Title":            "Edit Post",
		"Post":             postWithTags.Post,
		"Tags":             allTags,
		"SelectedTags":     postWithTags.Tags,
		"SelectedTagNames": strings.Join(selectedTagNames, ", "),
		"Categories":       allCategories,
		"Category":         *category,
	}, "")
}

func (h *Handler) PostUpdatePostHandler(c *fiber.Ctx) error {
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

	actionStr := c.FormValue("action")
	action := UpdatePostActionSave
	switch actionStr {
	case "publish":
		action = UpdatePostActionPublish
	case "update":
		action = UpdatePostActionUpdate
	}

	in := UpdatePostInput{
		Title:      c.FormValue("title"),
		Content:    c.FormValue("content"),
		Status:     c.FormValue("status"),
		Kind:       kind,
		TagIDs:     tagIDs,
		CategoryID: categoryID,
		Action:     action,
	}

	if err := UpdatePostService(h.Model, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

func (h *Handler) PostDeletePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeletePostService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/posts")
}

// Tags
func (h *Handler) GetTagListHandler(c *fiber.Ctx) error {
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

	return RenderAdminView(c, "tags_index", fiber.Map{
		"Title":      "Tags",
		"Tags":       tags,
		"Pager":      pager,
		"PostCounts": postCounts,
	}, "")
}

func (h *Handler) GetTagNewHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "tags_new", fiber.Map{
		"Title": "New Tag",
	}, "")
}

func (h *Handler) PostCreateTagHandler(c *fiber.Ctx) error {
	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		return RenderAdminView(c, "tags_new", fiber.Map{
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

	return c.Redirect("/admin/tags")
}

func (h *Handler) GetTagEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	tag, err := GetTagForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "tags_edit", fiber.Map{
		"Title": "Edit Tag",
		"Tag":   tag,
	}, "")
}

func (h *Handler) PostUpdateTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		tag, _ := GetTagForEdit(h.Model, id)
		return RenderAdminView(c, "tags_edit", fiber.Map{
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

	return c.Redirect("/admin/tags")
}

func (h *Handler) PostDeleteTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteTagService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/tags")
}

// Redirects
func (h *Handler) GetRedirectListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	redirects, err := ListRedirects(h.Model, &pager)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "redirects_index", fiber.Map{
		"Title":     "Redirects",
		"Redirects": redirects,
		"Pager":     pager,
	}, "")
}

func (h *Handler) GetRedirectNewHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "redirects_new", fiber.Map{
		"Title": "New Redirect",
	}, "")
}

func (h *Handler) PostCreateRedirectHandler(c *fiber.Ctx) error {
	status, err := strconv.Atoi(c.FormValue("status"))
	if err != nil || (status != 301 && status != 302) {
		status = 301 // default to 301 if invalid
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
		return RenderAdminView(c, "redirects_new", fiber.Map{
			"Title": "New Redirect",
			"Error": err,
		}, "")
	}

	return c.Redirect("/admin/redirects")
}

func (h *Handler) GetRedirectEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	redirect, err := GetRedirectForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "redirects_edit", fiber.Map{
		"Title":    "Edit Redirect",
		"Redirect": redirect,
	}, "")
}

func (h *Handler) PostUpdateRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	status, err := strconv.Atoi(c.FormValue("status"))
	if err != nil || (status != 301 && status != 302) {
		status = 301 // default to 301 if invalid
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

	return c.Redirect("/admin/redirects")
}

func (h *Handler) PostDeleteRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteRedirectService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/redirects")
}

// Encrypted Posts
func (h *Handler) GetEncryptedPostListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	posts, err := ListEncryptedPosts(h.Model, &pager)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "encrypted_posts_index", fiber.Map{
		"Title": "Encrypted Posts",
		"Posts": posts,
		"Pager": pager,
	}, "")
}

func (h *Handler) GetEncryptedPostNewHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "encrypted_posts_new", fiber.Map{
		"Title": "New Encrypted Post",
	}, "")
}

func (h *Handler) PostCreateEncryptedPostHandler(c *fiber.Ctx) error {
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

	return c.Redirect("/admin/encrypted-posts")
}

func (h *Handler) GetEncryptedPostEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	post, err := GetEncryptedPostForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "encrypted_posts_edit", fiber.Map{
		"Title": "Edit Encrypted Post",
		"Post":  post,
	}, "")
}

func (h *Handler) PostUpdateEncryptedPostHandler(c *fiber.Ctx) error {
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

	return c.Redirect("/admin/encrypted-posts")
}

func (h *Handler) PostDeleteEncryptedPostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteEncryptedPostService(h.Model, id); err != nil {
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

// Categories
func (h *Handler) GetCategoryListHandler(c *fiber.Ctx) error {
	categories, err := ListCategoriesService(h.Model)
	if err != nil {
		return err
	}

	// 创建分类ID到名称的映射，方便显示父分类名称
	categoryMap := make(map[int64]string)
	for _, cat := range categories {
		categoryMap[cat.ID] = cat.Name
	}

	// 创建父分类名称映射
	parentMap := make(map[int64]string)
	for _, cat := range categories {
		if cat.ParentID > 0 {
			if parentName, ok := categoryMap[cat.ParentID]; ok {
				parentMap[cat.ID] = parentName
			}
		}
	}

	// 统计每个分类的文章数量
	categoryIDs := make([]int64, len(categories))
	for i, cat := range categories {
		categoryIDs[i] = cat.ID
	}
	postCounts, err := db.CountPostsByCategories(h.Model, categoryIDs)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "categories_index", fiber.Map{
		"Title":      "Categories",
		"Categories": categories,
		"ParentMap":  parentMap,
		"PostCounts": postCounts,
	}, "")
}

func (h *Handler) GetCategoryTreeHandler(c *fiber.Ctx) error {
	allCategories, tree, err := GetCategoryTree(h.Model)
	if err != nil {
		return err
	}

	//allCategories, err := GetAllCategoriesFlat(h.Model)
	//if err != nil {
	//	return err
	//}

	return RenderAdminView(c, "categories_tree", fiber.Map{
		"Title":      "Category Tree",
		"Tree":       tree,
		"Categories": allCategories,
	}, "")
}

func (h *Handler) GetCategoryNewHandler(c *fiber.Ctx) error {
	// 获取所有分类用于选择父级
	all, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}

	// 从查询参数获取预选的父分类 ID
	parentIDStr := c.Query("parent_id", "")
	var parentID int64
	if parentIDStr != "" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	return RenderAdminView(c, "categories_new", fiber.Map{
		"Title":      "New Category",
		"Categories": all,
		"ParentID":   parentID,
	}, "")
}

func (h *Handler) PostCreateCategoryHandler(c *fiber.Ctx) error {
	parentIDStr := c.FormValue("parent_id")
	var parentID int64
	if parentIDStr != "" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	sortStr := c.FormValue("sort")
	var sort int64
	if sortStr != "" {
		var err error
		sort, err = strconv.ParseInt(sortStr, 10, 64)
		if err != nil {
			sort = 0
		}
	}

	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		all, _ := GetAllCategoriesFlat(h.Model)
		return RenderAdminView(c, "categories_new", fiber.Map{
			"Title":      "New Category",
			"Error":      errSlugInvalid("013", slug).Error(),
			"Categories": all,
		}, "")
	}

	in := CreateCategoryInput{
		ParentID:    parentID,
		Name:        c.FormValue("name"),
		Slug:        slug,
		Description: c.FormValue("description"),
		Sort:        sort,
	}

	if err := CreateCategoryService(h.Model, in); err != nil {
		return RenderAdminView(c, "categories_new", fiber.Map{
			"Title":      "New Category",
			"Error":      err.Error(),
			"Categories": []db.Category{},
		}, "")
	}

	return c.Redirect("/admin/categories")
}

func (h *Handler) GetCategoryEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	category, err := GetCategoryForEdit(h.Model, id)
	if err != nil {
		return err
	}

	// 获取所有分类用于选择父级（排除自己）
	all, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		return err
	}

	// 过滤掉自己和自己的子节点（防止循环）
	var availableCategories []db.Category
	for _, c := range all {
		if c.ID == id {
			continue
		}
		// 检查是否是当前分类的子节点
		isChild := false
		cur := c.ParentID
		for cur != 0 {
			if cur == id {
				isChild = true
				break
			}
			// 找到父节点
			var parent *db.Category
			for _, p := range all {
				if p.ID == cur {
					parent = &p
					break
				}
			}
			if parent == nil {
				break
			}
			cur = parent.ParentID
		}
		if !isChild {
			availableCategories = append(availableCategories, c)
		}
	}

	return RenderAdminView(c, "categories_edit", fiber.Map{
		"Title":      "Edit Category",
		"Category":   category,
		"Categories": availableCategories,
	}, "")
}

func (h *Handler) PostUpdateCategoryHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	parentIDStr := c.FormValue("parent_id")
	var parentID int64
	if parentIDStr != "" && parentIDStr != "0" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	sortStr := c.FormValue("sort")
	var sort int64
	if sortStr != "" {
		var err error
		sort, err = strconv.ParseInt(sortStr, 10, 64)
		if err != nil {
			sort = 0
		}
	}

	slug := strings.TrimSpace(c.FormValue("slug"))
	if !helper.IsSlug(slug) {
		category, _ := GetCategoryForEdit(h.Model, id)
		all, _ := GetAllCategoriesFlat(h.Model)
		return RenderAdminView(c, "categories_edit", fiber.Map{
			"Title":      "Edit Category",
			"Error":      errSlugInvalid("014", slug).Error(),
			"Category":   category,
			"Categories": all,
		}, "")
	}

	in := UpdateCategoryInput{
		ParentID:    parentID,
		Name:        c.FormValue("name"),
		Slug:        slug,
		Description: c.FormValue("description"),
		Sort:        sort,
	}

	if err := UpdateCategoryService(h.Model, id, in); err != nil {
		category, _ := GetCategoryForEdit(h.Model, id)
		all, _ := GetAllCategoriesFlat(h.Model)
		return RenderAdminView(c, "categories_edit", fiber.Map{
			"Title":      "Edit Category",
			"Error":      err.Error(),
			"Category":   category,
			"Categories": all,
		}, "")
	}

	return c.Redirect("/admin/categories")
}

func (h *Handler) PostDeleteCategoryHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteCategoryService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/categories")
}

func (h *Handler) PostUpdateCategoryParentHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	parentIDStr := c.FormValue("categories")
	var parentID int64
	if parentIDStr != "" && parentIDStr != "0" {
		var err error
		parentID, err = strconv.ParseInt(parentIDStr, 10, 64)
		if err != nil {
			parentID = 0
		}
	}

	if err := UpdateCategoryParentService(h.Model, id, parentID); err != nil {
		return err
	}

	return c.Redirect("/admin/categories/tree")
}

// Settings
func (h *Handler) GetSettingsHandler(c *fiber.Ctx) error {
	// 获取所有 settings，以表格形式展示
	settings, err := ListAllSettings(h.Model)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "settings_index", fiber.Map{
		"Title":    "Settings",
		"Settings": settings,
	}, "")
}

func (h *Handler) GetSettingsAllHandler(c *fiber.Ctx) error {
	// 获取分类参数，如果没有则显示所有
	kind := c.Query("kind", "")

	var settings []db.Setting
	var err error
	if kind == "" {
		settings, err = ListAllSettings(h.Model)
	} else {
		settings, err = ListSettingsByKind(h.Model, kind)
	}
	if err != nil {
		return err
	}

	// 按 kind 分组，key=kind，value=settings，保持查询结果顺序
	settingsByKind := make(map[string][]SettingView)
	settingKinds := make([]string, 0)
	for _, s := range settings {
		view := buildSettingView(s)

		if _, ok := settingsByKind[s.Kind]; !ok {
			settingKinds = append(settingKinds, s.Kind)
		}
		settingsByKind[s.Kind] = append(settingsByKind[s.Kind], view)
	}

	return RenderAdminView(c, "settings_all", fiber.Map{
		"Title":          "Settings - Edit All",
		"SettingsByKind": settingsByKind,
		"SettingKinds":   settingKinds,
		"Kind":           kind,
	}, "")
}

func buildSettingView(s db.Setting) SettingView {
	view := SettingView{Setting: s}

	if (s.Type == "select" || s.Type == "radio" || s.Type == "checkbox") && s.Options != "" {
		var options []map[string]string
		err := json.Unmarshal([]byte(s.Options), &options)
		if err == nil {
			view.OptionsParsed = options
		} else {
			log.Println("Error parsing options for setting", s.Options, err)
		}
	}

	if s.Type == "checkbox" {
		view.CheckboxValues = make(map[string]bool)
		if s.Value != "" {
			values := strings.Split(s.Value, ",")
			for _, v := range values {
				view.CheckboxValues[strings.TrimSpace(v)] = true
			}
		}
	}

	view.AttrsParsed = make(map[string]interface{})
	if s.Attrs != "" {
		var attrs map[string]interface{}
		err := json.Unmarshal([]byte(s.Attrs), &attrs)
		if err == nil {
			view.AttrsParsed = attrs
		} else {
			log.Println("Error parsing attrs for setting", s.Attrs, err)
		}
	}

	return view
}

func (h *Handler) PostUpdateSettingsAllHandler(c *fiber.Ctx) error {
	// 获取所有配置项，然后更新每个的值
	settings, err := ListAllSettings(h.Model)
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
			if err := UpdateSettingValueService(h.Model, setting.Code, value); err != nil {
				return err
			}
		} else {
			// 其他类型直接获取单个值
			value := c.FormValue(fieldName)
			// 对于 password 类型，如果为空则不更新（保持原值）
			if setting.Type == "password" && value == "" {
				continue
			}
			if err := UpdateSettingValueService(h.Model, setting.Code, value); err != nil {
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

	setting, err := GetSettingByID(h.Model, id)
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

	return RenderAdminView(c, "settings_edit", fiber.Map{
		"Title":   "Edit Setting",
		"Setting": view,
	}, "")
}

func (h *Handler) PostUpdateSettingHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// 获取现有设置
	setting, err := GetSettingByID(h.Model, id)
	if err != nil {
		return err
	}

	// 更新字段
	setting.Kind = c.FormValue("kind")
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
	if c.FormValue("reload") != "" {
		setting.Reload = 1
	} else {
		setting.Reload = 0
	}

	// 处理 value 字段
	value := c.FormValue("value")
	// 对于 password 类型，如果为空则不更新（保持原值）
	if setting.Type == "password" && value == "" {
		// 保持原值，不做更新
	} else {
		setting.Value = value
	}

	if err := UpdateSettingService(h.Model, setting); err != nil {
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

		return RenderAdminView(c, "settings_edit", fiber.Map{
			"Title":   "Edit Setting",
			"Error":   err.Error(),
			"Setting": view,
		}, "")
	}

	return c.Redirect("/admin/settings")
}

func (h *Handler) PostDeleteSettingHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteSettingService(h.Model, id); err != nil {
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

	return RenderAdminView(c, "settings_new", fiber.Map{
		"Title":              "New Setting",
		"OptionsParsed":      optionsParsed,
		"DefaultOptionValue": defaultOptionValue,
	}, "")
}

func (h *Handler) PostCreateSettingHandler(c *fiber.Ctx) error {
	s := &db.Setting{
		Kind:               c.FormValue("kind"),
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
	if c.FormValue("reload") != "" {
		s.Reload = 1
	} else {
		s.Reload = 0
	}

	if s.Kind == "" {
		s.Kind = "default"
	}

	if err := CreateSettingService(h.Model, s); err != nil {
		// 解析 options 用于错误回显
		var optionsParsed []map[string]string
		if s.Options != "" {
			json.Unmarshal([]byte(s.Options), &optionsParsed)
		}

		return RenderAdminView(c, "settings_new", fiber.Map{
			"Title":              "New Setting",
			"Error":              err.Error(),
			"Setting":            s,
			"OptionsParsed":      optionsParsed,
			"DefaultOptionValue": s.DefaultOptionValue,
		}, "")
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

	return RenderAdminView(c, "trash_index", fiber.Map{
		"Title":     "Trash",
		"Data":      data,
		"ModelType": modelType,
	}, "")
}

func (h *Handler) PostRestorePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestorePostService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostHardDeletePostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeletePostService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreEncryptedPostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreEncryptedPostService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostHardDeleteEncryptedPostHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteEncryptedPostService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreTagService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostHardDeleteTagHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteTagService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreRedirectService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostHardDeleteRedirectHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteRedirectService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostRestoreCategoryHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := RestoreCategoryService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

func (h *Handler) PostHardDeleteCategoryHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := HardDeleteCategoryService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/trash")
}

// HttpErrorLogs
func (h *Handler) GetHttpErrorLogListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)

	logs, err := ListHttpErrorLogs(h.Model, &pager)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "http_error_logs_index", fiber.Map{
		"Title": "Http Error Logs",
		"Logs":  logs,
		"Pager": pager,
	}, "")
}

func (h *Handler) PostDeleteHttpErrorLogHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteHttpErrorLogService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/http-error-logs")
}

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

func buildCommentListURL(status db.CommentStatus) string {
	baseURL := "/admin/comments"
	if status == "" {
		return baseURL
	}
	return baseURL + "?status=" + url.QueryEscape(string(status))
}

// Comments
func (h *Handler) GetCommentListHandler(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	status := normalizeCommentStatus(c.Query("status"))

	comments, err := ListCommentsService(h.Model, status, &pager)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "comments_index", fiber.Map{
		"Title":        "评论管理",
		"Comments":     comments,
		"Pager":        pager,
		"StatusFilter": string(status),
	}, "")
}

func (h *Handler) PostApproveCommentHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = UpdateCommentStatusService(h.Model, id, db.CommentStatusApproved); err != nil {
		return err
	}

	return c.Redirect(buildCommentListURL(normalizeCommentStatus(c.FormValue("status_filter"))))
}

func (h *Handler) PostPendingCommentHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = UpdateCommentStatusService(h.Model, id, db.CommentStatusPending); err != nil {
		return err
	}

	return c.Redirect(buildCommentListURL(normalizeCommentStatus(c.FormValue("status_filter"))))
}

func (h *Handler) PostSpamCommentHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = UpdateCommentStatusService(h.Model, id, db.CommentStatusSpam); err != nil {
		return err
	}

	return c.Redirect(buildCommentListURL(normalizeCommentStatus(c.FormValue("status_filter"))))
}

func (h *Handler) PostDeleteCommentHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.ErrBadRequest
	}

	if err = DeleteCommentService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect(buildCommentListURL(normalizeCommentStatus(c.FormValue("status_filter"))))
}

// Tasks
func (h *Handler) GetTaskListHandler(c *fiber.Ctx) error {
	tasks, err := ListTasksService(h.Model)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "tasks_index", fiber.Map{
		"Title": "Tasks",
		"Tasks": tasks,
	}, "")
}

func (h *Handler) GetTaskNewHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "tasks_new", fiber.Map{
		"Title": "New Task",
	}, "")
}

func (h *Handler) PostCreateTaskHandler(c *fiber.Ctx) error {
	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"

	kind := db.TaskInternal
	if k := c.FormValue("kind"); k == "1" {
		kind = db.TaskUser
	}

	in := CreateTaskInput{
		Code:        c.FormValue("code"),
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		Schedule:    c.FormValue("schedule"),
		Enabled:     enabled,
		Kind:        kind,
	}

	if err := CreateTaskService(h.Model, in); err != nil {
		return err
	}

	return c.Redirect("/admin/tasks")
}

func (h *Handler) GetTaskEditHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	task, err := GetTaskForEdit(h.Model, id)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "tasks_edit", fiber.Map{
		"Title": "Edit Task",
		"Task":  task,
	}, "")
}

func (h *Handler) PostUpdateTaskHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	enabled := c.FormValue("enabled") == "1" || c.FormValue("enabled") == "on" || c.FormValue("enabled") == "true"
	kind := db.TaskInternal
	if k := c.FormValue("kind"); k == "1" {
		kind = db.TaskUser
	}

	in := UpdateTaskInput{
		Code:        c.FormValue("code"),
		Name:        c.FormValue("name"),
		Description: c.FormValue("description"),
		Schedule:    c.FormValue("schedule"),
		Enabled:     enabled,
		Kind:        kind,
	}

	if err := UpdateTaskService(h.Model, id, in); err != nil {
		return err
	}

	return c.Redirect("/admin/tasks")
}

func (h *Handler) PostDeleteTaskHandler(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteTaskService(h.Model, id); err != nil {
		return err
	}

	return c.Redirect("/admin/tasks")
}

func (h *Handler) PostTriggerTaskHandler(c *fiber.Ctx) error {
	taskCode := c.Params("code")
	if taskCode == "" {
		return fiber.ErrBadRequest
	}

	task, err := db.GetTaskByCode(h.Model, taskCode)
	if err != nil {
		return err
	}
	go job.ExecuteTask(h.Model, *task)

	return c.Redirect("/admin/tasks")
}

func (h *Handler) GetTaskRunListHandler(c *fiber.Ctx) error {
	taskCode := c.Params("code")
	if taskCode == "" {
		return fiber.ErrBadRequest
	}

	// 获取 task 信息
	task, err := db.GetTaskByCode(h.Model, taskCode)
	if err != nil {
		return err
	}

	// 获取执行记录列表，默认限制 100 条
	runs, err := ListTaskRunsService(h.Model, taskCode, 100)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "task_runs_index", fiber.Map{
		"Title": "Task Runs: " + task.Name,
		"Task":  task,
		"Runs":  runs,
	}, "")
}

// Import/Export
func (h *Handler) GetImportHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "import", fiber.Map{
		"Title": "Import Markdown",
	}, "")
}

func (h *Handler) PostImportHandler(c *fiber.Ctx) error {
	// 获取上传的文件
	form, err := c.MultipartForm()
	if err != nil {
		return RenderAdminView(c, "import", fiber.Map{
			"Title": "Import Markdown",
			"Error": "Failed to parse form: " + err.Error(),
		}, "")
	}

	files := form.File["files"]
	if len(files) == 0 {
		return RenderAdminView(c, "import", fiber.Map{
			"Title": "Import Markdown",
			"Error": "Please select at least one file to import",
		}, "")
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

	// 获取 title 来源选择
	titleSourceStr := c.FormValue("title_source")
	titleSource := TitleFromFrontmatter // 默认从 frontmatter 获取
	switch titleSourceStr {
	case "filename":
		titleSource = TitleFromFilename
	case "frontmatter":
		titleSource = TitleFromFrontmatter
	case "markdown":
		titleSource = TitleFromMarkdown
	}

	// 如果是从 frontmatter，获取字段名
	titleField := c.FormValue("title_field")
	if titleField == "" {
		titleField = "title"
	}

	// 如果是从 markdown，获取标题级别
	titleLevel := 1 // 默认 H1
	if titleSourceStr == "markdown" {
		if levelStr := c.FormValue("title_level"); levelStr != "" {
			if level, err := strconv.Atoi(levelStr); err == nil && level >= 1 && level <= 3 {
				titleLevel = level
			}
		}
	}

	// 读取所有文件
	var importFiles []ImportFile
	for _, fileHeader := range files {
		src, err := fileHeader.Open()
		if err != nil {
			return RenderAdminView(c, "import", fiber.Map{
				"Title": "Import Markdown",
				"Error": "Failed to open file " + fileHeader.Filename + ": " + err.Error(),
			}, "")
		}

		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return RenderAdminView(c, "import", fiber.Map{
				"Title": "Import Markdown",
				"Error": "Failed to read file " + fileHeader.Filename + ": " + err.Error(),
			}, "")
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

	// 获取 created_at 来源选择
	createdSourceStr := c.FormValue("created_source")
	createdSource := CreatedFromFrontmatter // 默认从 frontmatter 获取
	switch createdSourceStr {
	case "frontmatter":
		createdSource = CreatedFromFrontmatter
	case "filetime":
		createdSource = CreatedFromFileTime
	default:
		createdSource = CreatedFromFrontmatter
	}

	// 如果是从 frontmatter，获取字段名
	createdField := c.FormValue("created_field")
	if createdField == "" {
		createdField = "date"
	}

	// 获取 status 来源选择
	statusSourceStr := c.FormValue("status_source")
	statusSource := StatusFromFrontmatter // 默认从 frontmatter 获取
	switch statusSourceStr {
	case "frontmatter":
		statusSource = StatusFromFrontmatter
	case "alldraft":
		statusSource = StatusAllDraft
	case "allpublished":
		statusSource = StatusAllPublished
	default:
		statusSource = StatusFromFrontmatter
	}

	// 如果是从 frontmatter，获取字段名
	statusField := c.FormValue("status_field")
	if statusField == "" {
		statusField = "draft"
	}

	// 获取 category 来源选择
	categorySourceStr := c.FormValue("category_source")
	categorySource := CategoryNone // 默认留空
	switch categorySourceStr {
	case "frontmatter":
		categorySource = CategoryFromFrontmatter
	case "autocreate":
		categorySource = CategoryAutoCreate
	case "none":
		categorySource = CategoryNone
	default:
		categorySource = CategoryNone
	}

	// 如果是从 frontmatter，获取字段名
	categoryField := c.FormValue("category_field")
	if categoryField == "" {
		categoryField = "category"
	}

	// 获取 tag 来源选择
	tagSourceStr := c.FormValue("tag_source")
	tagSource := TagNone // 默认留空
	switch tagSourceStr {
	case "frontmatter":
		tagSource = TagFromFrontmatter
	case "autocreate":
		tagSource = TagAutoCreate
	case "none":
		tagSource = TagNone
	default:
		tagSource = TagNone
	}

	// 如果是从 frontmatter，获取字段名
	tagField := c.FormValue("tag_field")
	if tagField == "" {
		tagField = "tags"
	}

	// 解析文件但不入库，返回预览数据
	items, err := ParseImportFiles(importFiles, slugSource, slugField, titleSource, titleField, titleLevel, createdSource, createdField, statusSource, statusField, categorySource, categoryField, tagSource, tagField)
	if err != nil && len(items) == 0 {
		// 如果全部解析失败，返回错误
		return RenderAdminView(c, "import", fiber.Map{
			"Title": "Import Markdown",
			"Error": err.Error(),
		}, "")
	}

	// 构建 title 来源描述
	titleSourceDesc := ""
	switch titleSourceStr {
	case "filename":
		titleSourceDesc = "filename"
	case "frontmatter":
		titleSourceDesc = fmt.Sprintf("frontmatter(%s)", titleField)
	case "markdown":
		levelMap := map[int]string{1: "H1", 2: "H2", 3: "H3"}
		levelName := levelMap[titleLevel]
		if levelName == "" {
			levelName = fmt.Sprintf("H%d", titleLevel)
		}
		titleSourceDesc = fmt.Sprintf("markdown(%s)", levelName)
	default:
		titleSourceDesc = "frontmatter(title)"
	}

	// 构建 slug 来源描述
	slugSourceDesc := ""
	switch slugSourceStr {
	case "filename":
		slugSourceDesc = "filename"
	case "frontmatter":
		slugSourceDesc = fmt.Sprintf("frontmatter(%s)", slugField)
	case "title":
		slugSourceDesc = "title"
	default:
		slugSourceDesc = "title"
	}

	// 构建 created_at 来源描述
	createdSourceDesc := ""
	switch createdSourceStr {
	case "frontmatter":
		createdSourceDesc = fmt.Sprintf("frontmatter(%s)", createdField)
	case "filetime":
		createdSourceDesc = "filetime"
	default:
		createdSourceDesc = "frontmatter(date)"
	}

	// 构建 status 来源描述
	statusSourceDesc := ""
	switch statusSourceStr {
	case "frontmatter":
		statusSourceDesc = fmt.Sprintf("frontmatter(%s)", statusField)
	case "alldraft":
		statusSourceDesc = "alldraft"
	case "allpublished":
		statusSourceDesc = "allpublished"
	default:
		statusSourceDesc = "frontmatter(draft)"
	}

	// 即使有部分错误，也显示预览页面（有警告信息）
	return RenderAdminView(c, "import_preview", fiber.Map{
		"Title":             "Import Preview",
		"Items":             items,
		"Error":             err, // 如果有错误，显示警告信息
		"TitleSourceDesc":   titleSourceDesc,
		"SlugSourceDesc":    slugSourceDesc,
		"CreatedSourceDesc": createdSourceDesc,
		"StatusSourceDesc":  statusSourceDesc,
	}, "")
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
		return RenderAdminView(c, "import_preview", fiber.Map{
			"Title": "Import Preview",
			"Items": []PreviewPostItem{},
			"Error": "No items to import",
		}, "")
	}

	var items []PreviewPostItem

	// 构建 items
	for i := 0; i < itemCount; i++ {
		kindVal := c.FormValue(fmt.Sprintf("items[%d][kind]", i))
		if kindVal != "0" && kindVal != "1" {
			kindVal = "0"
		}
		item := PreviewPostItem{
			Index:     i,
			Title:     c.FormValue(fmt.Sprintf("items[%d][title]", i)),
			Slug:      c.FormValue(fmt.Sprintf("items[%d][slug]", i)),
			Content:   c.FormValue(fmt.Sprintf("items[%d][content]", i)),
			Status:    c.FormValue(fmt.Sprintf("items[%d][status]", i)),
			Kind:      kindVal,
			CreatedAt: c.FormValue(fmt.Sprintf("items[%d][created_at]", i)),
			Tags:      c.FormValue(fmt.Sprintf("items[%d][tags]", i)),
			Category:  c.FormValue(fmt.Sprintf("items[%d][category]", i)),
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
		return RenderAdminView(c, "import_preview", fiber.Map{
			"Title": "Import Preview",
			"Items": items,
			"Error": "No items to import",
		}, "")
	}

	// 校验每条记录的 slug
	for _, item := range items {
		if !helper.IsSlug(strings.TrimSpace(item.Slug)) {
			return RenderAdminView(c, "import_preview", fiber.Map{
				"Title": "Import Preview",
				"Items": items,
				"Error": fmt.Sprintf("%s（第 %d 条记录）", errSlugInvalid("015", strings.TrimSpace(item.Slug)).Error(), item.Index+1),
			}, "")
		}
	}

	// 调用 service 进行导入
	if err := ImportPreviewService(h.Model, items); err != nil {
		return RenderAdminView(c, "import_preview", fiber.Map{
			"Title": "Import Preview",
			"Items": items,
			"Error": err.Error(),
		}, "")
	}

	return c.Redirect("/admin/posts")
}

// Metrics
func (h *Handler) GetMetricsHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "metrics", fiber.Map{
		"Title": "性能监控",
	}, "")
}

func (h *Handler) GetDevUIComponentsHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "dev_ui_components", fiber.Map{
		"Title": "UI组件",
	}, "")
}

// Export
func (h *Handler) GetExportHandler(c *fiber.Ctx) error {
	return RenderAdminView(c, "export", fiber.Map{
		"Title": "导出数据库",
	}, "")
}

func (h *Handler) GetExportDownloadHandler(c *fiber.Ctx) error {
	// 生成导出文件名（包含时间戳）
	name := strings.ToLower(c.App().Config().AppName) + "_export"

	log.Println("export to:", os.TempDir(), name)

	// 创建临时目录
	tmpDir := filepath.Join(os.TempDir(), name)
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		return RenderAdminView(c, "export", fiber.Map{
			"Title": "导出数据库",
			"Error": "Failed to create export directory: " + err.Error(),
		}, "")
	}

	// 调用 ExportSQLiteWithHash 函数（传目录，函数内自动生成文件名）
	result, err := db.ExportSQLiteWithHash(h.Model, tmpDir)
	if err != nil {
		return RenderAdminView(c, "export", fiber.Map{
			"Title": "导出数据库",
			"Error": "Failed to export database: " + err.Error(),
		}, "")
	}

	// 返回文件下载（从完整路径中提取文件名）
	filename := filepath.Base(result.File)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Set("Content-Type", "application/x-sqlite3")

	// 发送文件
	if err := c.SendFile(result.File); err != nil {
		return RenderAdminView(c, "export", fiber.Map{
			"Title": "导出数据库",
			"Error": "Failed to send file: " + err.Error(),
		}, "")
	}

	// 下载完成后删除临时文件
	go func() {
		time.Sleep(5 * time.Second) // 等待下载完成
		os.Remove(result.File)
	}()

	return nil
}
