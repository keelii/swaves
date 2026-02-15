package ui

import "swaves/internal/db"

// DisplayPost 包含了 Post 的原始数据，以及一些额外的显示相关字段，如 PermLink 和 HTML
type DisplayPost struct {
	db.Post
	PermLink string
	HTML     string
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
	Category    DisplayItem
	PublishedAt int64
	CreatedAt   int64
	UpdatedAt   int64
}
type DisplayItem struct {
	ID        int64
	Name      string
	Slug      string
	PermLink  string
	PostCount int
	CreatedAt int64
	UpdatedAt int64
}

func (p DisplayPost) Raw() db.Post {
	return p.Post
}
