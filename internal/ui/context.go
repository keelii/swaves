package ui

import (
	"fmt"
	"regexp"
	"strings"
	"swaves/internal/db"
	"swaves/internal/store"
	"time"

	"github.com/gofiber/fiber/v2"
)

func GetBasePath(c *fiber.Ctx) string {
	b := store.GetSetting("base_path")
	if b == "/" {
		return ""
	}

	return b
}
func GetPagePath(c *fiber.Ctx) string {
	b := store.GetSetting("page_path")
	if b == "/" {
		return ""
	}

	return b
}

func GetSiteUrl(c *fiber.Ctx) string {
	return fmt.Sprintf("%s%s", store.GetSetting("site_url"), GetBasePath(c))
}
func GetPageUrl(c *fiber.Ctx, post db.Post) string {
	return GetPagePath(c) + "/" + post.Slug
}

func PostPathToRegExp() string {
	postPath := store.GetSetting("post_url_prefix")
	postPath = strings.ReplaceAll(postPath, "{year}", `\d{4}`)
	postPath = strings.ReplaceAll(postPath, "{month}", `\d{2}`)
	postPath = strings.ReplaceAll(postPath, "{day}", `\d{2}`)
	return "^" + postPath + "$"
}

func MatchRouter(src, dst string) bool {
	// 加上开始和结束锚点，确保完全匹配
	pattern := "^" + src + "$"

	re, err := regexp.Compile(pattern)
	if err != nil {
		// 正则非法直接返回 false
		return false
	}

	return re.MatchString(dst)
}

func GetArticleUrl(c *fiber.Ctx, post db.Post) string {
	y := time.Unix(post.CreatedAt, 0).Format("2006")
	m := time.Unix(post.CreatedAt, 0).Format("01")
	d := time.Unix(post.CreatedAt, 0).Format("02")

	postPath := store.GetSetting("post_url_prefix")
	postPath = strings.ReplaceAll(postPath, "{year}", y)
	postPath = strings.ReplaceAll(postPath, "{month}", m)
	postPath = strings.ReplaceAll(postPath, "{day}", d)
	postPath = strings.ReplaceAll(postPath, "{slug}", post.Slug)

	return postPath
}
func GetPostUrl(c *fiber.Ctx, post db.Post) string {
	if post.Kind == db.PostKindPage {
		return GetPageUrl(c, post)
	}
	return GetArticleUrl(c, post)
}
func GetPostAbsUrl(c *fiber.Ctx, post db.Post) string {
	return fmt.Sprintf("%s%s", GetSiteUrl(c), GetPostUrl(c, post))
}
func GetSiteAuthor(c *fiber.Ctx) string {
	return store.GetSetting("author")
}
