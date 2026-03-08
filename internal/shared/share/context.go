package share

import (
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/store"
	"swaves/internal/shared/helper"
	"swaves/internal/shared/pathutil"
	"sync"
	"time"
)

type URLForResolver func(name string, params map[string]string, query map[string]string) (string, error)

type URLForStore struct {
	mu       sync.RWMutex
	resolver URLForResolver
}

func NewURLForStore() *URLForStore {
	return &URLForStore{}
}

func (s *URLForStore) SetResolver(resolver URLForResolver) {
	if s == nil {
		logger.Warn("[url_for] skip set resolver: store is nil")
		return
	}
	s.mu.Lock()
	s.resolver = resolver
	s.mu.Unlock()
}

func (s *URLForStore) URLFor(name string, params map[string]string, query map[string]string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		logger.Warn("[url_for] skip resolve: empty route name")
		return ""
	}

	if s == nil {
		logger.Warn("[url_for] skip resolve: store is nil (name=%s)", name)
		return ""
	}

	s.mu.RLock()
	resolver := s.resolver
	s.mu.RUnlock()
	if resolver == nil {
		logger.Warn("[url_for] skip resolve: resolver is nil (name=%s)", name)
		return ""
	}

	resolved, err := resolver(name, params, query)
	if err != nil {
		logger.Error("[url_for] resolve failed: name=%s err=%v", name, err)
		return ""
	}
	return resolved
}

var defaultURLForStore = NewURLForStore()

func SetURLForResolver(resolver URLForResolver) {
	defaultURLForStore.SetResolver(resolver)
}

func URLFor(name string, params map[string]string, query map[string]string) string {
	return defaultURLForStore.URLFor(name, params, query)
}

func settingValue(code string) string {
	return strings.TrimSpace(store.GetSetting(code))
}

func normalizedPathSettingValue(code string) string {
	return strings.Trim(strings.TrimSpace(settingValue(code)), "\"'")
}

func basePathValue() string {
	return pathutil.JoinRelative(settingValue("base_path"))
}

func pagePrefixValue() string {
	return pathutil.JoinRelative(settingValue("page_url_prefix"))
}

func postPrefixValue() string {
	return pathutil.JoinRelative(settingValue("post_url_prefix"))
}

func categoryPrefixValue() string {
	return pathutil.JoinRelative(settingValue("category_url_prefix"))
}

func tagPrefixValue() string {
	return pathutil.JoinRelative(settingValue("tag_url_prefix"))
}

func rssPathValue() string {
	return pathutil.JoinRelative(settingValue("rss_path"))
}

func prefixedPath(parts ...string) string {
	return pathutil.JoinAbsolute(parts...)
}

func routePath(parts ...string) string {
	return pathutil.JoinAbsolute(parts...)
}

func GetBasePath() string {
	return prefixedPath(basePathValue())
}

func GetBasePathWithSlash() string {
	basePath := GetBasePath()
	if basePath == "/" {
		return "/"
	}
	return basePath + "/"
}

func GetPagePath() string {
	return pagePrefixValue()
}

func GetSiteUrl() string {
	siteURL := strings.TrimSpace(settingValue("site_url"))
	if siteURL == "" {
		return GetBasePath()
	}
	siteURL = strings.TrimRight(siteURL, "/")
	basePath := GetBasePath()
	if basePath == "/" {
		return siteURL
	}
	return siteURL + basePath
}

func GetPageUrl(post db.Post) string {
	return prefixedPath(GetBasePath(), pagePrefixValue(), post.Slug)
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
	return settingValue("post_url_name") == "{id}"
}

func PostNameIsTitle() bool {
	return settingValue("post_url_name") == "{title}"
}

func GetPostExt() string {
	return settingValue("post_url_ext")
}

func GetPostName(post db.Post) string {
	postName := settingValue("post_url_name")
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
	datePath := ""
	if y != "" && m != "" && d != "" {
		datePath = pathutil.JoinRelative(y, m, d)
	}

	postPath := strings.ReplaceAll(postPrefixValue(), "{datetime}", datePath)
	return prefixedPath(GetBasePath(), postPath, GetPostName(post))
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
	siteURL := strings.TrimRight(settingValue("site_url"), "/")
	if siteURL == "" {
		return GetPostUrl(post)
	}
	return siteURL + GetPostUrl(post)
}

func GetSiteAuthor() string {
	return settingValue("author")
}

func GetSiteCopyright() string {
	return settingValue("site_copyright")
}

func GetCategoryPrefix() string {
	return prefixedPath(GetBasePath(), categoryPrefixValue())
}

func GetTagPrefix() string {
	return prefixedPath(GetBasePath(), tagPrefixValue())
}

func GetCategoryRoute() string {
	return routePath(categoryPrefixValue())
}

func GetTagRoute() string {
	return routePath(tagPrefixValue())
}

func GetCategoryUrl(category db.Category) string {
	return prefixedPath(GetCategoryPrefix(), category.Slug)
}

func GetTagUrl(tag db.Tag) string {
	return prefixedPath(GetTagPrefix(), tag.Slug)
}

func GetRSSUrl() string {
	return prefixedPath(GetBasePath(), rssPathValue())
}

func GetRSSRoute() string {
	return routePath(rssPathValue())
}

func GetDashUrl() string {
	base := normalizedPathSettingValue("dash_path")
	if strings.TrimSpace(base) == "" {
		base = "/dash"
	}
	return routePath(base)
}

func BuildDashPath(path string) string {
	path = strings.TrimSpace(path)
	basePath := GetDashUrl()
	if path == "" || path == "/" {
		return basePath
	}
	if strings.HasPrefix(path, "/dash") {
		path = strings.TrimPrefix(path, "/dash")
	}
	return pathutil.JoinAbsolute(basePath, path)
}

func CanonicalDashPath(path string) string {
	path = strings.TrimSpace(path)
	basePath := GetDashUrl()
	if basePath == "/" {
		return path
	}
	if path == basePath {
		return "/dash"
	}
	if strings.HasPrefix(path, basePath+"/") {
		suffix := strings.TrimPrefix(path, basePath)
		return pathutil.JoinAbsolute("/dash", suffix)
	}
	return path
}

func GetPagePrefix() string {
	return prefixedPath(GetBasePath(), pagePrefixValue())
}

func GetPostPrefix() string {
	return prefixedPath(GetBasePath(), postPrefixValue())
}

func GetPageRoute(route string) string {
	return routePath(pagePrefixValue(), route)
}

func GetPostRoute() string {
	return routePath(postPrefixValue())
}

func GetDashPostUrl(post db.Post) string {
	return prefixedPath(GetDashUrl(), GetPostUrl(post))
}

func GetDashEditPostUrl(post db.Post) string {
	return prefixedPath(GetDashUrl(), "posts", strconv.FormatInt(post.ID, 10), "edit")
}

func GetAuthorGravatarUrl(size int) string {
	return helper.BuildGAvatarURL(store.GetSetting("author_email"), GetSiteAuthor(), size)
}
