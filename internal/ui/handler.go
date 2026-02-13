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

func (h Handler) GetDate(ctx *fiber.Ctx) error {
	dt := ctx.Params("dob")
	fmt.Println("===")
	fmt.Println(dt)
	fmt.Println("===")
	return ctx.Send([]byte("ui home"))
}
func (h Handler) GetHome(ctx *fiber.Ctx) error {
	return ctx.Send([]byte("ui home"))
}
func (h Handler) GetPage(ctx *fiber.Ctx) error {
	pageSlug := ctx.Params("pageSlug")
	display, err := GetPage(h.Model, pageSlug)
	if err != nil {
		if db.IsErrNotFound(err) {
			return ctx.Status(fiber.StatusNotFound).SendString("page not found")
		}
		return err
	}
	return RenderUIView(ctx, "ui/post", fiber.Map{
		"Post": display,
	}, "")
}

func (h Handler) GetRSS(ctx *fiber.Ctx) error {
	pager := middleware.GetPagination(ctx)
	posts := ListDisplayPosts(h.Model, &pager)
	rss, err := GenerateRSS(posts, ctx, pager.Page, pager.Total)
	if err != nil {
		return err
	}
	ctx.Set("Content-Type", "application/xml; charset=utf-8")
	return ctx.SendString(rss)
}
func (h Handler) GetCategoryIndex(ctx *fiber.Ctx) error {
	return errors.New("not implemented")
}
func (h Handler) GetTagIndex(ctx *fiber.Ctx) error {
	return errors.New("not implemented")
}

func (h Handler) GetPost(ctx *fiber.Ctx) error {
	ret := MatchRouter(PostPathToRegExp(), ctx.Path())
	fmt.Println("path:", ret)
	return ctx.Send([]byte("ui home"))
	//return RenderUIView(ctx, "ui/post", fiber.Map{
	//	"Post": post,
	//}, "")
}

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
