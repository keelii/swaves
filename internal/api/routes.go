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

	// POST: JSON body { "content": "..." }，用于即时预览（支持长文）
	apiGroup.Post("/markdown", func(c *fiber.Ctx) error {
		var body struct {
			Content string `json:"content"`
		}
		if err := c.BodyParser(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid json"})
		}
		result := md.ParseMarkdown(body.Content)
		return c.JSON(fiber.Map{"data": result.HTML})
	})
	// GET: ?content=...，兼容旧用法
	apiGroup.Get("/markdown", func(c *fiber.Ctx) error {
		mdContent := c.Query("content")
		result := md.ParseMarkdown(mdContent)
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
