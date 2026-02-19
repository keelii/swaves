package site

import (
	"log"
	"swaves/internal/db"
	"swaves/internal/md"
	"swaves/internal/share"
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
			PermLink: share.GetPostUrl(p),
			HTML:     mdResult.HTML,
		})
	}

	return res
}
func ListPages(dbx *db.DB) []DisplayPostInfo {
	var res []DisplayPostInfo

	pages := db.ListPublishedPages(dbx)
	for _, p := range pages {
		res = append(res, DisplayPostInfo{
			ID:          p.ID,
			Title:       p.Title,
			Slug:        p.Slug,
			PermLink:    share.GetPostUrl(p),
			PublishedAt: p.PublishedAt,
			CreatedAt:   p.CreatedAt,
			UpdatedAt:   p.UpdatedAt,
		})
	}
	return res
}

func postToPostInfo(p *db.Post) *DisplayPostInfo {
	if p == nil {
		return nil
	}

	return &DisplayPostInfo{
		ID:          p.ID,
		Title:       p.Title,
		Slug:        p.Slug,
		PermLink:    share.GetPostUrl(*p),
		PublishedAt: p.PublishedAt,
		CreatedAt:   p.CreatedAt,
		UpdatedAt:   p.UpdatedAt,
	}
}

func GetPostBySlug(dbx *db.DB, slug string) *DisplayPostWithRelation {
	p, err := db.GetPostBySlugWithRelation(dbx, slug)
	if err != nil {
		log.Println(err)
		return nil
	}

	prev, next, err := db.GetPrevNextPost(dbx, p.Post.PublishedAt)
	if err != nil {
		log.Println(err)
	}

	return &DisplayPostWithRelation{
		DisplayPost: DisplayPost{
			Post:     *p.Post,
			Prev:     postToPostInfo(prev),
			Next:     postToPostInfo(next),
			PermLink: share.GetPostUrl(*p.Post),
			HTML:     md.ParseMarkdown(p.Post.Content, true).HTML,
		},
		Tags:     toDisplayTags(p.Tags),
		Category: toDisplayCategory(p.Category),
	}
}

func ListApprovedCommentsTree(dbx *db.DB, postID int64) []*DisplayComment {
	comments, err := db.ListApprovedPostComments(dbx, postID)
	if err != nil {
		log.Println(err)
		return []*DisplayComment{}
	}
	if len(comments) == 0 {
		return []*DisplayComment{}
	}

	nodeMap := make(map[int64]*DisplayComment, len(comments))
	for i := range comments {
		item := comments[i]
		nodeMap[item.ID] = &DisplayComment{
			Comment:  item,
			Children: make([]*DisplayComment, 0),
		}
	}

	roots := make([]*DisplayComment, 0)
	for i := range comments {
		node := nodeMap[comments[i].ID]
		if node.ParentID > 0 {
			if parent, ok := nodeMap[node.ParentID]; ok {
				node.ParentAuthor = parent.Author
				parent.Children = append(parent.Children, node)
				continue
			}
		}
		roots = append(roots, node)
	}

	return roots
}

func CountApprovedComments(dbx *db.DB, postID int64) int {
	count, err := db.CountPostComments(dbx, postID, db.CommentStatusApproved)
	if err != nil {
		log.Println(err)
		return 0
	}
	return count
}

func ListCategories(dbx *db.DB) []DisplayItem {
	res, err := db.ListCategories(dbx, true)
	if err != nil {
		return []DisplayItem{}
	}
	var items []DisplayItem
	for _, c := range res {
		items = append(items, DisplayItem{
			ID:        c.ID,
			Name:      c.Name,
			Slug:      c.Slug,
			PermLink:  share.GetCategoryUrl(c),
			PostCount: c.PostCount,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		})
	}
	return items
}
func ListTags(dbx *db.DB) []DisplayItem {
	res, err := db.ListTags(dbx, true)
	if err != nil {
		return []DisplayItem{}
	}
	var items []DisplayItem
	for _, c := range res {
		items = append(items, DisplayItem{
			ID:        c.ID,
			Name:      c.Name,
			Slug:      c.Slug,
			PermLink:  share.GetTagUrl(c),
			PostCount: c.PostCount,
			CreatedAt: c.CreatedAt,
			UpdatedAt: c.UpdatedAt,
		})
	}
	return items
}

