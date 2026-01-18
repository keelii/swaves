package db

import (
	"database/sql"
	"errors"
	"log"
	db2 "swaves/db"
	"swaves/internal/types"
	"sync/atomic"
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

type TableName string
type TableOp string

var OnDatabaseChanged func(tableName TableName, kind TableOp)

const (
	TableOpInsert TableOp = "insert"
	TableOpUpdate TableOp = "update"
	TableOpDelete TableOp = "delete"
)
const (
	TablePosts          TableName = "posts"
	TableEncryptedPosts TableName = "encrypted_posts"
	TableTags           TableName = "tags"
	TableRedirects      TableName = "redirects"
	TableSettings       TableName = "settings"
	TableTasks          TableName = "tasks"
	TableCategories     TableName = "categories"
)

func Open(opts Options) *DB {
	var sqlDB *sql.DB
	var err error

	sqlDB, err = sql.Open("sqlite3", opts.DSN)
	if err != nil {
		log.Fatalf("open sqlite failed: %v", err)
	}

	_, err = sqlDB.Exec(`PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`)
	if err != nil {
		log.Fatalf("set journal_mode failed: %v", err)
	}

	conn := &DB{DB: sqlDB}

	if r2 := Migrate(conn); r2 != nil {
		log.Fatalf("migrate failed: %v", r2)
	}

	return conn
}

const InitialSQL = `
CREATE TABLE IF NOT EXISTS posts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	content TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);

CREATE TABLE IF NOT EXISTS encrypted_posts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	title TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	content TEXT NOT NULL,
	password TEXT NOT NULL,
	expires_at INTEGER,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);

CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,

    parent_id INTEGER,                -- 父分类，NULL 表示顶级分类
    slug TEXT NOT NULL DEFAULT '',    -- 访问路径

    name TEXT NOT NULL,                -- 展示名称
    description TEXT NOT NULL DEFAULT '',

    sort INTEGER NOT NULL DEFAULT 0,   -- 同级排序
    enabled INTEGER NOT NULL DEFAULT 1, -- 1=启用 0=禁用

    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    deleted_at INTEGER
);
CREATE TABLE IF NOT EXISTS post_categories (
	id INTEGER PRIMARY KEY AUTOINCREMENT,

	post_id INTEGER NOT NULL,      -- posts.id
	category_id INTEGER NOT NULL,  -- categories.id
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);

CREATE TABLE IF NOT EXISTS tags (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);

CREATE TABLE IF NOT EXISTS post_tags (
	post_id INTEGER NOT NULL,
	tag_id INTEGER NOT NULL,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER,
	UNIQUE(post_id, tag_id)
);

CREATE TABLE IF NOT EXISTS redirects (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	from_path TEXT NOT NULL UNIQUE,
	to_path TEXT NOT NULL,
	status INTEGER NOT NULL DEFAULT 301,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);

CREATE TABLE IF NOT EXISTS tasks (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	code TEXT NOT NULL UNIQUE, --任务唯一标识，必须唯一
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	schedule TEXT NOT NULL, -- cron 表达式，如 "0 */5 * * *"
	enabled INTEGER NOT NULL DEFAULT 1,
	last_run_at INTEGER,
	last_status TEXT, -- 最后一次执行状态: "pending", "success", "error"
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);
CREATE TABLE IF NOT EXISTS task_runs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	task_code TEXT NOT NULL, -- 对应 tasks.code
	run_id TEXT NOT NULL, -- 本次执行唯一标识 UUID
	status TEXT NOT NULL, -- "pending", "success" 或 "error"
	message TEXT NOT NULL DEFAULT '',
	started_at INTEGER NOT NULL,
	finished_at INTEGER NOT NULL,
	duration INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS settings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,

	kind TEXT NOT NULL DEFAULT 'default',
	name TEXT NOT NULL,
	code TEXT NOT NULL UNIQUE,
	type TEXT NOT NULL,
	options TEXT,
	attrs TEXT,
	value TEXT,
	default_option_value TEXT,
	description TEXT,
	sort INTEGER NOT NULL DEFAULT 0,
	charset TEXT,
	author TEXT,
	keywords TEXT,

	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL,
	deleted_at INTEGER
);

CREATE TABLE IF NOT EXISTS http_error_logs (
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
`

func Migrate(db *DB) error {
	stmts := []string{InitialSQL}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	if err := EnsureDefaultSettings(db); err != nil {
		log.Fatalf("ensure default settings failed: %v", err)
	}

	return nil
}

func now() int64 {
	return time.Now().Unix()
}

// ============================================================================
// 通用基础函数（按 CRUD 顺序排列）
// ============================================================================

// TableConfig 表配置，用于通用查询
type TableConfig struct {
	TableName      string                               // 表名
	SelectFields   string                               // SELECT 字段列表
	IDField        string                               // ID 字段名，默认为 "id"
	DeletedAtField string                               // deleted_at 字段名，默认为 "deleted_at"
	DefaultOrderBy string                               // 默认排序，如 "created_at DESC"
	ScanFunc       func(*sql.Rows) (interface{}, error) // 扫描函数
}

// ============================================================================
// Read 操作（查询）
// ============================================================================

// GetRecordByID 根据 ID 获取记录（通用）
// selectFields: SELECT 字段列表，如 "id, name, slug, created_at, updated_at, deleted_at"
// scanFunc: 扫描函数，将 rows.Scan 的结果转换为具体类型
func GetRecordByID(db *DB, tableName TableName, selectFields string, id int64, scanFunc func(*sql.Row) (interface{}, error)) (interface{}, error) {
	row := db.QueryRow(
		`SELECT `+selectFields+` FROM `+string(tableName)+` WHERE id=? AND deleted_at IS NULL`,
		id,
	)
	return scanFunc(row)
}

// ListRecords 列出记录（通用，支持分页）
// selectFields: SELECT 字段列表
// whereClause: WHERE 子句，如 "status=?" 或 ""（空字符串表示无条件）
// whereArgs: WHERE 参数
// orderBy: ORDER BY 子句，如 "created_at DESC" 或 ""（使用默认排序）
// limit, offset: 分页参数
// scanFunc: 扫描函数，将 rows.Scan 的结果转换为具体类型
func ListRecords(db *DB, tableName TableName, selectFields, whereClause, orderBy string, whereArgs []interface{}, limit, offset int, scanFunc func(*sql.Rows) (interface{}, error)) ([]interface{}, error) {
	query := `SELECT ` + selectFields + ` FROM ` + string(tableName)
	args := []interface{}{}

	if whereClause != "" {
		query += ` WHERE ` + whereClause
		args = append(args, whereArgs...)
	} else {
		query += ` WHERE deleted_at IS NULL`
	}

	if orderBy != "" {
		query += ` ORDER BY ` + orderBy
	} else {
		query += ` ORDER BY created_at DESC`
	}

	if limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		args = append(args, limit, offset)
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []interface{}
	for rows.Next() {
		item, err := scanFunc(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, item)
	}
	return res, nil
}

// ListDeletedRecords 列出已软删除的记录（通用）
func ListDeletedRecords(db *DB, tableName TableName, selectFields, orderBy string, scanFunc func(*sql.Rows) (interface{}, error)) ([]interface{}, error) {
	query := `SELECT ` + selectFields + ` FROM ` + string(tableName) + ` WHERE deleted_at IS NOT NULL`
	if orderBy != "" {
		query += ` ORDER BY ` + orderBy
	} else {
		query += ` ORDER BY deleted_at DESC`
	}

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var res []interface{}
	for rows.Next() {
		item, err := scanFunc(rows)
		if err != nil {
			return nil, err
		}
		res = append(res, item)
	}
	return res, nil
}

// CountRecords 统计记录数（通用）
func CountRecords(db *DB, tableName TableName, whereClause string, whereArgs []interface{}) (int, error) {
	query := `SELECT COUNT(*) FROM ` + string(tableName)
	args := []interface{}{}

	if whereClause != "" {
		query += ` WHERE ` + whereClause
		args = append(args, whereArgs...)
	} else {
		query += ` WHERE deleted_at IS NULL`
	}

	var count int
	err := db.QueryRow(query, args...).Scan(&count)
	return count, err
}

// ============================================================================
// Update 操作（更新）
// ============================================================================

// RestoreRecord 恢复软删除的记录（通用）
func RestoreRecord(db *DB, tableName TableName, id int64) error {
	_, err := db.Exec(
		`UPDATE `+string(tableName)+` SET deleted_at=NULL WHERE id=? AND deleted_at IS NOT NULL`,
		id,
	)
	return err
}

// ============================================================================
// Delete 操作（删除）
// ============================================================================

// SoftDeleteRecord 软删除记录（通用）
func SoftDeleteRecord(db *DB, tableName TableName, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(tableName)+` SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		ts, id,
	)
	return err
}

// HardDeleteRecord 硬删除记录（通用）
func HardDeleteRecord(db *DB, tableName TableName, id int64) error {
	_, err := db.Exec(
		`DELETE FROM `+string(tableName)+` WHERE id=?`,
		id,
	)
	return err
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

type PostWithTags struct {
	Post     *Post
	Tags     []Tag
	Category *Category
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
	result, err := GetRecordByID(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
		var p Post
		var deletedAt sql.NullInt64
		if err := row.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status,
			&p.CreatedAt, &p.UpdatedAt, &deletedAt,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}
		return &p, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*Post), nil
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

func ListPosts(db *DB, pager *types.Pagination) ([]PostWithTags, error) {
	// 先查询总数
	var total int
	row := db.QueryRow(`SELECT COUNT(*) FROM posts WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := db.Query(`
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
		var p Post
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID,
			&p.Title,
			&p.Slug,
			&p.Content,
			&p.Status,
			&p.CreatedAt,
			&p.UpdatedAt,
			&deletedAt,
		); err != nil {
			return nil, err
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}

		// 获取该 post 的 tags
		tags, err := GetPostTags(db, p.ID)
		if err != nil {
			return nil, err
		}

		// 获取该 post 的 category（单选）
		category, err := GetPostCategory(db, p.ID)
		if err != nil {
			return nil, err
		}

		res = append(res, PostWithTags{
			Post:     &p,
			Tags:     tags,
			Category: category,
		})
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}

func SoftDeletePost(db *DB, id int64) error {
	return SoftDeleteRecord(db, TablePosts, id)
}

func RestorePost(db *DB, id int64) error {
	return RestoreRecord(db, TablePosts, id)
}

func ListDeletedPosts(db *DB) ([]Post, error) {
	results, err := ListDeletedRecords(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
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
		return p, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]Post, len(results))
	for i, v := range results {
		res[i] = v.(Post)
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
	result, err := GetRecordByID(db, TableEncryptedPosts, "id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
		var p EncryptedPost
		var deletedAt sql.NullInt64
		var expiresAt sql.NullInt64
		var encryptedContent string
		if err := row.Scan(
			&p.ID, &p.Title, &p.Slug, &encryptedContent, &p.Password,
			&expiresAt, &p.CreatedAt, &p.UpdatedAt, &deletedAt,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if expiresAt.Valid {
			p.ExpiresAt = &expiresAt.Int64
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}

		// 解密 content（使用系统密钥，不依赖 password 字段）
		decryptedContent, err := DecryptContent(encryptedContent)
		if err != nil {
			return nil, err
		}
		p.Content = decryptedContent

		return &p, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*EncryptedPost), nil
}

func UpdateEncryptedPost(db *DB, p *EncryptedPost) error {
	p.UpdatedAt = now()

	// 加密 content（使用系统密钥，不依赖 password 字段）
	encryptedContent, err := EncryptContent(p.Content)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`UPDATE encrypted_posts
		 SET title=?, content=?, password=?, expires_at=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		p.Title, encryptedContent, p.Password, p.ExpiresAt, p.UpdatedAt, p.ID,
	)
	return err
}

func SoftDeleteEncryptedPost(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableEncryptedPosts, id)
}

