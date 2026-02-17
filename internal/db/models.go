package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/consts"
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
	stmts := []string{string(InitialSQL)}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return WrapInternalErr("InitDatabase", err)
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

// TableSpec 表配置，用于通用 CRUD
type TableSpec struct {
	Name         TableName
	IDField      string
	HasDeletedAt bool
	DeletedAtCol string
	HardDelete   bool
	HasCreatedAt bool
	CreatedAtCol string
	HasUpdatedAt bool
	UpdatedAtCol string
	DefaultOrder string
}

func (s TableSpec) idField() string {
	if s.IDField == "" {
		return "id"
	}
	return s.IDField
}

func (s TableSpec) deletedAtCol() string {
	if s.DeletedAtCol == "" {
		return "deleted_at"
	}
	return s.DeletedAtCol
}

func (s TableSpec) createdAtCol() string {
	if s.CreatedAtCol == "" {
		return "created_at"
	}
	return s.CreatedAtCol
}

func (s TableSpec) updatedAtCol() string {
	if s.UpdatedAtCol == "" {
		return "updated_at"
	}
	return s.UpdatedAtCol
}

func (s TableSpec) defaultOrder() string {
	if s.DefaultOrder == "" {
		return "created_at DESC"
	}
	return s.DefaultOrder
}

var (
	specPosts = TableSpec{
		Name:         TablePosts,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
	specEncryptedPosts = TableSpec{
		Name:         TableEncryptedPosts,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
	specTags = TableSpec{
		Name:         TableTags,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
	specRedirects = TableSpec{
		Name:         TableRedirects,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
	specSettings = TableSpec{
		Name:         TableSettings,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
	specHttpErrorLogs = TableSpec{
		Name:         TableHttpErrorLogs,
		HasDeletedAt: false,
		HardDelete:   true,
		HasCreatedAt: true,
	}
	specTasks = TableSpec{
		Name:         TableTasks,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
	specTaskRuns = TableSpec{
		Name:         TableTaskRuns,
		HasDeletedAt: false,
		HasCreatedAt: true,
	}
	specCategories = TableSpec{
		Name:         TableCategories,
		HasDeletedAt: true,
		HasCreatedAt: true,
		HasUpdatedAt: true,
	}
)

// ============================================================================
// Create 操作（创建）
// ============================================================================

// Create 核心 Create
func Create(db *DB, spec TableSpec, data map[string]interface{}) (int64, error) {
	if db == nil {
		log.Fatal("db is nil")
	}
	if spec.Name == "" {
		log.Fatal("table name is empty")
	}
	if len(data) == 0 {
		log.Fatal("no data to insert")
	}

	if spec.HasCreatedAt {
		ensureTimeField(data, spec.createdAtCol())
	}
	if spec.HasUpdatedAt {
		ensureTimeField(data, spec.updatedAtCol())
	}

	cols := make([]string, 0, len(data))
	placeholders := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data))
	for k, v := range data {
		cols = append(cols, k)
		placeholders = append(placeholders, "?")
		args = append(args, v)
	}

	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		string(spec.Name),
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)
	res, err := db.Exec(query, args...)
	if err != nil {
		return 0, WrapInternalErr("Create", err)
	}
	id, _ := res.LastInsertId()
	return id, nil
}

// ============================================================================
// Read 操作（查询）
// ============================================================================

// ReadOptions 查询参数
type ReadOptions struct {
	SelectFields string
	WhereClause  string
	OrderBy      string
	WhereArgs    []interface{}
	Limit        int
	Offset       int
}

// Read 核心 Read（统一返回多条）
func Read(db *DB, spec TableSpec, opts ReadOptions, scanFunc func(*sql.Rows) (interface{}, error)) ([]interface{}, error) {
	where := opts.WhereClause
	args := append([]interface{}{}, opts.WhereArgs...)
	if err := validateWhereArgs(where, args); err != nil {
		return nil, WrapInternalErr("Read.whereArgs", err)
	}
	if spec.HasDeletedAt {
		where = appendWhere(where, spec.deletedAtCol()+" IS NULL")
	}
	query := `SELECT ` + opts.SelectFields + ` FROM ` + string(spec.Name)
	if where != "" {
		query += ` WHERE ` + where
	}
	if opts.OrderBy != "" {
		query += ` ORDER BY ` + opts.OrderBy
	} else {
		query += ` ORDER BY ` + spec.defaultOrder()
	}
	if opts.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, opts.Limit)
		if opts.Offset > 0 {
			query += ` OFFSET ?`
			args = append(args, opts.Offset)
		}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, WrapInternalErr("Read", err)
	}
	defer rows.Close()

	var res []interface{}
	for rows.Next() {
		item, err := scanFunc(rows)
		if err != nil {
			return nil, WrapInternalErr("Read.scan", err)
		}
		res = append(res, item)
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("Read.rows.Err", err)
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
		return nil, WrapInternalErr("ListDeletedRecords", err)
	}
	defer rows.Close()

	var res []interface{}
	for rows.Next() {
		item, err := scanFunc(rows)
		if err != nil {
			return nil, WrapInternalErr("ListDeletedRecords.scan", err)
		}
		res = append(res, item)
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("ListDeletedRecords.rows.Err", err)
	}
	return res, nil
}

// Count 通用 Count（会自动处理 soft delete）
func Count(db *DB, spec TableSpec, whereClause string, whereArgs []interface{}) (int, error) {
	where := whereClause
	args := append([]interface{}{}, whereArgs...)
	if err := validateWhereArgs(where, args); err != nil {
		return 0, WrapInternalErr("Count.whereArgs", err)
	}
	if spec.HasDeletedAt {
		where = appendWhere(where, spec.deletedAtCol()+" IS NULL")
	}
	query := `SELECT COUNT(*) FROM ` + string(spec.Name)
	if where != "" {
		query += ` WHERE ` + where
	}
	var count int
	if err := db.QueryRow(query, args...).Scan(&count); err != nil {
		return 0, WrapInternalErr("Count", err)
	}
	return count, nil
}

// ============================================================================
// Update 操作（更新）
// ============================================================================

// Update 核心 Update
func Update(db *DB, spec TableSpec, id int64, data map[string]interface{}) error {
	if spec.HasUpdatedAt {
		data[spec.updatedAtCol()] = now()
	}
	setPairs := make([]string, 0, len(data))
	args := make([]interface{}, 0, len(data)+1)
	for k, v := range data {
		if k == spec.createdAtCol() {
			continue
		}
		setPairs = append(setPairs, k+"=?")
		args = append(args, v)
	}
	if len(setPairs) == 0 {
		return errors.New("no data to update")
	}
	args = append(args, id)

	where := spec.idField() + "=?"
	if spec.HasDeletedAt {
		where += " AND " + spec.deletedAtCol() + " IS NULL"
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s",
		string(spec.Name),
		strings.Join(setPairs, ", "),
		where,
	)
	if _, err := db.Exec(query, args...); err != nil {
		return WrapInternalErr("Update", err)
	}
	return nil
}

// ============================================================================
// Delete 操作（删除）
// ============================================================================

// Delete 核心 Delete（软删）
func Delete(db *DB, spec TableSpec, id int64) error {
	if spec.HardDelete || !spec.HasDeletedAt {
		return HardDelete(db, spec, id)
	}
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(spec.Name)+` SET `+spec.deletedAtCol()+`=? WHERE `+spec.idField()+`=? AND `+spec.deletedAtCol()+` IS NULL`,
		ts, id,
	)
	if err != nil {
		return WrapInternalErr("Delete.Soft", err)
	}
	return nil
}

// HardDelete 物理删除
func HardDelete(db *DB, spec TableSpec, id int64) error {
	_, err := db.Exec(`DELETE FROM `+string(spec.Name)+` WHERE `+spec.idField()+`=?`, id)
	if err != nil {
		return WrapInternalErr("HardDelete", err)
	}
	return nil
}

// Restore 恢复软删除的记录
func Restore(db *DB, spec TableSpec, id int64) error {
	if !spec.HasDeletedAt {
		return nil
	}
	_, err := db.Exec(
		`UPDATE `+string(spec.Name)+` SET `+spec.deletedAtCol()+`=NULL WHERE `+spec.idField()+`=? AND `+spec.deletedAtCol()+` IS NOT NULL`,
		id,
	)
	if err != nil {
		return WrapInternalErr("Restore", err)
	}
	return nil
}

func appendWhere(whereClause, cond string) string {
	if whereClause == "" {
		return cond
	}
	return whereClause + " AND " + cond
}

func validateWhereArgs(where string, args []interface{}) error {
	if where == "" {
		if len(args) != 0 {
			return errors.New("where args provided but where clause is empty")
		}
		return nil
	}
	count := strings.Count(where, "?")
	if count != len(args) {
		return fmt.Errorf("where args count mismatch: want %d, got %d", count, len(args))
	}
	return nil
}

func ensureTimeField(data map[string]interface{}, key string) {
	if v, ok := data[key]; ok {
		switch tv := v.(type) {
		case int64:
			if tv != 0 {
				return
			}
		case *int64:
			if tv != nil && *tv != 0 {
				return
			}
		case int:
			if tv != 0 {
				return
			}
		}
	}
	data[key] = now()
}

