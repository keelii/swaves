package admin

import (
	"errors"

	"swaves/internal/db"

	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidPassword = errors.New("invalid password")

type Service struct {
	DB *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{DB: db}
}

func (a *Service) CheckPassword(raw string) error {
	cfg, err := db.GetConfig(a.DB)
	if err != nil {
		return err
	}

	return bcrypt.CompareHashAndPassword(
		[]byte(cfg.AdminPasswordHash),
		[]byte(raw),
	)
}

type CreatePostInput struct {
	Title   string
	Slug    string
	Content string
	Status  string
}
type UpdatePostInput struct {
	Title   string
	Content string
	Status  string
}

func ListPosts(dbx *db.DB) ([]db.Post, error) {
	rows, err := dbx.Query(`
		SELECT id, title, slug, content, status, created_at, updated_at, deleted_at
		FROM posts
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []db.Post
	for rows.Next() {
		var p db.Post
		if err := rows.Scan(
			&p.ID,
			&p.Title,
			&p.Slug,
			&p.Content,
			&p.Status,
			&p.CreatedAt,
			&p.UpdatedAt,
			&p.DeletedAt,
		); err != nil {
			return nil, err
		}
		res = append(res, p)
	}
	return res, nil
}
func CreatePostService(dbx *db.DB, in CreatePostInput) error {
	if in.Title == "" || in.Slug == "" {
		return errors.New("title and slug required")
	}

	p := &db.Post{
		Title:   in.Title,
		Slug:    in.Slug,
		Content: in.Content,
		Status:  in.Status,
	}
	return db.CreatePost(dbx, p)
}
func GetPostForEdit(dbx *db.DB, id int64) (*db.Post, error) {
	return db.GetPostByID(dbx, id)
}
func UpdatePostService(dbx *db.DB, id int64, in UpdatePostInput) error {
	p, err := db.GetPostByID(dbx, id)
	if err != nil {
		return err
	}

	p.Title = in.Title
	p.Content = in.Content
	p.Status = in.Status

	return db.UpdatePost(dbx, p)
}
func DeletePostService(dbx *db.DB, id int64) error {
	return db.SoftDeletePost(dbx, id)
}
