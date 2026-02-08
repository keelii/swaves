package ui

import (
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

func NewHandler(gStore *store.GlobalStore, service *Service) *Handler {
	return &Handler{
		Model:   gStore.Model,
		Session: gStore.Session,
		Service: service,
	}
}
