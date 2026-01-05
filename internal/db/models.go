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

		`CREATE TABLE IF NOT EXISTS settings (
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

		`CREATE TABLE IF NOT EXISTS http_error_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,

			req_id TEXT NOT NULL,
			client_ip TEXT NOT NULL,
			method TEXT NOT NULL,
			path TEXT NOT NULL,
			status INTEGER NOT NULL,
			user_agent TEXT NOT NULL,

			query_params TEXT,
			body_params TEXT,

			created_at INTEGER NOT NULL,
			expired_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_http_error_logs_created_at
		 ON http_error_logs(created_at);`,

		`CREATE INDEX IF NOT EXISTS idx_http_error_logs_expired_at
		 ON http_error_logs(expired_at);`,

		`CREATE INDEX IF NOT EXISTS idx_http_error_logs_path
		 ON http_error_logs(path);`,

		`CREATE INDEX IF NOT EXISTS idx_http_error_logs_status
		 ON http_error_logs(status);`,

		`CREATE TABLE IF NOT EXISTS cron_jobs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,

			name TEXT NOT NULL,                 -- 任务名称（后台展示）
			description TEXT NOT NULL DEFAULT '',

			schedule TEXT NOT NULL,             -- cron 表达式，如 "0 */5 * * *"
			enabled INTEGER NOT NULL DEFAULT 1, -- 1=启用 0=停用

			last_run_at INTEGER,                -- 最近一次开始执行时间（可选）
			last_success_at INTEGER,            -- 最近一次成功时间（可选）
			last_error_at INTEGER,              -- 最近一次失败时间（可选）

			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted_at INTEGER
		);`,

		`CREATE TABLE IF NOT EXISTS cron_job_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,

			job_id INTEGER NOT NULL,             -- cron_jobs.id（无外键）
			run_id TEXT NOT NULL,                -- 单次执行唯一标识（UUID）

			status TEXT NOT NULL,                -- "success" | "error"
			message TEXT NOT NULL DEFAULT '',    -- 简要结果 / 错误信息

			started_at INTEGER NOT NULL,         -- 执行开始时间
			finished_at INTEGER NOT NULL,        -- 执行结束时间
			duration INTEGER NOT NULL,           -- 执行耗时（毫秒）

			expire_at INTEGER,                   -- 过期时间（可为空）

			created_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_cron_job_logs_job_id
		ON cron_job_logs(job_id);

		CREATE INDEX IF NOT EXISTS idx_cron_job_logs_job_id_status
		ON cron_job_logs(job_id, status);

		CREATE INDEX IF NOT EXISTS idx_cron_job_logs_created_at
		ON cron_job_logs(created_at);

		CREATE INDEX IF NOT EXISTS idx_cron_job_logs_expire_at
		ON cron_job_logs(expire_at);`,
	}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	if _, err := EnsureSettingsExists(db); err != nil {
		log.Fatalf("ensure settings exists failed: %v", err)
	}

	return nil
}

func EnsureSettingsExists(db *DB) (*Settings, error) {
	_, err := GetSettings(db)
	if err == nil {
		// 已经存在
		return nil, nil
	}
	if err != ErrNotFound {
		return nil, err
	}

	// 创建默认配置
	now := time.Now().Unix()
	cfg := &Settings{
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

	if err := CreateSettings(db, cfg); err != nil {
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

type Settings struct {
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

func CreateSettings(db *DB, c *Settings) error {
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
		`INSERT INTO settings
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

