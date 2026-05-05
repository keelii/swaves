package site

import (
	"bytes"
	"fmt"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/notify"
	"swaves/internal/platform/perftrace"
	"swaves/internal/platform/store"
	"swaves/internal/shared/helper"
	"swaves/internal/shared/pathutil"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"
	"swaves/internal/shared/webutil"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

type Handler struct {
	Model     *db.DB
	Session   *types.SessionStore
	Service   *Service
	Views     fiber.Views
	UVTracker *UVTracker
}

const (
	siteNotFoundTitle = "404 Not Found"
	siteErrorTitle    = "Error"
)

func (h Handler) trackSiteUV(c fiber.Ctx) {
	h.trackEntityUV(c, db.UVEntitySite, 0)
}

func (h Handler) trackUV(c fiber.Ctx, entityType db.UVEntityType, entityID int64) {
	if !entityType.IsValid() || entityID <= 0 {
		h.trackSiteUV(c)
		return
	}

	h.trackEntityUV(c, entityType, entityID)
}

func (h Handler) trackEntityUV(c fiber.Ctx, entityType db.UVEntityType, entityID int64) {
	var trace *perftrace.Trace
	if perftrace.Enabled() {
		trace = perftrace.Start("site.track_uv",
			perftrace.Field("entity_type", entityType),
			perftrace.Field("entity_id", entityID),
		)
	}
	visitorID := middleware.GetOrCreateVisitorID(c, "")
	trace.Step("visitor_id")
	if visitorID == "" {
		trace.Finish(perftrace.Field("skipped", "empty_visitor"))
		return
	}

	if h.UVTracker != nil {
		h.UVTracker.Track(entityType, entityID, visitorID)
		trace.Step("enqueue")
		trace.Finish(perftrace.Field("mode", "async"))
		return
	}

	updated, err := db.UpsertUVUnique(h.Model, entityType, entityID, visitorID)
	trace.Step("upsert")
	if err != nil {
		logger.Warn("track entity uv failed: %v", err)
		trace.Finish(perftrace.Field("mode", "sync"), perftrace.Field("err", true))
		return
	}
	trace.Finish(
		perftrace.Field("mode", "sync"),
		perftrace.Field("updated", updated),
		perftrace.Field("err", false),
	)
}

func (h Handler) getEntityUVCount(entityType db.UVEntityType, entityID int64) int {
	count, err := db.CountUVUnique(h.Model, entityType, entityID)
	if err != nil {
		logger.Warn("count entity uv failed: %v", err)
		return 0
	}
	return count
}

func (h Handler) getPostLikeState(c fiber.Ctx, postID int64) (int, bool) {
	likeCount, err := db.CountEntityLikes(h.Model, postID)
	if err != nil {
		logger.Warn("count entity like failed: %v", err)
		return 0, false
	}

	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		return likeCount, false
	}

	liked, err := db.IsEntityLikedByVisitor(h.Model, postID, visitorID)
	if err != nil {
		logger.Warn("check entity like failed: %v", err)
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
	return share.GetBasePath()
}

func resolveReturnPath(c fiber.Ctx) string {
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

func shouldReturnLikeJSON(c fiber.Ctx) bool {
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
	case commentFeedbackDuplicate:
		return commentFeedbackDuplicate
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
	return pathutil.JoinAbsolute(share.GetBasePath(), path)
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

func buildSiteErrorRedirectPath(c fiber.Ctx, targetPath string) string {
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

func (h Handler) redirectNotFound(c fiber.Ctx) error {
	if c.Method() == fiber.MethodGet || c.Method() == fiber.MethodHead {
		if redirect, ok := store.GetRedirect(c.Path()); ok {
			return webutil.RedirectTo(c, redirect.To, redirect.Status)
		}
	}

	returnURL := strings.TrimSpace(c.Query("returnUrl"))
	if returnURL == "" {
		returnURL = strings.TrimSpace(c.OriginalURL())
	}
	returnURL = normalizeErrorReturnURL(returnURL)

	c.Status(fiber.StatusNotFound)
	return h.renderView(c, "404.html", fiber.Map{
		"Pages":     ListPages(h.Model),
		"ReturnURL": returnURL,
		"ReqID":     requestid.FromContext(c),
	})
}

func (h Handler) redirectError(c fiber.Ctx) error {
	targetPath := getSitePath("/error")
	return webutil.RedirectTo(c, buildSiteErrorRedirectPath(c, targetPath), fiber.StatusFound)
}

func (h Handler) ensureLikePostExists(postID int64) (db.Post, error) {
	post, err := db.GetPostByID(h.Model, postID)
	if err != nil {
		return db.Post{}, err
	}
	return post, nil
}

func injectDefaultTitle(routeName, view string, data fiber.Map) {
	if _, hasTitle := data["Title"]; hasTitle {
		return
	}

	switch routeName {
	case "site.home":
		data["Title"] = buildPageTitle("")
	case "site.not_found":
		data["Title"] = siteNotFoundTitle
	case "site.error":
		data["Title"] = siteErrorTitle
	case "site.categories":
		data["Title"] = buildPageTitle("Categories")
	case "site.tags":
		data["Title"] = buildPageTitle("Tags")
	}

	if _, hasTitle := data["Title"]; !hasTitle {
		switch view {
		case "404.html":
			data["Title"] = siteNotFoundTitle
		case "error.html":
			data["Title"] = siteErrorTitle
		}
	}
}

func (h Handler) renderView(c fiber.Ctx, view string, data fiber.Map) error {
	var trace *perftrace.Trace
	if perftrace.Enabled() {
		routeName := ""
		if route := c.Route(); route != nil {
			routeName = strings.TrimSpace(route.Name)
		}
		trace = perftrace.Start("site.render_view",
			perftrace.Field("template", view),
			perftrace.Field("route", routeName),
			perftrace.Field("path", c.Path()),
		)
	}

	if data == nil {
		data = fiber.Map{}
	}

	routeName := ""
	if route := c.Route(); route != nil {
		routeName = strings.TrimSpace(route.Name)
	}
	injectDefaultTitle(routeName, view, data)

	data["UrlPath"] = c.Path()
	data["Query"] = c.Queries()
	data["IsLogin"] = fiber.Locals[bool](c, "IsLogin")
	data["IsMobile"] = webutil.IsMobileRequest(c)
	data["RouteName"] = routeName
	trace.Step("prepare_context")

	//// 注入 Locals
	//c.Context().VisitUserValues(func(k []byte, v interface{}) {
	//	//logger.Error("Injecting local:", string(k))
	//	data[string(k)] = v
	//})

	if h.Views == nil {
		err := c.Render(view, data)
		trace.Step("fiber_render")
		trace.Finish(perftrace.Field("err", err != nil))
		return err
	}

	var out bytes.Buffer
	if err := h.Views.Render(&out, view, data); err != nil {
		trace.Step("view_render")
		trace.Finish(
			perftrace.Field("bytes", out.Len()),
			perftrace.Field("err", true),
		)
		return err
	}
	trace.Step("view_render")
	c.Type("html", "utf-8")
	err := c.SendString(out.String())
	trace.Step("send_string")
	trace.Finish(
		perftrace.Field("bytes", out.Len()),
		perftrace.Field("err", err != nil),
	)
	return err
}

func (h Handler) GetDate(c fiber.Ctx) error {
	return c.Send([]byte("ui home"))
}

func (h Handler) GetNotFound(c fiber.Ctx) error {
	returnURL := normalizeErrorReturnURL(c.Query("returnUrl"))
	c.Status(fiber.StatusNotFound)
	return h.renderView(c, "404.html", fiber.Map{
		"Pages":     ListPages(h.Model),
		"ReturnURL": returnURL,
		"ReqID":     requestid.FromContext(c),
	})
}

func (h Handler) GetError(c fiber.Ctx) error {
	returnURL := normalizeErrorReturnURL(c.Query("returnUrl"))
	c.Status(fiber.StatusInternalServerError)
	return h.renderView(c, "error.html", fiber.Map{
		"Pages":     ListPages(h.Model),
		"ReturnURL": returnURL,
		"ReqID":     requestid.FromContext(c),
	})
}

func (h Handler) GetHome(c fiber.Ctx) error {
	var trace *perftrace.Trace
	if perftrace.Enabled() {
		trace = perftrace.Start("site.home",
			perftrace.Field("path", c.Path()),
			perftrace.Field("url", c.OriginalURL()),
		)
	}
	pager := middleware.GetPagination(c)
	trace.Step("pagination")
	articles := ListDisplayPosts(h.Model, db.PostKindPost, &pager, false)
	trace.Step("list_posts")
	templatePosts := ToTemplatePosts(articles)
	trace.Step("template_posts")
	h.trackSiteUV(c)
	trace.Step("track_uv")
	pages := ListPages(h.Model)
	trace.Step("list_pages")

	err := h.renderView(c, "home.html", fiber.Map{
		"CanonicalURL": absoluteSiteURL(c, share.GetBasePath()),
		"Articles":     templatePosts,
		"Pages":        pages,
		"Pager":        pager,
	})
	trace.Step("render")
	trace.Finish(
		perftrace.Field("articles", len(templatePosts)),
		perftrace.Field("pages", len(pages)),
		perftrace.Field("page", pager.Page),
		perftrace.Field("page_size", pager.PageSize),
		perftrace.Field("err", err != nil),
	)
	return err
}
func (h Handler) GetRaw(c fiber.Ctx) error {
	filename := c.Params("*")
	post, err := GetPostBySlugRawWithError(h.Model, filename)
	if err != nil {
		if db.IsErrNotFound(err) {
			return c.Status(fiber.StatusNotFound).SendString("not found")
		}
		return h.redirectError(c)
	}
	if !helper.IsSlug(filename) {
		return c.Status(fiber.StatusBadRequest).SendString("invalid filename")
	}

	return c.SendString(fmt.Sprintf("# %s\n\n%s", post.Title, post.Content))
}

func (h Handler) GetPostByDateAndSlug(c fiber.Ctx) error {
	year := c.Params("year")
	month := c.Params("month")
	day := c.Params("day")

	if year == "" || month == "" || day == "" {
		return h.redirectNotFound(c)
	}

	post, err := h.getPostByIDSlugTitle(c, "post")
	if err != nil {
		if db.IsErrNotFound(err) {
			return h.redirectNotFound(c)
		}
		return h.redirectError(c)
	}

	if post == nil {
		return h.redirectNotFound(c)
	}

	y, m, d := share.GetArticlePublishedDate(post.Post)

	if y != year || m != month || d != day {
		return h.redirectNotFound(c)
	}

	h.trackUV(c, db.UVEntityPost, post.Post.ID)
	readUV, likeCount, liked, comments, commentCount, commentPager, commentFeedback, commentForm, captchaRequired, commentCaptcha := h.funcName(c, post)
	templatePost := ToTemplatePost(post)

	return h.renderView(c, "post.html", fiber.Map{
		"Title":                  buildPageTitle(post.Post.Title),
		"CanonicalURL":           absoluteSiteURL(c, share.GetPostUrl(post.Post)),
		"MetaDescription":        excerptFromHTML(post.HTML, 160),
		"Post":                   templatePost,
		"ReadUV":                 readUV,
		"LikeCount":              likeCount,
		"Liked":                  liked,
		"Comments":               comments,
		"CommentCount":           commentCount,
		"CommentPager":           commentPager,
		"CommentFeedback":        commentFeedback,
		"CommentForm":            commentForm,
		"CommentCaptchaRequired": captchaRequired,
		"CommentCaptcha":         commentCaptcha,
		//"Pages": ListPages(h.Model),
	})
}

func (h Handler) getPostByIDSlugTitle(c fiber.Ctx, t string) (*DisplayPostWithRelation, error) {
	ist, ext := h.getIST(c)

	if ext != "" && ext != share.GetPostExt() {
		return nil, db.ErrNotFound("site.getPostByIDSlugTitle.ext")
	}

	var post *DisplayPostWithRelation
	var err error

	if t == "page" {
		post, err = GetPostBySlugWithError(h.Model, ist)
		if err != nil {
			return nil, err
		}
	} else {
		if share.PostNameIsID() {
			id, err := strconv.ParseInt(strings.TrimSpace(ist), 10, 64)
			if err != nil {
				return nil, db.ErrNotFound("site.getPostByIDSlugTitle.invalid_id")
			}
			post, err = GetPostByIDWithError(h.Model, id)
			if err != nil {
				return nil, err
			}
		} else if share.PostNameIsTitle() {
			title := ist
			if unescapedTitle, err := url.PathUnescape(ist); err == nil {
				title = unescapedTitle
			}
			post, err = GetPostByTitleWithError(h.Model, title)
			if err != nil {
				return nil, err
			}
		} else {
			post, err = GetPostBySlugWithError(h.Model, ist)
			if err != nil {
				return nil, err
			}
		}
	}

	return post, nil
}

func (h Handler) funcName(c fiber.Ctx, post *DisplayPostWithRelation) (int, int, bool, []*DisplayComment, int, types.Pagination, string, commentFormDefaults, bool, commentCaptchaChallenge) {
	readUV := h.getEntityUVCount(db.UVEntityPost, post.Post.ID)
	likeCount, liked := h.getPostLikeState(c, post.Post.ID)
	commentPager := middleware.GetPagination(c)
	comments := ListApprovedCommentsTree(h.Model, post.Post.ID, &commentPager)
	commentCount := CountApprovedComments(h.Model, post.Post.ID)
	commentFeedback := normalizeCommentFeedbackStatus(c.Query("comment_status"))
	commentForm := readCommentFormDefaults(c, post.Post.ID)
	visitorID := middleware.GetOrCreateVisitorID(c, "")
	captchaRequired := isCommentCaptchaRequired(visitorID)
	commentCaptcha := commentCaptchaChallenge{}
	if captchaRequired {
		commentCaptcha = buildCommentCaptchaChallenge(visitorID)
	}
	return readUV, likeCount, liked, comments, commentCount, commentPager, commentFeedback, commentForm, captchaRequired, commentCaptcha
}
func (h Handler) GetPostByPage(c fiber.Ctx) error {
	ist, _ := h.getIST(c)
	id, _ := strconv.ParseInt(strings.TrimSpace(ist), 10, 64)

	if id > 0 {
		return h.getPostByIST(c, "article")
	}

	// 在没有设置 base_path 的情况下，页面和文章的 URL 规则可能会冲突
	if helper.IsSlug(ist) {
		return h.getPostByIST(c, "page")
	}

	return h.getPostByIST(c, "article")
}
func (h Handler) GetPostByArticle(c fiber.Ctx) error {
	return h.getPostByIST(c, "post")
}
func (h Handler) GetPostByDefault(c fiber.Ctx) error {
	return h.getPostByIST(c, "default")
}
func (h Handler) getPostByIST(c fiber.Ctx, t string) error {
	post, err := h.getPostByIDSlugTitle(c, t)
	if err != nil {
		if db.IsErrNotFound(err) {
			return h.redirectNotFound(c)
		}
		return h.redirectError(c)
	}
	if post == nil {
		return h.redirectNotFound(c)
	}

	readUV, likeCount, liked, comments, commentCount, commentPager, commentFeedback, commentForm, captchaRequired, commentCaptcha := h.funcName(c, post)
	templatePost := ToTemplatePost(post)

	return h.renderView(c, "post.html", fiber.Map{
		"Title":                  buildPageTitle(post.Post.Title),
		"CanonicalURL":           absoluteSiteURL(c, share.GetPostUrl(post.Post)),
		"MetaDescription":        excerptFromHTML(post.HTML, 160),
		"Post":                   templatePost,
		"ReadUV":                 readUV,
		"LikeCount":              likeCount,
		"Liked":                  liked,
		"Comments":               comments,
		"CommentCount":           commentCount,
		"CommentPager":           commentPager,
		"CommentFeedback":        commentFeedback,
		"CommentForm":            commentForm,
		"CommentCaptchaRequired": captchaRequired,
		"CommentCaptcha":         commentCaptcha,
		//"Pages": ListPages(h.Model),
	})
}
func (h Handler) getIST(c fiber.Ctx) (string, string) {
	filename := c.Params("ist")
	ext := filepath.Ext(filename)
	ist := strings.TrimSuffix(filename, ext)
	return ist, ext
}
func (h Handler) GetRSS(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	posts := ListDisplayPosts(h.Model, db.PostKindPost, &pager, true)
	rss, err := GenerateRSS(posts, c, pager.Page, pager.Total)
	if err != nil {
		return err
	}
	c.Set("Content-Type", "application/xml; charset=utf-8")
	return c.SendString(rss)
}
func (h Handler) GetCategoryIndex(c fiber.Ctx) error {
	categories := ListCategories(h.Model)

	if categories == nil {
		return h.redirectError(c)
	}

	pages := ListPages(h.Model)
	h.trackSiteUV(c)
	return h.renderView(c, "list.html", fiber.Map{
		"CanonicalURL": absoluteSiteURL(c, share.GetCategoryPrefix()),
		"Pages":        pages,
		"List":         categories,
		"IsCategory":   true,
	})
}
func (h Handler) GetTagIndex(c fiber.Ctx) error {
	tags := ListTags(h.Model)

	if tags == nil {
		return h.redirectError(c)
	}

	h.trackSiteUV(c)
	return h.renderView(c, "list.html", fiber.Map{
		"CanonicalURL": absoluteSiteURL(c, share.GetTagPrefix()),
		"Pages":        ListPages(h.Model),
		"List":         tags,
	})
}
func (h Handler) GetCategoryDetail(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	slug := c.Params("categorySlug")
	category, err := GetCategoryBySlugWithError(h.Model, slug)
	if err != nil {
		if db.IsErrNotFound(err) {
			return h.redirectNotFound(c)
		}
		return h.redirectError(c)
	}

	h.trackUV(c, db.UVEntityCategory, category.ID)

	posts := ListPostsByCategory(h.Model, category.ID, &pager)
	if posts == nil {
		return h.redirectError(c)
	}

	return h.renderView(c, "detail.html", fiber.Map{
		"Title":           buildPageTitle(category.Name),
		"CanonicalURL":    absoluteSiteURL(c, category.PermLink),
		"MetaDescription": strings.TrimSpace(category.Description),
		"IsCategory":      true,
		"Entity":          category,
		"List":            posts,
		"ListPage":        share.GetCategoryPrefix(),
		"Pages":           ListPages(h.Model),
	})
}
func (h Handler) GetTagDetail(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	slug := c.Params("tagSlug")
	tag, err := GetTagBySlugWithError(h.Model, slug)
	if err != nil {
		if db.IsErrNotFound(err) {
			return h.redirectNotFound(c)
		}
		return h.redirectError(c)
	}

	h.trackUV(c, db.UVEntityTag, tag.ID)

	posts := ListPostsByTag(h.Model, tag.ID, &pager)
	if posts == nil {
		return h.redirectError(c)
	}

	return h.renderView(c, "detail.html", fiber.Map{
		"Title":           buildPageTitle(tag.Name),
		"CanonicalURL":    absoluteSiteURL(c, tag.PermLink),
		"MetaDescription": "",
		"IsTag":           true,
		"Entity":          tag,
		"List":            posts,
		"ListPage":        share.GetTagPrefix(),
		"Pages":           ListPages(h.Model),
	})
}

func (h Handler) PostEntityLike(c fiber.Ctx) error {
	postID, err := strconv.ParseInt(c.Params("postID"), 10, 64)
	if err != nil || postID <= 0 {
		if shouldReturnLikeJSON(c) {
			return fiber.ErrBadRequest
		}
		return h.redirectError(c)
	}

	post, err := h.ensureLikePostExists(postID)
	if err != nil {
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

	if nextStatus == db.LikeStatusActive && notify.IsPostLikeNotificationEnabled() {
		if notifyErr := notify.CreatePostLikeNotification(h.Model, post, likeCount, time.Now().Unix()); notifyErr != nil {
			logger.Error("[notify] create like notification failed: post_id=%d err=%v", postID, notifyErr)
		}
	}

	if shouldReturnLikeJSON(c) {
		return c.JSON(fiber.Map{
			"liked":     nextStatus == db.LikeStatusActive,
			"likeCount": likeCount,
		})
	}

	return webutil.RedirectTo(c, resolveReturnPath(c))
}

func (h Handler) PostComment(c fiber.Ctx) error {
	postID, err := strconv.ParseInt(c.Params("postID"), 10, 64)
	if err != nil || postID <= 0 {
		return h.redirectError(c)
	}
	post, err := h.ensureLikePostExists(postID)
	if err != nil {
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
			saveCommentFormFlash(c, postID, commentFormDefaultsFromRequest(c))
			redirectPath := appendQueryParam(resolveReturnPath(c), "comment_status", commentFeedbackCaptchaFailed)
			if !strings.Contains(redirectPath, "#") {
				redirectPath += "#comments"
			}
			return webutil.RedirectTo(c, redirectPath, fiber.StatusSeeOther)
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

	isLogin := fiber.Locals[bool](c, "IsLogin")
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
		if db.IsErrDuplicateComment(err) {
			saveCommentFormFlash(c, postID, commentFormDefaultsFromRequest(c))
			redirectPath := appendQueryParam(resolveReturnPath(c), "comment_status", commentFeedbackDuplicate)
			if !strings.Contains(redirectPath, "#") {
				redirectPath += "#comments"
			}
			return webutil.RedirectTo(c, redirectPath, fiber.StatusSeeOther)
		}
		return h.redirectError(c)
	}

	if notify.IsCommentNotificationEnabled() {
		if notifyErr := notify.CreateCommentNotification(h.Model, post, *comment, time.Now().Unix()); notifyErr != nil {
			logger.Error("[notify] create comment notification failed: post_id=%d comment_id=%d err=%v", postID, comment.ID, notifyErr)
		}
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
	return webutil.RedirectTo(c, redirectPath)
}

func NewHandler(gStore *store.GlobalStore, service *Service, views fiber.Views, uvTracker ...*UVTracker) *Handler {
	var tracker *UVTracker
	if len(uvTracker) > 0 {
		tracker = uvTracker[0]
	}
	return &Handler{
		Model:     gStore.Model,
		Session:   gStore.Session,
		Service:   service,
		Views:     views,
		UVTracker: tracker,
	}
}
