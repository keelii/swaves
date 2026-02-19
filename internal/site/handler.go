package site

import (
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/db"
	"swaves/internal/middleware"
	"swaves/internal/share"
	"swaves/internal/store"
	"swaves/internal/types"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	Model   *db.DB
	Session *types.SessionStore
	Service *Service
}

func (h Handler) trackSiteUV(c *fiber.Ctx) {
	h.trackEntityUV(c, db.UVEntitySite, 0)
}

func (h Handler) trackUV(c *fiber.Ctx, entityType db.UVEntityType, entityID int64) {
	if !entityType.IsValid() || entityID <= 0 {
		h.trackSiteUV(c)
		return
	}

	h.trackEntityUV(c, entityType, entityID)
}

func (h Handler) trackEntityUV(c *fiber.Ctx, entityType db.UVEntityType, entityID int64) {
	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		return
	}

	if _, err := db.UpsertUVUnique(h.Model, entityType, entityID, visitorID); err != nil {
		log.Printf("track entity uv failed: %v", err)
	}
}

func (h Handler) getEntityUVCount(entityType db.UVEntityType, entityID int64) int {
	count, err := db.CountUVUnique(h.Model, entityType, entityID)
	if err != nil {
		log.Printf("count entity uv failed: %v", err)
		return 0
	}
	return count
}

func (h Handler) getPostLikeState(c *fiber.Ctx, postID int64) (int, bool) {
	likeCount, err := db.CountEntityLikes(h.Model, postID)
	if err != nil {
		log.Printf("count entity like failed: %v", err)
		return 0, false
	}

	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		return likeCount, false
	}

	liked, err := db.IsEntityLikedByVisitor(h.Model, postID, visitorID)
	if err != nil {
		log.Printf("check entity like failed: %v", err)
		return likeCount, false
	}

	return likeCount, liked
}

func isSafeReturnPath(path string) bool {
	if path == "" || !strings.HasPrefix(path, "/") {
		return false
	}
	return !strings.Contains(path, "//")
}

func getSiteFallbackPath() string {
	basePath := store.GetSetting("base_path")
	if basePath == "" {
		return "/"
	}
	return basePath
}

func resolveReturnPath(c *fiber.Ctx) string {
	if path := strings.TrimSpace(c.FormValue("return_url")); isSafeReturnPath(path) {
		return path
	}
	referer := strings.TrimSpace(c.Get("Referer"))
	if isSafeReturnPath(referer) {
		return referer
	}
	if referer != "" {
		if parsed, err := url.Parse(referer); err == nil {
			candidate := parsed.EscapedPath()
			if parsed.RawQuery != "" {
				candidate += "?" + parsed.RawQuery
			}
			if isSafeReturnPath(candidate) {
				return candidate
			}
		}
	}
	return getSiteFallbackPath()
}

func shouldReturnLikeJSON(c *fiber.Ctx) bool {
	accept := strings.ToLower(strings.TrimSpace(c.Get(fiber.HeaderAccept)))
	if strings.Contains(accept, fiber.MIMEApplicationJSON) {
		return true
	}

	requestedWith := strings.ToLower(strings.TrimSpace(c.Get("X-Requested-With")))
	return requestedWith == "xmlhttprequest"
}

func normalizeCommentFeedbackStatus(raw string) string {
	status := strings.TrimSpace(raw)
	switch status {
	case string(db.CommentStatusApproved):
		return string(db.CommentStatusApproved)
	case string(db.CommentStatusPending):
		return string(db.CommentStatusPending)
	case commentFeedbackCaptchaFailed:
		return commentFeedbackCaptchaFailed
	case commentFeedbackRateLimited:
		return commentFeedbackRateLimited
	default:
		return ""
	}
}

