package site

import (
	"html"
	"regexp"
	"strings"

	"swaves/internal/platform/store"
	"swaves/internal/shared/pathutil"

	"github.com/gofiber/fiber/v3"
)

var (
	htmlTagPattern    = regexp.MustCompile(`(?s)<[^>]*>`)
	whitespacePattern = regexp.MustCompile(`\s+`)
)

func buildPageTitle(page string) string {
	page = strings.TrimSpace(page)

	siteTitle := strings.TrimSpace(store.GetSetting("site_title"))
	if siteTitle == "" {
		siteTitle = strings.TrimSpace(store.GetSetting("site_name"))
	}

	if page == "" {
		return siteTitle
	}
	if siteTitle == "" || page == siteTitle {
		return page
	}

	return page + " - " + siteTitle
}

func absoluteSiteURL(c fiber.Ctx, path string) string {
	if path == "" {
		path = "/"
	}
	if strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://") {
		return path
	}

	origin := strings.TrimSpace(store.GetSetting("site_url"))
	if origin == "" {
		origin = strings.TrimSpace(c.BaseURL())
	}
	origin = strings.TrimRight(origin, "/")

	normalizedPath := pathutil.JoinAbsolute(path)
	if origin == "" {
		return normalizedPath
	}

	return origin + normalizedPath
}

func excerptFromHTML(source string, limit int) string {
	text := strings.TrimSpace(source)
	if text == "" {
		return ""
	}

	text = html.UnescapeString(text)
	text = htmlTagPattern.ReplaceAllString(text, " ")
	text = whitespacePattern.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return text
	}

	return strings.TrimSpace(string(runes[:limit])) + "..."
}
