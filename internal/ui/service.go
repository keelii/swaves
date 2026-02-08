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

func ListDisplayPosts(dbx *db.DB, pager *types.Pagination) []DisplayPost {
	var res []DisplayPost

	posts := db.ListPublishedPosts(dbx, pager)

	for _, p := range posts {
		mdResult := md.ParseMarkdown(p.Content, false)
		res = append(res, DisplayPost{
			Post: p,
			HTML: mdResult.HTML,
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
