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
	if b == "" {
		return "/"
	}

	return "/" + b
}
func GetBasePathWithSlash() string {
	basePath := GetBasePath()
	if basePath == "/" {
		return "/"
	}
	return GetBasePath() + "/"
}
func GetPagePath() string {
	b := store.GetSetting("page_url_prefix")
	if b == "/" {
		return ""
	}

	return b
}

func GetSiteUrl() string {
	return fmt.Sprintf("%s%s", store.GetSetting("site_url"), GetBasePath())
}
func GetPageUrl(post db.Post) string {
	//postName := GetPostName(post)
	postName := post.Slug
	pagePrefix := GetPagePrefix()
	if pagePrefix == "/" {
		return pagePrefix + postName
	}
	return pagePrefix + "/" + postName
}

func GetArticlePublishedDate(post db.Post) (string, string, string) {
	if post.PublishedAt == 0 {
		return "", "", ""
	}
	published := time.Unix(post.PublishedAt, 0)
	y := published.Format("2006")
	m := published.Format("01")
	d := published.Format("02")
	return y, m, d
}

func PostNameIsID() bool {
	return store.GetSetting("post_url_name") == "{id}"
}
func PostNameIsTitle() bool {
	return store.GetSetting("post_url_name") == "{title}"
}
func GetPostExt() string {
	postExt := store.GetSetting("post_url_ext")
	if postExt == "" {
		return ""
	}
	return postExt
}
func GetPostName(post db.Post) string {
	postName := store.GetSetting("post_url_name")
	if postName == "" {
		return post.Slug + GetPostExt()
	}

	postName = strings.ReplaceAll(postName, "{slug}", post.Slug)
	postName = strings.ReplaceAll(postName, "{id}", strconv.FormatInt(post.ID, 10))
	if post.Title != "" {
		postName = strings.ReplaceAll(postName, "{title}", post.Title)
	}

	if postName == "" {
		return post.Slug
	}

	return postName + GetPostExt()
}
func GetArticleUrl(post db.Post) string {
	y, m, d := GetArticlePublishedDate(post)
	postPath := GetPostPrefix()
	postPath = strings.ReplaceAll(postPath, "{datetime}", fmt.Sprintf("%s/%s/%s", y, m, d))

	postName := GetPostName(post)

	if postPath == "/" {
		return "/" + postName
	}

	return postPath + "/" + postName
}

func BuildPostURL(id int64, kind db.PostKind, slug string, publishedAt int64) string {
	return GetPostUrl(db.Post{
		ID:          id,
		Kind:        kind,
		Slug:        slug,
		PublishedAt: publishedAt,
	})
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
func GetSiteCopyright() string {
	return store.GetSetting("site_copyright")
}
func GetCategoryPrefix() string {
	return GetBasePathWithSlash() + store.GetSetting("category_url_prefix")
}
func GetTagPrefix() string {
	return GetBasePathWithSlash() + store.GetSetting("tag_url_prefix")
}
func GetCategoryRoute() string {
	return "/" + store.GetSetting("category_url_prefix")
}
func GetTagRoute() string {
	return "/" + store.GetSetting("tag_url_prefix")
}
func GetCategoryUrl(category db.Category) string {
	return GetCategoryPrefix() + "/" + category.Slug
}
func GetTagUrl(tag db.Tag) string {
	return GetTagPrefix() + "/" + tag.Slug
}

func GetRSSUrl() string {
	return GetBasePathWithSlash() + store.GetSetting("rss_path")
}
func GetRSSRoute() string {
	return "/" + store.GetSetting("rss_path")
}
func GetAdminUrl() string {
	return store.GetSetting("admin_path")
}
func GetPagePrefix() string {
	return GetBasePathWithSlash() + store.GetSetting("page_url_prefix")
}
func GetPostPrefix() string {
	return GetBasePathWithSlash() + store.GetSetting("post_url_prefix")
}
func GetPageRoute() string {
	return "/" + store.GetSetting("page_url_prefix")
}
func GetPostRoute() string {
	return "/" + store.GetSetting("post_url_prefix")
}
func GetAdminPostUrl(post db.Post) string {
	return fmt.Sprintf("%s%s", GetAdminUrl(), GetPostUrl(post))
}
func GetAdminEditPostUrl(post db.Post) string {
	return fmt.Sprintf("%s%s/posts/%d/edit", GetAdminUrl(), GetBasePathWithSlash(), post.ID)
}