func firstResult(results []interface{}) (interface{}, bool) {
	if len(results) == 0 {
		return nil, false
	}
	return results[0], true
}

type sqlScanner interface {
	Scan(dest ...interface{}) error
}

func scanPost(scanner sqlScanner, withContent bool) (Post, error) {
	var p Post
	var deletedAt sql.NullInt64
	if withContent {
		if err := scanner.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
			&p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &deletedAt,
		); err != nil {
			return Post{}, err
		}
	} else {
		if err := scanner.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Status, &p.Kind,
			&p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &deletedAt,
		); err != nil {
			return Post{}, err
		}
	}
	if deletedAt.Valid {
		p.DeletedAt = &deletedAt.Int64
	}
	return p, nil
}

func scanTag(scanner sqlScanner) (Tag, error) {
	var t Tag
	var deletedAt sql.NullInt64
	if err := scanner.Scan(
		&t.ID, &t.Name, &t.Slug,
		&t.CreatedAt, &t.UpdatedAt, &deletedAt,
	); err != nil {
		return Tag{}, err
	}
	if deletedAt.Valid {
		t.DeletedAt = &deletedAt.Int64
	}
	return t, nil
}

func scanCategory(scanner sqlScanner) (Category, error) {
	var c Category
	var parentID sql.NullInt64
	var deletedAt sql.NullInt64
	if err := scanner.Scan(
		&c.ID, &parentID, &c.Name, &c.Slug, &c.Description, &c.Sort,
		&c.CreatedAt, &c.UpdatedAt, &deletedAt,
	); err != nil {
		return Category{}, err
	}
	if parentID.Valid {
		c.ParentID = parentID.Int64
	}
	if deletedAt.Valid {
		c.DeletedAt = &deletedAt.Int64
	}
	return c, nil
}

func scanRedirect(scanner sqlScanner) (Redirect, error) {
	var r Redirect
	var deletedAt sql.NullInt64
	var status sql.NullInt64
	var enabled sql.NullInt64
	if err := scanner.Scan(
		&r.ID, &r.From, &r.To, &status, &enabled,
		&r.CreatedAt, &r.UpdatedAt, &deletedAt,
	); err != nil {
		return Redirect{}, err
	}
	if status.Valid {
		r.Status = int(status.Int64)
	} else {
		r.Status = 301
	}
	if enabled.Valid {
		r.Enabled = int(enabled.Int64)
	} else {
		r.Enabled = 1
	}
	if deletedAt.Valid {
		r.DeletedAt = &deletedAt.Int64
	}
	return r, nil
}

func scanSetting(scanner sqlScanner) (Setting, error) {
	var s Setting
	var deletedAt sql.NullInt64
	if err := scanner.Scan(
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
		&s.Reload,
		&s.CreatedAt,
		&s.UpdatedAt,
		&deletedAt,
	); err != nil {
		return Setting{}, err
	}
	if deletedAt.Valid {
		s.DeletedAt = &deletedAt.Int64
	}
	return s, nil
}

func scanTask(scanner sqlScanner) (Task, error) {
	var t Task
	var lastRun sql.NullInt64
	var lastStatus sql.NullString
	var deletedAt sql.NullInt64
	if err := scanner.Scan(
		&t.ID, &t.Code, &t.Name, &t.Description, &t.Schedule, &t.Enabled, &t.Kind,
		&lastRun, &lastStatus, &t.CreatedAt, &t.UpdatedAt, &deletedAt,
	); err != nil {
		return Task{}, err
	}
	if lastRun.Valid {
		t.LastRunAt = &lastRun.Int64
	}
	if lastStatus.Valid {
		t.LastStatus = lastStatus.String
	}
	if deletedAt.Valid {
		t.DeletedAt = &deletedAt.Int64
	}
	return t, nil
}

func scanEncryptedPost(scanner sqlScanner, decrypt bool) (EncryptedPost, error) {
	var p EncryptedPost
	var deletedAt sql.NullInt64
	var expiresAt sql.NullInt64
	var encryptedContent string
	if err := scanner.Scan(
		&p.ID, &p.Title, &p.Slug, &encryptedContent, &p.Password,
		&expiresAt, &p.CreatedAt, &p.UpdatedAt, &deletedAt,
	); err != nil {
		return EncryptedPost{}, err
	}
	if expiresAt.Valid {
		p.ExpiresAt = &expiresAt.Int64
	}
	if deletedAt.Valid {
		p.DeletedAt = &deletedAt.Int64
	}
	if decrypt {
		decryptedContent, err := DecryptContent(encryptedContent)
		if err != nil {
			return EncryptedPost{}, WrapInternalErr("scanEncryptedPost.DecryptContent", err)
		}
		p.Content = decryptedContent
	} else {
		p.Content = encryptedContent
	}
	return p, nil
}

func scanHttpErrorLog(scanner sqlScanner) (HttpErrorLog, error) {
	var l HttpErrorLog
	if err := scanner.Scan(
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
		return HttpErrorLog{}, err
	}
	return l, nil
}

func scanTaskRun(scanner sqlScanner) (TaskRun, error) {
	var r TaskRun
	if err := scanner.Scan(
		&r.ID, &r.TaskCode, &r.RunID, &r.Status, &r.Message,
		&r.StartedAt, &r.FinishedAt, &r.Duration, &r.CreatedAt,
	); err != nil {
		return TaskRun{}, err
	}
	return r, nil
}

type UVEntityType int

const (
	UVEntitySite     UVEntityType = 1
	UVEntityPost     UVEntityType = 2
	UVEntityCategory UVEntityType = 3
	UVEntityTag      UVEntityType = 4
)

const UVVisitorIDBytes = 12
const UVVisitorIDEncodedLength = 16
const UVLastSeenUpdateMinIntervalSeconds int64 = 10 * 60
const UVVisitorIDMaxLength = 64

func (t UVEntityType) IsValid() bool {
	switch t {
	case UVEntitySite, UVEntityPost, UVEntityCategory, UVEntityTag:
		return true
	default:
		return false
	}
}

func parseVisitorIDBytes(visitorID string) ([]byte, error) {
	if visitorID == "" {
		return nil, errors.New("visitor_id is required")
	}
	if len(visitorID) > UVVisitorIDMaxLength {
		return nil, errors.New("visitor_id is too long")
	}
	if len(visitorID) != UVVisitorIDEncodedLength {
		return nil, errors.New("visitor_id is invalid")
	}

	decoded, err := base64.RawURLEncoding.DecodeString(visitorID)
	if err != nil || len(decoded) != UVVisitorIDBytes {
		return nil, errors.New("visitor_id is invalid")
	}

	return decoded, nil
}

type UVUnique struct {
	EntityType  UVEntityType
	EntityID    int64
	VisitorID   []byte
	FirstSeenAt int64
	LastSeenAt  int64
}

type UVPostRank struct {
	PostID int64
	Title  string
	Slug   string
	UV     int
}

func UpsertUVUnique(db *DB, entityType UVEntityType, entityID int64, visitorID string) (bool, error) {
	if !entityType.IsValid() {
		return false, errors.New("entity_type is invalid")
	}
	visitorIDBytes, err := parseVisitorIDBytes(visitorID)
	if err != nil {
		return false, err
	}

	ts := now()
	res, err := db.Exec(
		`INSERT INTO `+string(TableUVUnique)+` (entity_type, entity_id, visitor_id, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entity_type, entity_id, visitor_id) DO UPDATE SET last_seen_at = excluded.last_seen_at
		WHERE `+string(TableUVUnique)+`.last_seen_at < ?`,
		entityType, entityID, visitorIDBytes, ts, ts, ts-UVLastSeenUpdateMinIntervalSeconds,
	)
	if err != nil {
		if isUVUpsertConflictTargetErr(err) {
			return upsertUVUniqueCompat(db, entityType, entityID, visitorIDBytes, ts)
		}
		return false, WrapInternalErr("UpsertUVUnique.Upsert", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return false, WrapInternalErr("UpsertUVUnique.RowsAffected", err)
	}
	return affected > 0, nil
}

func isUVUpsertConflictTargetErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(
		strings.ToLower(err.Error()),
		"on conflict clause does not match any primary key or unique constraint",
	)
}

