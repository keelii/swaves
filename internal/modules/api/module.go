package api

import (
	"swaves/internal/shared/helper"
	"swaves/internal/shared/md"

	"github.com/gofiber/fiber/v3"
)

func RegisterModule(app *fiber.App) {
	apiGroup := app.Group("/api")

	apiGroup.Get("/slug", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"data": helper.MakeSlug(c.Query("name")),
		})
	}).Name("api.slug")

	// POST: JSON body { "content": "...", "toc": true/false }，用于即时预览（支持长文）
	apiGroup.Post("/markdown", func(c fiber.Ctx) error {
		var body struct {
			Content string `json:"content"`
			TOC     bool   `json:"toc"`
		}
		if err := c.Bind().Body(&body); err != nil {
			return c.Status(400).JSON(fiber.Map{"error": "invalid json"})
		}
		result := md.ParseMarkdown(body.Content, true)
		return c.JSON(fiber.Map{"data": result.HTML})
	}).Name("api.markdown")
	//apiGroup.Get("/translate", func(c fiber.Ctx) error {
	//	ret, err := translateText("en", c.Query("name"))
	//	if err != nil {
	//		return err
	//	}
	//	return c.JSON(fiber.Map{
	//		"data": ret,
	//	})
	//})
}
