package ui

import (
	"fmt"
	"strings"
	"swaves/internal/db"
	"time"

	"github.com/gofiber/fiber/v2"
)

func GetSiteUrl(c *fiber.Ctx) string {
	return fmt.Sprintf("%s/%s", c.Locals("site_url"), c.Locals("base_path"))
}
func GetPostPath(c *fiber.Ctx, post db.Post) string {
	y := time.Unix(post.CreatedAt, 0).Format("2006")
	m := time.Unix(post.CreatedAt, 0).Format("01")
	d := time.Unix(post.CreatedAt, 0).Format("02")

	postPath := c.Locals("post_url_pattern").(string)
	postPath = strings.ReplaceAll(postPath, "{year}", y)
	postPath = strings.ReplaceAll(postPath, "{month}", m)
	postPath = strings.ReplaceAll(postPath, "{day}", d)
	postPath = strings.ReplaceAll(postPath, "{slug}", post.Slug)

	return postPath
}
func GetPostUrl(c *fiber.Ctx, post db.Post) string {
	return fmt.Sprintf("%s/%s", GetSiteUrl(c), GetPostPath(c, post))
}
func GetSiteAuthor(c *fiber.Ctx) string {
	return c.Locals("author").(string)
}
