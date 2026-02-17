package site

import (
	"fmt"
	"swaves/internal/share"
	"swaves/internal/store"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gorilla/feeds"
)

type Article struct {
	Title       string
	Link        string
	Description string
	AuthorName  string
	CreatedAt   time.Time
}

func GenerateRSS(posts []DisplayPost, ctx *fiber.Ctx, page int, total int) (string, error) {
	title := store.GetSetting("site_name")

	// 创建 Feed
	feed := &feeds.Feed{
		Title:       title,
		Link:        &feeds.Link{Href: share.GetSiteUrl()},
		Description: fmt.Sprintf("博客第 %d 页文章 RSS，共 %d 篇", page, total),
		Author:      &feeds.Author{Name: share.GetSiteAuthor()},
		Created:     time.Now(),
	}

	// 添加文章
	for _, p := range posts {
		item := &feeds.Item{
			Title:       p.Title,
			Content:     p.HTML,
			Link:        &feeds.Link{Href: share.GetPostAbsUrl(p.Raw())},
			Description: "",
			Author:      &feeds.Author{Name: share.GetSiteAuthor()},
			Created:     time.Unix(p.CreatedAt, 0),
		}
		feed.Items = append(feed.Items, item)
	}

	return feed.ToRss()
}
