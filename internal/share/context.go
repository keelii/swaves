package share

import (
	"fmt"
	"strconv"
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
	postName := GetPostName(post)
	return GetPagePath() + "/" + postName
}

func GetArticlePublishedDate(post db.Post) (string, string, string) {
	published := time.Unix(post.PublishedAt, 0)
	y := published.Format("2006")
	m := published.Format("01")
	d := published.Format("02")
	return y, m, d
}

func PostNameIsID() bool {
	return store.GetSetting("post_url_name") == "{id}"
}
func GetPostName(post db.Post) string {
	postName := store.GetSetting("post_url_name")
	if postName == "" {
		return post.Slug
	}

	postName = strings.ReplaceAll(postName, "{slug}", post.Slug)
	postName = strings.ReplaceAll(postName, "{id}", strconv.FormatInt(post.ID, 10))
	if post.Title != "" {
		postName = strings.ReplaceAll(postName, "{title}", post.Title)
	}

	if postName == "" {
		return post.Slug
	}

	return postName
}
func GetArticleUrl(post db.Post) string {
	y, m, d := GetArticlePublishedDate(post)
	postPath := store.GetSetting("post_url_prefix")
	postPath = strings.ReplaceAll(postPath, "{datetime}", fmt.Sprintf("%s/%s/%s", y, m, d))

	postName := GetPostName(post)

	if postPath == "/" {
		return "/" + postName
	}

	return postPath + "/" + postName
}

func BuildPostURL(kind db.PostKind, slug string, publishedAt int64) string {
	return GetPostUrl(db.Post{
		Kind:        kind,
		Slug:        slug,
		PublishedAt: publishedAt,
	})
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

func GetRSSUrl() string {
	return store.GetSetting("rss_path")
}
func GetAdminUrl() string {
	return store.GetSetting("admin_path")
}
func GetAdminPostUrl(post db.Post) string {
	return fmt.Sprintf("%s%s", GetAdminUrl(), GetPostUrl(post))
}
func GetAdminEditPostUrl(post db.Post) string {
	return fmt.Sprintf("%s%s/posts/%d/edit", GetAdminUrl(), GetBasePath(), post.ID)
}