func upsertUVUniqueCompat(db *DB, entityType UVEntityType, entityID int64, visitorIDBytes []byte, ts int64) (bool, error) {
	threshold := ts - UVLastSeenUpdateMinIntervalSeconds

	var lastSeenAt int64
	err := db.QueryRow(
		`SELECT last_seen_at
		FROM `+string(TableUVUnique)+`
		WHERE entity_type = ? AND entity_id = ? AND visitor_id = ?
		ORDER BY last_seen_at DESC
		LIMIT 1`,
		entityType, entityID, visitorIDBytes,
	).Scan(&lastSeenAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false, WrapInternalErr("UpsertUVUnique.CompatSelect", err)
	}

	if errors.Is(err, sql.ErrNoRows) {
		res, insertErr := db.Exec(
			`INSERT INTO `+string(TableUVUnique)+` (entity_type, entity_id, visitor_id, first_seen_at, last_seen_at)
			VALUES (?, ?, ?, ?, ?)`,
			entityType, entityID, visitorIDBytes, ts, ts,
		)
		if insertErr != nil {
			return false, WrapInternalErr("UpsertUVUnique.CompatInsert", insertErr)
		}
		affected, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			return false, WrapInternalErr("UpsertUVUnique.CompatInsertRowsAffected", rowsErr)
		}
		return affected > 0, nil
	}

	if lastSeenAt >= threshold {
		return false, nil
	}

	res, updateErr := db.Exec(
		`UPDATE `+string(TableUVUnique)+`
		SET last_seen_at = ?
		WHERE entity_type = ? AND entity_id = ? AND visitor_id = ? AND last_seen_at < ?`,
		ts, entityType, entityID, visitorIDBytes, threshold,
	)
	if updateErr != nil {
		return false, WrapInternalErr("UpsertUVUnique.CompatUpdate", updateErr)
	}
	affected, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return false, WrapInternalErr("UpsertUVUnique.CompatUpdateRowsAffected", rowsErr)
	}
	return affected > 0, nil
}

func CountUVUnique(db *DB, entityType UVEntityType, entityID int64) (int, error) {
	if !entityType.IsValid() {
		return 0, errors.New("entity_type is invalid")
	}

	var count int
	if entityType == UVEntitySite {
		if err := db.QueryRow(
			`SELECT COUNT(DISTINCT visitor_id) FROM ` + string(TableUVUnique),
		).Scan(&count); err != nil {
			return 0, WrapInternalErr("CountUVUnique.CountSiteDistinctVisitor", err)
		}
		return count, nil
	}

	if err := db.QueryRow(
		`SELECT COUNT(*) FROM `+string(TableUVUnique)+` WHERE entity_type = ? AND entity_id = ?`,
		entityType, entityID,
	).Scan(&count); err != nil {
		return 0, WrapInternalErr("CountUVUnique", err)
	}
	return count, nil
}

func ListTopUVPosts(db *DB, limit int) ([]UVPostRank, error) {
	return listTopUVPostsByKind(db, PostKindPost, limit)
}

func ListTopUVPages(db *DB, limit int) ([]UVPostRank, error) {
	return listTopUVPostsByKind(db, PostKindPage, limit)
}

