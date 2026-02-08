package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/types"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-sqlite3"
	_ "github.com/mattn/go-sqlite3"
	"github.com/simukti/sqldb-logger"
	"golang.org/x/crypto/bcrypt"
)

type DB struct {
	*sql.DB
}

type Options struct {
	DSN string
}

type TableName string

var OnDatabaseChanged func(tableName TableName, kind TableOp)

func Open(opts Options) *DB {
	var sqlDB *sql.DB
	var err error

	sqlDB = sqldblogger.OpenDriver(opts.DSN, &sqlite3.SQLiteDriver{}, &SqlLogger{})

	//sqlDB, err = sql.Open("sqlite3", opts.DSN)
	//if err != nil {
	//	log.Fatalf("open sqlite failed: %v", err)
	//}
	//
	_, err = sqlDB.Exec(`PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`)
	if err != nil {
		log.Fatalf("set journal_mode failed: %v", err)
	}

	conn := &DB{DB: sqlDB}

	if r2 := InitDatabase(conn); r2 != nil {
		log.Fatalf("migrate failed: %v", r2)
	}

	return conn
}

func InitDatabase(db *DB) error {
	// 迁移：为 t_tasks 添加 kind 列（须在 InitialSQL 的 INSERT 前执行，否则旧库 INSERT 会因缺列失败）
	_, _ = db.Exec(`ALTER TABLE ` + string(TableTasks) + ` ADD COLUMN kind INTEGER NOT NULL DEFAULT 0`)
	// 迁移：为 t_posts 添加 kind 列
	_, _ = db.Exec(`ALTER TABLE ` + string(TablePosts) + ` ADD COLUMN kind INTEGER NOT NULL DEFAULT 0`)

	stmts := []string{string(InitialSQL)}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return err
		}
	}

	if err := EnsureDefaultSettings(db); err != nil {
		log.Fatalf("ensure default settings failed: %v", err)
	}

	// FTS5 全文搜索表（首次建库或表为空时创建并重建索引）
	if _, err := db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS ` + string(TablePostsFTS) + ` USING fts5(title, content, tokenize='unicode61')`); err != nil {
		log.Printf("create fts5 table: %v", err)
	} else {
		var n int
		if _ = db.QueryRow(`SELECT COUNT(*) FROM ` + string(TablePostsFTS)).Scan(&n); n == 0 {
			_ = RebuildPostsFTS(db)
		}
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
// Create 操作（创建）
// ============================================================================

// CreateRecord 创建记录（通用）
// tableName: 表名
// data: 字段名和值的映射，字段名应排除 id 和 deleted_at
// 注意：由于 map 迭代顺序不确定，建议使用有序的数据结构或确保字段顺序一致
func CreateRecord(db *DB, tableName TableName, data map[string]interface{}) (int64, error) {
	if db == nil {
		return 0, errors.New("db is nil")
	}
	if tableName == "" {
		return 0, errors.New("tableName is empty")
	}
	if len(data) == 0 {
		return 0, errors.New("no data to insert")
	}

	cols := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data))

	// 保证插入列与参数顺序一致
	for k, v := range data {
		cols = append(cols, k)
		placeholders = append(placeholders, "?")
		args = append(args, v)
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		string(tableName),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	id, _ := res.LastInsertId()
	return id, nil
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

// GetRecordByField 根据指定字段获取记录（通用）
// selectFields: SELECT 字段列表，如 "id, name, slug, created_at, updated_at, deleted_at"
// fieldName: 查询字段名，如 "code", "from_path"
// fieldValue: 查询字段值
// scanFunc: 扫描函数，将 rows.Scan 的结果转换为具体类型
func GetRecordByField(db *DB, tableName TableName, selectFields, fieldName string, fieldValue interface{}, scanFunc func(*sql.Row) (interface{}, error)) (interface{}, error) {
	row := db.QueryRow(
		`SELECT `+selectFields+` FROM `+string(tableName)+` WHERE `+fieldName+`=? AND deleted_at IS NULL`,
		fieldValue,
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

// UpdateRecord 更新记录（通用）
// tableName: 表名
// id: 记录 ID
// data: 要更新的字段名和值的映射
// 注意：由于 map 迭代顺序不确定，建议使用有序的数据结构或确保字段顺序一致
func UpdateRecord(db *DB, tableName TableName, id int64, data map[string]interface{}) error {
	if db == nil {
		return errors.New("db is nil")
	}
	if tableName == "" {
		return errors.New("tableName is empty")
	}
	if len(data) == 0 {
		return errors.New("no data to update")
	}

	setPairs := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data)+1)

	// 构建 SET 子句
	for k, v := range data {
		setPairs = append(setPairs, k+"=?")
		args = append(args, v)
	}

	// 添加 WHERE 条件的参数
	args = append(args, id)

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE id=? AND deleted_at IS NULL",
		string(tableName),
		strings.Join(setPairs, ", "),
	)

	_, err := db.Exec(query, args...)
	return err
}

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

// PostKind 文章类型
type PostKind int

const (
	PostKindPost PostKind = 0 // 文章
	PostKindPage PostKind = 1 // 页面
)

type Post struct {
	ID        int64
	Title     string
	Slug      string
	Content   string
	Status    string
	Kind      PostKind
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

type PostWithTags struct {
	Post     *Post
	Tags     []Tag
	Category *Category
}

func CreatePost(db *DB, p *Post) (int64, error) {
	if p.CreatedAt == 0 {
		p.CreatedAt = now()
	}
	if p.UpdatedAt == 0 {
		p.UpdatedAt = p.CreatedAt
	}

	id, err := CreateRecord(db, TablePosts, map[string]interface{}{
		"title":      p.Title,
		"slug":       p.Slug,
		"content":    p.Content,
		"status":     p.Status,
		"kind":       p.Kind,
		"created_at": p.CreatedAt,
		"updated_at": p.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	p.ID = id
	_ = EnsurePostFTS(db, id, p.Title, p.Content)
	return id, nil
}

func GetPostByID(db *DB, id int64) (*Post, error) {
	result, err := GetRecordByID(db, TablePosts, "id, title, slug, content, status, kind, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
		var p Post
		var deletedAt sql.NullInt64
		if err := row.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
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
	if err := UpdateRecord(db, TablePosts, p.ID, map[string]interface{}{
		"title":      p.Title,
		"content":    p.Content,
		"status":     p.Status,
		"kind":       p.Kind,
		"updated_at": p.UpdatedAt,
	}); err != nil {
		return err
	}
	_ = EnsurePostFTS(db, p.ID, p.Title, p.Content)
	return nil
}

// CountPostsByKind 按类型统计文章数（未删除）
func CountPostsByKind(db *DB, kind PostKind) (int, error) {
	return CountRecords(db, TablePosts, "deleted_at IS NULL AND kind = ?", []interface{}{kind})
}

// ListPublishedPosts 分页列出已发布文章（用于 RSS 等），返回 []Post
func ListPublishedPosts(db *DB, pager *types.Pagination) []Post {
	total, err := CountRecords(db, TablePosts, "deleted_at IS NULL AND status = ?", []interface{}{"published"})
	if err != nil {
		log.Println(err)
		return []Post{}
	}
	offset := (pager.Page - 1) * pager.PageSize
	rows, err := db.Query(`
		SELECT id, title, slug, content, status, kind, created_at, updated_at, deleted_at
		FROM `+string(TablePosts)+`
		WHERE deleted_at IS NULL AND status = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, "published", pager.PageSize, offset)
	if err != nil {
		log.Println(err)
		return []Post{}
	}
	defer rows.Close()
	var res []Post
	for rows.Next() {
		var p Post
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
			&p.CreatedAt, &p.UpdatedAt, &deletedAt,
		); err != nil {
			log.Println(err)
			return []Post{}
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}
		res = append(res, p)
	}
	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return res
}