func GetCategoryBySlug(dbx *db.DB, slug string) *DisplayItem {
	category, err := db.GetCategoryBySlug(dbx, slug)
	if err != nil {
		log.Println(err)
		return nil
	}

	return &DisplayItem{
		ID:        category.ID,
		Name:      category.Name,
		Slug:      category.Slug,
		PermLink:  share.GetCategoryUrl(*category),
		PostCount: category.PostCount,
		CreatedAt: category.CreatedAt,
		UpdatedAt: category.UpdatedAt,
	}
}
func GetTagBySlug(dbx *db.DB, slug string) *DisplayItem {
	tag, err := db.GetTagBySlug(dbx, slug)
	if err != nil {
		log.Println(err)
		return nil
	}
	return &DisplayItem{
		ID:        tag.ID,
		Name:      tag.Name,
		Slug:      tag.Slug,
		PermLink:  share.GetTagUrl(*tag),
		PostCount: tag.PostCount,
		CreatedAt: tag.CreatedAt,
		UpdatedAt: tag.UpdatedAt,
	}
}
func ListPostsByCategory(dbx *db.DB, categoryID int64, pager *types.Pagination) []DisplayPostRelativeInfo {
	var res []DisplayPostRelativeInfo

	posts, err := db.ListPostsByCategory(dbx, &db.PostQueryOptions{
		Kind:        nil,
		CategoryID:  categoryID,
		TagID:       0,
		Pager:       pager,
		WithContent: false,
	})
	if err != nil {
		return []DisplayPostRelativeInfo{}
	}

	for _, p := range posts {
		res = append(res, DisplayPostRelativeInfo{
			ID:          p.Post.ID,
			Title:       p.Post.Title,
			Slug:        p.Post.Slug,
			PermLink:    share.GetPostUrl(*p.Post),
			Tags:        toDisplayTags(p.Tags),
			Category:    toDisplayCategory(p.Category),
			PublishedAt: p.Post.PublishedAt,
			CreatedAt:   p.Post.CreatedAt,
			UpdatedAt:   p.Post.UpdatedAt,
		})
	}

	return res
}
func ListPostsByTag(dbx *db.DB, tagID int64, pager *types.Pagination) []DisplayPostRelativeInfo {
	var res []DisplayPostRelativeInfo

	posts, err := db.ListPostsByCategory(dbx, &db.PostQueryOptions{
		Kind:        nil,
		CategoryID:  0,
		TagID:       tagID,
		Pager:       pager,
		WithContent: false,
	})
	if err != nil {
		return []DisplayPostRelativeInfo{}
	}

	for _, p := range posts {
		res = append(res, DisplayPostRelativeInfo{
			ID:          p.Post.ID,
			Title:       p.Post.Title,
			Slug:        p.Post.Slug,
			PermLink:    share.GetPostUrl(*p.Post),
			Tags:        toDisplayTags(p.Tags),
			Category:    toDisplayCategory(p.Category),
			PublishedAt: p.Post.PublishedAt,
			CreatedAt:   p.Post.CreatedAt,
			UpdatedAt:   p.Post.UpdatedAt,
		})
	}

	return res
}
func toDisplayTags(tags []db.Tag) []DisplayItem {
	var items []DisplayItem
	for _, t := range tags {
		items = append(items, DisplayItem{
			ID:        t.ID,
			Name:      t.Name,
			Slug:      t.Slug,
			PermLink:  share.GetTagUrl(t),
			PostCount: t.PostCount,
			CreatedAt: t.CreatedAt,
			UpdatedAt: t.UpdatedAt,
		})
	}
	return items
}
func toDisplayCategory(category *db.Category) *DisplayItem {
	if category == nil {
		return nil
	}
	return &DisplayItem{
		ID:        category.ID,
		Name:      category.Name,
		Slug:      category.Slug,
		PermLink:  share.GetCategoryUrl(*category),
		PostCount: category.PostCount,
		CreatedAt: category.CreatedAt,
		UpdatedAt: category.UpdatedAt,
	}
}