func RestoreEncryptedPost(db *DB, id int64) error {
	return RestoreRecord(db, TableEncryptedPosts, id)
}

func ListDeletedEncryptedPosts(db *DB) ([]EncryptedPost, error) {
	results, err := ListDeletedRecords(db, TableEncryptedPosts, "id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
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
		return p, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]EncryptedPost, len(results))
	for i, v := range results {
		res[i] = v.(EncryptedPost)
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

	// 加密 content（使用系统密钥，不依赖 password 字段）
	encryptedContent, err := EncryptContent(p.Content)
	if err != nil {
		return err
	}

	res, err := db.Exec(
		`INSERT INTO encrypted_posts
		 (title, slug, content, password, expires_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		p.Title, p.Slug, encryptedContent, p.Password, p.ExpiresAt, p.CreatedAt, p.UpdatedAt,
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
	result, err := GetRecordByID(db, TableTags, "id, name, slug, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
		var t Tag
		var deletedAt sql.NullInt64
		if err := row.Scan(
			&t.ID, &t.Name, &t.Slug,
			&t.CreatedAt, &t.UpdatedAt, &deletedAt,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if deletedAt.Valid {
			t.DeletedAt = &deletedAt.Int64
		}
		return &t, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*Tag), nil
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
	return SoftDeleteRecord(db, TableTags, id)
}

func RestoreTag(db *DB, id int64) error {
	return RestoreRecord(db, TableTags, id)
}

func ListDeletedTags(db *DB) ([]Tag, error) {
	results, err := ListDeletedRecords(db, TableTags, "id, name, slug, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
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
		return t, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]Tag, len(results))
	for i, v := range results {
		res[i] = v.(Tag)
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
	// 先尝试恢复已存在的软删除关联
	res, err := db.Exec(
		`UPDATE post_tags 
		 SET deleted_at=NULL, updated_at=?
		 WHERE post_id=? AND tag_id=? AND deleted_at IS NOT NULL`,
		ts, postID, tagID,
	)
	if err != nil {
		return err
	}
	// 如果更新了记录，说明恢复了软删除的关联，直接返回
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected > 0 {
		return nil
	}
	// 如果没有已存在的记录（包括软删除的），则插入新记录
	_, err = db.Exec(
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
	Status    int
	Enabled   int
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func GetRedirectByID(db *DB, id int64) (*Redirect, error) {
	row := db.QueryRow(
		`SELECT id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at
		 FROM redirects WHERE id=? AND deleted_at IS NULL`,
		id,
	)

	var r Redirect
	var deletedAt sql.NullInt64
	var status sql.NullInt64
	var enabled sql.NullInt64
	if err := row.Scan(
		&r.ID, &r.From, &r.To, &status, &enabled,
		&r.CreatedAt, &r.UpdatedAt, &deletedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status.Valid {
		r.Status = int(status.Int64)
	} else {
		r.Status = 301 // default
	}
	if enabled.Valid {
		r.Enabled = int(enabled.Int64)
	} else {
		r.Enabled = 1 // default
	}
	if deletedAt.Valid {
		r.DeletedAt = &deletedAt.Int64
	}
	return &r, nil
}

// GetRedirectByFrom 根据 from_path 路径查找 redirect
func GetRedirectByFrom(db *DB, fromPath string) (*Redirect, error) {
	row := db.QueryRow(
		`SELECT id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at
		 FROM redirects WHERE from_path=? AND deleted_at IS NULL`,
		fromPath,
	)

	var r Redirect
	var deletedAt sql.NullInt64
	var status sql.NullInt64
	var enabled sql.NullInt64
	if err := row.Scan(
		&r.ID, &r.From, &r.To, &status, &enabled,
		&r.CreatedAt, &r.UpdatedAt, &deletedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if status.Valid {
		r.Status = int(status.Int64)
	} else {
		r.Status = 301 // default
	}
	if enabled.Valid {
		r.Enabled = int(enabled.Int64)
	} else {
		r.Enabled = 1 // default
	}
	if deletedAt.Valid {
		r.DeletedAt = &deletedAt.Int64
	}
	return &r, nil
}

func UpdateRedirect(db *DB, r *Redirect) error {
	r.UpdatedAt = now()
	if r.Status == 0 {
		r.Status = 301 // default
	}
	_, err := db.Exec(
		`UPDATE redirects
		 SET from_path=?, to_path=?, status=?, enabled=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		r.From, r.To, r.Status, r.Enabled, r.UpdatedAt, r.ID,
	)
	return err
}

