package ui

import (
	"errors"
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
	pages := ListDisplayPosts(h.Model, db.PostKindPage, &pager)
	return RenderUIView(c, "ui/home", fiber.Map{
		"Articles": articles,
		"Pages":    pages,
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
	}, "")
}

func (h Handler) DispatchHandler(c *fiber.Ctx) error {
	if c.Path() == "/" {
		return h.GetHome(c)
	}

	return h.GetPost(c)
}
func (h Handler) GetPage(c *fiber.Ctx) error {
	pageSlug := c.Params("pageSlug")
	display, err := GetPage(h.Model, pageSlug)
	if err != nil {
		if db.IsErrNotFound(err) {
			return c.Status(fiber.StatusNotFound).SendString("page not found")
		}
		return err
	}
	return RenderUIView(c, "ui/post", fiber.Map{
		"Post": display,
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
	return errors.New("not implemented")
}
func (h Handler) GetTagIndex(c *fiber.Ctx) error {
	return errors.New("not implemented")
}

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