func listTopUVPostsByKind(db *DB, kind PostKind, limit int) ([]UVPostRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT p.id, p.title, p.slug, u.uv
		FROM (
			SELECT entity_id, COUNT(*) AS uv
			FROM `+string(TableUVUnique)+`
			WHERE entity_type = ?
			GROUP BY entity_id
			ORDER BY uv DESC
			LIMIT ?
		) AS u
		INNER JOIN `+string(TablePosts)+` AS p
			ON p.id = u.entity_id
		WHERE p.deleted_at IS NULL
			AND p.kind = ?
			AND p.status = ?
		ORDER BY u.uv DESC, p.published_at DESC, p.id DESC`,
		UVEntityPost, limit, kind, "published",
	)
	if err != nil {
		return nil, WrapInternalErr("listTopUVPostsByKind.Query", err)
	}
	defer rows.Close()

	res := make([]UVPostRank, 0, limit)
	for rows.Next() {
		var item UVPostRank
		if err = rows.Scan(&item.PostID, &item.Title, &item.Slug, &item.UV); err != nil {
			return nil, WrapInternalErr("listTopUVPostsByKind.Scan", err)
		}
		res = append(res, item)
	}
	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("listTopUVPostsByKind.Rows", err)
	}

	return res, nil
}

// PostKind 文章类型
type PostKind int

const (
	PostKindPost PostKind = 0 // 文章
	PostKindPage PostKind = 1 // 页面
)

type Post struct {
	ID          int64
	Title       string
	Slug        string
	Content     string
	Status      string
	Kind        PostKind
	PublishedAt int64 // 首次发布时间，0 表示未发布
	CreatedAt   int64
	UpdatedAt   int64
	DeletedAt   *int64
}

type PostWithRelation struct {
	Post     *Post
	Tags     []Tag
	Category *Category
}

func CreatePost(db *DB, p *Post) (int64, error) {
	if p.Status == "published" && p.PublishedAt == 0 {
		p.PublishedAt = now()
	}
	id, err := Create(db, specPosts, map[string]interface{}{
		"title":        p.Title,
		"slug":         p.Slug,
		"content":      p.Content,
		"status":       p.Status,
		"kind":         p.Kind,
		"created_at":   p.CreatedAt,
		"updated_at":   p.UpdatedAt,
		"published_at": p.PublishedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreatePost", err)
	}
	p.ID = id
	return id, nil
}

func GetPostByID(db *DB, id int64) (*Post, error) {
	result, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		p, err := scanPost(rows, true)
		if err != nil {
			return nil, WrapInternalErr("GetPostByID", err)
		}
		return p, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetPostByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetPostByID")
	}
	p := item.(Post)
	return &p, nil
}

func UpdatePost(db *DB, p *Post) error {
	data := map[string]interface{}{
		"title":   p.Title,
		"content": p.Content,
		"kind":    p.Kind,
	}
	if err := Update(db, specPosts, p.ID, data); err != nil {
		return err
	}
	return nil
}

// PublishPost 将文章发布：仅在 published_at 为 0 时写入发布时间
func PublishPost(db *DB, id int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(TablePosts)+` 
		 SET status='published',
		     published_at=CASE WHEN published_at=0 THEN ? ELSE published_at END,
		     updated_at=?
		 WHERE id=? AND deleted_at IS NULL`,
		ts, ts, id,
	)
	if err != nil {
		return WrapInternalErr("PublishPost", err)
	}
	return nil
}

// CountPostsByKind 按类型统计文章数（未删除）
func CountPostsByKind(db *DB, kind PostKind) (int, error) {
	return Count(db, specPosts, "kind = ?", []interface{}{kind})
}

// CountCategories 统计分类数（未删除）
func CountCategories(db *DB) (int, error) {
	return Count(db, specCategories, "", nil)
}

// CountTags 统计标签数（未删除）
func CountTags(db *DB) (int, error) {
	return Count(db, specTags, "", nil)
}

// ListPublishedPosts 分页列出已发布文章（用于 RSS 等），返回 []Post
func ListPublishedPosts(db *DB, kind PostKind, pager *types.Pagination) []Post {
	if pager == nil {
		pager = &types.Pagination{}
	}
	if pager.Page < 1 {
		pager.Page = consts.DefaultPage
	}
	if pager.PageSize < 1 {
		pager.PageSize = consts.DefaultPageSize
	}

	total, err := Count(db, specPosts, "status = ? AND kind = ?", []interface{}{"published", kind})
	if err != nil {
		log.Printf("[db] ListPublishedPosts Count: %v", err)
		return []Post{}
	}
	if kind == PostKindPage {
		pager.Page = 1
		pager.PageSize = 1024
	}
	offset := (pager.Page - 1) * pager.PageSize
	opts := ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "status = ? AND kind = ?",
		OrderBy:      "published_at DESC",
		WhereArgs:    []interface{}{"published", kind},
		Limit:        pager.PageSize,
	}
	if offset > 0 {
		opts.Offset = offset
	}
	results, err := Read(db, specPosts, opts, func(rows *sql.Rows) (interface{}, error) {
		p, err := scanPost(rows, true)
		if err != nil {
			return nil, err
		}
		return p, nil
	})
	if err != nil {
		log.Printf("[db] ListPublishedPosts Read: %v", err)
		return []Post{}
	}
	res := make([]Post, len(results))
	for i, v := range results {
		res[i] = v.(Post)
	}
	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return res
}
func ListPublishedPages(db *DB) []Post {
	results, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, status, kind, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "status = ? AND kind = ?",
		OrderBy:      "published_at DESC",
		WhereArgs:    []interface{}{"published", PostKindPage},
	}, func(rows *sql.Rows) (interface{}, error) {
		p, err := scanPost(rows, false)
		if err != nil {
			return nil, err
		}
		return p, nil
	})
	if err != nil {
		log.Printf("[db] ListPublishedPages Read: %v", err)
		return []Post{}
	}
	res := make([]Post, len(results))
	for i, v := range results {
		res[i] = v.(Post)
	}
	return res
}

// PostQueryOptions 文章列表查询参数
type PostQueryOptions struct {
	Kind        *PostKind // nil 表示不限类型
	TagID       int64     // 0 表示不限标签
	CategoryID  int64     // 0 表示不限分类
	Pager       *types.Pagination
	WithContent bool // 是否查询 content 字段
}

func normalizePostQueryOptions(opts *PostQueryOptions) *PostQueryOptions {
	if opts == nil {
		opts = &PostQueryOptions{}
	}
	if opts.Pager == nil {
		opts.Pager = &types.Pagination{}
	}
	if opts.Pager.Page < 1 {
		opts.Pager.Page = consts.DefaultPage
	}
	if opts.Pager.PageSize < 1 {
		opts.Pager.PageSize = consts.DefaultPageSize
	}
	return opts
}

func listPostsFilterClause(opts *PostQueryOptions) (clause string, args []interface{}) {
	if opts.TagID != 0 {
		clause += ` AND id IN (SELECT post_id FROM ` + string(TablePostTags) + ` WHERE tag_id=? AND deleted_at IS NULL)`
		args = append(args, opts.TagID)
	}
	if opts.CategoryID != 0 {
		clause += ` AND id IN (SELECT post_id FROM ` + string(TablePostCategories) + ` WHERE category_id=? AND deleted_at IS NULL)`
		args = append(args, opts.CategoryID)
	}
	return clause, args
}

func ListPosts(db *DB, opts *PostQueryOptions) ([]PostWithRelation, error) {
	opts = normalizePostQueryOptions(opts)

	whereBase := ""
	whereArgs := []interface{}{}
	if opts.Kind != nil {
		whereBase += " AND kind = ?"
		whereArgs = append(whereArgs, *opts.Kind)
	}
	filterClause, filterArgs := listPostsFilterClause(opts)
	whereBase += filterClause
	whereArgs = append(whereArgs, filterArgs...)

	whereBase = strings.TrimPrefix(whereBase, " AND ")
	total, err := Count(db, specPosts, whereBase, whereArgs)
	if err != nil {
		return nil, WrapInternalErr("ListPosts.Count", err)
	}

	pager := opts.Pager
	offset := (pager.Page - 1) * pager.PageSize
	selectFields := "id, title, slug, status, kind, created_at, updated_at, published_at, deleted_at"
	if opts.WithContent {
		selectFields = "id, title, slug, content, status, kind, created_at, updated_at, published_at, deleted_at"
	}
	results, err := Read(db, specPosts, ReadOptions{
		SelectFields: selectFields,
		WhereClause:  whereBase,
		OrderBy:      "created_at DESC",
		WhereArgs:    whereArgs,
		Limit:        pager.PageSize,
		Offset:       offset,
	}, func(rows *sql.Rows) (interface{}, error) {
		p, err := scanPost(rows, opts.WithContent)
		if err != nil {
			return nil, WrapInternalErr("ListPosts.Scan", err)
		}
		return p, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListPosts", err)
	}

	posts := make([]*Post, 0, len(results))
	for _, item := range results {
		p := item.(Post)
		posts = append(posts, &p)
	}

	postIDs := make([]int64, len(posts))
	for i, p := range posts {
		postIDs[i] = p.ID
	}
	tagsByPost, err := getPostTagsByPostIDs(db, postIDs)
	if err != nil {
		return nil, WrapInternalErr("ListPosts.GetPostTags", err)
	}
	categoriesByPost, err := getPostCategoriesByPostIDs(db, postIDs)
	if err != nil {
		return nil, WrapInternalErr("ListPosts.GetPostCategories", err)
	}

	res := make([]PostWithRelation, 0, len(posts))
	for _, p := range posts {
		tags := tagsByPost[p.ID]
		if tags == nil {
			tags = []Tag{}
		}
		res = append(res, PostWithRelation{Post: p, Tags: tags, Category: categoriesByPost[p.ID]})
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return res, nil
}

// ListPostsByTag 按标签 ID 分页列出文章（未删除），返回 PostWithRelation
func ListPostsByTag(db *DB, options *PostQueryOptions) ([]PostWithRelation, error) {
	return ListPosts(db, options)
}

// ListPostsByCategory 按分类 ID 分页列出文章（未删除），返回 PostWithRelation
func ListPostsByCategory(db *DB, options *PostQueryOptions) ([]PostWithRelation, error) {
	return ListPosts(db, options)
}

func SoftDeletePost(db *DB, id int64) error {
	return Delete(db, specPosts, id)
}

func HardDeletePost(db *DB, id int64) error {
	return HardDelete(db, specPosts, id)
}

func RestorePost(db *DB, id int64) error {
	return Restore(db, specPosts, id)
}

func ListDeletedPosts(db *DB) ([]Post, error) {
	results, err := ListDeletedRecords(db, TablePosts, "id, title, slug, content, status, kind, created_at, updated_at, published_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		var p Post
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
			&p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &deletedAt,
		); err != nil {
			return nil, WrapInternalErr("ListDeletedPosts.Scan", err)
		}
		if deletedAt.Valid {
			p.DeletedAt = &deletedAt.Int64
		}
		return p, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListDeletedPosts", err)
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
	result, err := Read(db, specEncryptedPosts, ReadOptions{
		SelectFields: "id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		p, err := scanEncryptedPost(rows, true)
		if err != nil {
			return nil, WrapInternalErr("GetEncryptedPostByID", err)
		}
		return p, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetEncryptedPostByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetEncryptedPostByID")
	}
	p := item.(EncryptedPost)
	return &p, nil
}

func UpdateEncryptedPost(db *DB, p *EncryptedPost) error {
	// 加密 content（使用系统密钥，不依赖 password 字段）
	encryptedContent, err := EncryptContent(p.Content)
	if err != nil {
		return WrapInternalErr("UpdateEncryptedPost.EncryptContent", err)
	}

	return Update(db, specEncryptedPosts, p.ID, map[string]interface{}{
		"title":      p.Title,
		"content":    encryptedContent,
		"password":   p.Password,
		"expires_at": p.ExpiresAt,
	})
}

func SoftDeleteEncryptedPost(db *DB, id int64) error {
	return Delete(db, specEncryptedPosts, id)
}

func HardDeleteEncryptedPost(db *DB, id int64) error {
	return HardDelete(db, specEncryptedPosts, id)
}

func RestoreEncryptedPost(db *DB, id int64) error {
	return Restore(db, specEncryptedPosts, id)
}

// SoftDeleteExpiredEncryptedPosts 软删除所有已过期的加密文章（expires_at IS NOT NULL AND expires_at < beforeUnix），返回删除条数
func SoftDeleteExpiredEncryptedPosts(db *DB, beforeUnix int64) (int64, error) {
	ts := now()
	res, err := db.Exec(
		`UPDATE `+string(TableEncryptedPosts)+` SET deleted_at=?, updated_at=? WHERE expires_at IS NOT NULL AND expires_at < ? AND deleted_at IS NULL`,
		ts, ts, beforeUnix,
	)
	if err != nil {
		return 0, WrapInternalErr("SoftDeleteExpiredEncryptedPosts", err)
	}
	return res.RowsAffected()
}

func ListDeletedEncryptedPosts(db *DB) ([]EncryptedPost, error) {
	results, err := ListDeletedRecords(db, TableEncryptedPosts, "id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		p, err := scanEncryptedPost(rows, false)
		if err != nil {
			return nil, WrapInternalErr("ListDeletedEncryptedPosts.Scan", err)
		}
		return p, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListDeletedEncryptedPosts", err)
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

	// 加密 content（使用系统密钥，不依赖 password 字段）
	encryptedContent, err := EncryptContent(p.Content)
	if err != nil {
		return 0, WrapInternalErr("CreateEncryptedPost.EncryptContent", err)
	}

	id, err := Create(db, specEncryptedPosts, map[string]interface{}{
		"title":      p.Title,
		"slug":       p.Slug,
		"content":    encryptedContent,
		"password":   p.Password,
		"expires_at": p.ExpiresAt,
		"created_at": p.CreatedAt,
		"updated_at": p.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateEncryptedPost", err)
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
	PostCount int // 仅当 ListTags(withPostCount=true) 时关联查询填充
}

func CreateTag(db *DB, t *Tag) (int64, error) {
	id, err := Create(db, specTags, map[string]interface{}{
		"name":       t.Name,
		"slug":       t.Slug,
		"created_at": t.CreatedAt,
		"updated_at": t.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateTag", err)
	}
	t.ID = id
	return id, nil
}

func GetTagBySlug(db *DB, slug string) (*Tag, error) {
	result, err := Read(db, specTags, ReadOptions{
		SelectFields: "id, name, slug, created_at, updated_at, deleted_at",
		WhereClause:  "slug=?",
		WhereArgs:    []interface{}{slug},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTag(rows)
		if err != nil {
			return nil, WrapInternalErr("GetTagBySlug.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetTagBySlug", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetTagBySlug")
	}
	t := item.(Tag)
	return &t, nil
}

func GetTagByID(db *DB, id int64) (*Tag, error) {
	result, err := Read(db, specTags, ReadOptions{
		SelectFields: "id, name, slug, created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTag(rows)
		if err != nil {
			return nil, WrapInternalErr("GetTagByID.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetTagByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetTagByID")
	}
	t := item.(Tag)
	return &t, nil
}

func UpdateTag(db *DB, t *Tag) error {
	return Update(db, specTags, t.ID, map[string]interface{}{
		"name": t.Name,
		"slug": t.Slug,
	})
}

func SoftDeleteTag(db *DB, id int64) error {
	return Delete(db, specTags, id)
}

func HardDeleteTag(db *DB, id int64) error {
	return HardDelete(db, specTags, id)
}

func RestoreTag(db *DB, id int64) error {
	return Restore(db, specTags, id)
}

// UpdateTagCreatedAtIfEarlier 仅当 createdAt 早于当前 created_at 时更新（导入时用于“按最早出现该标签的文章创建时间”）
func UpdateTagCreatedAtIfEarlier(db *DB, id int64, createdAt int64) error {
	_, err := db.Exec(
		`UPDATE `+string(TableTags)+` SET created_at = ? WHERE id = ? AND deleted_at IS NULL AND (created_at IS NULL OR created_at > ?)`,
		createdAt, id, createdAt,
	)
	if err != nil {
		return WrapInternalErr("UpdateTagCreatedAtIfEarlier", err)
	}
	return nil
}

func ListTags(db *DB, withPostCount bool) ([]Tag, error) {
	results, err := Read(db, specTags, ReadOptions{
		SelectFields: "id, name, slug, created_at, updated_at, deleted_at",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTag(rows)
		if err != nil {
			return nil, WrapInternalErr("ListTags.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListTags", err)
	}
	res := make([]Tag, len(results))
	for i, v := range results {
		res[i] = v.(Tag)
	}
	if withPostCount && len(res) > 0 {
		ids := make([]int64, len(res))
		for i := range res {
			ids[i] = res[i].ID
		}
		counts, err := CountPostsByTags(db, ids)
		if err != nil {
			return nil, err
		}
		for i := range res {
			if n, ok := counts[res[i].ID]; ok {
				res[i].PostCount = n
			}
		}
	}
	return res, nil
}

func ListDeletedTags(db *DB) ([]Tag, error) {
	results, err := ListDeletedRecords(db, TableTags, "id, name, slug, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTag(rows)
		if err != nil {
			return nil, WrapInternalErr("ListDeletedTags.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListDeletedTags", err)
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
		return nil, WrapInternalErr("GetPostTags", err)
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		t, err := scanTag(rows)
		if err != nil {
			return nil, WrapInternalErr("GetPostTags.Scan", err)
		}
		tags = append(tags, t)
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("GetPostTags.rows.Err", err)
	}
	return tags, nil
}

// getPostTagsByPostIDs 批量查询多篇文章的标签，返回 postID -> []Tag，避免 N+1
func getPostTagsByPostIDs(db *DB, postIDs []int64) (map[int64][]Tag, error) {
	out := make(map[int64][]Tag)
	if len(postIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT pt.post_id, t.id, t.name, t.slug, t.created_at, t.updated_at, t.deleted_at
		FROM ` + string(TableTags) + ` t
		INNER JOIN ` + string(TablePostTags) + ` pt ON t.id = pt.tag_id
		WHERE pt.post_id IN (` + strings.Join(placeholders, ",") + `) AND pt.deleted_at IS NULL AND t.deleted_at IS NULL
		ORDER BY pt.post_id, t.name`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, WrapInternalErr("getPostTagsByPostIDs", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postID int64
		var t Tag
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&postID,
			&t.ID, &t.Name, &t.Slug,
			&t.CreatedAt, &t.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, WrapInternalErr("getPostTagsByPostIDs.Scan", err)
		}
		if deletedAt.Valid {
			t.DeletedAt = &deletedAt.Int64
		}
		out[postID] = append(out[postID], t)
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("getPostTagsByPostIDs.rows.Err", err)
	}
	return out, nil
}