func SoftDeleteRedirect(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableRedirects, id)
}

func RestoreRedirect(db *DB, id int64) error {
	return RestoreRecord(db, TableRedirects, id)
}

func ListRedirects(db *DB, limit, offset int) ([]Redirect, int, error) {
	// 获取总数
	var total int
	row := db.QueryRow(`SELECT COUNT(*) FROM redirects WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, 0, err
	}

	// 查询列表
	rows, err := db.Query(`
		SELECT id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at
		FROM redirects
		WHERE deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var res []Redirect
	for rows.Next() {
		var r Redirect
		var deletedAt sql.NullInt64
		var status sql.NullInt64
		var enabled sql.NullInt64
		if err := rows.Scan(
			&r.ID, &r.From, &r.To, &status, &enabled,
			&r.CreatedAt, &r.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, 0, err
		}
		if status.Valid {
			r.Status = int(status.Int64)
		} else {
			r.Status = 301 // default
		}
		if enabled.Valid {
			r.Enabled = int(enabled.Int64)
		} else {
			r.Enabled = 1 // default
		}
		if deletedAt.Valid {
			r.DeletedAt = &deletedAt.Int64
		}
		res = append(res, r)
	}
	return res, total, nil
}

func ListDeletedRedirects(db *DB) ([]Redirect, error) {
	results, err := ListDeletedRecords(db, TableRedirects, "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		var r Redirect
		var deletedAt sql.NullInt64
		var status sql.NullInt64
		var enabled sql.NullInt64
		if err := rows.Scan(
			&r.ID, &r.From, &r.To, &status, &enabled,
			&r.CreatedAt, &r.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		if status.Valid {
			r.Status = int(status.Int64)
		} else {
			r.Status = 301 // default
		}
		if enabled.Valid {
			r.Enabled = int(enabled.Int64)
		} else {
			r.Enabled = 1 // default
		}
		if deletedAt.Valid {
			r.DeletedAt = &deletedAt.Int64
		}
		return r, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]Redirect, len(results))
	for i, v := range results {
		res[i] = v.(Redirect)
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
	if r.Status == 0 {
		r.Status = 301 // default
	}
	if r.Enabled == 0 {
		r.Enabled = 1 // default
	}

	res, err := db.Exec(
		`INSERT INTO redirects (from_path, to_path, status, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		r.From, r.To, r.Status, r.Enabled, r.CreatedAt, r.UpdatedAt,
	)
	if err != nil {
		return err
	}
	r.ID, _ = res.LastInsertId()
	return nil
}

type Setting struct {
	ID                 int64
	Kind               string
	Name               string
	Code               string
	Type               string
	Options            string // JSON string
	Attrs              string // JSON string
	Value              string
	DefaultOptionValue string // Default value for select/radio/checkbox when options are provided
	Description        string
	Sort               int64
	Charset            string
	Author             string
	Keywords           string
	CreatedAt          int64
	UpdatedAt          int64
	DeletedAt          *int64
}

func CreateSetting(db *DB, s *Setting) error {
	if s.Code == "" {
		return errors.New("code is required")
	}
	if s.Type == "" {
		return errors.New("type is required")
	}
	if s.Kind == "" {
		s.Kind = "default"
	}

	if s.CreatedAt == 0 {
		s.CreatedAt = now()
	}
	if s.UpdatedAt == 0 {
		s.UpdatedAt = s.CreatedAt
	}

	// 如果是 password 类型，需要对 value 进行 bcrypt 加密
	if s.Type == "password" && s.Value != "" && len(s.Value) < 60 {
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(s.Value),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return err
		}
		s.Value = string(hashed)
	}

	res, err := db.Exec(
		`INSERT INTO settings
		 (kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Kind,
		s.Name,
		s.Code,
		s.Type,
		s.Options,
		s.Attrs,
		s.Value,
		s.DefaultOptionValue,
		s.Description,
		s.Sort,
		s.Charset,
		s.Author,
		s.Keywords,
		s.CreatedAt,
		s.UpdatedAt,
	)
	if err != nil {
		return err
	}

	s.ID, _ = res.LastInsertId()
	return nil
}

func GetSettingByCode(db *DB, code string) (*Setting, error) {
	row := db.QueryRow(`
		SELECT id, kind, name, code, type, options, attrs, value, default_option_value, description, sort,
		       charset, author, keywords, created_at, updated_at, deleted_at
		FROM settings
		WHERE code=? AND deleted_at IS NULL
	`, code)

	var s Setting
	var deletedAt sql.NullInt64

	err := row.Scan(
		&s.ID,
		&s.Kind,
		&s.Name,
		&s.Code,
		&s.Type,
		&s.Options,
		&s.Attrs,
		&s.Value,
		&s.DefaultOptionValue,
		&s.Description,
		&s.Sort,
		&s.Charset,
		&s.Author,
		&s.Keywords,
		&s.CreatedAt,
		&s.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if deletedAt.Valid {
		s.DeletedAt = &deletedAt.Int64
	}
	return &s, nil
}

func GetSettingByID(db *DB, id int64) (*Setting, error) {
	row := db.QueryRow(`
		SELECT id, kind, name, code, type, options, attrs, value, default_option_value, description, sort,
		       charset, author, keywords, created_at, updated_at, deleted_at
		FROM settings
		WHERE id=? AND deleted_at IS NULL
	`, id)

	var s Setting
	var deletedAt sql.NullInt64

	err := row.Scan(
		&s.ID,
		&s.Kind,
		&s.Name,
		&s.Code,
		&s.Type,
		&s.Options,
		&s.Attrs,
		&s.Value,
		&s.DefaultOptionValue,
		&s.Description,
		&s.Sort,
		&s.Charset,
		&s.Author,
		&s.Keywords,
		&s.CreatedAt,
		&s.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if deletedAt.Valid {
		s.DeletedAt = &deletedAt.Int64
	}
	return &s, nil
}

func ListSettingsByKind(db *DB, kind string) ([]Setting, error) {
	query := `
		SELECT id, kind, name, code, type, options, attrs, value, default_option_value, description, sort,
		       charset, author, keywords, created_at, updated_at, deleted_at
		FROM settings
		WHERE deleted_at IS NULL
	`
	args := []interface{}{}

	if kind != "" {
		query += ` AND kind=?`
		args = append(args, kind)
	}

	//query += ` ORDER BY kind, sort, id`

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Setting
	for rows.Next() {
		var s Setting
		var deletedAt sql.NullInt64

		if err := rows.Scan(
			&s.ID,
			&s.Kind,
			&s.Name,
			&s.Code,
			&s.Type,
			&s.Options,
			&s.Attrs,
			&s.Value,
			&s.DefaultOptionValue,
			&s.Description,
			&s.Sort,
			&s.Charset,
			&s.Author,
			&s.Keywords,
			&s.CreatedAt,
			&s.UpdatedAt,
			&deletedAt,
		); err != nil {
			return nil, err
		}

		if deletedAt.Valid {
			s.DeletedAt = &deletedAt.Int64
		}
		settings = append(settings, s)
	}

	return settings, nil
}

func ListAllSettings(db *DB) ([]Setting, error) {
	return ListSettingsByKind(db, "")
}

func UpdateSetting(db *DB, s *Setting) error {
	s.UpdatedAt = now()

	// 如果是 password 类型，需要对 value 进行 bcrypt 加密
	if s.Type == "password" && s.Value != "" && len(s.Value) < 60 {
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(s.Value),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return err
		}
		s.Value = string(hashed)
	}

	_, err := db.Exec(
		`UPDATE settings
		 SET kind=?, name=?, type=?, options=?, attrs=?, value=?, default_option_value=?, description=?, sort=?, charset=?, author=?, keywords=?, updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		s.Kind,
		s.Name,
		s.Type,
		s.Options,
		s.Attrs,
		s.Value,
		s.DefaultOptionValue,
		s.Description,
		s.Sort,
		s.Charset,
		s.Author,
		s.Keywords,
		s.UpdatedAt,
		s.ID,
	)

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpUpdate)
	}

	return err
}

func UpdateSettingByCode(db *DB, code string, value string) error {
	// 获取原有设置
	setting, err := GetSettingByCode(db, code)
	if err != nil {
		return err
	}

	// 如果是 password 类型，需要对 value 进行 bcrypt 加密
	if setting.Type == "password" && value != "" && len(value) < 60 {
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(value),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return err
		}
		value = string(hashed)
	}

	_, err = db.Exec(
		`UPDATE settings
		 SET value=?, updated_at=?
		 WHERE code=? AND deleted_at IS NULL`,
		value,
		now(),
		code,
	)

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpUpdate)
	}

	return err
}

func DeleteSetting(db *DB, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE settings SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		ts, id,
	)

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpUpdate)
	}

	return err
}

// CheckPassword 检查管理员密码
func CheckPassword(db *DB, raw string) error {
	setting, err := GetSettingByCode(db, "admin_password")
	if err != nil {
		return err
	}
	if setting.Value == "" {
		return ErrNotFound
	}

	return bcrypt.CompareHashAndPassword([]byte(setting.Value), []byte(raw))
}

// EnsureDefaultSettings 确保存在默认配置项
func EnsureDefaultSettings(db *DB) error {
	defaultSettings := []Setting{
		{Sort: 2, Kind: "General", Name: "Site Name", Code: "site_name", Type: "text", Value: "swaves", Description: "站点名称"},
		{Sort: 4, Kind: "General", Name: "Author", Code: "author", Type: "text", Value: "keelii", Description: "作者"},
		{Sort: 5, Kind: "General", Name: "Keywords", Code: "keyword", Type: "text", Value: "", Description: "关键字"},
		{Sort: 6, Kind: "General", Name: "Language", Code: "language", Type: "select", Value: "zh-CN", Description: "语言", Options: db2.InternalLang},
		{Sort: 7, Kind: "General", Name: "Charset", Code: "charset", Type: "text", Value: "utf-8", Description: "编码", Options: db2.InternalLang},
		{Sort: 9, Kind: "General", Name: "Timezone", Code: "timezone", Type: "select", Value: "Asia/Shanghai", Description: "时区", Options: db2.InternalTimezone},
		{Sort: 11, Kind: "General", Name: "Admin Password", Code: "admin_password", Type: "password", Value: "admin", Description: "管理员密码", Attrs: `{"minlength": 6}`},
		{Sort: 11, Kind: "Appearance", Name: "Font size", Code: "font_size", Type: "range", Value: "14", Description: "UI font size", Attrs: `{"min": 12, "max": 20, "step": 2}`},
		{Sort: 11, Kind: "Appearance", Name: "Mode", Code: "mode", Type: "radio", Value: "light", Description: "UI mode", DefaultOptionValue: "light", Options: `[{"label": "Light", "value": "light"}, {"label": "Dark", "value": "dark"}]`},
		{Sort: 11, Kind: "Appearance", Name: "Admin main width", Code: "admin_main_width", Type: "number", Value: "950", DefaultOptionValue: "950", Description: "Admin UI main width"},
		{Sort: 11, Kind: "Appearance", Name: "Page size", Code: "page_size", Type: "number", Value: "10", DefaultOptionValue: "10", Description: "每页显示的文章数量", Attrs: `{"min": 1, "max": 100}`},
		{Sort: 13, Kind: "Post", Name: "Post Slug Pattern", Code: "post_slug_pattern", Type: "text", Value: "/{yyyy}/{MM}/{dd}/{name}", Description: "文章 URL 模式"},
		{Sort: 15, Kind: "Post", Name: "Tag Slug Pattern", Code: "tag_slug_pattern", Type: "text", Value: "/tags/{name}", Description: "标签 URL 模式"},
		{Sort: 17, Kind: "Post", Name: "Tags Pattern", Code: "tags_pattern", Type: "text", Value: "/tags", Description: "标签列表 URL 模式"},
		{Sort: 19, Kind: "ThirdPart", Name: "GA4 ID", Code: "ga4_id", Type: "text", Value: "", Description: "Google Analytics 4 ID"},
		{Sort: 21, Kind: "ThirdPart", Name: "Giscus Config", Code: "giscus_config", Type: "textarea", Value: "", Description: "Giscus 配置 (JSON)"},
	}

	for _, s := range defaultSettings {
		// 检查是否已存在
		_, err := GetSettingByCode(db, s.Code)
		if err == nil {
			// 已存在，跳过
			continue
		}
		if err != ErrNotFound {
			// 其他错误，返回
			return err
		}

		// 不存在，创建
		if err := CreateSetting(db, &s); err != nil {
			return err
		}
	}

	return nil
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

var AppSettings atomic.Value // map[string]string

// LoadSettingsToMap 从 settings 表加载 code -> value 映射
func LoadSettingsToMap(db *DB) (map[string]string, error) {
	rows, err := db.Query(`
		SELECT code, value FROM settings WHERE deleted_at IS NULL
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	settingsMap := make(map[string]string)
	for rows.Next() {
		var code, value string
		if err := rows.Scan(&code, &value); err != nil {
			return nil, err
		}
		settingsMap[code] = value
	}

	// 检查遍历是否有错误
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return settingsMap, nil
}

type Task struct {
	ID          int64
	Code        string
	Name        string
	Description string
	Schedule    string
	Enabled     int
	LastRunAt   *int64
	LastStatus  string
	CreatedAt   int64
	UpdatedAt   int64
	DeletedAt   *int64
}

type TaskRun struct {
	ID         int64
	TaskCode   string
	RunID      string
	Status     string
	Message    string
	StartedAt  int64
	FinishedAt int64
	Duration   int64
	CreatedAt  int64
}

func CreateTask(db *DB, task *Task) error {
	now := time.Now().Unix()
	task.CreatedAt = now
	task.UpdatedAt = now
	res, err := db.Exec(`INSERT INTO tasks
		(code, name, description, schedule, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		task.Code, task.Name, task.Description, task.Schedule, task.Enabled, task.CreatedAt, task.UpdatedAt,
	)
	if err != nil {
		return err
	}
	task.ID, _ = res.LastInsertId()
	return nil
}

func GetTaskByID(db *DB, id int64) (*Task, error) {
	row := db.QueryRow(`SELECT id, code, name, description, schedule, enabled,
		last_run_at, last_status, created_at, updated_at, deleted_at
		FROM tasks WHERE id=? AND deleted_at IS NULL`, id)

	var t Task
	var lastRun sql.NullInt64
	var lastStatus sql.NullString
	var deleted sql.NullInt64

	if err := row.Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled,
		&lastRun, &lastStatus, &t.CreatedAt, &t.UpdatedAt, &deleted,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if lastRun.Valid {
		t.LastRunAt = &lastRun.Int64
	}
	if lastStatus.Valid {
		t.LastStatus = lastStatus.String
	}
	if deleted.Valid {
		t.DeletedAt = &deleted.Int64
	}

	return &t, nil
}

func GetTaskByCode(db *DB, code string) (*Task, error) {
	row := db.QueryRow(`SELECT id, code, name, description, schedule, enabled,
		last_run_at, last_status, created_at, updated_at, deleted_at
		FROM tasks WHERE code=? AND deleted_at IS NULL`, code)

	var t Task
	var lastRun sql.NullInt64
	var lastStatus sql.NullString
	var deleted sql.NullInt64

	if err := row.Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled,
		&lastRun, &lastStatus, &t.CreatedAt, &t.UpdatedAt, &deleted,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, err
	}

	if lastRun.Valid {
		t.LastRunAt = &lastRun.Int64
	}
	if lastStatus.Valid {
		t.LastStatus = lastStatus.String
	}
	if deleted.Valid {
		t.DeletedAt = &deleted.Int64
	}

	return &t, nil
}

func ListTasks(db *DB) ([]Task, error) {
	rows, err := db.Query(`SELECT id, code, name, description, schedule, enabled,
		last_run_at, last_status, created_at, updated_at, deleted_at
		FROM tasks WHERE deleted_at IS NULL ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var t Task
		var lastRun sql.NullInt64
		var lastStatus sql.NullString
		var deleted sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled,
			&lastRun, &lastStatus, &t.CreatedAt, &t.UpdatedAt, &deleted,
		); err != nil {
			return nil, err
		}
		if lastRun.Valid {
			t.LastRunAt = &lastRun.Int64
		}
		if lastStatus.Valid {
			t.LastStatus = lastStatus.String
		}
		if deleted.Valid {
			t.DeletedAt = &deleted.Int64
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func UpdateTask(db *DB, task *Task) error {
	task.UpdatedAt = now()
	enabled := 0
	if task.Enabled == 1 {
		enabled = 1
	}
	// Code 不可修改，不更新 code 字段
	_, err := db.Exec(`UPDATE tasks
		SET name=?, description=?, schedule=?, enabled=?, updated_at=?
		WHERE id=? AND deleted_at IS NULL`,
		task.Name, task.Description, task.Schedule, enabled, task.UpdatedAt, task.ID,
	)
	return err
}
func UpdateTaskStatus(db *DB, taskCode string, lastStatus string, lastRunAt int64) error {
	_, err := db.Exec(`UPDATE tasks
		SET last_status=?, last_run_at=?
		WHERE code=? AND deleted_at IS NULL`,
		lastStatus, lastRunAt, taskCode,
	)
	return err
}

func SoftDeleteTask(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableTasks, id)
}

func CreateTaskRun(db *DB, run *TaskRun) error {
	now := time.Now().Unix()
	run.RunID = uuid.NewString()
	run.CreatedAt = now

	if run.StartedAt == 0 {
		run.StartedAt = now
	}
	if run.FinishedAt == 0 {
		run.FinishedAt = now
	}
	if run.Duration == 0 {
		run.Duration = 0
	}

	res, err := db.Exec(`INSERT INTO task_runs
		(task_code, run_id, status, message, started_at, finished_at, duration, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		run.TaskCode, run.RunID, run.Status, run.Message,
		run.StartedAt, run.FinishedAt, run.Duration, run.CreatedAt,
	)
	if err != nil {
		return err
	}
	run.ID, _ = res.LastInsertId()

	// 同步更新 tasks 的 last_run_at 和 last_status 字段
	_, _ = db.Exec(`UPDATE tasks
		SET last_run_at=?, last_status=?, updated_at=?
		WHERE code=? AND deleted_at IS NULL`,
		run.StartedAt, run.Status, now, run.TaskCode,
	)
	return nil
}

func ListTaskRuns(db *DB, taskCode string, status string, limit int) ([]TaskRun, error) {
	query := `
        SELECT id, task_code, run_id, status, message, started_at, finished_at, duration, created_at
        FROM task_runs WHERE 1=1`

	args := []interface{}{}

	if taskCode != "" {
		query += ` AND task_code = ?`
		args = append(args, taskCode)
	}

	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC LIMIT ?"
	args = append(args, limit)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []TaskRun
	for rows.Next() {
		var r TaskRun
		if err := rows.Scan(
			&r.ID, &r.TaskCode, &r.RunID, &r.Status, &r.Message,
			&r.StartedAt, &r.FinishedAt, &r.Duration, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		runs = append(runs, r)
	}
	return runs, nil
}
func UpdateTaskRunStatus(db *DB, run *TaskRun) error {
	if run.ID == 0 {
		return errors.New("run.ID is zero")
	}

	// 更新 task_runs 表
	_, err := db.Exec(`
		UPDATE task_runs
		SET status=?, message=?, finished_at=?, duration=?
		WHERE id=?
	`, run.Status, run.Message, run.FinishedAt, run.Duration, run.ID)
	if err != nil {
		return err
	}

	// 同步更新 tasks 表的 last_run_at 和 last_status 字段
	_, _ = db.Exec(`
		UPDATE tasks
		SET last_run_at=?, last_status=?, updated_at=?
		WHERE code=? AND deleted_at IS NULL
	`, run.StartedAt, run.Status, now(), run.TaskCode)
	return nil
}

var ErrNotFound = errors.New("not found")

type Category struct {
	ID          int64
	ParentID    int64 // 0表示顶级分类
	Name        string
	Slug        string
	Description string
	Sort        int64
	CreatedAt   int64
	UpdatedAt   int64
	DeletedAt   *int64
}

func GetCategoryByID(db *DB, id int64) (*Category, error) {
	row := db.QueryRow(`
		SELECT id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at
		FROM categories
		WHERE id=? AND deleted_at IS NULL
	`, id)

	var c Category
	var parentID sql.NullInt64
	var deletedAt sql.NullInt64

	err := row.Scan(
		&c.ID,
		&parentID,
		&c.Name,
		&c.Slug,
		&c.Description,
		&c.Sort,
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

	if parentID.Valid {
		c.ParentID = parentID.Int64
	}
	if deletedAt.Valid {
		c.DeletedAt = &deletedAt.Int64
	}

	return &c, nil
}

func CategoryExists(db *DB, id int64) (bool, error) {
	var cnt int
	err := db.QueryRow(`SELECT COUNT(1) FROM categories WHERE id=? AND deleted_at IS NULL`, id).Scan(&cnt)
	return cnt > 0, err
}

func CreateCategory(db *DB, c *Category) error {
	if c.ParentID != 0 {
		ok, err := CategoryExists(db, c.ParentID)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("parent category not exists")
		}
	}

	// 检查唯一性：同一父级下slug必须唯一（包括已软删除的）
	var existingID int64
	var err error
	if c.ParentID == 0 {
		err = db.QueryRow(`
			SELECT id FROM categories WHERE (parent_id IS NULL OR parent_id=0) AND slug=?
		`, c.Slug).Scan(&existingID)
	} else {
		err = db.QueryRow(`
			SELECT id FROM categories WHERE parent_id=? AND slug=?
		`, c.ParentID, c.Slug).Scan(&existingID)
	}
	if err == nil {
		return errors.New("slug already exists under this parent")
	} else if err != sql.ErrNoRows {
		return err
	}

	if c.CreatedAt == 0 {
		c.CreatedAt = now()
	}
	if c.UpdatedAt == 0 {
		c.UpdatedAt = c.CreatedAt
	}

	var parentID interface{}
	if c.ParentID == 0 {
		parentID = nil
	} else {
		parentID = c.ParentID
	}

	res, err := db.Exec(`
		INSERT INTO categories (parent_id, name, slug, description, sort, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, parentID, c.Name, c.Slug, c.Description, c.Sort, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	c.ID = id

	// 如果 sort 为 0（默认值），则设置为 id
	if c.Sort == 0 {
		c.Sort = id
		_, err = db.Exec(`
			UPDATE categories SET sort=? WHERE id=?
		`, c.Sort, id)
		if err != nil {
			return err
		}
	}
	return nil
}

func ListCategories(db *DB) ([]Category, error) {
	query := `
		SELECT id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at
		FROM categories
		WHERE deleted_at IS NULL
		ORDER BY sort
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Category
	for rows.Next() {
		var c Category
		var parentID sql.NullInt64
		var deletedAt sql.NullInt64

		if err := rows.Scan(
			&c.ID,
			&parentID,
			&c.Name,
			&c.Slug,
			&c.Description,
			&c.Sort,
			&c.CreatedAt,
			&c.UpdatedAt,
			&deletedAt,
		); err != nil {
			return nil, err
		}

		if parentID.Valid {
			c.ParentID = parentID.Int64
		}
		if deletedAt.Valid {
			c.DeletedAt = &deletedAt.Int64
		}

		list = append(list, c)
	}

	return list, nil
}

func UpdateCategory(db *DB, c *Category) error {
	c.UpdatedAt = now()

	// 如果slug或parent_id改变了，需要检查唯一性
	var existingID int64
	var err error
	if c.ParentID == 0 {
		err = db.QueryRow(`
			SELECT id FROM categories WHERE (parent_id IS NULL OR parent_id=0) AND slug=? AND id!=? AND deleted_at IS NULL
		`, c.Slug, c.ID).Scan(&existingID)
	} else {
		err = db.QueryRow(`
			SELECT id FROM categories WHERE parent_id=? AND slug=? AND id!=? AND deleted_at IS NULL
		`, c.ParentID, c.Slug, c.ID).Scan(&existingID)
	}
	if err == nil {
		return errors.New("slug already exists under this parent")
	} else if err != sql.ErrNoRows {
		return err
	}

	var parentID interface{}
	if c.ParentID == 0 {
		parentID = nil
	} else {
		parentID = c.ParentID
	}

	_, err = db.Exec(`
		UPDATE categories
		SET parent_id=?, name=?, slug=?, description=?, sort=?, updated_at=?
		WHERE id=? AND deleted_at IS NULL
	`, parentID, c.Name, c.Slug, c.Description, c.Sort, c.UpdatedAt, c.ID)
	return err
}

func UpdateCategoryParent(db *DB, id int64, newParentID int64) error {
	// 验证新父级是否存在（如果提供了）
	if newParentID != 0 {
		exists, err := CategoryExists(db, newParentID)
		if err != nil {
			return err
		}
		if !exists {
			return errors.New("parent category not exists")
		}
	}

	// 检查是否会造成循环
	all, err := ListCategories(db)
	if err != nil {
		return err
	}

	categoryMap := make(map[int64]*Category)
	for i := range all {
		categoryMap[all[i].ID] = &all[i]
	}

	if newParentID != 0 {
		cur := newParentID
		for cur != 0 {
			if cur == id {
				return errors.New("category cycle detected")
			}
			parent, ok := categoryMap[cur]
			if !ok {
				break
			}
			cur = parent.ParentID
		}
	}

	var parentID interface{}
	if newParentID == 0 {
		parentID = nil
	} else {
		parentID = newParentID
	}

	_, err = db.Exec(`
		UPDATE categories SET parent_id=?, updated_at=? WHERE id=?
	`, parentID, now(), id)
	return err
}

func SoftDeleteCategory(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableCategories, id)
}

func RestoreCategory(db *DB, id int64) error {
	return RestoreRecord(db, TableCategories, id)
}

func GetPostCategory(db *DB, postID int64) (*Category, error) {
	row := db.QueryRow(`
		SELECT c.id, c.parent_id, c.name, c.slug, c.description, c.sort, c.created_at, c.updated_at, c.deleted_at
		FROM categories c
		INNER JOIN post_categories pc ON c.id = pc.category_id
		WHERE pc.post_id = ? AND pc.deleted_at IS NULL AND c.deleted_at IS NULL
		ORDER BY c.name
		LIMIT 1
	`, postID)

	var c Category
	var parentID sql.NullInt64
	var deletedAt sql.NullInt64
	if err := row.Scan(
		&c.ID, &parentID, &c.Name, &c.Slug, &c.Description, &c.Sort,
		&c.CreatedAt, &c.UpdatedAt, &deletedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	if parentID.Valid {
		c.ParentID = parentID.Int64
	}
	if deletedAt.Valid {
		c.DeletedAt = &deletedAt.Int64
	}
	return &c, nil
}

func AttachCategoryToPost(db *DB, postID, categoryID int64) error {
	ts := now()
	_, err := db.Exec(
		`INSERT OR IGNORE INTO post_categories
		 (post_id, category_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		postID, categoryID, ts, ts,
	)
	return err
}

func DetachCategoryFromPost(db *DB, postID, categoryID int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE post_categories SET deleted_at=? WHERE post_id=? AND category_id=? AND deleted_at IS NULL`,
		ts, postID, categoryID,
	)
	return err
}

func SetPostCategory(db *DB, postID int64, categoryID int64) error {
	// 先获取当前关联的分类
	currentCategory, err := GetPostCategory(db, postID)
	if err != nil {
		return err
	}

	// 如果当前分类存在且与新分类不同，则删除旧分类
	if currentCategory != nil && currentCategory.ID != categoryID {
		if err := DetachCategoryFromPost(db, postID, currentCategory.ID); err != nil {
			return err
		}
	}

	// 如果新分类ID不为0，则添加新分类关联
	if categoryID > 0 {
		// 如果当前没有分类，或者当前分类与新分类不同，则添加新分类
		if currentCategory == nil || currentCategory.ID != categoryID {
			if err := AttachCategoryToPost(db, postID, categoryID); err != nil {
				return err
			}
		}
	}

	return nil
}
