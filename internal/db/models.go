package db

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	*sql.DB
}

type Options struct {
	DSN string
}

func Open(opts Options) *DB {
	sqlDB, e1 := sql.Open("sqlite3", opts.DSN)
	if e1 != nil {
		log.Fatalf("open sqlite failed: %v", e1)
	}

	sqlDB.Exec(`PRAGMA journal_mode = WAL;`)
	sqlDB.Exec(`PRAGMA busy_timeout = 5000;`)

	conn := &DB{DB: sqlDB}

	if r2 := Migrate(conn); r2 != nil {
		log.Fatalf("migrate failed: %v", r2)
	}

	return conn
}

func Migrate(db *DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS encrypted_posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			title TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			content TEXT NOT NULL,
			password TEXT NOT NULL,
			expires_at INTEGER,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS tags (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			slug TEXT NOT NULL UNIQUE,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS post_tags (
			post_id INTEGER NOT NULL,
			tag_id INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER,
			UNIQUE(post_id, tag_id)
		);`,

		`CREATE TABLE IF NOT EXISTS redirects (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_path TEXT NOT NULL UNIQUE,
			to_path TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS configs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			language TEXT NOT NULL,
			timezone TEXT NOT NULL,
			post_slug_pattern TEXT NOT NULL,
			tag_slug_pattern TEXT NOT NULL,
			tags_pattern TEXT NOT NULL,
			giscus_config TEXT,
			ga4_id TEXT,
			admin_password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS admin_sessions (
		  id TEXT PRIMARY KEY,
		  sid TEXT NOT NULL UNIQUE,
		  expires_at INTEGER NOT NULL,
		  created_at INTEGER NOT NULL,
		  updated_at INTEGER NOT NULL,
		  deleted_at INTEGER
		);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	if _, err := EnsureConfigExists(db); err != nil {
		log.Fatalf("ensure config exists failed: %v", err)
	}

	return nil
}

func EnsureConfigExists(db *DB) (*Configs, error) {
	_, err := GetConfig(db)
	if err == nil {
		// 已经存在
		return nil, nil
	}
	if err != ErrNotFound {
		return nil, err
	}

	// 创建默认配置
	now := time.Now().Unix()
	cfg := &Configs{
		Name:              "swaves",
		Language:          "zh-CN",
		Timezone:          "Asia/Shanghai",
		PostSlugPattern:   "/{yyyy}/{MM}/{dd}/{name}",
		TagSlugPattern:    "/tags/{name}",
		TagsPattern:       "/tags",
		AdminPasswordHash: "admin",
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	if err := CreateConfig(db, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

func now() int64 {
	return time.Now().Unix()
}

type Post struct {
	ID        int64
	Title     string
	Slug      string
	Content   string
	Status    string
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func CreatePost(db *DB, p *Post) error {
	if p.CreatedAt == 0 {
		p.CreatedAt = now()
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = p.CreatedAt
	}

	res, err := db.Exec(
		`INSERT INTO posts (title, slug, content, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.Title, p.Slug, p.Content, p.Status, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

func GetPostByID(db *DB, id int64) (*Post, error) {
	row := db.QueryRow(
		`SELECT id, title, slug, content, status, created_at, updated_at, deleted_at
		 FROM posts WHERE id=? AND deleted_at IS NULL`,
		id,
	)

	var p Post
	var deletedAt sql.NullInt64
	if err := row.Scan(
		&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status,
		&p.CreatedAt, &p.UpdatedAt, &deletedAt,
	); err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		p.DeletedAt = &deletedAt.Int64
	}
	return &p, nil
}

func UpdatePost(db *DB, p *Post) error {
	p.UpdatedAt = now()
	_, err := db.Exec(
		`UPDATE posts
		 SET title=?, content=?, status=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		p.Title, p.Content, p.Status, p.UpdatedAt, p.ID,
	)
	return err
}

func SoftDeletePost(db *DB, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE posts SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		ts, id,
	)
	return err
}

func RestorePost(db *DB, id int64) error {
	_, err := db.Exec(
		`UPDATE posts SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`,
		id,
	)
	return err
}

func ListDeletedPosts(db *DB) ([]Post, error) {
	rows, err := db.Query(`
		SELECT id, title, slug, content, status, created_at, updated_at, deleted_at
		FROM posts
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Post
	for rows.Next() {
		var p Post
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status,
			&p.CreatedAt, &p.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}
		res = append(res, p)
	}
	return res, nil
}

type EncryptedPost struct {
	ID        int64
	Title     string
	Slug      string
	Content   string
	Password  string
	ExpiresAt *int64
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func GetEncryptedPostByID(db *DB, id int64) (*EncryptedPost, error) {
	row := db.QueryRow(
		`SELECT id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at
		 FROM encrypted_posts WHERE id=? AND deleted_at IS NULL`,
		id,
	)

	var p EncryptedPost
	var deletedAt sql.NullInt64
	var expiresAt sql.NullInt64
	if err := row.Scan(
		&p.ID, &p.Title, &p.Slug, &p.Content, &p.Password,
		&expiresAt, &p.CreatedAt, &p.UpdatedAt, &deletedAt,
	); err != nil {
		return nil, err
	}
	if expiresAt.Valid {
		p.ExpiresAt = &expiresAt.Int64
	}
	if deletedAt.Valid {
		p.DeletedAt = &deletedAt.Int64
	}
	return &p, nil
}

func UpdateEncryptedPost(db *DB, p *EncryptedPost) error {
	p.UpdatedAt = now()
	_, err := db.Exec(
		`UPDATE encrypted_posts
		 SET title=?, content=?, password=?, expires_at=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		p.Title, p.Content, p.Password, p.ExpiresAt, p.UpdatedAt, p.ID,
	)
	return err
}

func SoftDeleteEncryptedPost(db *DB, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE encrypted_posts SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		ts, id,
	)
	return err
}

func RestoreEncryptedPost(db *DB, id int64) error {
	_, err := db.Exec(
		`UPDATE encrypted_posts SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`,
		id,
	)
	return err
}

func ListDeletedEncryptedPosts(db *DB) ([]EncryptedPost, error) {
	rows, err := db.Query(`
		SELECT id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at
		FROM encrypted_posts
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []EncryptedPost
	for rows.Next() {
		var p EncryptedPost
		var deletedAt sql.NullInt64
		var expiresAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Password,
			&expiresAt, &p.CreatedAt, &p.UpdatedAt, &deletedAt,
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
	return res, nil
}

func CreateEncryptedPost(db *DB, p *EncryptedPost) error {
	if p.Slug == "" {
		p.Slug = uuid.NewString()
	}
	if p.CreatedAt == 0 {
		p.CreatedAt = now()
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = p.CreatedAt
	}

	res, err := db.Exec(
		`INSERT INTO encrypted_posts
		 (title, slug, content, password, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.Title, p.Slug, p.Content, p.Password, p.ExpiresAt, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return err
	}
	p.ID, _ = res.LastInsertId()
	return nil
}

type Tag struct {
	ID        int64
	Name      string
	Slug      string
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func CreateTag(db *DB, t *Tag) error {
	if t.CreatedAt == 0 {
		t.CreatedAt = now()
	}
	if t.UpdatedAt == 0 {
		t.UpdatedAt = t.CreatedAt
	}

	res, err := db.Exec(
		`INSERT INTO tags (name, slug, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		t.Name, t.Slug, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return err
	}
	t.ID, _ = res.LastInsertId()
	return nil
}

func GetTagByID(db *DB, id int64) (*Tag, error) {
	row := db.QueryRow(
		`SELECT id, name, slug, created_at, updated_at, deleted_at
		 FROM tags WHERE id=? AND deleted_at IS NULL`,
		id,
	)

	var t Tag
	var deletedAt sql.NullInt64
	if err := row.Scan(
		&t.ID, &t.Name, &t.Slug,
		&t.CreatedAt, &t.UpdatedAt, &deletedAt,
	); err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		t.DeletedAt = &deletedAt.Int64
	}
	return &t, nil
}

func UpdateTag(db *DB, t *Tag) error {
	t.UpdatedAt = now()
	_, err := db.Exec(
		`UPDATE tags
		 SET name=?, slug=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		t.Name, t.Slug, t.UpdatedAt, t.ID,
	)
	return err
}

func SoftDeleteTag(db *DB, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE tags SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		ts, id,
	)
	return err
}

func RestoreTag(db *DB, id int64) error {
	_, err := db.Exec(
		`UPDATE tags SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`,
		id,
	)
	return err
}

func ListDeletedTags(db *DB) ([]Tag, error) {
	rows, err := db.Query(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM tags
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Tag
	for rows.Next() {
		var t Tag
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug,
			&t.CreatedAt, &t.UpdatedAt, &deletedAt,
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

func GetPostTags(db *DB, postID int64) ([]Tag, error) {
	rows, err := db.Query(`
		SELECT t.id, t.name, t.slug, t.created_at, t.updated_at, t.deleted_at
		FROM tags t
		INNER JOIN post_tags pt ON t.id = pt.tag_id
		WHERE pt.post_id = ? AND pt.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY t.name
	`, postID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		var t Tag
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Slug,
			&t.CreatedAt, &t.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			t.DeletedAt = &deletedAt.Int64
		}
		tags = append(tags, t)
	}
	return tags, nil
}

func AttachTagToPost(db *DB, postID, tagID int64) error {
	ts := now()
	_, err := db.Exec(
		`INSERT OR IGNORE INTO post_tags
		 (post_id, tag_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		postID, tagID, ts, ts,
	)
	return err
}

func DetachTagFromPost(db *DB, postID, tagID int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE post_tags SET deleted_at=? WHERE post_id=? AND tag_id=? AND deleted_at IS NULL`,
		ts, postID, tagID,
	)
	return err
}

func SetPostTags(db *DB, postID int64, tagIDs []int64) error {
	// 先获取当前关联的标签
	currentTags, err := GetPostTags(db, postID)
	if err != nil {
		return err
	}

	currentTagIDs := make(map[int64]bool)
	for _, tag := range currentTags {
		currentTagIDs[tag.ID] = true
	}

	newTagIDs := make(map[int64]bool)
	for _, tagID := range tagIDs {
		newTagIDs[tagID] = true
	}

	// 删除不再需要的标签关联
	for _, tag := range currentTags {
		if !newTagIDs[tag.ID] {
			if err := DetachTagFromPost(db, postID, tag.ID); err != nil {
				return err
			}
		}
	}

	// 添加新的标签关联
	for _, tagID := range tagIDs {
		if !currentTagIDs[tagID] {
			if err := AttachTagToPost(db, postID, tagID); err != nil {
				return err
			}
		}
	}

	return nil
}

type Redirect struct {
	ID        int64
	From      string
	To        string
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func GetRedirectByID(db *DB, id int64) (*Redirect, error) {
	row := db.QueryRow(
		`SELECT id, from_path, to_path, created_at, updated_at, deleted_at
		 FROM redirects WHERE id=? AND deleted_at IS NULL`,
		id,
	)

	var r Redirect
	var deletedAt sql.NullInt64
	if err := row.Scan(
		&r.ID, &r.From, &r.To,
		&r.CreatedAt, &r.UpdatedAt, &deletedAt,
	); err != nil {
		return nil, err
	}
	if deletedAt.Valid {
		r.DeletedAt = &deletedAt.Int64
	}
	return &r, nil
}

func UpdateRedirect(db *DB, r *Redirect) error {
	r.UpdatedAt = now()
	_, err := db.Exec(
		`UPDATE redirects
		 SET from_path=?, to_path=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		r.From, r.To, r.UpdatedAt, r.ID,
	)
	return err
}

func SoftDeleteRedirect(db *DB, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE redirects SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		ts, id,
	)
	return err
}

func RestoreRedirect(db *DB, id int64) error {
	_, err := db.Exec(
		`UPDATE redirects SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`,
		id,
	)
	return err
}

func ListDeletedRedirects(db *DB) ([]Redirect, error) {
	rows, err := db.Query(`
		SELECT id, from_path, to_path, created_at, updated_at, deleted_at
		FROM redirects
		WHERE deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []Redirect
	for rows.Next() {
		var r Redirect
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&r.ID, &r.From, &r.To,
			&r.CreatedAt, &r.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			r.DeletedAt = &deletedAt.Int64
		}
		res = append(res, r)
	}
	return res, nil
}

func CreateRedirect(db *DB, r *Redirect) error {
	if r.CreatedAt == 0 {
		r.CreatedAt = now()
	}
	if r.UpdatedAt == 0 {
		r.UpdatedAt = r.CreatedAt
	}

	res, err := db.Exec(
		`INSERT INTO redirects (from_path, to_path, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		r.From, r.To, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

type Configs struct {
	ID                int64
	Name              string
	Language          string
	Timezone          string
	PostSlugPattern   string
	TagSlugPattern    string
	TagsPattern       string
	GiscusConfig      string
	GA4ID             string
	AdminPasswordHash string
	CreatedAt         int64
	UpdatedAt         int64
	DeletedAt         *int64
}

func CreateConfig(db *DB, c *Configs) error {
	if c.AdminPasswordHash == "" {
		return errors.New("admin password required")
	}

	// bcrypt 原始密码
	hashed, err := bcrypt.GenerateFromPassword(
		[]byte(c.AdminPasswordHash),
		bcrypt.DefaultCost,
	)
	if err != nil {
		return err
	}
	c.AdminPasswordHash = string(hashed)

	if c.CreatedAt == 0 {
		c.CreatedAt = now()
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = c.CreatedAt
	}

	res, err := db.Exec(
		`INSERT INTO configs
		 (name, language, timezone,
		  post_slug_pattern, tag_slug_pattern, tags_pattern,
		  giscus_config, ga4_id, admin_password_hash,
		  created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Name,
		c.Language,
		c.Timezone,
		c.PostSlugPattern,
		c.TagSlugPattern,
		c.TagsPattern,
		c.GiscusConfig,
		c.GA4ID,
		c.AdminPasswordHash,
		c.CreatedAt,
		c.UpdatedAt,
	)
	if err != nil {
		return err
	}

	c.ID, _ = res.LastInsertId()
	return nil
}

func GetConfig(db *DB) (*Configs, error) {
	row := db.QueryRow(`
		SELECT id, name, language, timezone,
		       post_slug_pattern, tag_slug_pattern, tags_pattern,
		       giscus_config, ga4_id, admin_password_hash,
		       created_at, updated_at, deleted_at
		FROM configs
		WHERE deleted_at IS NULL
		LIMIT 1
	`)

	var c Configs
	var deletedAt sql.NullInt64

	err := row.Scan(
		&c.ID,
		&c.Name,
		&c.Language,
		&c.Timezone,
		&c.PostSlugPattern,
		&c.TagSlugPattern,
		&c.TagsPattern,
		&c.GiscusConfig,
		&c.GA4ID,
		&c.AdminPasswordHash,
		&c.CreatedAt,
		&c.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if deletedAt.Valid {
		c.DeletedAt = &deletedAt.Int64
	}
	return &c, nil
}

func UpdateConfig(db *DB, c *Configs) error {
	c.UpdatedAt = now()

	// 如果提供了新密码，需要重新加密
	if c.AdminPasswordHash != "" && len(c.AdminPasswordHash) < 60 {
		// 如果密码看起来不是 bcrypt hash（长度小于60），则加密它
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(c.AdminPasswordHash),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return err
		}
		c.AdminPasswordHash = string(hashed)
	}

	_, err := db.Exec(
		`UPDATE configs
		 SET name=?, language=?, timezone=?,
		     post_slug_pattern=?, tag_slug_pattern=?, tags_pattern=?,
		     giscus_config=?, ga4_id=?,
		     admin_password_hash=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		c.Name,
		c.Language,
		c.Timezone,
		c.PostSlugPattern,
		c.TagSlugPattern,
		c.TagsPattern,
		c.GiscusConfig,
		c.GA4ID,
		c.AdminPasswordHash,
		c.UpdatedAt,
		c.ID,
	)
	return err
}

func (c *Configs) CheckPassword(raw string) error {
	if c.AdminPasswordHash == "" {
		return ErrNotFound
	}
	fmt.Printf("%s\n%s\n%s\n", c.AdminPasswordHash, raw, bcrypt.CompareHashAndPassword([]byte(c.AdminPasswordHash), []byte(raw)))
	return bcrypt.CompareHashAndPassword([]byte(c.AdminPasswordHash), []byte(raw))
}

type AdminSession struct {
	ID        string
	Sid       string
	ExpiresAt int64
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func CreateAdminSession(db *DB, ttl time.Duration) (*AdminSession, error) {
	now := time.Now().Unix()
	expiresAt := now + int64(ttl.Seconds())

	uuid := uuid.NewString()

	_, err := db.Exec(`
		INSERT INTO admin_sessions (
			sid, created_at, expires_at
		) VALUES (?, ?, ?)
	`, uuid, now, expiresAt)
	if err != nil {
		return nil, err
	}

	return &AdminSession{
		Sid:       uuid,
		CreatedAt: now,
		ExpiresAt: expiresAt,
	}, nil
}

var ErrNotFound = errors.New("not found")