// getPostCategoriesByPostIDs 批量查询多篇文章的分类，返回 postID -> *Category（每篇最多一个），避免 N+1
func getPostCategoriesByPostIDs(db *DB, postIDs []int64) (map[int64]*Category, error) {
	out := make(map[int64]*Category)
	if len(postIDs) == 0 {
		return out, nil
	}
	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, len(postIDs))
	for i, id := range postIDs {
		placeholders[i] = "?"
		args[i] = id
	}
	query := `
		SELECT pc.post_id, c.id, c.parent_id, c.name, c.slug, c.description, c.sort, c.created_at, c.updated_at, c.deleted_at
		FROM ` + string(TableCategories) + ` c
		INNER JOIN ` + string(TablePostCategories) + ` pc ON c.id = pc.category_id
		WHERE pc.post_id IN (` + strings.Join(placeholders, ",") + `) AND pc.deleted_at IS NULL AND c.deleted_at IS NULL
		ORDER BY pc.post_id, c.name`
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, WrapInternalErr("getPostCategoriesByPostIDs", err)
	}
	defer rows.Close()
	for rows.Next() {
		var postID int64
		var c Category
		var parentID sql.NullInt64
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&postID,
			&c.ID, &parentID, &c.Name, &c.Slug, &c.Description, &c.Sort,
			&c.CreatedAt, &c.UpdatedAt, &deletedAt,
		); err != nil {
			return nil, WrapInternalErr("getPostCategoriesByPostIDs.Scan", err)
		}
		if parentID.Valid {
			c.ParentID = parentID.Int64
		}
		if deletedAt.Valid {
			c.DeletedAt = &deletedAt.Int64
		}
		// 每篇只保留第一个（与 GetPostCategory LIMIT 1 一致）
		if _, ok := out[postID]; !ok {
			out[postID] = &c
		}
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("getPostCategoriesByPostIDs.rows.Err", err)
	}
	return out, nil
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
		return nil, WrapInternalErr("CountPostsByTags", err)
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var tagID int64
		var count int
		if err := rows.Scan(&tagID, &count); err != nil {
			return nil, WrapInternalErr("CountPostsByTags.Scan", err)
		}
		result[tagID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("CountPostsByTags.rows.Err", err)
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
	result, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "slug=? AND status=? AND published_at>0",
		WhereArgs:    []interface{}{slug, "published"},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		p2, err := scanPost(rows, true)
		if err != nil {
			return nil, WrapInternalErr("GetPostBySlug.Scan", err)
		}
		return p2, nil
	})
	if err != nil {
		return Post{}, WrapInternalErr("GetPostBySlug", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return Post{}, ErrNotFound("GetPostBySlug")
	}
	p = item.(Post)
	return p, nil
}

// GetPostBySlugWithRelation 按 slug 查询文章并带出关联的分类与标签，返回 PostWithRelation
func GetPostBySlugWithRelation(db *DB, slug string) (PostWithRelation, error) {
	p, err := GetPostBySlug(db, slug)
	if err != nil {
		return PostWithRelation{}, err
	}
	tags, err := GetPostTags(db, p.ID)
	if err != nil {
		return PostWithRelation{}, WrapInternalErr("GetPostBySlugWithRelation.GetPostTags", err)
	}
	category, _ := GetPostCategory(db, p.ID) // 无分类时返回 nil，不视为错误
	return PostWithRelation{Post: &p, Tags: tags, Category: category}, nil
}

// GetPrevNextPost 按当前文章 ID 查询前一篇、后一篇（按 published_at 排序，仅已发布文章）
// prev 为 published_at 小于当前文章的最近一篇，next 为 published_at 大于当前文章的最近一篇；若不存在则对应为 nil
func GetPrevNextPost(db *DB, publishedAt int64) (prev, next *Post, err error) {
	loadOne := func(where string, args []interface{}, orderBy string) (*Post, error) {
		results, err := Read(db, specPosts, ReadOptions{
			SelectFields: "id, title, slug, status, kind, created_at, updated_at, published_at, deleted_at",
			WhereClause:  where,
			OrderBy:      orderBy,
			WhereArgs:    args,
			Limit:        1,
		}, func(rows *sql.Rows) (interface{}, error) {
			p, err := scanPost(rows, false)
			if err != nil {
				return nil, WrapInternalErr("GetPrevNextPost.Scan", err)
			}
			return p, nil
		})
		if err != nil {
			return nil, err
		}
		item, ok := firstResult(results)
		if !ok {
			return nil, nil
		}
		p := item.(Post)
		return &p, nil
	}

	prev, err = loadOne(
		"status = ? AND kind = ? AND published_at > 0 AND published_at < ?",
		[]interface{}{"published", PostKindPost, publishedAt},
		"published_at DESC, id DESC",
	)
	if err != nil {
		return nil, nil, WrapInternalErr("GetPrevNextPost.prev", err)
	}

	next, err = loadOne(
		"status = ? AND kind = ? AND published_at > ?",
		[]interface{}{"published", PostKindPost, publishedAt},
		"published_at ASC, id ASC",
	)
	if err != nil {
		return nil, nil, WrapInternalErr("GetPrevNextPost.next", err)
	}

	return prev, next, nil
}

