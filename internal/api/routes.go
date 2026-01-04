package api

import (
	"context"
	"fmt"
	"swaves/internal/db"

	"cloud.google.com/go/translate"
	"github.com/gofiber/fiber/v2"
	"github.com/gosimple/slug"
	"golang.org/x/text/language"
)

func translateText(targetLanguage, text string) (string, error) {

	ctx := context.Background()

	lang, err := language.Parse(targetLanguage)
	if err != nil {
		return "", fmt.Errorf("language.Parse: %w", err)
	}

	client, err := translate.NewClient(ctx)
	if err != nil {
		return "", err
	}
	defer client.Close()

	resp, err := client.Translate(ctx, []string{text}, lang, nil)
	if err != nil {
		return "", fmt.Errorf("translate: %w", err)
	}
	if len(resp) == 0 {
		return "", fmt.Errorf("translate returned empty response to text: %s", text)
	}
	return resp[0].Text, nil
}

func RegisterRoutes(app *fiber.App, conn *db.DB) {
	apiGroup := app.Group("/api")

	apiGroup.Get("/slug", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"data": slug.Make(c.Query("name")),
		})
	})
	apiGroup.Get("/translate", func(c *fiber.Ctx) error {
		ret, err := translateText("en", c.Query("name"))
		if err != nil {
			return err
		}
		return c.JSON(fiber.Map{
			"data": ret,
		})
	})
}
