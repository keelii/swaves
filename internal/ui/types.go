package ui

import "swaves/internal/db"

type DisplayPost struct {
	db.Post
	HTML string
}

func (p DisplayPost) Raw() db.Post {
	return p.Post
}
