package ui

import (
	"errors"
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

func (h Handler) GetHome(ctx *fiber.Ctx) error {
	return ctx.Send([]byte("ui home"))
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
	slug := ctx.Params("slug")
	post := GetPostBySlug(h.Model, slug)

	return RenderUIView(ctx, "ui/post", fiber.Map{
		"Post": post,
	}, "")
}

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
