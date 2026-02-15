package ui

import (
	"fmt"
	"swaves/internal/db"
	"swaves/internal/middleware"
	"swaves/internal/store"
	"swaves/internal/types"

	"github.com/gofiber/fiber/v2"
)

type Handler struct {
	Model   *db.DB
	Session *types.SessionStore
	Service *Service
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

	return RenderUIView(c, "ui/home", fiber.Map{
		"Articles": articles,
		"Pages":    ListPages(h.Model),
		"Pager":    pager,
	}, "")
}
func (h Handler) GetPost(c *fiber.Ctx) error {
	matched := MatchRouter(c.Path())
	if len(matched) == 0 {
		return c.Status(fiber.StatusNotFound).SendString("post not found")
	}

	post := GetPostBySlug(h.Model, matched["slug"])

	return RenderUIView(c, "ui/post", fiber.Map{
		"Post": post,
		//"Pages": ListPages(h.Model),
	}, "")
}

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

	y, m, d := GetArticlePublishedDate(post.Post)

	if y != year || m != month || d != day {
		return c.Status(fiber.StatusNotFound).SendString("page not found, maybe the date is wrong")
	}

	return RenderUIView(c, "ui/post", fiber.Map{
		"Post": post,
		//"Pages": ListPages(h.Model),
	}, "")
}
func (h Handler) GetPostBySlug(c *fiber.Ctx) error {
	pSlug := c.Params("slug")
	post := GetPostBySlug(h.Model, pSlug)
	if post == nil {
		return c.Status(fiber.StatusNotFound).SendString("page not found")
	}

	return RenderUIView(c, "ui/post", fiber.Map{
		"Post": post,
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

	posts := ListPostsByCategory(h.Model, category.ID, &pager)

	return RenderUIView(c, "ui/detail", fiber.Map{
		"IsCategory": true,
		"Entity":     category,
		"List":       posts,
		"ListPage":   GetCategoryIndex(),
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

	posts := ListPostsByTag(h.Model, tag.ID, &pager)

	return RenderUIView(c, "ui/detail", fiber.Map{
		"IsTag":    true,
		"Entity":   tag,
		"List":     posts,
		"ListPage": GetTagIndex(),
		"Pages":    ListPages(h.Model),
	}, "")
}

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
