package site

import (
	"encoding/xml"
	"sort"
	"strings"
	"time"

	"swaves/internal/platform/db"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

type sitemapURLSet struct {
	XMLName xml.Name     `xml:"urlset"`
	Xmlns   string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func lastModifiedValue(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func formatSitemapLastMod(ts int64) string {
	if ts <= 0 {
		return ""
	}
	return time.Unix(ts, 0).UTC().Format(time.RFC3339)
}

func (h Handler) buildSitemapURLs(c fiber.Ctx) ([]sitemapURL, error) {
	urls := make([]sitemapURL, 0, 256)
	seen := make(map[string]struct{})

	addPath := func(path string, lastModified int64) {
		loc := absoluteSiteURL(c, path)
		if _, ok := seen[loc]; ok {
			return
		}
		seen[loc] = struct{}{}

		urls = append(urls, sitemapURL{
			Loc:     loc,
			LastMod: formatSitemapLastMod(lastModified),
		})
	}

	addPath(share.GetBasePath(), 0)
	addPath(share.GetCategoryPrefix(), 0)
	addPath(share.GetTagPrefix(), 0)

	for _, page := range db.ListPublishedPages(h.Model) {
		addPath(share.GetPostUrl(page), lastModifiedValue(page.UpdatedAt, page.PublishedAt, page.CreatedAt))
	}

	for currentPage := 1; ; currentPage++ {
		pager := types.Pagination{Page: currentPage, PageSize: 200}
		posts := db.ListPublishedPosts(h.Model, db.PostKindPost, &pager)
		for _, post := range posts {
			addPath(share.GetPostUrl(post), lastModifiedValue(post.UpdatedAt, post.PublishedAt, post.CreatedAt))
		}
		if pager.Num == 0 || currentPage >= pager.Num {
			break
		}
	}

	categories, err := db.ListCategories(h.Model, false)
	if err != nil {
		return nil, err
	}
	for _, category := range categories {
		addPath(share.GetCategoryUrl(category), lastModifiedValue(category.UpdatedAt, category.CreatedAt))
	}

	tags, err := db.ListTags(h.Model, false)
	if err != nil {
		return nil, err
	}
	for _, tag := range tags {
		addPath(share.GetTagUrl(tag), lastModifiedValue(tag.UpdatedAt, tag.CreatedAt))
	}

	sort.Slice(urls, func(i, j int) bool {
		return urls[i].Loc < urls[j].Loc
	})

	return urls, nil
}

func (h Handler) GetSitemap(c fiber.Ctx) error {
	urls, err := h.buildSitemapURLs(c)
	if err != nil {
		return err
	}

	payload, err := xml.MarshalIndent(sitemapURLSet{
		Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9",
		URLs:  urls,
	}, "", "  ")
	if err != nil {
		return err
	}

	c.Set(fiber.HeaderContentType, "application/xml; charset=utf-8")
	return c.SendString(xml.Header + string(payload))
}

func (h Handler) GetRobots(c fiber.Ctx) error {
	sitemapURL := absoluteSiteURL(c, getSitePath("/sitemap.xml"))
	body := strings.Join([]string{
		"User-agent: *",
		"Allow: /",
		"",
		"Sitemap: " + sitemapURL,
		"",
	}, "\n")

	c.Set(fiber.HeaderContentType, "text/plain; charset=utf-8")
	return c.SendString(body)
}