func ListPosts(db *DB, pager *types.Pagination, kind *PostKind, searchIDs []int64) ([]PostWithTags, error) {
	var total int
	var err error
	if searchIDs != nil {
		total = len(searchIDs)
		if total == 0 {
			pager.Total = 0
			pager.Num = 0
			return []PostWithTags{}, nil
		}
	} else if kind == nil {
		total, err = CountRecords(db, TablePosts, "", nil)
	} else {
		total, err = CountPostsByKind(db, *kind)
	}
	if err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	query := `
		SELECT id, title, slug, content, status, kind, created_at, updated_at, deleted_at
		FROM ` + string(TablePosts) + `
		WHERE deleted_at IS NULL`
	args := []interface{}{}
	if kind != nil {
		query += ` AND kind = ?`
		args = append(args, *kind)
	}
	if searchIDs != nil && len(searchIDs) > 0 {
		placeholders := make([]string, len(searchIDs))
		for i := range searchIDs {
			placeholders[i] = "?"
		}
		query += ` AND id IN (` + strings.Join(placeholders, ",") + `)`
		for _, id := range searchIDs {
			args = append(args, id)
		}
	}
	query += `
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`
	args = append(args, pager.PageSize, offset)
	rows, err := db.Query(query, args...)
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
			&p.Kind,
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
	if err := SoftDeleteRecord(db, TablePosts, id); err != nil {
		return err
	}
	_ = RemovePostFTS(db, id)
	return nil
}

func RestorePost(db *DB, id int64) error {
	if err := RestoreRecord(db, TablePosts, id); err != nil {
		return err
	}
	post, err := GetPostByID(db, id)
	if err != nil {
		return err
	}
	_ = EnsurePostFTS(db, post.ID, post.Title, post.Content)
	return nil
}