// likePattern 对关键词做 LIKE 通配符转义，用于 '%'||?||'%' 的 ?
func likePattern(q string) string {
	replacer := strings.NewReplacer(
		`%`, `\%`,
		`_`, `\_`,
		`[`, `\[`,
		`]`, `\]`,
		`^`, `\^`,
		`\`, `\\`,
	)
	return replacer.Replace(strings.TrimSpace(q))
}

// ListPostsBySearch 后台文章搜索：单条 SQL，优先级 title(10) > slug(5) > content(1)，ORDER BY score DESC, published_at DESC；支持 tag/category 过滤
func ListPostsBySearch(db *DB, opts *PostQueryOptions, q string) ([]PostWithRelation, error) {
	opts = normalizePostQueryOptions(opts)

	q = strings.TrimSpace(q)
	if q == "" {
		return []PostWithRelation{}, nil
	}
	pattern := likePattern(q)
	likeCond := `(title LIKE '%' || ? || '%' ESCAPE '\' OR slug LIKE '%' || ? || '%' ESCAPE '\' OR content LIKE '%' || ? || '%' ESCAPE '\')`
	filterClause, filterArgs := listPostsFilterClause(opts)

	countWhere := likeCond + filterClause
	countArgs := []interface{}{pattern, pattern, pattern}
	countArgs = append(countArgs, filterArgs...)
	if opts.Kind != nil {
		countWhere = "kind = ? AND " + countWhere
		countArgs = append([]interface{}{*opts.Kind}, countArgs...)
	}
	total, err := Count(db, specPosts, countWhere, countArgs)
	if err != nil {
		return nil, WrapInternalErr("ListPostsBySearch.Count", err)
	}
	if total == 0 {
		return []PostWithRelation{}, nil
	}

	pager := opts.Pager
	offset := (pager.Page - 1) * pager.PageSize
	scoreExpr := `(
		(CASE WHEN title   LIKE '%' || ? || '%' ESCAPE '\' THEN 10 ELSE 0 END) +
		(CASE WHEN slug    LIKE '%' || ? || '%' ESCAPE '\' THEN 5  ELSE 0 END) +
		(CASE WHEN content LIKE '%' || ? || '%' ESCAPE '\' THEN 1  ELSE 0 END)
	) AS score`
	selectFields := "id, title, slug, status, kind, created_at, updated_at, published_at, deleted_at"
	if opts.WithContent {
		selectFields = "id, title, slug, content, status, kind, created_at, updated_at, published_at, deleted_at"
	}
	var mainQuery string
	var mainArgs []interface{}
	if opts.Kind != nil {
		mainQuery = `SELECT ` + selectFields + `, ` + scoreExpr + `
			FROM ` + string(TablePosts) + `
			WHERE deleted_at IS NULL AND kind = ? AND ` + likeCond + filterClause
		mainArgs = []interface{}{pattern, pattern, pattern, *opts.Kind, pattern, pattern, pattern}
	} else {
		mainQuery = `SELECT ` + selectFields + `, ` + scoreExpr + `
			FROM ` + string(TablePosts) + `
			WHERE deleted_at IS NULL AND ` + likeCond + filterClause
		mainArgs = []interface{}{pattern, pattern, pattern, pattern, pattern, pattern}
	}
	mainArgs = append(mainArgs, filterArgs...)
	mainQuery += ` ORDER BY score DESC, published_at DESC LIMIT ? OFFSET ?`
	mainArgs = append(mainArgs, pager.PageSize, offset)

	rows, err := db.Query(mainQuery, mainArgs...)
	if err != nil {
		return nil, WrapInternalErr("ListPostsBySearch", err)
	}
	defer rows.Close()

	var posts []*Post
	for rows.Next() {
		var score int
		var p Post
		var err error
		if opts.WithContent {
			var deletedAt sql.NullInt64
			if err = rows.Scan(
				&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind,
				&p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &deletedAt, &score,
			); err != nil {
				return nil, WrapInternalErr("ListPostsBySearch.Scan", err)
			}
			if deletedAt.Valid {
				p.DeletedAt = &deletedAt.Int64
			}
		} else {
			var deletedAt sql.NullInt64
			if err = rows.Scan(
				&p.ID, &p.Title, &p.Slug, &p.Status, &p.Kind,
				&p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &deletedAt, &score,
			); err != nil {
				return nil, WrapInternalErr("ListPostsBySearch.Scan", err)
			}
			if deletedAt.Valid {
				p.DeletedAt = &deletedAt.Int64
			}
		}
		posts = append(posts, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("ListPostsBySearch.rows.Err", err)
	}

	postIDs := make([]int64, len(posts))
	for i, p := range posts {
		postIDs[i] = p.ID
	}
	tagsByPost, err := getPostTagsByPostIDs(db, postIDs)
	if err != nil {
		return nil, WrapInternalErr("ListPostsBySearch.GetPostTags", err)
	}
	categoriesByPost, err := getPostCategoriesByPostIDs(db, postIDs)
	if err != nil {
		return nil, WrapInternalErr("ListPostsBySearch.GetPostCategories", err)
	}

	res := make([]PostWithRelation, 0, len(posts))
	for _, p := range posts {
		tags := tagsByPost[p.ID]
		if tags == nil {
			tags = []Tag{}
		}
		res = append(res, PostWithRelation{Post: p, Tags: tags, Category: categoriesByPost[p.ID]})
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return res, nil
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
		return WrapInternalErr("AttachTagToPost.Update", err)
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
	if err != nil {
		return WrapInternalErr("AttachTagToPost.Insert", err)
	}
	return nil
}

func DetachTagFromPost(db *DB, postID, tagID int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(TablePostTags)+` SET deleted_at=? WHERE post_id=? AND tag_id=? AND deleted_at IS NULL`,
		ts, postID, tagID,
	)
	if err != nil {
		return WrapInternalErr("DetachTagFromPost", err)
	}
	return nil
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
		return nil, WrapInternalErr("CountPostsByCategories", err)
	}
	defer rows.Close()

	result := make(map[int64]int)
	for rows.Next() {
		var categoryID int64
		var count int
		if err := rows.Scan(&categoryID, &count); err != nil {
			return nil, WrapInternalErr("CountPostsByCategories.Scan", err)
		}
		result[categoryID] = count
	}
	if err := rows.Err(); err != nil {
		return nil, WrapInternalErr("CountPostsByCategories.rows.Err", err)
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
	result, err := Read(db, specRedirects, ReadOptions{
		SelectFields: "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		r, err := scanRedirect(rows)
		if err != nil {
			return nil, WrapInternalErr("GetRedirectByID.Scan", err)
		}
		return r, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetRedirectByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetRedirectByID")
	}
	r := item.(Redirect)
	return &r, nil
}

// GetRedirectByFrom 根据 from_path 路径查找 redirect
func GetRedirectByFrom(db *DB, fromPath string) (*Redirect, error) {
	result, err := Read(db, specRedirects, ReadOptions{
		SelectFields: "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at",
		WhereClause:  "from_path=?",
		WhereArgs:    []interface{}{fromPath},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		r, err := scanRedirect(rows)
		if err != nil {
			return nil, WrapInternalErr("GetRedirectByFrom.Scan", err)
		}
		return r, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetRedirectByFrom", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetRedirectByFrom")
	}
	r := item.(Redirect)
	return &r, nil
}

func UpdateRedirect(db *DB, r *Redirect) error {
	if r.Status == 0 {
		r.Status = 301 // default
	}
	return Update(db, specRedirects, r.ID, map[string]interface{}{
		"from_path": r.From,
		"to_path":   r.To,
		"status":    r.Status,
		"enabled":   r.Enabled,
	})
}

func SoftDeleteRedirect(db *DB, id int64) error {
	return Delete(db, specRedirects, id)
}

func HardDeleteRedirect(db *DB, id int64) error {
	return HardDelete(db, specRedirects, id)
}

func RestoreRedirect(db *DB, id int64) error {
	return Restore(db, specRedirects, id)
}

func ListRedirects(db *DB, limit, offset int) ([]Redirect, int, error) {
	// 获取总数
	total, err := Count(db, specRedirects, "", nil)
	if err != nil {
		return nil, 0, WrapInternalErr("ListRedirects.Count", err)
	}

	// 查询列表
	results, err := Read(db, specRedirects, ReadOptions{
		SelectFields: "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        limit,
		Offset:       offset,
	}, func(rows *sql.Rows) (interface{}, error) {
		r, err := scanRedirect(rows)
		if err != nil {
			return nil, WrapInternalErr("ListRedirects.Scan", err)
		}
		return r, nil
	})
	if err != nil {
		return nil, 0, WrapInternalErr("ListRedirects", err)
	}

	res := make([]Redirect, len(results))
	for i, v := range results {
		res[i] = v.(Redirect)
	}
	return res, total, nil
}

func ListDeletedRedirects(db *DB) ([]Redirect, error) {
	results, err := ListDeletedRecords(db, TableRedirects, "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		r, err := scanRedirect(rows)
		if err != nil {
			return nil, WrapInternalErr("ListDeletedRedirects.Scan", err)
		}
		return r, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListDeletedRedirects", err)
	}
	res := make([]Redirect, len(results))
	for i, v := range results {
		res[i] = v.(Redirect)
	}
	return res, nil
}

func CreateRedirect(db *DB, r *Redirect) (int64, error) {
	if r.Status == 0 {
		r.Status = 301 // default
	}
	if r.Enabled == 0 {
		r.Enabled = 1 // default
	}

	id, err := Create(db, specRedirects, map[string]interface{}{
		"from_path":  r.From,
		"to_path":    r.To,
		"status":     r.Status,
		"enabled":    r.Enabled,
		"created_at": r.CreatedAt,
		"updated_at": r.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateRedirect", err)
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
	Reload             int // 1 表示该项配置修改后需重启应用才能生效
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

	// 如果是 password 类型，需要对 value 进行 bcrypt 加密
	if s.Type == "password" && s.Value != "" && len(s.Value) < 60 {
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(s.Value),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return 0, WrapInternalErr("CreateSetting.bcrypt", err)
		}
		s.Value = string(hashed)
	}

	id, err := Create(db, specSettings, map[string]interface{}{
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
		"reload":               s.Reload,
		"created_at":           s.CreatedAt,
		"updated_at":           s.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateSetting", err)
	}

	s.ID = id
	return id, nil
}

func GetSettingByCode(db *DB, code string) (*Setting, error) {
	result, err := Read(db, specSettings, ReadOptions{
		SelectFields: "id, kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, reload, created_at, updated_at, deleted_at",
		WhereClause:  "code=?",
		WhereArgs:    []interface{}{code},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		s, err := scanSetting(rows)
		if err != nil {
			return nil, WrapInternalErr("GetSettingByCode.Scan", err)
		}
		return s, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetSettingByCode", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetSettingByCode")
	}
	s := item.(Setting)
	return &s, nil
}

func GetSettingByID(db *DB, id int64) (*Setting, error) {
	result, err := Read(db, specSettings, ReadOptions{
		SelectFields: "id, kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, reload, created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		s, err := scanSetting(rows)
		if err != nil {
			return nil, WrapInternalErr("GetSettingByID.Scan", err)
		}
		return s, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetSettingByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetSettingByID")
	}
	s := item.(Setting)
	return &s, nil
}

func ListSettingsByKind(db *DB, kind string) ([]Setting, error) {
	whereClause := ""
	whereArgs := []interface{}{}
	if kind != "" {
		whereClause = "kind=?"
		whereArgs = append(whereArgs, kind)
	}

	results, err := Read(db, specSettings, ReadOptions{
		SelectFields: "id, kind, name, code, type, options, attrs, value, default_option_value, description, sort, charset, author, keywords, reload, created_at, updated_at, deleted_at",
		WhereClause:  whereClause,
		OrderBy:      "",
		WhereArgs:    whereArgs,
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		s, err := scanSetting(rows)
		if err != nil {
			return nil, WrapInternalErr("ListSettingsByKind.Scan", err)
		}
		return s, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListSettingsByKind", err)
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
	// 如果是 password 类型，需要对 value 进行 bcrypt 加密
	if s.Type == "password" && s.Value != "" && len(s.Value) < 60 {
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(s.Value),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return WrapInternalErr("UpdateSetting.bcrypt", err)
		}
		s.Value = string(hashed)
	}

	err := Update(db, specSettings, s.ID, map[string]interface{}{
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
		"reload":               s.Reload,
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
			return WrapInternalErr("UpdateSettingByCode.bcrypt", err)
		}
		value = string(hashed)
	}

	if err = Update(db, specSettings, setting.ID, map[string]interface{}{
		"value": value,
	}); err != nil {
		return WrapInternalErr("UpdateSettingByCode", err)
	}

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpUpdate)
	}

	return nil
}

func DeleteSetting(db *DB, id int64) error {
	if err := Delete(db, specSettings, id); err != nil {
		return WrapInternalErr("DeleteSetting", err)
	}

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpUpdate)
	}

	return nil
}

// CheckPassword 检查管理员密码
func CheckPassword(db *DB, raw string) error {
	setting, err := GetSettingByCode(db, "admin_password")
	if err != nil {
		return err
	}
	if setting.Value == "" {
		return ErrNotFound("CheckPassword")
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
		if !IsErrNotFound(err) {
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

	id, err := Create(db, specHttpErrorLogs, map[string]interface{}{
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
		return 0, WrapInternalErr("CreateHttpErrorLog", err)
	}
	l.ID = id
	return id, nil
}

func ListHttpErrorLogs(db *DB, limit, offset int) ([]HttpErrorLog, error) {
	results, err := Read(db, specHttpErrorLogs, ReadOptions{
		SelectFields: "id, req_id, client_ip, method, path, status, user_agent, query_params, body_params, created_at, expired_at",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        limit,
		Offset:       offset,
	}, func(rows *sql.Rows) (interface{}, error) {
		l, err := scanHttpErrorLog(rows)
		if err != nil {
			return nil, WrapInternalErr("ListHttpErrorLogs.Scan", err)
		}
		return l, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListHttpErrorLogs", err)
	}

	res := make([]HttpErrorLog, len(results))
	for i, v := range results {
		res[i] = v.(HttpErrorLog)
	}
	return res, nil
}

func CountHttpErrorLogs(db *DB) (int, error) {
	return Count(db, specHttpErrorLogs, "", nil)
}

func DeleteHttpErrorLog(db *DB, id int64) error {
	return HardDelete(db, specHttpErrorLogs, id)
}

var AppSettings atomic.Value // map[string]string

// LoadSettingsToMap 从 settings 表加载 code -> value 映射
func LoadSettingsToMap(db *DB) (map[string]string, error) {
	results, err := Read(db, specSettings, ReadOptions{
		SelectFields: "code, value",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		var code, value string
		if err := rows.Scan(&code, &value); err != nil {
			return nil, WrapInternalErr("LoadSettingsToMap.Scan", err)
		}
		return map[string]string{code: value}, nil
	})
	if err != nil {
		return nil, WrapInternalErr("LoadSettingsToMap", err)
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

	id, err := Create(db, specTasks, map[string]interface{}{
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
		return 0, WrapInternalErr("CreateTask", err)
	}
	task.ID = id
	return id, nil
}

func GetTaskByID(db *DB, id int64) (*Task, error) {
	result, err := Read(db, specTasks, ReadOptions{
		SelectFields: "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTask(rows)
		if err != nil {
			return nil, WrapInternalErr("GetTaskByID.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetTaskByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetTaskByID")
	}
	t := item.(Task)
	return &t, nil
}

func GetTaskByCode(db *DB, code string) (*Task, error) {
	result, err := Read(db, specTasks, ReadOptions{
		SelectFields: "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at",
		WhereClause:  "code=?",
		WhereArgs:    []interface{}{code},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTask(rows)
		if err != nil {
			return nil, WrapInternalErr("GetTaskByCode.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetTaskByCode", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetTaskByCode")
	}
	t := item.(Task)
	return &t, nil
}

func ListTasks(db *DB) ([]Task, error) {
	results, err := Read(db, specTasks, ReadOptions{
		SelectFields: "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTask(rows)
		if err != nil {
			return nil, WrapInternalErr("ListTasks.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListTasks", err)
	}
	res := make([]Task, len(results))
	for i, v := range results {
		res[i] = v.(Task)
	}
	return res, nil
}

func UpdateTask(db *DB, task *Task) error {
	enabled := 0
	if task.Enabled == 1 {
		enabled = 1
	}
	// Code 不可修改，不更新 code 字段
	return Update(db, specTasks, task.ID, map[string]interface{}{
		"name":        task.Name,
		"description": task.Description,
		"schedule":    task.Schedule,
		"enabled":     enabled,
		"kind":        task.Kind,
	})
}
func UpdateTaskStatus(db *DB, taskCode string, lastStatus string, lastRunAt int64) error {
	task, err := GetTaskByCode(db, taskCode)
	if err != nil {
		if IsErrNotFound(err) {
			return nil
		}
		return WrapInternalErr("UpdateTaskStatus.GetTaskByCode", err)
	}

	if err = Update(db, specTasks, task.ID, map[string]interface{}{
		"last_status": lastStatus,
		"last_run_at": lastRunAt,
	}); err != nil {
		return WrapInternalErr("UpdateTaskStatus", err)
	}
	return nil
}

func SoftDeleteTask(db *DB, id int64) error {
	return Delete(db, specTasks, id)
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

	id, err := Create(db, specTaskRuns, map[string]interface{}{
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
		return 0, WrapInternalErr("CreateTaskRun", err)
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

	results, err := Read(db, specTaskRuns, ReadOptions{
		SelectFields: "id, task_code, run_id, status, message, started_at, finished_at, duration, created_at",
		WhereClause:  whereClause,
		OrderBy:      "",
		WhereArgs:    whereArgs,
		Limit:        limit,
	}, func(rows *sql.Rows) (interface{}, error) {
		r, err := scanTaskRun(rows)
		if err != nil {
			return nil, WrapInternalErr("ListTaskRuns.Scan", err)
		}
		return r, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListTaskRuns", err)
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

	if err := Update(db, specTaskRuns, run.ID, map[string]interface{}{
		"status":      run.Status,
		"message":     run.Message,
		"finished_at": run.FinishedAt,
		"duration":    run.Duration,
	}); err != nil {
		return WrapInternalErr("UpdateTaskRunStatus", err)
	}

	if err := UpdateTaskStatus(db, run.TaskCode, run.Status, run.StartedAt); err != nil {
		return WrapInternalErr("UpdateTaskRunStatus.UpdateTaskStatus", err)
	}
	return nil
}

var errNotFoundSentinel = errors.New("not found")

// ErrNotFound 返回带标识的 not found 错误，便于排查来源；判断请用 IsErrNotFound(err)
func ErrNotFound(label string) error {
	return fmt.Errorf("%s: %w", label, errNotFoundSentinel)
}

var errInternalSentinel = errors.New("internal error")

// ErrInternalError 返回带标识的内部错误
func ErrInternalError(label string) error {
	return fmt.Errorf("%s: %w", label, errInternalSentinel)
}

// WrapInternalErr 包装 SQL 等执行产生的 error：先 log.Printf 再返回带 label 的包装错误，便于上层区分
func WrapInternalErr(label string, err error) error {
	if err == nil {
		return nil
	}
	log.Printf("[db] %s: %v", label, err)
	return fmt.Errorf("%s: %w", label, errors.Join(errInternalSentinel, err))
}

// IsErrInternalError 判断是否为内部错误（由 ErrInternalError 或包含 errInternalSentinel 的包装产生）
func IsErrInternalError(err error) bool {
	return errors.Is(err, errInternalSentinel)
}

// IsErrNotFound 判断是否为“未找到”错误
func IsErrNotFound(err error) bool {
	return errors.Is(err, errNotFoundSentinel)
}

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
	PostCount   int
}

func GetCategoryByID(db *DB, id int64) (*Category, error) {
	result, err := Read(db, specCategories, ReadOptions{
		SelectFields: "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, WrapInternalErr("GetCategoryByID.Scan", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetCategoryByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetCategoryByID")
	}
	c := item.(Category)
	return &c, nil
}

func GetCategoryBySlug(db *DB, slug string) (*Category, error) {
	result, err := Read(db, specCategories, ReadOptions{
		SelectFields: "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at",
		WhereClause:  "slug=?",
		WhereArgs:    []interface{}{slug},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, WrapInternalErr("GetCategoryBySlug.Scan", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetCategoryBySlug", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetCategoryBySlug")
	}
	c := item.(Category)
	return &c, nil
}

func CategoryExists(db *DB, id int64) (bool, error) {
	cnt, err := Count(db, specCategories, "id=?", []interface{}{id})
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
		return 0, WrapInternalErr("CreateCategory.CheckSlug", err)
	}

	var parentID interface{}
	if c.ParentID == 0 {
		parentID = nil
	} else {
		parentID = c.ParentID
	}

	id, err := Create(db, specCategories, map[string]interface{}{
		"parent_id":   parentID,
		"name":        c.Name,
		"slug":        c.Slug,
		"description": c.Description,
		"sort":        c.Sort,
		"created_at":  c.CreatedAt,
		"updated_at":  c.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateCategory", err)
	}
	c.ID = id

	// 如果 sort 为 0（默认值），则设置为 id
	if c.Sort == 0 {
		c.Sort = id
		if err = Update(db, specCategories, id, map[string]interface{}{
			"sort": c.Sort,
		}); err != nil {
			return 0, WrapInternalErr("CreateCategory.UpdateSort", err)
		}
	}
	return id, nil
}

func ListCategories(db *DB, withPostCount bool) ([]Category, error) {
	results, err := Read(db, specCategories, ReadOptions{
		SelectFields: "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, WrapInternalErr("ListCategories.Scan", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListCategories", err)
	}
	res := make([]Category, len(results))
	for i, v := range results {
		res[i] = v.(Category)
	}
	if withPostCount && len(res) > 0 {
		ids := make([]int64, len(res))
		for i := range res {
			ids[i] = res[i].ID
		}
		counts, err := CountPostsByCategories(db, ids)
		if err != nil {
			return nil, err
		}
		for i := range res {
			if n, ok := counts[res[i].ID]; ok {
				res[i].PostCount = n
			}
		}
	}
	return res, nil
}

func UpdateCategory(db *DB, c *Category) error {
	// 如果slug或parent_id改变了，需要检查唯一性
	var existingID int64
	var err error
	if c.ParentID == 0 {
		err = db.QueryRow(`
			SELECT id FROM `+string(TableCategories)+` WHERE (parent_id IS NULL OR parent_id=0) AND slug=? AND id!=?
		`, c.Slug, c.ID).Scan(&existingID)
	} else {
		err = db.QueryRow(`
			SELECT id FROM `+string(TableCategories)+` WHERE parent_id=? AND slug=? AND id!=?
		`, c.ParentID, c.Slug, c.ID).Scan(&existingID)
	}
	if err == nil {
		return errors.New("slug already exists under this parent")
	} else if err != sql.ErrNoRows {
		return WrapInternalErr("UpdateCategory.CheckSlug", err)
	}

	var parentID interface{}
	if c.ParentID == 0 {
		parentID = nil
	} else {
		parentID = c.ParentID
	}

	return Update(db, specCategories, c.ID, map[string]interface{}{
		"parent_id":   parentID,
		"name":        c.Name,
		"slug":        c.Slug,
		"description": c.Description,
		"sort":        c.Sort,
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
	all, err := ListCategories(db, false)
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

	if err = Update(db, specCategories, id, map[string]interface{}{
		"parent_id": parentID,
	}); err != nil {
		return WrapInternalErr("UpdateCategoryParent", err)
	}
	return nil
}

func SoftDeleteCategory(db *DB, id int64) error {
	return Delete(db, specCategories, id)
}

func HardDeleteCategory(db *DB, id int64) error {
	return HardDelete(db, specCategories, id)
}

func RestoreCategory(db *DB, id int64) error {
	return Restore(db, specCategories, id)
}

// UpdateCategoryCreatedAtIfEarlier 仅当 createdAt 早于当前 created_at 时更新（导入时用于“按最早出现该分类的文章创建时间”）
func UpdateCategoryCreatedAtIfEarlier(db *DB, id int64, createdAt int64) error {
	_, err := db.Exec(
		`UPDATE `+string(TableCategories)+` SET created_at = ? WHERE id = ? AND deleted_at IS NULL AND (created_at IS NULL OR created_at > ?)`,
		createdAt, id, createdAt,
	)
	if err != nil {
		return WrapInternalErr("UpdateCategoryCreatedAtIfEarlier", err)
	}
	return nil
}

func ListDeletedCategories(db *DB) ([]Category, error) {
	results, err := ListDeletedRecords(db, TableCategories, "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, WrapInternalErr("ListDeletedCategories.Scan", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListDeletedCategories", err)
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

	c, err := scanCategory(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, WrapInternalErr("GetPostCategory.Scan", err)
	}
	return &c, nil
}

func AttachCategoryToPost(db *DB, postID, categoryID int64) error {
	ts := now()
	tx, err := db.Begin()
	if err != nil {
		return WrapInternalErr("AttachCategoryToPost.Begin", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	// 单篇文章最多保留一个有效分类：先软删其他分类关联
	if _, err = tx.Exec(
		`UPDATE `+string(TablePostCategories)+`
		 SET deleted_at=?, updated_at=?
		 WHERE post_id=? AND category_id<>? AND deleted_at IS NULL`,
		ts, ts, postID, categoryID,
	); err != nil {
		return WrapInternalErr("AttachCategoryToPost.ClearOthers", err)
	}

	// 已存在激活关联时直接返回（幂等）
	res, err := tx.Exec(
		`UPDATE `+string(TablePostCategories)+`
		 SET updated_at=?
		 WHERE post_id=? AND category_id=? AND deleted_at IS NULL`,
		ts, postID, categoryID,
	)
	if err != nil {
		return WrapInternalErr("AttachCategoryToPost.TouchActive", err)
	}
	if rowsAffected, _ := res.RowsAffected(); rowsAffected > 0 {
		if err = tx.Commit(); err != nil {
			return WrapInternalErr("AttachCategoryToPost.CommitTouch", err)
		}
		committed = true
		return nil
	}

	// 优先恢复最近一条已软删除的关联，避免重复堆积历史行
	res, err = tx.Exec(
		`UPDATE `+string(TablePostCategories)+`
		 SET deleted_at=NULL, updated_at=?
		 WHERE id = (
			SELECT id FROM `+string(TablePostCategories)+`
			WHERE post_id=? AND category_id=? AND deleted_at IS NOT NULL
			ORDER BY id DESC
			LIMIT 1
		 )`,
		ts, postID, categoryID,
	)
	if err != nil {
		return WrapInternalErr("AttachCategoryToPost.Restore", err)
	}
	if rowsAffected, _ := res.RowsAffected(); rowsAffected > 0 {
		if err = tx.Commit(); err != nil {
			return WrapInternalErr("AttachCategoryToPost.CommitRestore", err)
		}
		committed = true
		return nil
	}

	if _, err = tx.Exec(
		`INSERT OR IGNORE INTO `+string(TablePostCategories)+`
		 (post_id, category_id, created_at, updated_at)
		 VALUES (?, ?, ?, ?)`,
		postID, categoryID, ts, ts,
	); err != nil {
		return WrapInternalErr("AttachCategoryToPost.Insert", err)
	}

	if err = tx.Commit(); err != nil {
		return WrapInternalErr("AttachCategoryToPost.CommitInsert", err)
	}
	committed = true
	return nil
}

func DetachCategoryFromPost(db *DB, postID, categoryID int64) error {
	ts := now()
	_, err := db.Exec(
		`UPDATE `+string(TablePostCategories)+` SET deleted_at=? WHERE post_id=? AND category_id=? AND deleted_at IS NULL`,
		ts, postID, categoryID,
	)
	if err != nil {
		return WrapInternalErr("DetachCategoryFromPost", err)
	}
	return nil
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
		return nil, WrapInternalErr("ExportSQLiteWithHash.checkpoint", err)
	}
	// 3. 导出
	if _, err = db.Exec("VACUUM INTO ?", tmpPath); err != nil {
		return nil, WrapInternalErr("ExportSQLiteWithHash.VACUUM", err)
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
