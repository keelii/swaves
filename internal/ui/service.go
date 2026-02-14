package ui

import (
	"log"
	"swaves/internal/db"
	"swaves/internal/md"
	"swaves/internal/types"
)

type Service struct {
	DB *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{DB: db}
}

func ListDisplayPosts(dbx *db.DB, kind db.PostKind, pager *types.Pagination) []DisplayPost {
	var res []DisplayPost

	articles := db.ListPublishedPosts(dbx, kind, pager)

	for _, p := range articles {
		mdResult := md.ParseMarkdown(p.Content, false)
		res = append(res, DisplayPost{
			Post:     p,
			PostLink: GetPostUrl(p),
			HTML:     mdResult.HTML,
		})
	}

	return res
}
func ListPages(dbx *db.DB) []DisplayPage {
	var res []DisplayPage

	pages := db.ListPublishedPages(dbx)
	for _, p := range pages {
		res = append(res, DisplayPage{
			ID:          p.ID,
			Title:       p.Title,
			Slug:        p.Slug,
			Status:      p.Status,
			PostLink:    GetPostUrl(p),
			PublishedAt: p.PublishedAt,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		})
	}
	return res
}

func GetPostBySlug(dbx *db.DB, slug string) *DisplayPost {
	post, err := db.GetPostBySlug(dbx, slug)
	if err != nil {
		log.Println(err)
		return nil
	}

	return &DisplayPost{
		Post: post,
		HTML: md.ParseMarkdown(post.Content, false).HTML,
	}
}