func appendQueryParam(path, key, value string) string {
	parsed, err := url.Parse(path)
	if err != nil {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		return path + sep + key + "=" + url.QueryEscape(value)
	}

	query := parsed.Query()
	query.Set(key, value)
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func (h Handler) ensureLikePostExists(postID int64) error {
	post, err := db.GetPostByID(h.Model, postID)
	if err != nil {
		return err
	}
	if post.Status != "published" || post.PublishedAt <= 0 {
		return db.ErrNotFound("PostEntityLike.Post")
	}
	return nil
}

func RenderUIView(c *fiber.Ctx, view string, data fiber.Map, layout string) error {
	if layout == "" {
		layout = "site/layout"
	}
	if data == nil {
		data = fiber.Map{}
	}

	data["UrlPath"] = c.Path()
	data["Query"] = c.Queries()
	data["IsLogin"] = c.Locals("IsLogin")

	//// 注入 Locals
	//c.Context().VisitUserValues(func(k []byte, v interface{}) {
	//	//log.Println("Injecting local:", string(k))
	//	data[string(k)] = v
	//})

	return c.Render(view, data, layout)
}

func (h Handler) GetDate(c *fiber.Ctx) error {
	dt := c.Params("dob")
	fmt.Println("===")
	fmt.Println(dt)
	fmt.Println("===")
	return c.Send([]byte("ui home"))
}
func (h Handler) GetHome(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	articles := ListDisplayPosts(h.Model, db.PostKindPost, &pager)
	h.trackSiteUV(c)

	return RenderUIView(c, "site/home", fiber.Map{
		"Articles": articles,
		"Pages":    ListPages(h.Model),
		"Pager":    pager,
	}, "")
}

//func (h Handler) GetPost(c *fiber.Ctx) error {
//	matched := MatchRouter(c.Path())
//	if len(matched) == 0 {
//		return c.Status(fiber.StatusNotFound).SendString("post not found")
//	}
//
//	post := GetPostBySlug(h.Model, matched["slug"])
//
//	return RenderUIView(c, "site/post", fiber.Map{
//		"Post": post,
//		//"Pages": ListPages(h.Model),
//	}, "")
//}

func (h Handler) GetPostByDateAndSlug(c *fiber.Ctx) error {
	year := c.Params("year")
	month := c.Params("month")
	day := c.Params("day")

	if year == "" || month == "" || day == "" {
		return c.Status(fiber.StatusBadRequest).SendString("invalid date format")
	}

	pSlug := c.Params("slug")
	post := GetPostBySlug(h.Model, pSlug)
	if post == nil {
		return c.Status(fiber.StatusNotFound).SendString("page not found")
	}

	y, m, d := share.GetArticlePublishedDate(post.Post)

	if y != year || m != month || d != day {
		return c.Status(fiber.StatusNotFound).SendString("page not found, maybe the date is wrong")
	}

	h.trackUV(c, db.UVEntityPost, post.Post.ID)
	readUV := h.getEntityUVCount(db.UVEntityPost, post.Post.ID)
	likeCount, liked := h.getPostLikeState(c, post.Post.ID)
	comments := ListApprovedCommentsTree(h.Model, post.Post.ID)
	commentCount := CountApprovedComments(h.Model, post.Post.ID)
	commentFeedback := normalizeCommentFeedbackStatus(c.Query("comment_status"))
	commentCaptcha := buildCommentCaptchaChallenge(middleware.GetOrCreateVisitorID(c, ""))

	return RenderUIView(c, "site/post", fiber.Map{
		"Post":            post,
		"ReadUV":          readUV,
		"LikeCount":       likeCount,
		"Liked":           liked,
		"Comments":        comments,
		"CommentCount":    commentCount,
		"CommentFeedback": commentFeedback,
		"CommentCaptcha":  commentCaptcha,
		//"Pages": ListPages(h.Model),
	}, "")
}
func (h Handler) GetPostBySlug(c *fiber.Ctx) error {
	pSlug := c.Params("slug")
	post := GetPostBySlug(h.Model, pSlug)
	if post == nil {
		return c.Status(fiber.StatusNotFound).SendString("page not found")
	}

	h.trackUV(c, db.UVEntityPost, post.Post.ID)
	readUV := h.getEntityUVCount(db.UVEntityPost, post.Post.ID)
	likeCount, liked := h.getPostLikeState(c, post.Post.ID)
	comments := ListApprovedCommentsTree(h.Model, post.Post.ID)
	commentCount := CountApprovedComments(h.Model, post.Post.ID)
	commentFeedback := normalizeCommentFeedbackStatus(c.Query("comment_status"))
	commentCaptcha := buildCommentCaptchaChallenge(middleware.GetOrCreateVisitorID(c, ""))

	return RenderUIView(c, "site/post", fiber.Map{
		"Post":            post,
		"ReadUV":          readUV,
		"LikeCount":       likeCount,
		"Liked":           liked,
		"Comments":        comments,
		"CommentCount":    commentCount,
		"CommentFeedback": commentFeedback,
		"CommentCaptcha":  commentCaptcha,
		//"Pages": ListPages(h.Model),
	}, "")
}

func (h Handler) GetRSS(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	posts := ListDisplayPosts(h.Model, db.PostKindPost, &pager)
	rss, err := GenerateRSS(posts, c, pager.Page, pager.Total)
	if err != nil {
		return err
	}
	c.Set("Content-Type", "application/xml; charset=utf-8")
	return c.SendString(rss)
}
func (h Handler) GetCategoryIndex(c *fiber.Ctx) error {
	categories := ListCategories(h.Model)

	if categories == nil {
		return c.Status(fiber.StatusNotFound).SendString("categories not found")
	}

	pages := ListPages(h.Model)
	h.trackSiteUV(c)
	return RenderUIView(c, "site/list", fiber.Map{
		"Title": "Categories",
		"Pages": pages,
		"List":  categories,
	}, "")
}
func (h Handler) GetTagIndex(c *fiber.Ctx) error {
	tags := ListTags(h.Model)

	if tags == nil {
		return c.Status(fiber.StatusNotFound).SendString("tags not found")
	}

	h.trackSiteUV(c)
	return RenderUIView(c, "site/list", fiber.Map{
		"Title": "Tags",
		"Pages": ListPages(h.Model),
		"List":  tags,
	}, "")
}
func (h Handler) GetCategoryDetail(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	slug := c.Params("categorySlug")
	category := GetCategoryBySlug(h.Model, slug)
	if category == nil {
		return c.Status(fiber.StatusNotFound).SendString("category not found")
	}

	h.trackUV(c, db.UVEntityCategory, category.ID)

	posts := ListPostsByCategory(h.Model, category.ID, &pager)

	return RenderUIView(c, "site/detail", fiber.Map{
		"IsCategory": true,
		"Entity":     category,
		"List":       posts,
		"ListPage":   share.GetCategoryIndex(),
		"Pages":      ListPages(h.Model),
	}, "")
}
func (h Handler) GetTagDetail(c *fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	slug := c.Params("tagSlug")
	tag := GetTagBySlug(h.Model, slug)
	if tag == nil {
		return c.Status(fiber.StatusNotFound).SendString("tag not found")
	}

	h.trackUV(c, db.UVEntityTag, tag.ID)

	posts := ListPostsByTag(h.Model, tag.ID, &pager)

	return RenderUIView(c, "site/detail", fiber.Map{
		"IsTag":    true,
		"Entity":   tag,
		"List":     posts,
		"ListPage": share.GetTagIndex(),
		"Pages":    ListPages(h.Model),
	}, "")
}

func (h Handler) PostEntityLike(c *fiber.Ctx) error {
	postID, err := strconv.ParseInt(c.Params("postID"), 10, 64)
	if err != nil || postID <= 0 {
		return fiber.ErrBadRequest
	}

	if err = h.ensureLikePostExists(postID); err != nil {
		if db.IsErrNotFound(err) {
			return fiber.ErrNotFound
		}
		return err
	}

	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		return fiber.ErrBadRequest
	}

	liked, err := db.IsEntityLikedByVisitor(h.Model, postID, visitorID)
	if err != nil {
		return err
	}

	nextStatus := db.LikeStatusActive
	if liked {
		nextStatus = db.LikeStatusInactive
	}

	if err = db.UpsertEntityLike(h.Model, postID, visitorID, nextStatus); err != nil {
		return err
	}

	likeCount, err := db.CountEntityLikes(h.Model, postID)
	if err != nil {
		return err
	}

	if shouldReturnLikeJSON(c) {
		return c.JSON(fiber.Map{
			"liked":     nextStatus == db.LikeStatusActive,
			"likeCount": likeCount,
		})
	}

	return c.Redirect(resolveReturnPath(c))
}

