package admin

import (
	"database/sql"
	"errors"
	"strconv"
	"swaves/internal/middleware"

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
	TagIDs  []int64
}

type UpdatePostInput struct {
	Title   string
	Content string
	Status  string
	TagIDs  []int64
}

type PostWithTags struct {
	Post *db.Post
	Tags []db.Tag
}

func ListPosts(dbx *db.DB, pager *middleware.Pagination) ([]PostWithTags, error) {
	// 先查询总数
	var total int
	row := dbx.QueryRow(`SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := dbx.Query(`
		SELECT id, title, slug, content, status, created_at, updated_at, deleted_at
		FROM posts
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []PostWithTags
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

		// 获取该 post 的 tags
		tags, err := db.GetPostTags(dbx, p.ID)
		if err != nil {
			return nil, err
		}

		res = append(res, PostWithTags{
			Post: &p,
			Tags: tags,
		})
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}
func GetAllTags(dbx *db.DB) ([]db.Tag, error) {
	rows, err := dbx.Query(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM tags
		WHERE deleted_at IS NULL
		ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []db.Tag
	for rows.Next() {
		var t db.Tag
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&t.ID,
			&t.Name,
			&t.Slug,
			&t.CreatedAt,
			&t.UpdatedAt,
			&t.DeletedAt,
		); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			t.DeletedAt = &deletedAt.Int64
		}
		res = append(res, t)
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
	if err := db.CreatePost(dbx, p); err != nil {
		return err
	}

	// 关联标签
	if len(in.TagIDs) > 0 {
		if err := db.SetPostTags(dbx, p.ID, in.TagIDs); err != nil {
			return err
		}
	}

	return nil
}

func GetPostForEdit(dbx *db.DB, id int64) (*PostWithTags, error) {
	post, err := db.GetPostByID(dbx, id)
	if err != nil {
		return nil, err
	}

	tags, err := db.GetPostTags(dbx, id)
	if err != nil {
		return nil, err
	}

	return &PostWithTags{
		Post: post,
		Tags: tags,
	}, nil
}

func UpdatePostService(dbx *db.DB, id int64, in UpdatePostInput) error {
	p, err := db.GetPostByID(dbx, id)
	if err != nil {
		return err
	}

	p.Title = in.Title
	p.Content = in.Content
	p.Status = in.Status

	if err := db.UpdatePost(dbx, p); err != nil {
		return err
	}

	// 更新标签关联
	if err := db.SetPostTags(dbx, id, in.TagIDs); err != nil {
		return err
	}

	return nil
}
func DeletePostService(dbx *db.DB, id int64) error {
	return db.SoftDeletePost(dbx, id)
}

// Tags
type CreateTagInput struct {
	Name string
	Slug string
}

type UpdateTagInput struct {
	Name string
	Slug string
}

func ListTags(dbx *db.DB, pager *middleware.Pagination) ([]db.Tag, error) {
	var total int
	row := dbx.QueryRow(`SELECT COUNT(*) FROM tags WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := dbx.Query(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM tags
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []db.Tag
	for rows.Next() {
		var t db.Tag
		if err := rows.Scan(
			&t.ID,
			&t.Name,
			&t.Slug,
			&t.CreatedAt,
			&t.UpdatedAt,
			&t.DeletedAt,
		); err != nil {
			return nil, err
		}
		res = append(res, t)
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}

func CreateTagService(dbx *db.DB, in CreateTagInput) error {
	if in.Name == "" || in.Slug == "" {
		return errors.New("name and slug required")
	}

	t := &db.Tag{
		Name: in.Name,
		Slug: in.Slug,
	}
	return db.CreateTag(dbx, t)
}

func GetTagForEdit(dbx *db.DB, id int64) (*db.Tag, error) {
	return db.GetTagByID(dbx, id)
}

func UpdateTagService(dbx *db.DB, id int64, in UpdateTagInput) error {
	t, err := db.GetTagByID(dbx, id)
	if err != nil {
		return err
	}

	t.Name = in.Name
	t.Slug = in.Slug

	return db.UpdateTag(dbx, t)
}

func DeleteTagService(dbx *db.DB, id int64) error {
	return db.SoftDeleteTag(dbx, id)
}

// Redirects
type CreateRedirectInput struct {
	From string
	To   string
}

type UpdateRedirectInput struct {
	From string
	To   string
}

func ListRedirects(dbx *db.DB, pager *middleware.Pagination) ([]db.Redirect, error) {
	var total int
	row := dbx.QueryRow(`SELECT COUNT(*) FROM redirects WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := dbx.Query(`
		SELECT id, from_path, to_path, created_at, updated_at, deleted_at
		FROM redirects
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []db.Redirect
	for rows.Next() {
		var r db.Redirect
		if err := rows.Scan(
			&r.ID,
			&r.From,
			&r.To,
			&r.CreatedAt,
			&r.UpdatedAt,
			&r.DeletedAt,
		); err != nil {
			return nil, err
		}
		res = append(res, r)
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}

func CreateRedirectService(dbx *db.DB, in CreateRedirectInput) error {
	if in.From == "" || in.To == "" {
		return errors.New("from and to required")
	}

	r := &db.Redirect{
		From: in.From,
		To:   in.To,
	}
	return db.CreateRedirect(dbx, r)
}

func GetRedirectForEdit(dbx *db.DB, id int64) (*db.Redirect, error) {
	return db.GetRedirectByID(dbx, id)
}

func UpdateRedirectService(dbx *db.DB, id int64, in UpdateRedirectInput) error {
	r, err := db.GetRedirectByID(dbx, id)
	if err != nil {
		return err
	}

	r.From = in.From
	r.To = in.To

	return db.UpdateRedirect(dbx, r)
}

func DeleteRedirectService(dbx *db.DB, id int64) error {
	return db.SoftDeleteRedirect(dbx, id)
}

// Encrypted Posts
type CreateEncryptedPostInput struct {
	Title     string
	Content   string
	Password  string
	ExpiresAt string
}

type UpdateEncryptedPostInput struct {
	Title     string
	Content   string
	Password  string
	ExpiresAt string
}

func ListEncryptedPosts(dbx *db.DB, pager *middleware.Pagination) ([]db.EncryptedPost, error) {
	var total int
	row := dbx.QueryRow(`SELECT COUNT(*) FROM encrypted_posts WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := dbx.Query(`
		SELECT id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at
		FROM encrypted_posts
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []db.EncryptedPost
	for rows.Next() {
		var p db.EncryptedPost
		var expiresAt sql.NullInt64
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID,
			&p.Title,
			&p.Slug,
			&p.Content,
			&p.Password,
			&expiresAt,
			&p.CreatedAt,
			&p.UpdatedAt,
			&deletedAt,
		); err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			p.ExpiresAt = &expiresAt.Int64
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}
		res = append(res, p)
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}

func CreateEncryptedPostService(dbx *db.DB, in CreateEncryptedPostInput) error {
	if in.Title == "" || in.Password == "" {
		return errors.New("title and password required")
	}

	var expiresAt *int64
	if in.ExpiresAt != "" {
		// 解析时间戳字符串
		ts, err := strconv.ParseInt(in.ExpiresAt, 10, 64)
		if err == nil {
			expiresAt = &ts
		}
	}

	p := &db.EncryptedPost{
		Title:     in.Title,
		Content:   in.Content,
		Password:  in.Password,
		ExpiresAt: expiresAt,
	}
	return db.CreateEncryptedPost(dbx, p)
}

func GetEncryptedPostForEdit(dbx *db.DB, id int64) (*db.EncryptedPost, error) {
	return db.GetEncryptedPostByID(dbx, id)
}

func UpdateEncryptedPostService(dbx *db.DB, id int64, in UpdateEncryptedPostInput) error {
	p, err := db.GetEncryptedPostByID(dbx, id)
	if err != nil {
		return err
	}

	p.Title = in.Title
	p.Content = in.Content
	if in.Password != "" {
		p.Password = in.Password
	}

	if in.ExpiresAt != "" {
		ts, err := strconv.ParseInt(in.ExpiresAt, 10, 64)
		if err == nil {
			p.ExpiresAt = &ts
		} else {
			p.ExpiresAt = nil
		}
	} else {
		p.ExpiresAt = nil
	}

	return db.UpdateEncryptedPost(dbx, p)
}

func DeleteEncryptedPostService(dbx *db.DB, id int64) error {
	return db.SoftDeleteEncryptedPost(dbx, id)
}

// Configs
type UpdateConfigInput struct {
	Name            string
	Language        string
	Timezone        string
	PostSlugPattern string
	TagSlugPattern  string
	TagsPattern     string
	GiscusConfig    string
	GA4ID           string
	AdminPassword   string
}

func GetConfigForEdit(dbx *db.DB) (*db.Configs, error) {
	return db.GetConfig(dbx)
}

func UpdateConfigService(dbx *db.DB, in UpdateConfigInput) error {
	cfg, err := db.GetConfig(dbx)
	if err != nil {
		return err
	}

	cfg.Name = in.Name
	cfg.Language = in.Language
	cfg.Timezone = in.Timezone
	cfg.PostSlugPattern = in.PostSlugPattern
	cfg.TagSlugPattern = in.TagSlugPattern
	cfg.TagsPattern = in.TagsPattern
	cfg.GiscusConfig = in.GiscusConfig
	cfg.GA4ID = in.GA4ID

	// 只有提供了新密码时才更新
	if in.AdminPassword != "" {
		cfg.AdminPasswordHash = in.AdminPassword
	}

	return db.UpdateConfig(dbx, cfg)
}
