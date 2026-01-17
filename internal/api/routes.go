package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gosimple/slug"
)

func RegisterRoutes(app *fiber.App) {
	apiGroup := app.Group("/api")

	apiGroup.Get("/slug", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"data": slug.Make(c.Query("name")),
		})
	})
	//apiGroup.Get("/translate", func(c *fiber.Ctx) error {
	//	ret, err := translateText("en", c.Query("name"))
	//	if err != nil {
	//		return err
	//	}
	//	return c.JSON(fiber.Map{
	//		"data": ret,
	//	})
	//})
}
