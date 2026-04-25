package site

import "swaves/internal/platform/db"

// DisplayPost 包含了 Post 的原始数据，以及一些额外的显示相关字段，如 PermLink 和 HTML
type DisplayPost struct {
	db.Post
	Prev     *DisplayPostInfo
	Next     *DisplayPostInfo
	PermLink string
	HTML     string
	TOCHTML  string
}
type DisplayTag struct {
	db.Tag
	PermLink string
}
type DisplayCategory struct {
	db.Category
	PermLink string
}

type DisplayPostWithRelation struct {
	DisplayPost
	Tags     []DisplayItem
	Category *DisplayItem
}

type TemplatePost struct {
	ID             int64
	Title          string
	Slug           string
	Content        string
	Status         string
	Kind           db.PostKind
	CommentEnabled int
	PublishedAt    int64
	CreatedAt      int64
	UpdatedAt      int64
	PermLink       string
	HTML           string
	TOCHTML        string
	Prev           *DisplayPostInfo
	Next           *DisplayPostInfo
	Tags           []DisplayItem
	Category       *DisplayItem
}

type DisplayComment struct {
	db.Comment
	ParentAuthor string
	Children     []*DisplayComment
}

func (receiver DisplayPostWithRelation) Raw() *db.Post {
	return &receiver.Post
}

func ToTemplatePost(post *DisplayPostWithRelation) TemplatePost {
	if post == nil {
		return TemplatePost{}
	}

	return TemplatePost{
		ID:             post.Post.ID,
		Title:          post.Post.Title,
		Slug:           post.Post.Slug,
		Content:        post.Post.Content,
		Status:         post.Post.Status,
		Kind:           post.Post.Kind,
		CommentEnabled: post.Post.CommentEnabled,
		PublishedAt:    post.Post.PublishedAt,
		CreatedAt:      post.Post.CreatedAt,
		UpdatedAt:      post.Post.UpdatedAt,
		PermLink:       post.PermLink,
		HTML:           post.HTML,
		TOCHTML:        post.TOCHTML,
		Prev:           post.Prev,
		Next:           post.Next,
		Tags:           post.Tags,
		Category:       post.Category,
	}
}

func ToTemplatePosts(posts []DisplayPost) []TemplatePost {
	if len(posts) == 0 {
		return []TemplatePost{}
	}

	result := make([]TemplatePost, 0, len(posts))
	for _, post := range posts {
		result = append(result, TemplatePost{
			ID:             post.Post.ID,
			Title:          post.Post.Title,
			Slug:           post.Post.Slug,
			Content:        post.Post.Content,
			Status:         post.Post.Status,
			Kind:           post.Post.Kind,
			CommentEnabled: post.Post.CommentEnabled,
			PublishedAt:    post.Post.PublishedAt,
			CreatedAt:      post.Post.CreatedAt,
			UpdatedAt:      post.Post.UpdatedAt,
			PermLink:       post.PermLink,
			HTML:           post.HTML,
			TOCHTML:        post.TOCHTML,
			Prev:           post.Prev,
			Next:           post.Next,
		})
	}
	return result
}

// DisplayPostInfo 包含了 Post 的基本信息，适用于在列表中显示，没有 HTML 内容 和 content
type DisplayPostInfo struct {
	ID          int64
	Title       string
	Slug        string
	PermLink    string
	PublishedAt int64
	CreatedAt   int64
	UpdatedAt   int64
}
type DisplayPostRelativeInfo struct {
	ID          int64
	Title       string
	Slug        string
	PermLink    string
	Tags        []DisplayItem
	Category    *DisplayItem
	PublishedAt int64
	CreatedAt   int64
	UpdatedAt   int64
}

type DisplayCategoryNode struct {
	Item     DisplayItem
	Children []*DisplayCategoryNode
}

type DisplayItem struct {
	ID          int64
	Name        string
	Slug        string
	Description string
	PermLink    string
	PostCount   int
	CreatedAt   int64
	UpdatedAt   int64
}

func (p DisplayPost) Raw() db.Post {
	return p.Post
}
