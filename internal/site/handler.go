package site

import (
	"errors"
	"fmt"
	"log"
	"net/url"
	"path/filepath"
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
	case commentFeedbackCaptchaRequired:
		return commentFeedbackCaptchaRequired
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

func getSitePath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return share.GetBasePath() + path
}

func normalizeErrorReturnURL(raw string) string {
	candidate := strings.TrimSpace(raw)
	if isSafeReturnPath(candidate) {
		return candidate
	}
	if parsed, err := url.Parse(candidate); err == nil {
		path := parsed.EscapedPath()
		if parsed.RawQuery != "" {
			path += "?" + parsed.RawQuery
		}
		if isSafeReturnPath(path) {
			return path
		}
	}
	return getSiteFallbackPath()
}

func buildSiteErrorRedirectPath(c *fiber.Ctx, targetPath string) string {
	returnURL := strings.TrimSpace(c.Query("returnUrl"))
	if returnURL == "" {
		returnURL = strings.TrimSpace(c.OriginalURL())
	}
	returnURL = normalizeErrorReturnURL(returnURL)
	if returnURL == targetPath {
		returnURL = getSiteFallbackPath()
	}
	return appendQueryParam(targetPath, "returnUrl", returnURL)
}

func (h Handler) redirectNotFound(c *fiber.Ctx) error {
	returnURL := strings.TrimSpace(c.Query("returnUrl"))
	if returnURL == "" {
		returnURL = strings.TrimSpace(c.OriginalURL())
	}
	returnURL = normalizeErrorReturnURL(returnURL)

	c.Status(fiber.StatusNotFound)
	return RenderUIView(c, "site/404", fiber.Map{
		"Title":     "404 Not Found",
		"Pages":     ListPages(h.Model),
		"ReturnURL": returnURL,
		"ReqID":     c.Locals("reqId"),
	}, "")
}

func (h Handler) redirectError(c *fiber.Ctx) error {
	targetPath := getSitePath("/error")
	return c.Redirect(buildSiteErrorRedirectPath(c, targetPath), fiber.StatusFound)
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

func (h Handler) GetNotFound(c *fiber.Ctx) error {
	returnURL := normalizeErrorReturnURL(c.Query("returnUrl"))
	c.Status(fiber.StatusNotFound)
	return RenderUIView(c, "site/404", fiber.Map{
		"Title":     "404 Not Found",
		"Pages":     ListPages(h.Model),
		"ReturnURL": returnURL,
		"ReqID":     c.Locals("reqId"),
	}, "")
}

func (h Handler) GetError(c *fiber.Ctx) error {
	returnURL := normalizeErrorReturnURL(c.Query("returnUrl"))
	c.Status(fiber.StatusInternalServerError)
	return RenderUIView(c, "site/error", fiber.Map{
		"Title":     "Error",
		"Pages":     ListPages(h.Model),
		"ReturnURL": returnURL,
		"ReqID":     c.Locals("reqId"),
	}, "")
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
//	post := GetPostByIST(h.Model, matched["slug"])
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
		return h.redirectNotFound(c)
	}

	post, err := h.GetPostByIDSlugTitle(c)
	if err != nil {
		return h.redirectNotFound(c)
	}

	if post == nil {
		return h.redirectNotFound(c)
	}

	y, m, d := share.GetArticlePublishedDate(post.Post)

	if y != year || m != month || d != day {
		return h.redirectNotFound(c)
	}

	h.trackUV(c, db.UVEntityPost, post.Post.ID)
	readUV, likeCount, liked, comments, commentCount, commentFeedback, commentForm, captchaRequired, commentCaptcha := h.funcName(c, post)

	return RenderUIView(c, "site/post", fiber.Map{
		"Post":                   post,
		"ReadUV":                 readUV,
		"LikeCount":              likeCount,
		"Liked":                  liked,
		"Comments":               comments,
		"CommentCount":           commentCount,
		"CommentFeedback":        commentFeedback,
		"CommentForm":            commentForm,
		"CommentCaptchaRequired": captchaRequired,
		"CommentCaptcha":         commentCaptcha,
		//"Pages": ListPages(h.Model),
	}, "")
}

func (h Handler) GetPostByIDSlugTitle(c *fiber.Ctx) (*DisplayPostWithRelation, error) {
	filename := c.Params("ist")
	ext := filepath.Ext(filename)
	ist := strings.TrimSuffix(filename, ext)

	if ext != share.GetPostExt() {
		return nil, errors.New(fmt.Sprintf("%s not found", share.GetPostExt()))
	}

	var post *DisplayPostWithRelation

	if share.PostNameIsID() {
		id, err := strconv.ParseInt(strings.TrimSpace(ist), 10, 64)
		if err != nil {
			return nil, errors.New("invalid post identifier in url")
		}
		post = GetPostByID(h.Model, id)
	} else if share.PostNameIsTitle() {
		title := ist
		if unescapedTitle, err := url.PathUnescape(ist); err == nil {
			title = unescapedTitle
		}
		post = GetPostByTitle(h.Model, title)
	} else {
		post = GetPostBySlug(h.Model, ist)
	}
	return post, nil
}

