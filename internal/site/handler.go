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

func (h Handler) getEntityLikeState(c *fiber.Ctx, entityType db.UVEntityType, entityID int64) (int, bool) {
	likeCount, err := db.CountEntityLikes(h.Model, entityType, entityID)
	if err != nil {
		log.Printf("count entity like failed: %v", err)
		return 0, false
	}

	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		return likeCount, false
	}

	liked, err := db.IsEntityLikedByVisitor(h.Model, entityType, entityID, visitorID)
	if err != nil {
		log.Printf("check entity like failed: %v", err)
		return likeCount, false
	}

	return likeCount, liked
}

func parseLikeEntityType(value string) (db.UVEntityType, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "post":
		return db.UVEntityPost, true
	case "category":
		return db.UVEntityCategory, true
	case "tag":
		return db.UVEntityTag, true
	default:
		return 0, false
	}
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

func (h Handler) ensureLikeEntityExists(entityType db.UVEntityType, entityID int64) error {
	switch entityType {
	case db.UVEntityPost:
		post, err := db.GetPostByID(h.Model, entityID)
		if err != nil {
			return err
		}
		if post.Status != "published" || post.PublishedAt <= 0 {
			return db.ErrNotFound("PostEntityLike.Post")
		}
		return nil
	case db.UVEntityCategory:
		_, err := db.GetCategoryByID(h.Model, entityID)
		return err
	case db.UVEntityTag:
		_, err := db.GetTagByID(h.Model, entityID)
		return err
	default:
		return fiber.ErrBadRequest
	}
}

func RenderUIView(c *fiber.Ctx, view string, data fiber.Map, layout string) error {
	if layout == "" {
		layout = "ui/layout"
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

	return RenderUIView(c, "ui/home", fiber.Map{
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
//	return RenderUIView(c, "ui/post", fiber.Map{
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
	likeCount, liked := h.getEntityLikeState(c, db.UVEntityPost, post.Post.ID)

	return RenderUIView(c, "ui/post", fiber.Map{
		"Post":      post,
		"LikeCount": likeCount,
		"Liked":     liked,
		"LikeType":  "post",
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
	likeCount, liked := h.getEntityLikeState(c, db.UVEntityPost, post.Post.ID)

	return RenderUIView(c, "ui/post", fiber.Map{
		"Post":      post,
		"LikeCount": likeCount,
		"Liked":     liked,
		"LikeType":  "post",
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
	return RenderUIView(c, "ui/list", fiber.Map{
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
	return RenderUIView(c, "ui/list", fiber.Map{
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
	likeCount, liked := h.getEntityLikeState(c, db.UVEntityCategory, category.ID)

	posts := ListPostsByCategory(h.Model, category.ID, &pager)

	return RenderUIView(c, "ui/detail", fiber.Map{
		"IsCategory": true,
		"Entity":     category,
		"LikeCount":  likeCount,
		"Liked":      liked,
		"LikeType":   "category",
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
	likeCount, liked := h.getEntityLikeState(c, db.UVEntityTag, tag.ID)

	posts := ListPostsByTag(h.Model, tag.ID, &pager)

	return RenderUIView(c, "ui/detail", fiber.Map{
		"IsTag":     true,
		"Entity":    tag,
		"LikeCount": likeCount,
		"Liked":     liked,
		"LikeType":  "tag",
		"List":      posts,
		"ListPage":  share.GetTagIndex(),
		"Pages":     ListPages(h.Model),
	}, "")
}

func (h Handler) PostEntityLike(c *fiber.Ctx) error {
	entityType, ok := parseLikeEntityType(c.Params("entityType"))
	if !ok {
		return fiber.ErrBadRequest
	}

	entityID, err := strconv.ParseInt(c.Params("entityID"), 10, 64)
	if err != nil || entityID <= 0 {
		return fiber.ErrBadRequest
	}

	if err = h.ensureLikeEntityExists(entityType, entityID); err != nil {
		if db.IsErrNotFound(err) {
			return fiber.ErrNotFound
		}
		return err
	}

	visitorID := middleware.GetOrCreateVisitorID(c, "")
	if visitorID == "" {
		return fiber.ErrBadRequest
	}

	liked, err := db.IsEntityLikedByVisitor(h.Model, entityType, entityID, visitorID)
	if err != nil {
		return err
	}

	nextStatus := db.LikeStatusActive
	if liked {
		nextStatus = db.LikeStatusInactive
	}

	if err = db.UpsertEntityLike(h.Model, entityType, entityID, visitorID, nextStatus); err != nil {
		return err
	}

	likeCount, err := db.CountEntityLikes(h.Model, entityType, entityID)
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

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
