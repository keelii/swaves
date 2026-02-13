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

// GetPage 根据 slug 查询 type=page 的页面，返回渲染用 DisplayPost；未找到返回 nil, ErrNotFound
func GetPage(dbx *db.DB, slug string) (*DisplayPost, error) {
	post, err := db.GetPage(dbx, slug)
	if err != nil {
		return nil, err
	}
	return &DisplayPost{
		Post: *post,
		HTML: md.ParseMarkdown(post.Content, false).HTML,
	}, nil
}