func (h Handler) funcName(c *fiber.Ctx, post *DisplayPostWithRelation) (int, int, bool, []*DisplayComment, int, string, commentFormDefaults, bool, commentCaptchaChallenge) {
	readUV := h.getEntityUVCount(db.UVEntityPost, post.Post.ID)
	likeCount, liked := h.getPostLikeState(c, post.Post.ID)
	comments := ListApprovedCommentsTree(h.Model, post.Post.ID)
	commentCount := CountApprovedComments(h.Model, post.Post.ID)
	commentFeedback := normalizeCommentFeedbackStatus(c.Query("comment_status"))
	commentForm := readCommentFormDefaults(c)
	visitorID := middleware.GetOrCreateVisitorID(c, "")
	captchaRequired := isCommentCaptchaRequired(visitorID)
	commentCaptcha := commentCaptchaChallenge{}
	if captchaRequired {
		commentCaptcha = buildCommentCaptchaChallenge(visitorID)
	}
	return readUV, likeCount, liked, comments, commentCount, commentFeedback, commentForm, captchaRequired, commentCaptcha
}
func (h Handler) GetPostByIST(c *fiber.Ctx) error {
	post, err := h.GetPostByIDSlugTitle(c)
	if err != nil {
		return h.redirectNotFound(c)
	}
	if post == nil {
		return h.redirectNotFound(c)
	}

	readUV, likeCount, liked, comments, commentCount, commentFeedback, commentForm, captchaRequired, commentCaptcha := h.funcName(c, post)

	return RenderUIView(c, "site/post", fiber.Map{
		"Post":                   post,
		"ReadUV":                 readUV,
		"LikeCount":              likeCount,
		"Liked":                  liked,
		"Comments":               comments,
		"CommentCount":           commentCount,
		"CommentFeedback":        commentFeedback,
		"CommentForm":            commentForm,
		"CommentCaptchaRequired": captchaRequired,
		"CommentCaptcha":         commentCaptcha,
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
		return h.redirectError(c)
	}

	pages := ListPages(h.Model)
	h.trackSiteUV(c)
	return RenderUIView(c, "site/list", fiber.Map{
		"Title":      "Categories",
		"Pages":      pages,
		"List":       categories,
		"IsCategory": true,
	}, "")
}
func (h Handler) GetTagIndex(c *fiber.Ctx) error {
	tags := ListTags(h.Model)

	if tags == nil {
		return h.redirectError(c)
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
		return h.redirectNotFound(c)
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
		return h.redirectNotFound(c)
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
		if shouldReturnLikeJSON(c) {
			return fiber.ErrBadRequest
		}
		return h.redirectError(c)
	}

	if err = h.ensureLikePostExists(postID); err != nil {
		if db.IsErrNotFound(err) {
			if shouldReturnLikeJSON(c) {
				return fiber.ErrNotFound
			}
			return h.redirectNotFound(c)
		}
		if shouldReturnLikeJSON(c) {
			return err
		}
		return h.redirectError(c)
	}

	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		if shouldReturnLikeJSON(c) {
			return fiber.ErrBadRequest
		}
		return h.redirectError(c)
	}

	liked, err := db.IsEntityLikedByVisitor(h.Model, postID, visitorID)
	if err != nil {
		if shouldReturnLikeJSON(c) {
			return err
		}
		return h.redirectError(c)
	}

	nextStatus := db.LikeStatusActive
	if liked {
		nextStatus = db.LikeStatusInactive
	}

	if err = db.UpsertEntityLike(h.Model, postID, visitorID, nextStatus); err != nil {
		if shouldReturnLikeJSON(c) {
			return err
		}
		return h.redirectError(c)
	}

	likeCount, err := db.CountEntityLikes(h.Model, postID)
	if err != nil {
		if shouldReturnLikeJSON(c) {
			return err
		}
		return h.redirectError(c)
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
		return h.redirectError(c)
	}
	if err = h.ensureLikePostExists(postID); err != nil {
		if db.IsErrNotFound(err) {
			return h.redirectNotFound(c)
		}
		return h.redirectError(c)
	}
	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if isCommentCaptchaRequired(visitorID) {
		captchaToken := strings.TrimSpace(c.FormValue(commentCaptchaTokenField))
		captchaAnswer := strings.TrimSpace(c.FormValue(commentCaptchaAnswerField))
		if !verifyCommentCaptchaChallenge(visitorID, captchaToken, captchaAnswer) {
			redirectPath := appendQueryParam(resolveReturnPath(c), "comment_status", commentFeedbackCaptchaFailed)
			if !strings.Contains(redirectPath, "#") {
				redirectPath += "#comments"
			}
			return c.Redirect(redirectPath, fiber.StatusSeeOther)
		}
	}

	parentID := int64(0)
	if rawParentID := strings.TrimSpace(c.FormValue("parent_id")); rawParentID != "" {
		parentID, err = strconv.ParseInt(rawParentID, 10, 64)
		if err != nil || parentID < 0 {
			return h.redirectError(c)
		}
	}
	if parentID > 0 {
		parentComment, parentErr := db.GetCommentByID(h.Model, parentID)
		if parentErr != nil {
			if db.IsErrNotFound(parentErr) {
				return h.redirectError(c)
			}
			return h.redirectError(c)
		}
		if parentComment.PostID != postID {
			return h.redirectError(c)
		}
	}

	author := strings.TrimSpace(c.FormValue("author"))
	if author == "" || len(author) > 80 {
		return h.redirectError(c)
	}

	content := strings.TrimSpace(c.FormValue("content"))
	if content == "" || len(content) > 5000 {
		return h.redirectError(c)
	}

	authorEmail := strings.TrimSpace(c.FormValue("author_email"))
	if len(authorEmail) > 120 {
		return h.redirectError(c)
	}
	authorURL := strings.TrimSpace(c.FormValue("author_url"))
	if len(authorURL) > 300 {
		return h.redirectError(c)
	}
	rememberMe := isCommentRememberMeEnabled(c.FormValue("remember_me"))

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
		return h.redirectError(c)
	}

	saveCommentFormDefaults(c, commentFormDefaults{
		Author:      author,
		AuthorEmail: authorEmail,
		AuthorURL:   authorURL,
		RememberMe:  rememberMe,
	})

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
