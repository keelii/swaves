package ui

import "swaves/internal/db"

type DisplayPost struct {
	db.Post
	PostLink string
	HTML     string
}
type DisplayPage struct {
	ID          int64
	Title       string
	Slug        string
	PostLink    string
	Status      string
	PublishedAt int64
	CreatedAt   int64
	UpdatedAt   int64
}

func (p DisplayPost) Raw() db.Post {
	return p.Post
}
