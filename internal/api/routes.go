package api

import (
	"swaves/internal/db"

	"github.com/gofiber/fiber/v2"
	"github.com/gosimple/slug"
)

func RegisterRoutes(app *fiber.App, conn *db.DB) {
	apiGroup := app.Group("/api")

	apiGroup.Get("/slug", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"data": slug.Make(c.Query("name")),
		})
	})
}