func ListDeletedPosts(db *DB) ([]Post, error) {
	results, err := ListDeletedRecords(db, TablePosts, "id, title, slug, content, status, kind, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		var p Post
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
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

	return UpdateRecord(db, TableEncryptedPosts, p.ID, map[string]interface{}{
		"title":      p.Title,
		"content":    encryptedContent,
		"password":   p.Password,
		"expires_at": p.ExpiresAt,
		"updated_at": p.UpdatedAt,
	})
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

func CreateEncryptedPost(db *DB, p *EncryptedPost) (int64, error) {
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
		return 0, err
	}

	id, err := CreateRecord(db, TableEncryptedPosts, map[string]interface{}{
		"title":      p.Title,
		"slug":       p.Slug,
		"content":    encryptedContent,
		"password":   p.Password,
		"expires_at": p.ExpiresAt,
		"created_at": p.CreatedAt,
		"updated_at": p.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	p.ID = id
	return id, nil
}

type Tag struct {
	ID        int64
	Name      string
	Slug      string
	CreatedAt int64
	UpdatedAt int64
	DeletedAt *int64
}

func CreateTag(db *DB, t *Tag) (int64, error) {
	if t.CreatedAt == 0 {
		t.CreatedAt = now()
	}
	if t.UpdatedAt == 0 {
		t.UpdatedAt = t.CreatedAt
	}

	id, err := CreateRecord(db, TableTags, map[string]interface{}{
		"name":       t.Name,
		"slug":       t.Slug,
		"created_at": t.CreatedAt,
		"updated_at": t.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	t.ID = id
	return id, nil
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
	return UpdateRecord(db, TableTags, t.ID, map[string]interface{}{
		"name":       t.Name,
		"slug":       t.Slug,
		"updated_at": t.UpdatedAt,
	})
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
		FROM `+string(TableTags)+` t
		INNER JOIN `+string(TablePostTags)+` pt ON t.id = pt.tag_id
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

// CountPostsByTags 批量统计标签的文章数量
// 返回 map[tagID]count，只统计未删除的关联和未删除的文章
func CountPostsByTags(db *DB, tagIDs []int64) (map[int64]int, error) {
	if len(tagIDs) == 0 {
		return make(map[int64]int), nil
	}

	placeholders := make([]string, len(tagIDs))
	args := make([]interface{}, len(tagIDs))
	for i, id := range tagIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT pt.tag_id, COUNT(DISTINCT pt.post_id) as count
		FROM %s pt
		INNER JOIN %s p ON pt.post_id = p.id
		WHERE pt.tag_id IN (%s)
		  AND pt.deleted_at IS NULL
		  AND p.deleted_at IS NULL
		GROUP BY pt.tag_id
	`, string(TablePostTags), string(TablePosts), strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var tagID int64
		var count int
		if err := rows.Scan(&tagID, &count); err != nil {
			return nil, err
		}
		result[tagID] = count
	}

	// 确保所有 tagID 都在 map 中（没有关联文章时为 0）
	for _, id := range tagIDs {
		if _, ok := result[id]; !ok {
			result[id] = 0
		}
	}

	return result, nil
}

func GetPostBySlug(db *DB, slug string) (Post, error) {
	var p Post
	result, err := GetRecordByField(db, TablePosts, "id, title, slug, content, status, kind, created_at, updated_at, deleted_at", "slug", slug, func(row *sql.Row) (interface{}, error) {
		var deletedAt sql.NullInt64
		if err := row.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
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
		return Post{}, err
	}
	return *result.(*Post), nil
}

func ensurePostsFTSTable(db *DB) {
	_, _ = db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS ` + string(TablePostsFTS) + ` USING fts5(title, content, tokenize='unicode61')`)
}

func RebuildPostsFTS(db *DB) error {
	ensurePostsFTSTable(db)
	if _, err := db.Exec(`DELETE FROM ` + string(TablePostsFTS)); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT id, title, content FROM ` + string(TablePosts) + ` WHERE deleted_at IS NULL`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var title, content string
		if err := rows.Scan(&id, &title, &content); err != nil {
			return err
		}
		if _, err := db.Exec(`INSERT INTO `+string(TablePostsFTS)+`(rowid, title, content) VALUES (?, ?, ?)`, id, title, content); err != nil {
			return err
		}
	}
	return rows.Err()
}

func EnsurePostFTS(db *DB, id int64, title, content string) error {
	ensurePostsFTSTable(db)
	_, _ = db.Exec(`INSERT INTO `+string(TablePostsFTS)+`(`+string(TablePostsFTS)+`, rowid, title, content) VALUES ('delete', ?, '', '')`, id)
	_, err := db.Exec(`INSERT INTO `+string(TablePostsFTS)+`(rowid, title, content) VALUES (?, ?, ?)`, id, title, content)
	return err
}

func RemovePostFTS(db *DB, id int64) error {
	ensurePostsFTSTable(db)
	_, err := db.Exec(`INSERT INTO `+string(TablePostsFTS)+`(`+string(TablePostsFTS)+`, rowid, title, content) VALUES ('delete', ?, '', '')`, id)
	return err
}

func SearchPostIDsByFTS(db *DB, query string) ([]int64, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	ensurePostsFTSTable(db)
	query = strings.ReplaceAll(query, `"`, `""`)
	rows, err := db.Query(`SELECT rowid FROM `+string(TablePostsFTS)+` WHERE `+string(TablePostsFTS)+` MATCH ? ORDER BY rank`, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func AttachTagToPost(db *DB, postID, tagID int64) error {
	ts := now()
	// 先尝试恢复已存在的软删除关联
	res, err := db.Exec(
		`UPDATE `+string(TablePostTags)+` 
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
		`INSERT OR IGNORE INTO `+string(TablePostTags)+`
		 (post_id, tag_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		postID, tagID, ts, ts,
	)
	return err
}

func DetachTagFromPost(db *DB, postID, tagID int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(TablePostTags)+` SET deleted_at=? WHERE post_id=? AND tag_id=? AND deleted_at IS NULL`,
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

// CountPostsByCategories 批量统计分类的文章数量
// 返回 map[categoryID]count，只统计未删除的关联和未删除的文章
func CountPostsByCategories(db *DB, categoryIDs []int64) (map[int64]int, error) {
	if len(categoryIDs) == 0 {
		return make(map[int64]int), nil
	}

	placeholders := make([]string, len(categoryIDs))
	args := make([]interface{}, len(categoryIDs))
	for i, id := range categoryIDs {
		placeholders[i] = "?"
		args[i] = id
	}

	query := fmt.Sprintf(`
		SELECT pc.category_id, COUNT(DISTINCT pc.post_id) as count
		FROM %s pc
		INNER JOIN %s p ON pc.post_id = p.id
		WHERE pc.category_id IN (%s)
		  AND pc.deleted_at IS NULL
		  AND p.deleted_at IS NULL
		GROUP BY pc.category_id
	`, string(TablePostCategories), string(TablePosts), strings.Join(placeholders, ","))

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var categoryID int64
		var count int
		if err := rows.Scan(&categoryID, &count); err != nil {
			return nil, err
		}
		result[categoryID] = count
	}

	// 确保所有 categoryID 都在 map 中（没有关联文章时为 0）
	for _, id := range categoryIDs {
		if _, ok := result[id]; !ok {
			result[id] = 0
		}
	}

	return result, nil
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
	result, err := GetRecordByID(db, TableRedirects, "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Redirect), nil
}

// GetRedirectByFrom 根据 from_path 路径查找 redirect
func GetRedirectByFrom(db *DB, fromPath string) (*Redirect, error) {
	result, err := GetRecordByField(db, TableRedirects, "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at", "from_path", fromPath, func(row *sql.Row) (interface{}, error) {
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Redirect), nil
}

func UpdateRedirect(db *DB, r *Redirect) error {
	r.UpdatedAt = now()
	if r.Status == 0 {
		r.Status = 301 // default
	}
	return UpdateRecord(db, TableRedirects, r.ID, map[string]interface{}{
		"from_path":  r.From,
		"to_path":    r.To,
		"status":     r.Status,
		"enabled":    r.Enabled,
		"updated_at": r.UpdatedAt,
	})
}

func SoftDeleteRedirect(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableRedirects, id)
}

func RestoreRedirect(db *DB, id int64) error {
	return RestoreRecord(db, TableRedirects, id)
}

func ListRedirects(db *DB, limit, offset int) ([]Redirect, int, error) {
	// 获取总数
	total, err := CountRecords(db, TableRedirects, "", nil)
	if err != nil {
		return nil, 0, err
	}

	// 查询列表
	results, err := ListRecords(db, TableRedirects, "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at", "", "created_at DESC", nil, limit, offset, func(rows *sql.Rows) (interface{}, error) {
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
		return nil, 0, err
	}

	res := make([]Redirect, len(results))
	for i, v := range results {
		res[i] = v.(Redirect)
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

func CreateRedirect(db *DB, r *Redirect) (int64, error) {
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

	id, err := CreateRecord(db, TableRedirects, map[string]interface{}{
		"from_path":  r.From,
		"to_path":    r.To,
		"status":     r.Status,
		"enabled":    r.Enabled,
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	r.ID = id
	return id, nil
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

func (s Setting) String() string {
	return fmt.Sprintf("Setting{Code:%s, Value:%s}", s.Code, s.Value)
}

func CreateSetting(db *DB, s *Setting) (int64, error) {
	if s.Code == "" {
		return 0, errors.New("code is required")
	}
	if s.Type == "" {
		return 0, errors.New("type is required")
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
			return 0, err
		}
		s.Value = string(hashed)
	}

	id, err := CreateRecord(db, TableSettings, map[string]interface{}{
		"kind":                 s.Kind,
		"name":                 s.Name,
		"code":                 s.Code,
		"type":                 s.Type,
		"options":              s.Options,
		"attrs":                s.Attrs,
		"value":                s.Value,
		"default_option_value": s.DefaultOptionValue,
		"description":          s.Description,
		"sort":                 s.Sort,
		"charset":              s.Charset,
		"author":               s.Author,
		"keywords":             s.Keywords,
		"created_at":           s.CreatedAt,
		"updated_at":           s.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}

	s.ID = id
	return id, nil
}

func GetSettingByCode(db *DB, code string) (*Setting, error) {
	result, err := GetRecordByField(db, TableSettings, "id, kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, created_at, updated_at, deleted_at", "code", code, func(row *sql.Row) (interface{}, error) {
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Setting), nil
}

func GetSettingByID(db *DB, id int64) (*Setting, error) {
	result, err := GetRecordByID(db, TableSettings, "id, kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Setting), nil
}

func ListSettingsByKind(db *DB, kind string) ([]Setting, error) {
	whereClause := ""
	whereArgs := []interface{}{}
	if kind != "" {
		whereClause = "kind=?"
		whereArgs = append(whereArgs, kind)
	}

	results, err := ListRecords(db, TableSettings, "id, kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, created_at, updated_at, deleted_at", whereClause, "", whereArgs, 0, 0, func(rows *sql.Rows) (interface{}, error) {
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
		return s, nil
	})
	if err != nil {
		return nil, err
	}

	settings := make([]Setting, len(results))
	for i, v := range results {
		settings[i] = v.(Setting)
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

	err := UpdateRecord(db, TableSettings, s.ID, map[string]interface{}{
		"kind":                 s.Kind,
		"name":                 s.Name,
		"type":                 s.Type,
		"options":              s.Options,
		"attrs":                s.Attrs,
		"value":                s.Value,
		"default_option_value": s.DefaultOptionValue,
		"description":          s.Description,
		"sort":                 s.Sort,
		"charset":              s.Charset,
		"author":               s.Author,
		"keywords":             s.Keywords,
		"updated_at":           s.UpdatedAt,
	})

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
		`UPDATE `+string(TableSettings)+`
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
		`UPDATE `+string(TableSettings)+` SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
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
	for _, s := range DefaultSettings {
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
		if _, err := CreateSetting(db, &s); err != nil {
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

func CreateHttpErrorLog(db *DB, l *HttpErrorLog) (int64, error) {
	if l.CreatedAt == 0 {
		l.CreatedAt = now()
	}
	if l.ExpiredAt == 0 {
		// 默认 7 天
		l.ExpiredAt = l.CreatedAt + 7*24*60*60
	}

	id, err := CreateRecord(db, TableHttpErrorLogs, map[string]interface{}{
		"req_id":       l.ReqID,
		"client_ip":    l.ClientIP,
		"method":       l.Method,
		"path":         l.Path,
		"status":       l.Status,
		"user_agent":   l.UserAgent,
		"query_params": l.QueryParams,
		"body_params":  l.BodyParams,
		"created_at":   l.CreatedAt,
		"expired_at":   l.ExpiredAt,
	})
	if err != nil {
		return 0, err
	}
	l.ID = id
	return id, nil
}

func ListHttpErrorLogs(db *DB, limit, offset int) ([]HttpErrorLog, error) {
	// http_error_logs 表没有 deleted_at 字段，使用 "1=1" 避免通用函数自动添加 deleted_at 条件
	results, err := ListRecords(db, TableHttpErrorLogs, "id, req_id, client_ip, method, path, status, user_agent, query_params, body_params, created_at, expired_at", "1=1", "created_at DESC", nil, limit, offset, func(rows *sql.Rows) (interface{}, error) {
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
		return l, nil
	})
	if err != nil {
		return nil, err
	}

	res := make([]HttpErrorLog, len(results))
	for i, v := range results {
		res[i] = v.(HttpErrorLog)
	}
	return res, nil
}

func CountHttpErrorLogs(db *DB) (int, error) {
	// http_error_logs 表没有 deleted_at 字段，使用 "1=1" 避免通用函数自动添加 deleted_at 条件
	return CountRecords(db, TableHttpErrorLogs, "1=1", nil)
}

func DeleteHttpErrorLog(db *DB, id int64) error {
	return HardDeleteRecord(db, TableHttpErrorLogs, id)
}

var AppSettings atomic.Value // map[string]string

// LoadSettingsToMap 从 settings 表加载 code -> value 映射
func LoadSettingsToMap(db *DB) (map[string]string, error) {
	results, err := ListRecords(db, TableSettings, "code, value", "", "", nil, 0, 0, func(rows *sql.Rows) (interface{}, error) {
		var code, value string
		if err := rows.Scan(&code, &value); err != nil {
			return nil, err
		}
		return map[string]string{code: value}, nil
	})
	if err != nil {
		return nil, err
	}

	settingsMap := make(map[string]string)
	for _, v := range results {
		m := v.(map[string]string)
		for code, value := range m {
			settingsMap[code] = value
		}
	}
	return settingsMap, nil
}

// TaskKind 任务类型，与 job.JobKind 枚举值复用；JobInternal(0) 执行时不生成 TaskRun
type TaskKind int

const (
	TaskInternal TaskKind = 0 // 内部任务，不生成 TaskRun 日志
	TaskUser     TaskKind = 1 // 用户任务，生成 TaskRun 日志
)

type Task struct {
	ID          int64
	Code        string
	Name        string
	Description string
	Schedule    string
	Enabled     int
	Kind        TaskKind
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

func CreateTask(db *DB, task *Task) (int64, error) {
	now := time.Now().Unix()
	task.CreatedAt = now
	task.UpdatedAt = now

	id, err := CreateRecord(db, TableTasks, map[string]interface{}{
		"code":        task.Code,
		"name":        task.Name,
		"description": task.Description,
		"schedule":    task.Schedule,
		"enabled":     task.Enabled,
		"kind":        task.Kind,
		"created_at":  task.CreatedAt,
		"updated_at":  task.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	task.ID = id
	return id, nil
}

func GetTaskByID(db *DB, id int64) (*Task, error) {
	result, err := GetRecordByID(db, TableTasks, "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
		var t Task
		var lastRun sql.NullInt64
		var lastStatus sql.NullString
		var deleted sql.NullInt64

		if err := row.Scan(
			&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled, &t.Kind,
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Task), nil
}

func GetTaskByCode(db *DB, code string) (*Task, error) {
	result, err := GetRecordByField(db, TableTasks, "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at", "code", code, func(row *sql.Row) (interface{}, error) {
		var t Task
		var lastRun sql.NullInt64
		var lastStatus sql.NullString
		var deleted sql.NullInt64

		if err := row.Scan(
			&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled, &t.Kind,
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Task), nil
}

func ListTasks(db *DB) ([]Task, error) {
	results, err := ListRecords(db, TableTasks, "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at", "", "id DESC", nil, 0, 0, func(rows *sql.Rows) (interface{}, error) {
		var t Task
		var lastRun sql.NullInt64
		var lastStatus sql.NullString
		var deleted sql.NullInt64
		if err := rows.Scan(
			&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled, &t.Kind,
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
		return t, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]Task, len(results))
	for i, v := range results {
		res[i] = v.(Task)
	}
	return res, nil
}

func UpdateTask(db *DB, task *Task) error {
	task.UpdatedAt = now()
	enabled := 0
	if task.Enabled == 1 {
		enabled = 1
	}
	// Code 不可修改，不更新 code 字段
	return UpdateRecord(db, TableTasks, task.ID, map[string]interface{}{
		"name":        task.Name,
		"description": task.Description,
		"schedule":    task.Schedule,
		"enabled":     enabled,
		"kind":        task.Kind,
		"updated_at":  task.UpdatedAt,
	})
}
func UpdateTaskStatus(db *DB, taskCode string, lastStatus string, lastRunAt int64) error {
	_, err := db.Exec(`UPDATE `+string(TableTasks)+`
		SET last_status=?, last_run_at=?
		WHERE code=? AND deleted_at IS NULL`,
		lastStatus, lastRunAt, taskCode,
	)
	return err
}

func SoftDeleteTask(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableTasks, id)
}

func CreateTaskRun(db *DB, run *TaskRun) (int64, error) {
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

	id, err := CreateRecord(db, TableTaskRuns, map[string]interface{}{
		"task_code":   run.TaskCode,
		"run_id":      run.RunID,
		"status":      run.Status,
		"message":     run.Message,
		"started_at":  run.StartedAt,
		"finished_at": run.FinishedAt,
		"duration":    run.Duration,
		"created_at":  run.CreatedAt,
	})
	if err != nil {
		return 0, err
	}
	run.ID = id
	return id, nil

	//// 同步更新 tasks 的 last_run_at 和 last_status 字段
	//_, _ = db.Exec(`UPDATE `+string(TableTasks)+`
	//	SET last_run_at=?, last_status=?, updated_at=?
	//	WHERE code=? AND deleted_at IS NULL`,
	//	run.StartedAt, run.Status, now, run.TaskCode,
	//)
	//return nil
}

func ListTaskRuns(db *DB, taskCode string, status string, limit int) ([]TaskRun, error) {
	// task_runs 表没有 deleted_at 字段，使用 whereClause 构建条件
	whereClause := "1=1"
	whereArgs := []interface{}{}

	if taskCode != "" {
		whereClause += ` AND task_code = ?`
		whereArgs = append(whereArgs, taskCode)
	}

	if status != "" {
		whereClause += " AND status = ?"
		whereArgs = append(whereArgs, status)
	}

	results, err := ListRecords(db, TableTaskRuns, "id, task_code, run_id, status, message, started_at, finished_at, duration, created_at", whereClause, "created_at DESC", whereArgs, limit, 0, func(rows *sql.Rows) (interface{}, error) {
		var r TaskRun
		if err := rows.Scan(
			&r.ID, &r.TaskCode, &r.RunID, &r.Status, &r.Message,
			&r.StartedAt, &r.FinishedAt, &r.Duration, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		return r, nil
	})
	if err != nil {
		return nil, err
	}

	runs := make([]TaskRun, len(results))
	for i, v := range results {
		runs[i] = v.(TaskRun)
	}
	return runs, nil
}
func UpdateTaskRunStatus(db *DB, run *TaskRun) error {
	if run.ID == 0 {
		return errors.New("run.ID is zero")
	}

	// 更新 task_runs 表
	_, err := db.Exec(`
		UPDATE `+string(TableTaskRuns)+`
		SET status=?, message=?, finished_at=?, duration=?
		WHERE id=?
	`, run.Status, run.Message, run.FinishedAt, run.Duration, run.ID)
	if err != nil {
		return err
	}

	// 同步更新 tasks 表的 last_run_at 和 last_status 字段
	_, _ = db.Exec(`
		UPDATE `+string(TableTasks)+`
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
	result, err := GetRecordByID(db, TableCategories, "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at", id, func(row *sql.Row) (interface{}, error) {
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
	})
	if err != nil {
		return nil, err
	}
	return result.(*Category), nil
}

func CategoryExists(db *DB, id int64) (bool, error) {
	cnt, err := CountRecords(db, TableCategories, "id=?", []interface{}{id})
	return cnt > 0, err
}

func CreateCategory(db *DB, c *Category) (int64, error) {
	if c.ParentID != 0 {
		ok, err := CategoryExists(db, c.ParentID)
		if err != nil {
			return 0, err
		}
		if !ok {
			return 0, errors.New("parent category not exists")
		}
	}

	// 检查唯一性：同一父级下slug必须唯一（包括已软删除的）
	var existingID int64
	var err error
	if c.ParentID == 0 {
		err = db.QueryRow(`
			SELECT id FROM `+string(TableCategories)+` WHERE (parent_id IS NULL OR parent_id=0) AND slug=?
		`, c.Slug).Scan(&existingID)
	} else {
		err = db.QueryRow(`
			SELECT id FROM `+string(TableCategories)+` WHERE parent_id=? AND slug=?
		`, c.ParentID, c.Slug).Scan(&existingID)
	}
	if err == nil {
		return 0, errors.New("slug already exists under this parent")
	} else if err != sql.ErrNoRows {
		return 0, err
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

	id, err := CreateRecord(db, TableCategories, map[string]interface{}{
		"parent_id":   parentID,
		"name":        c.Name,
		"slug":        c.Slug,
		"description": c.Description,
		"sort":        c.Sort,
		"created_at":  c.CreatedAt,
		"updated_at":  c.UpdatedAt,
	})
	if err != nil {
		return 0, err
	}
	c.ID = id

	// 如果 sort 为 0（默认值），则设置为 id
	if c.Sort == 0 {
		c.Sort = id
		_, err = db.Exec(`
			UPDATE `+string(TableCategories)+` SET sort=? WHERE id=?
		`, c.Sort, id)
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

func ListCategories(db *DB) ([]Category, error) {
	results, err := ListRecords(db, TableCategories, "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at", "", "sort", nil, 0, 0, func(rows *sql.Rows) (interface{}, error) {
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

		return c, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]Category, len(results))
	for i, v := range results {
		res[i] = v.(Category)
	}
	return res, nil
}

func UpdateCategory(db *DB, c *Category) error {
	c.UpdatedAt = now()

	// 如果slug或parent_id改变了，需要检查唯一性
	var existingID int64
	var err error
	if c.ParentID == 0 {
		err = db.QueryRow(`
			SELECT id FROM `+string(TableCategories)+` WHERE (parent_id IS NULL OR parent_id=0) AND slug=? AND id!=? AND deleted_at IS NULL
		`, c.Slug, c.ID).Scan(&existingID)
	} else {
		err = db.QueryRow(`
			SELECT id FROM `+string(TableCategories)+` WHERE parent_id=? AND slug=? AND id!=? AND deleted_at IS NULL
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

	return UpdateRecord(db, TableCategories, c.ID, map[string]interface{}{
		"parent_id":   parentID,
		"name":        c.Name,
		"slug":        c.Slug,
		"description": c.Description,
		"sort":        c.Sort,
		"updated_at":  c.UpdatedAt,
	})
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
		UPDATE `+string(TableCategories)+` SET parent_id=?, updated_at=? WHERE id=?
	`, parentID, now(), id)
	return err
}

func SoftDeleteCategory(db *DB, id int64) error {
	return SoftDeleteRecord(db, TableCategories, id)
}

func RestoreCategory(db *DB, id int64) error {
	return RestoreRecord(db, TableCategories, id)
}

func ListDeletedCategories(db *DB) ([]Category, error) {
	results, err := ListDeletedRecords(db, TableCategories, "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		var c Category
		var parentID sql.NullInt64
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&c.ID, &parentID, &c.Name, &c.Slug, &c.Description, &c.Sort,
			&c.CreatedAt, &c.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, err
		}
		if parentID.Valid {
			c.ParentID = parentID.Int64
		}
		if deletedAt.Valid {
			c.DeletedAt = &deletedAt.Int64
		}
		return c, nil
	})
	if err != nil {
		return nil, err
	}
	res := make([]Category, len(results))
	for i, v := range results {
		res[i] = v.(Category)
	}
	return res, nil
}

func GetPostCategory(db *DB, postID int64) (*Category, error) {
	row := db.QueryRow(`
		SELECT c.id, c.parent_id, c.name, c.slug, c.description, c.sort, c.created_at, c.updated_at, c.deleted_at
		FROM `+string(TableCategories)+` c
		INNER JOIN `+string(TablePostCategories)+` pc ON c.id = pc.category_id
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
		`INSERT OR IGNORE INTO `+string(TablePostCategories)+`
		 (post_id, category_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		postID, categoryID, ts, ts,
	)
	return err
}

func DetachCategoryFromPost(db *DB, postID, categoryID int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(TablePostCategories)+` SET deleted_at=? WHERE post_id=? AND category_id=? AND deleted_at IS NULL`,
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

// ExportResult 导出结果
type ExportResult struct {
	Size int64  // 文件大小
	Hash string // SHA256 哈希值（十六进制）
	Date string // 导出日期
	File string // 导出文件路径
}

func (receiver ExportResult) String() string {
	return fmt.Sprintf("Date=%s, Size=%d, Hash=%s", receiver.Date, receiver.Size, receiver.Hash[:8])
}

func ExportSQLiteWithHash(db *DB, dir string) (res *ExportResult, err error) {
	timestamp := time.Now().Format("2006-01-02-15-04-05")

	// 1. 确保目录存在
	if err = os.MkdirAll(dir, 0755); err != nil {
		log.Printf("[WARN] failed to create directory: %v", err)
		return nil, errors.New("failed to create directory")
	}

	// 2. 临时文件
	tmpFile, err := os.Create(filepath.Join(dir, "__tmp__"))
	if err != nil {
		log.Println("[WARN] failed to create temp file:", err)
		return nil, errors.New("failed to create temp file")
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// 出错时清理
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err = db.Exec("PRAGMA wal_checkpoint(FULL)"); err != nil {
		log.Printf("[WARN] failed to checkpoint WAL: %v", err)
		return nil, err
	}
	// 3. 导出
	if _, err = db.Exec("VACUUM INTO ?", tmpPath); err != nil {
		log.Println("[WARN] failed to write to temp file:", err)
		return nil, err
	}

	// 4. 计算 SHA-256
	f, err := os.Open(tmpPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		return nil, err
	}

	hash := hex.EncodeToString(h.Sum(nil))

	// 5. 文件大小
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	size := info.Size()

	realFile := fmt.Sprintf("%s_%s.sqlite", timestamp, hash)
	finalPath := filepath.Join(dir, realFile)

	// 查看目录下是否有文件名中hash包括当前文件hash的
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, file := range files {
		if strings.Contains(file.Name(), hash) {
			// 有相同hash的文件，删除临时文件，直接返回已有文件信息
			return nil, errors.New("数据没有变更，无需重复导出: " + hash[:8])
		}
	}

	// 6. 原子 rename
	if err = os.Rename(tmpPath, finalPath); err != nil {
		return nil, err
	}

	return &ExportResult{
		Size: size,
		Hash: hash,
		Date: timestamp,
		File: finalPath,
	}, nil
}
