package api

import (
	"swaves/internal/md"

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

	// POST: JSON body { "content": "...", "toc": true/false }，用于即时预览（支持长文）
	apiGroup.Post("/markdown", func(c *fiber.Ctx) error {
		var body struct {
			Content string `json:"content"`
			TOC     bool   `json:"toc"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid json"})
		}
		result := md.ParseMarkdown(body.Content, body.TOC)
		return c.JSON(fiber.Map{"data": result.HTML})
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
