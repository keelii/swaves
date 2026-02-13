package ui

import "swaves/internal/db"

type DisplayPost struct {
	db.Post
	PostLink string
	HTML     string
}

func (p DisplayPost) Raw() db.Post {
	return p.Post
}