func GetSettings(db *DB) (*Settings, error) {
	row := db.QueryRow(`
		SELECT id, name, language, timezone,
		       post_slug_pattern, tag_slug_pattern, tags_pattern,
		       giscus_config, ga4_id, admin_password_hash,
		       created_at, updated_at, deleted_at
		FROM settings
		WHERE deleted_at IS NULL
		LIMIT 1
	`)

	var c Settings
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

func UpdateSettings(db *DB, c *Settings) error {
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
		`UPDATE settings
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

func (c *Settings) CheckPassword(raw string) error {
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

type HttpErrorLog struct {
	ID          int64
	ReqID       string
	ClientIP    string
	Method      string
	Path        string
	Status      int
	UserAgent   string
	QueryParams string
	BodyParams  string
	CreatedAt   int64
	ExpiredAt   int64
}

func CreateHttpErrorLog(db *DB, l *HttpErrorLog) error {
	if l.CreatedAt == 0 {
		l.CreatedAt = now()
	}
	if l.ExpiredAt == 0 {
		// 默认 7 天
		l.ExpiredAt = l.CreatedAt + 7*24*60*60
	}

	res, err := db.Exec(`
		INSERT INTO http_error_logs (
			req_id,
			client_ip,
			method,
			path,
			status,
			user_agent,
			query_params,
			body_params,
			created_at,
			expired_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		l.ReqID,
		l.ClientIP,
		l.Method,
		l.Path,
		l.Status,
		l.UserAgent,
		l.QueryParams,
		l.BodyParams,
		l.CreatedAt,
		l.ExpiredAt,
	)
	if err != nil {
		return err
	}

	l.ID, _ = res.LastInsertId()
	return nil
}

func ListHttpErrorLogs(db *DB, limit, offset int) ([]HttpErrorLog, error) {
	rows, err := db.Query(`
		SELECT id, req_id, client_ip, method, path, status, user_agent,
		       query_params, body_params, created_at, expired_at
		FROM http_error_logs
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []HttpErrorLog
	for rows.Next() {
		var l HttpErrorLog
		if err := rows.Scan(
			&l.ID,
			&l.ReqID,
			&l.ClientIP,
			&l.Method,
			&l.Path,
			&l.Status,
			&l.UserAgent,
			&l.QueryParams,
			&l.BodyParams,
			&l.CreatedAt,
			&l.ExpiredAt,
		); err != nil {
			return nil, err
		}
		res = append(res, l)
	}
	return res, nil
}

func CountHttpErrorLogs(db *DB) (int, error) {
	var total int
	row := db.QueryRow(`SELECT COUNT(*) FROM http_error_logs`)
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func DeleteHttpErrorLog(db *DB, id int64) error {
	_, err := db.Exec(`DELETE FROM http_error_logs WHERE id=?`, id)
	return err
}

// CronJob 定义
type CronJob struct {
	ID            int64
	Name          string
	Description   string
	Schedule      string
	Enabled       bool
	LastRunAt     *int64
	LastSuccessAt *int64
	LastErrorAt   *int64
	CreatedAt     int64
	UpdatedAt     int64
	DeletedAt     *int64
}

func CreateCronJob(db *DB, job *CronJob) error {
	if job.CreatedAt == 0 {
		job.CreatedAt = now()
	}
	if job.UpdatedAt == 0 {
		job.UpdatedAt = job.CreatedAt
	}

	enabled := 0
	if job.Enabled {
		enabled = 1
	}

	res, err := db.Exec(`
		INSERT INTO cron_jobs (
			name, description, schedule, enabled,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`,
		job.Name,
		job.Description,
		job.Schedule,
		enabled,
		job.CreatedAt,
		job.UpdatedAt,
	)
	if err != nil {
		return err
	}

	job.ID, _ = res.LastInsertId()
	return nil
}
func ListCronJobs(db *DB) ([]CronJob, error) {
	rows, err := db.Query(`
		SELECT
			id, name, description, schedule, enabled,
			last_run_at, last_success_at, last_error_at,
			created_at, updated_at, deleted_at
		FROM cron_jobs
		WHERE deleted_at IS NULL
		ORDER BY id DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []CronJob

	for rows.Next() {
		var job CronJob
		var enabled int
		var lastRun, lastSuccess, lastError sql.NullInt64
		var deletedAt sql.NullInt64

		if err := rows.Scan(
			&job.ID,
			&job.Name,
			&job.Description,
			&job.Schedule,
			&enabled,
			&lastRun,
			&lastSuccess,
			&lastError,
			&job.CreatedAt,
			&job.UpdatedAt,
			&deletedAt,
		); err != nil {
			return nil, err
		}

		job.Enabled = enabled == 1
		if lastRun.Valid {
			job.LastRunAt = &lastRun.Int64
		}
		if lastSuccess.Valid {
			job.LastSuccessAt = &lastSuccess.Int64
		}
		if lastError.Valid {
			job.LastErrorAt = &lastError.Int64
		}
		if deletedAt.Valid {
			job.DeletedAt = &deletedAt.Int64
		}

		res = append(res, job)
	}

	return res, nil
}

// CronJobLog 单次执行日志
type CronJobLog struct {
	ID      int64
	JobID   int64
	RunID   string
	Status  string // "success" | "error"
	Message string

	StartedAt  int64
	FinishedAt int64
	Duration   int64

	ExpireAt  *int64
	CreatedAt int64
}

func CreateCronJobLog(db *DB, log *CronJobLog) error {
	// 生成 run_id
	log.RunID = uuid.NewString()
	if log.CreatedAt == 0 {
		log.CreatedAt = now()
	}

	// 写入 cron_job_logs 表
	res, err := db.Exec(`
		INSERT INTO cron_job_logs (
			job_id, run_id, status, message,
			started_at, finished_at, duration,
			expire_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		log.JobID,
		log.RunID,
		log.Status,
		log.Message,
		log.StartedAt,
		log.FinishedAt,
		log.Duration,
		log.ExpireAt,
		log.CreatedAt,
	)
	if err != nil {
		return err
	}

	log.ID, _ = res.LastInsertId()

	// 同步更新 cron_jobs 的快速状态字段
	switch log.Status {
	case "success":
		_, _ = db.Exec(`
			UPDATE cron_jobs
			SET last_run_at = ?, last_success_at = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL
		`, log.StartedAt, log.FinishedAt, now(), log.JobID)
	case "error":
		_, _ = db.Exec(`
			UPDATE cron_jobs
			SET last_run_at = ?, last_error_at = ?, updated_at = ?
			WHERE id = ? AND deleted_at IS NULL
		`, log.StartedAt, log.FinishedAt, now(), log.JobID)
	}

	// 裁剪旧日志：保留每个状态最近 N 条
	if err := trimCronJobLogs(db, log.JobID, log.Status, DefaultCronJobLogLimit); err != nil {
		// 裁剪失败不影响主流程，可记录日志
		fmt.Printf("trim cron job logs failed: %v\n", err)
	}

	return nil
}

// 内部函数，裁剪超过 limit 的旧日志
func trimCronJobLogs(db *DB, jobID int64, status string, limit int) error {
	if limit <= 0 {
		return nil
	}

	_, err := db.Exec(`
		DELETE FROM cron_job_logs
		WHERE run_id IN (
			SELECT run_id
			FROM cron_job_logs
			WHERE job_id = ? AND status = ?
			ORDER BY created_at DESC
			LIMIT -1 OFFSET ?
		)
	`, jobID, status, limit)

	return err
}

var DefaultCronJobLogLimit = 100 // 默认最大条数，可以放到 settings 中

// ListCronJobLogs 返回指定 job 的最近日志
// status: "success" / "error"
// limit: 最大返回条数
func ListCronJobLogs(db *DB, jobID int64) ([]*CronJobLog, error) {
	rows, err := db.Query(`
		SELECT run_id, job_id, status, message, duration, created_at, expire_at
		FROM cron_job_logs
		WHERE job_id = ?
		ORDER BY created_at DESC
		LIMIT ?
	`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var logs []*CronJobLog
	for rows.Next() {
		var l CronJobLog
		var expireAt sql.NullInt64
		if err := rows.Scan(
			&l.RunID,
			&l.JobID,
			&l.Status,
			&l.Message,
			&l.Duration,
			&l.CreatedAt,
			&expireAt,
		); err != nil {
			return nil, err
		}
		if expireAt.Valid {
			l.ExpireAt = &expireAt.Int64
		}
		logs = append(logs, &l)
	}
	return logs, nil
}

var ErrNotFound = errors.New("not found")