func (h Handler) PostComment(c *fiber.Ctx) error {
	postID, err := strconv.ParseInt(c.Params("postID"), 10, 64)
	if err != nil || postID <= 0 {
		return fiber.ErrBadRequest
	}
	if err = h.ensureLikePostExists(postID); err != nil {
		if db.IsErrNotFound(err) {
			return fiber.ErrNotFound
		}
		return err
	}
	visitorID := middleware.GetOrCreateVisitorID(c, "")
	captchaToken := strings.TrimSpace(c.FormValue(commentCaptchaTokenField))
	captchaAnswer := strings.TrimSpace(c.FormValue(commentCaptchaAnswerField))
	if !verifyCommentCaptchaChallenge(visitorID, captchaToken, captchaAnswer) {
		redirectPath := appendQueryParam(resolveReturnPath(c), "comment_status", commentFeedbackCaptchaFailed)
		if !strings.Contains(redirectPath, "#") {
			redirectPath += "#comments"
		}
		return c.Redirect(redirectPath, fiber.StatusSeeOther)
	}

	parentID := int64(0)
	if rawParentID := strings.TrimSpace(c.FormValue("parent_id")); rawParentID != "" {
		parentID, err = strconv.ParseInt(rawParentID, 10, 64)
		if err != nil || parentID < 0 {
			return fiber.ErrBadRequest
		}
	}
	if parentID > 0 {
		parentComment, parentErr := db.GetCommentByID(h.Model, parentID)
		if parentErr != nil {
			if db.IsErrNotFound(parentErr) {
				return fiber.ErrBadRequest
			}
			return parentErr
		}
		if parentComment.PostID != postID {
			return fiber.ErrBadRequest
		}
	}

	author := strings.TrimSpace(c.FormValue("author"))
	if author == "" || len(author) > 80 {
		return fiber.ErrBadRequest
	}

	content := strings.TrimSpace(c.FormValue("content"))
	if content == "" || len(content) > 5000 {
		return fiber.ErrBadRequest
	}

	authorEmail := strings.TrimSpace(c.FormValue("author_email"))
	if len(authorEmail) > 120 {
		return fiber.ErrBadRequest
	}
	authorURL := strings.TrimSpace(c.FormValue("author_url"))
	if len(authorURL) > 300 {
		return fiber.ErrBadRequest
	}

	isLogin, _ := c.Locals("IsLogin").(bool)
	status := db.CommentStatusPending
	if isLogin {
		status = db.CommentStatusApproved
	}

	comment := &db.Comment{
		PostID:      postID,
		ParentID:    parentID,
		Author:      author,
		AuthorEmail: authorEmail,
		AuthorURL:   authorURL,
		AuthorIP:    strings.TrimSpace(c.IP()),
		VisitorID:   visitorID,
		UserAgent:   strings.TrimSpace(c.Get("User-Agent")),
		Content:     content,
		Status:      status,
	}
	if _, err = db.CreateComment(h.Model, comment); err != nil {
		return err
	}

	redirectPath := appendQueryParam(resolveReturnPath(c), "comment_status", string(status))
	if !strings.Contains(redirectPath, "#") {
		redirectPath += "#comments"
	}
	return c.Redirect(redirectPath)
}

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
