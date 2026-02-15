package ui

import (
	"fmt"
	"regexp"
	"strings"
	"swaves/internal/db"
	"swaves/internal/store"
	"time"
)

func GetBasePath() string {
	b := store.GetSetting("base_path")
	if b == "/" {
		return ""
	}

	return b
}
func GetPagePath() string {
	b := store.GetSetting("page_path")
	if b == "/" {
		return ""
	}

	return b
}

func GetSiteUrl() string {
	return fmt.Sprintf("%s%s", store.GetSetting("site_url"), GetBasePath())
}
func GetPageUrl(post db.Post) string {
	return GetPagePath() + "/" + post.Slug
}

func PostPathToRegExp() string {
	postPath := store.GetSetting("post_url_prefix")
	postPath = strings.ReplaceAll(postPath, "{year}", `(?P<year>\d{4})`)
	postPath = strings.ReplaceAll(postPath, "{month}", `(?P<month>\d{2})`)
	postPath = strings.ReplaceAll(postPath, "{day}", `(?P<day>\d{2})`)
	postPath += `/(?P<slug>[a-z0-9-]+)`
	return "^" + postPath + "$"
}

func MatchRouter(dst string) map[string]string {
	result := map[string]string{}

	pattern := PostPathToRegExp()
	// 加上开始和结束锚点，确保完全匹配

	re, err := regexp.Compile(pattern)
	if err != nil {
		return result
	}

	matches := re.FindStringSubmatch(dst)
	names := re.SubexpNames()

	if len(matches) == 0 || len(names) == 0 {
		return result
	}
	fmt.Println(len(matches), len(names))

	for i, name := range names {
		if i != 0 && name != "" {
			result[name] = matches[i]
		}
	}

	return result
}

func GetArticlePublishedDate(post db.Post) (string, string, string) {
	published := time.Unix(post.PublishedAt, 0)
	y := published.Format("2006")
	m := published.Format("01")
	d := published.Format("02")
	return y, m, d
}
func GetArticleUrl(post db.Post) string {
	y, m, d := GetArticlePublishedDate(post)
	postPath := store.GetSetting("post_url_prefix")
	postPath = strings.ReplaceAll(postPath, "{datetime}", fmt.Sprintf("%s/%s/%s", y, m, d))

	if postPath == "/" {
		return "/" + post.Slug
	}

	return postPath + "/" + post.Slug
}
func GetPostUrl(post db.Post) string {
	base := GetBasePath()
	if post.Kind == db.PostKindPage {
		return base + GetPageUrl(post)
	}
	return base + GetArticleUrl(post)
}
func GetPostAbsUrl(post db.Post) string {
	return fmt.Sprintf("%s%s", GetSiteUrl(), GetPostUrl(post))
}
func GetSiteAuthor() string {
	return store.GetSetting("author")
}
func GetSiteCopyright() string {
	return store.GetSetting("site_copyright")
}
func GetCategoryIndex() string {
	return store.GetSetting("category_url_prefix")
}
func GetTagIndex() string {
	return store.GetSetting("tag_url_prefix")
}
func GetCategoryUrl(category db.Category) string {
	return GetCategoryIndex() + "/" + category.Slug
}
func GetTagUrl(tag db.Tag) string {
	return GetTagIndex() + "/" + tag.Slug
}
