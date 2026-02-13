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

	//fmt.Println(dst)
	//fmt.Println(pattern)

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

func GetArticleUrl(post db.Post) string {
	y := time.Unix(post.CreatedAt, 0).Format("2006")
	m := time.Unix(post.CreatedAt, 0).Format("01")
	d := time.Unix(post.CreatedAt, 0).Format("02")

	postPath := store.GetSetting("post_url_prefix")
	postPath = strings.ReplaceAll(postPath, "{year}", y)
	postPath = strings.ReplaceAll(postPath, "{month}", m)
	postPath = strings.ReplaceAll(postPath, "{day}", d)

	return postPath + "/" + post.Slug
}
func GetPostUrl(post db.Post) string {
	if post.Kind == db.PostKindPage {
		return GetPageUrl(post)
	}
	return GetArticleUrl(post)
}
func GetPostAbsUrl(post db.Post) string {
	return fmt.Sprintf("%s%s", GetSiteUrl(), GetPostUrl(post))
}
func GetSiteAuthor() string {
	return store.GetSetting("author")
}
