package site

import (
	"net/url"
	"sort"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/md"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"
)

type Service struct {
	DB *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{DB: db}
}

func ListDisplayPosts(dbx *db.DB, kind db.PostKind, pager *types.Pagination, withContent bool) []DisplayPost {
	var res []DisplayPost

	articles := db.ListPublishedPosts(dbx, kind, pager, withContent)

	for _, p := range articles {
		var html string
		if withContent {
			html = md.ParseMarkdown(p.Content, false).HTML
		}
		res = append(res, DisplayPost{
			Post:     p,
			PermLink: share.GetPostUrl(p),
			HTML:     html,
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

func GetPostByID(dbx *db.DB, id int64) *DisplayPostWithRelation {
	p, err := db.GetPostByIDWithRelation(dbx, id)
	if err != nil {
		logger.Warn("get post by id failed: id=%d err=%v", id, err)
		return nil
	}

	return toDisplayPostWithRelation(dbx, p)
}
func GetPostBySlug(dbx *db.DB, slug string) *DisplayPostWithRelation {
	p, err := db.GetPostBySlugWithRelation(dbx, slug)
	if err != nil {
		logger.Warn("get post by slug failed: slug=%s err=%v", slug, err)
		return nil
	}

	return toDisplayPostWithRelation(dbx, p)
}
func GetPostByTitle(dbx *db.DB, ist string) *DisplayPostWithRelation {
	title, err := url.PathUnescape(ist)
	if err != nil {
		logger.Warn("failed to unescape title: raw=%s err=%v", ist, err)
		return nil
	}

	p, err := db.GetPostByTitleWithRelation(dbx, title)
	if err != nil {
		logger.Warn("get post by title failed: title=%s err=%v", title, err)
		return nil
	}

	return toDisplayPostWithRelation(dbx, p)
}

func GetPostBySlugRaw(dbx *db.DB, slug string) *db.Post {
	p, err := db.GetPostBySlug(dbx, slug)
	if err != nil {
		logger.Warn("get post by slug raw failed: slug=%s err=%v", slug, err)
		return nil
	}
	return &p
}

func toDisplayPostWithRelation(dbx *db.DB, p db.PostWithRelation) *DisplayPostWithRelation {
	prev, next, err := db.GetPrevNextPost(dbx, p.Post.PublishedAt)
	if err != nil {
		logger.Warn("get prev/next post failed: published_at=%d err=%v", p.Post.PublishedAt, err)
	}

	mdResult := md.ParseMarkdown(p.Post.Content, true)

	return &DisplayPostWithRelation{
		DisplayPost: DisplayPost{
			Post:     *p.Post,
			Prev:     postToPostInfo(prev),
			Next:     postToPostInfo(next),
			PermLink: share.GetPostUrl(*p.Post),
			HTML:     mdResult.HTML,
			TOCHTML:  mdResult.TOCHTML,
		},
		Tags:     toDisplayTags(p.Tags),
		Category: toDisplayCategory(p.Category),
	}
}

func ListApprovedCommentsTree(dbx *db.DB, postID int64, pager *types.Pagination) []*DisplayComment {
	comments, err := db.ListApprovedPostComments(dbx, postID)
	if err != nil {
		logger.Error("list approved comments failed: post_id=%d err=%v", postID, err)
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

	sortCommentNodesByTimeDesc := func(nodes []*DisplayComment) {
		sort.Slice(nodes, func(i, j int) bool {
			left := nodes[i]
			right := nodes[j]
			if left.CreatedAt != right.CreatedAt {
				return left.CreatedAt > right.CreatedAt
			}
			return left.ID > right.ID
		})
	}

	var sortCommentTreeByTimeDesc func(nodes []*DisplayComment)
	sortCommentTreeByTimeDesc = func(nodes []*DisplayComment) {
		if len(nodes) == 0 {
			return
		}
		sortCommentNodesByTimeDesc(nodes)
		for _, node := range nodes {
			if len(node.Children) > 0 {
				sortCommentTreeByTimeDesc(node.Children)
			}
		}
	}
	sortCommentTreeByTimeDesc(roots)

	if pager != nil {
		if pager.Page < 1 {
			pager.Page = config.DefaultPage
		}
		if pager.PageSize < 1 {
			pager.PageSize = config.DefaultPageSize
		}

		pager.Total = len(roots)
		pager.Num = 0
		if pager.Total > 0 {
			pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
		}

		if pager.Num == 0 {
			pager.Page = config.DefaultPage
			return []*DisplayComment{}
		}
		if pager.Page > pager.Num {
			pager.Page = pager.Num
		}

		start := (pager.Page - 1) * pager.PageSize
		end := start + pager.PageSize
		if start < 0 {
			start = 0
		}
		if start >= len(roots) {
			return []*DisplayComment{}
		}
		if end > len(roots) {
			end = len(roots)
		}
		return roots[start:end]
	}

	return roots
}

func CountApprovedComments(dbx *db.DB, postID int64) int {
	count, err := db.CountPostComments(dbx, postID, db.CommentStatusApproved)
	if err != nil {
		logger.Error("count approved comments failed: post_id=%d err=%v", postID, err)
		return 0
	}
	return count
}

func ListCategories(dbx *db.DB) []*DisplayCategoryNode {
	res, err := db.ListCategories(dbx, true)
	if err != nil {
		return []*DisplayCategoryNode{}
	}

	if len(res) == 0 {
		return []*DisplayCategoryNode{}
	}

	categoryByID := make(map[int64]db.Category, len(res))
	for _, category := range res {
		categoryByID[category.ID] = category
	}

	roots := make([]db.Category, 0, len(res))
	childrenByParent := make(map[int64][]db.Category, len(res))
	for _, category := range res {
		parentID := category.ParentID
		if parentID == 0 || parentID == category.ID {
			roots = append(roots, category)
			continue
		}
		if _, ok := categoryByID[parentID]; !ok {
			roots = append(roots, category)
			continue
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], category)
	}

	sortCategories := func(items []db.Category) {
		sort.Slice(items, func(i, j int) bool {
			left := items[i]
			right := items[j]
			if left.Sort != right.Sort {
				return left.Sort < right.Sort
			}
			if left.CreatedAt != right.CreatedAt {
				return left.CreatedAt < right.CreatedAt
			}
			if left.ID != right.ID {
				return left.ID < right.ID
			}
			return left.Name < right.Name
		})
	}

	sortCategories(roots)
	for parentID := range childrenByParent {
		sortCategories(childrenByParent[parentID])
	}

	toDisplayItem := func(category db.Category) DisplayItem {
		return DisplayItem{
			ID:        category.ID,
			Name:      category.Name,
			Slug:      category.Slug,
			PermLink:  share.GetCategoryUrl(category),
			PostCount: category.PostCount,
			CreatedAt: category.CreatedAt,
			UpdatedAt: category.UpdatedAt,
		}
	}

	items := make([]*DisplayCategoryNode, 0, len(res))
	visited := make(map[int64]bool, len(res))
	var buildNode func(category db.Category) *DisplayCategoryNode
	buildNode = func(category db.Category) *DisplayCategoryNode {
		if visited[category.ID] {
			return nil
		}
		visited[category.ID] = true
		node := &DisplayCategoryNode{
			Item:     toDisplayItem(category),
			Children: make([]*DisplayCategoryNode, 0),
		}

		for _, child := range childrenByParent[category.ID] {
			if childNode := buildNode(child); childNode != nil {
				node.Children = append(node.Children, childNode)
			}
		}

		return node
	}

	for _, root := range roots {
		if node := buildNode(root); node != nil {
			items = append(items, node)
		}
	}

	if len(visited) < len(res) {
		remaining := make([]db.Category, 0, len(res)-len(visited))
		for _, category := range res {
			if !visited[category.ID] {
				remaining = append(remaining, category)
			}
		}
		sortCategories(remaining)
		for _, category := range remaining {
			if node := buildNode(category); node != nil {
				items = append(items, node)
			}
		}
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
		logger.Warn("get category by slug failed: slug=%s err=%v", slug, err)
		return nil
	}

	return &DisplayItem{
		ID:          category.ID,
		Name:        category.Name,
		Slug:        category.Slug,
		Description: category.Description,
		PermLink:    share.GetCategoryUrl(*category),
		PostCount:   category.PostCount,
		CreatedAt:   category.CreatedAt,
		UpdatedAt:   category.UpdatedAt,
	}
}
func GetTagBySlug(dbx *db.DB, slug string) *DisplayItem {
	tag, err := db.GetTagBySlug(dbx, slug)
	if err != nil {
		logger.Warn("get tag by slug failed: slug=%s err=%v", slug, err)
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
