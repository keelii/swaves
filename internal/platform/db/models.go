package db

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/types"
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
	DSN string
}

type Options struct {
	DSN          string
	EnableSQLLog bool
}

type TableName string

var OnDatabaseChanged func(tableName TableName, kind TableOp)

func Open(opts Options) *DB {
	conn, err := OpenWithError(opts)
	if err != nil {
		logger.Fatal("open sqlite failed: %v", err)
	}
	return conn
}

func OpenWithError(opts Options) (*DB, error) {
	var sqlDB *sql.DB
	var err error

	sqlDB = sqldblogger.OpenDriver(opts.DSN, &sqlite3.SQLiteDriver{}, &SqlLogger{
		Enabled: opts.EnableSQLLog,
	})

	//sqlDB, err = sql.Open("sqlite3", opts.DSN)
	//if err != nil {
	//	logger.Fatal("open sqlite failed: %v", err)
	//}
	//
	_, err = sqlDB.Exec(`PRAGMA journal_mode = WAL; PRAGMA busy_timeout = 5000;`)
	if err != nil {
		_ = sqlDB.Close()
		return nil, WrapInternalErr("Open.pragma", err)
	}

	conn := &DB{DB: sqlDB, DSN: opts.DSN}

	if r2 := InitDatabase(conn); r2 != nil {
		_ = conn.Close()
		return nil, r2
	}

	return conn, nil
}

func InitDatabase(db *DB) error {
	stmts := []string{string(InitialSQL)}

	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return WrapInternalErr("InitDatabase", err)
		}
	}

	if config.ShouldEnsureDefaultSettings() {
		if err := EnsureDefaultSettings(db); err != nil {
			return WrapInternalErr("InitDatabase.EnsureDefaultSettings", err)
		}
	}

	return nil
}

func isBcryptHash(value string) bool {
	if value == "" {
		return false
	}
	_, err := bcrypt.Cost([]byte(value))
	return err == nil
}

func normalizeSettingValueForStorage(settingType string, value string) (string, error) {
	if settingType == "password" && value != "" && !isBcryptHash(value) {
		hashed, err := bcrypt.GenerateFromPassword(
			[]byte(value),
			bcrypt.DefaultCost,
		)
		if err != nil {
			return "", err
		}
		return string(hashed), nil
	}

	return value, nil
}

func normalizeSettingForWrite(s *Setting) error {
	if s == nil {
		return errors.New("setting is required")
	}
	if s.Code == "" {
		return errors.New("code is required")
	}
	if s.Type == "" {
		return errors.New("type is required")
	}
	if s.Kind == "" {
		s.Kind = "default"
	}

	value, err := normalizeSettingValueForStorage(s.Type, s.Value)
	if err != nil {
		return err
	}
	s.Value = value
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
	specComments = TableSpec{
		Name:         TableComments,
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
	specNotifications = TableSpec{
		Name:         TableNotifications,
		HasDeletedAt: false,
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
		logger.Fatal("db is nil")
	}
	if spec.Name == "" {
		logger.Fatal("table name is empty")
	}
	if len(data) == 0 {
		logger.Fatal("no data to insert")
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
	res, err := db.Exec(query, args...)
	if err != nil {
		return WrapInternalErr("Update", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return WrapInternalErr("Update.RowsAffected", err)
	}
	if affected == 0 {
		return ErrNotFound("Update")
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
	res, err := db.Exec(
		`UPDATE `+string(spec.Name)+` SET `+spec.deletedAtCol()+`=? WHERE `+spec.idField()+`=? AND `+spec.deletedAtCol()+` IS NULL`,
		ts, id,
	)
	if err != nil {
		return WrapInternalErr("Delete.Soft", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return WrapInternalErr("Delete.Soft.RowsAffected", err)
	}
	if affected == 0 {
		return ErrNotFound("Delete.Soft")
	}
	return nil
}

// HardDelete 物理删除
func HardDelete(db *DB, spec TableSpec, id int64) error {
	res, err := db.Exec(`DELETE FROM `+string(spec.Name)+` WHERE `+spec.idField()+`=?`, id)
	if err != nil {
		return WrapInternalErr("HardDelete", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return WrapInternalErr("HardDelete.RowsAffected", err)
	}
	if affected == 0 {
		return ErrNotFound("HardDelete")
	}
	return nil
}

// Restore 恢复软删除的记录
func Restore(db *DB, spec TableSpec, id int64) error {
	if !spec.HasDeletedAt {
		return nil
	}
	res, err := db.Exec(
		`UPDATE `+string(spec.Name)+` SET `+spec.deletedAtCol()+`=NULL WHERE `+spec.idField()+`=? AND `+spec.deletedAtCol()+` IS NOT NULL`,
		id,
	)
	if err != nil {
		return WrapInternalErr("Restore", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return WrapInternalErr("Restore.RowsAffected", err)
	}
	if affected == 0 {
		return ErrNotFound("Restore")
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
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind, &p.CommentEnabled,
			&p.CreatedAt, &p.UpdatedAt, &p.PublishedAt, &deletedAt,
		); err != nil {
			return Post{}, err
		}
	} else {
		if err := scanner.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Status, &p.Kind, &p.CommentEnabled,
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

func scanComment(scanner sqlScanner) (Comment, error) {
	var c Comment
	var deletedAt sql.NullInt64
	if err := scanner.Scan(
		&c.ID,
		&c.PostID,
		&c.ParentID,
		&c.Author,
		&c.AuthorEmail,
		&c.AuthorURL,
		&c.AuthorIP,
		&c.VisitorID,
		&c.UserAgent,
		&c.Content,
		&c.Status,
		&c.Type,
		&c.CreatedAt,
		&c.UpdatedAt,
		&deletedAt,
	); err != nil {
		return Comment{}, err
	}
	if deletedAt.Valid {
		c.DeletedAt = &deletedAt.Int64
	}
	return c, nil
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
		&s.SubKind,
		&s.Name,
		&s.Code,
		&s.Type,
		&s.Options,
		&s.Attrs,
		&s.Value,
		&s.DefaultOptionValue,
		&s.PrefixValue,
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
		&r.ID, &r.TaskCode, &r.Status, &r.Message,
		&r.StartedAt, &r.FinishedAt, &r.Duration, &r.CreatedAt,
	); err != nil {
		return TaskRun{}, err
	}
	return r, nil
}

func scanNotification(scanner sqlScanner) (Notification, error) {
	var n Notification
	var readAt sql.NullInt64
	if err := scanner.Scan(
		&n.ID,
		&n.Receiver,
		&n.EventType,
		&n.Level,
		&n.Title,
		&n.Body,
		&n.AggregateKey,
		&n.AggregateCount,
		&readAt,
		&n.CreatedAt,
		&n.UpdatedAt,
	); err != nil {
		return Notification{}, err
	}
	if readAt.Valid {
		n.ReadAt = &readAt.Int64
	}
	return n, nil
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
	ID          int64
	Title       string
	Slug        string
	Kind        PostKind
	PublishedAt int64
	UV          int
}

type UVCategoryRank struct {
	CategoryID int64
	Name       string
	Slug       string
	UV         int
}

type UVTagRank struct {
	TagID int64
	Name  string
	Slug  string
	UV    int
}

type UVBucketCount struct {
	BucketIndex int
	UV          int
}

type LikePostRank struct {
	PostID int64
	Title  string
	Slug   string
	Likes  int
}

type LikeContentRank struct {
	PostID int64
	Title  string
	Slug   string
	Kind   PostKind
	Likes  int
}

type LikeCategoryRank struct {
	CategoryID int64
	Name       string
	Slug       string
	Likes      int
}

type LikeTagRank struct {
	TagID int64
	Name  string
	Slug  string
	Likes int
}

type LikeStatus int

const (
	LikeStatusInactive LikeStatus = 0
	LikeStatusActive   LikeStatus = 1
)

func (s LikeStatus) IsValid() bool {
	return s == LikeStatusInactive || s == LikeStatusActive
}

type EntityLike struct {
	EntityID  int64
	VisitorID []byte
	Status    LikeStatus
	CreatedAt int64
	UpdatedAt int64
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

	// Legacy compat path when ON CONFLICT target is unavailable:
	// keep one-step writes to avoid read-then-write races.
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
	if affected > 0 {
		return true, nil
	}

	res, insertErr := db.Exec(
		`INSERT INTO `+string(TableUVUnique)+` (entity_type, entity_id, visitor_id, first_seen_at, last_seen_at)
		SELECT ?, ?, ?, ?, ?
		WHERE NOT EXISTS (
			SELECT 1
			FROM `+string(TableUVUnique)+`
			WHERE entity_type = ? AND entity_id = ? AND visitor_id = ?
		)`,
		entityType, entityID, visitorIDBytes, ts, ts,
		entityType, entityID, visitorIDBytes,
	)
	if insertErr != nil {
		return false, WrapInternalErr("UpsertUVUnique.CompatInsert", insertErr)
	}
	affected, rowsErr = res.RowsAffected()
	if rowsErr != nil {
		return false, WrapInternalErr("UpsertUVUnique.CompatInsertRowsAffected", rowsErr)
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

func CountActiveVisitors(db *DB, sinceSeconds int64) (int, error) {
	if sinceSeconds <= 0 {
		sinceSeconds = 30 * 24 * 60 * 60
	}

	threshold := now() - sinceSeconds
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(DISTINCT visitor_id)
		FROM `+string(TableUVUnique)+`
		WHERE last_seen_at >= ?`,
		threshold,
	).Scan(&count); err != nil {
		return 0, WrapInternalErr("CountActiveVisitors", err)
	}

	return count, nil
}

func CountDistinctVisitorsBetween(db *DB, startAt, endAt int64) (int, error) {
	if endAt <= startAt {
		return 0, nil
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(DISTINCT visitor_id)
		FROM `+string(TableUVUnique)+`
		WHERE last_seen_at >= ? AND last_seen_at < ?`,
		startAt, endAt,
	).Scan(&count); err != nil {
		return 0, WrapInternalErr("CountDistinctVisitorsBetween", err)
	}

	return count, nil
}

func ListDistinctVisitorsByBucket(db *DB, startAt, endAt, bucketSeconds int64) ([]UVBucketCount, error) {
	if endAt <= startAt || bucketSeconds <= 0 {
		return []UVBucketCount{}, nil
	}

	rows, err := db.Query(
		`SELECT CAST((last_seen_at - ?) / ? AS INTEGER) AS bucket_index,
			COUNT(DISTINCT visitor_id) AS uv
		FROM `+string(TableUVUnique)+`
		WHERE last_seen_at >= ? AND last_seen_at < ?
		GROUP BY bucket_index
		ORDER BY bucket_index ASC`,
		startAt, bucketSeconds, startAt, endAt,
	)
	if err != nil {
		return nil, WrapInternalErr("ListDistinctVisitorsByBucket.Query", err)
	}
	defer rows.Close()

	res := make([]UVBucketCount, 0, 32)
	for rows.Next() {
		var item UVBucketCount
		if err = rows.Scan(&item.BucketIndex, &item.UV); err != nil {
			return nil, WrapInternalErr("ListDistinctVisitorsByBucket.Scan", err)
		}
		res = append(res, item)
	}

	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("ListDistinctVisitorsByBucket.Rows", err)
	}

	return res, nil
}

func CountPostUVByIDs(db *DB, postIDs []int64) (map[int64]int, error) {
	result := make(map[int64]int, len(postIDs))
	if len(postIDs) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(postIDs))
	args := make([]interface{}, 0, len(postIDs)+1)
	args = append(args, UVEntityPost)
	for i, id := range postIDs {
		placeholders[i] = "?"
		args = append(args, id)
		result[id] = 0
	}

	query := fmt.Sprintf(
		`SELECT entity_id, COUNT(*) AS uv
		FROM %s
		WHERE entity_type = ?
			AND entity_id IN (%s)
		GROUP BY entity_id`,
		string(TableUVUnique),
		strings.Join(placeholders, ","),
	)

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, WrapInternalErr("CountPostUVByIDs.Query", err)
	}
	defer rows.Close()

	for rows.Next() {
		var postID int64
		var uv int
		if err = rows.Scan(&postID, &uv); err != nil {
			return nil, WrapInternalErr("CountPostUVByIDs.Scan", err)
		}
		result[postID] = uv
	}

	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("CountPostUVByIDs.Rows", err)
	}

	return result, nil
}

func ListTopUVPosts(db *DB, limit int) ([]UVPostRank, error) {
	return listTopUVPostsByKind(db, PostKindPost, limit)
}

func ListTopUVPages(db *DB, limit int) ([]UVPostRank, error) {
	return listTopUVPostsByKind(db, PostKindPage, limit)
}

func ListTopUVContents(db *DB, limit int) ([]UVPostRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT p.id, p.title, p.slug, p.kind, p.published_at, COUNT(*) AS uv
		FROM `+string(TableUVUnique)+` AS u
		INNER JOIN `+string(TablePosts)+` AS p
			ON p.id = u.entity_id
		WHERE u.entity_type = ?
			AND p.deleted_at IS NULL
			AND p.status = ?
		GROUP BY p.id, p.title, p.slug, p.kind, p.published_at
		ORDER BY uv DESC, p.published_at DESC, p.id DESC
		LIMIT ?`,
		UVEntityPost, "published", limit,
	)
	if err != nil {
		return nil, WrapInternalErr("ListTopUVContents.Query", err)
	}
	defer rows.Close()

	res := make([]UVPostRank, 0, limit)
	for rows.Next() {
		var item UVPostRank
		if err = rows.Scan(&item.ID, &item.Title, &item.Slug, &item.Kind, &item.PublishedAt, &item.UV); err != nil {
			return nil, WrapInternalErr("ListTopUVContents.Scan", err)
		}
		res = append(res, item)
	}
	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("ListTopUVContents.Rows", err)
	}

	return res, nil
}

func ListTopUVCategories(db *DB, limit int) ([]UVCategoryRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT c.id, c.name, c.slug, u.uv
		FROM (
			SELECT entity_id, COUNT(*) AS uv
			FROM `+string(TableUVUnique)+`
			WHERE entity_type = ?
			GROUP BY entity_id
			ORDER BY uv DESC
			LIMIT ?
		) AS u
		INNER JOIN `+string(TableCategories)+` AS c
			ON c.id = u.entity_id
		WHERE c.deleted_at IS NULL
		ORDER BY u.uv DESC, c.sort ASC, c.id ASC`,
		UVEntityCategory, limit,
	)
	if err != nil {
		return nil, WrapInternalErr("ListTopUVCategories.Query", err)
	}
	defer rows.Close()

	res := make([]UVCategoryRank, 0, limit)
	for rows.Next() {
		var item UVCategoryRank
		if err = rows.Scan(&item.CategoryID, &item.Name, &item.Slug, &item.UV); err != nil {
			return nil, WrapInternalErr("ListTopUVCategories.Scan", err)
		}
		res = append(res, item)
	}

	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("ListTopUVCategories.Rows", err)
	}

	return res, nil
}

func ListTopUVTags(db *DB, limit int) ([]UVTagRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT t.id, t.name, t.slug, u.uv
		FROM (
			SELECT entity_id, COUNT(*) AS uv
			FROM `+string(TableUVUnique)+`
			WHERE entity_type = ?
			GROUP BY entity_id
			ORDER BY uv DESC
			LIMIT ?
		) AS u
		INNER JOIN `+string(TableTags)+` AS t
			ON t.id = u.entity_id
		WHERE t.deleted_at IS NULL
		ORDER BY u.uv DESC, t.id DESC`,
		UVEntityTag, limit,
	)
	if err != nil {
		return nil, WrapInternalErr("ListTopUVTags.Query", err)
	}
	defer rows.Close()

	res := make([]UVTagRank, 0, limit)
	for rows.Next() {
		var item UVTagRank
		if err = rows.Scan(&item.TagID, &item.Name, &item.Slug, &item.UV); err != nil {
			return nil, WrapInternalErr("ListTopUVTags.Scan", err)
		}
		res = append(res, item)
	}

	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("ListTopUVTags.Rows", err)
	}

	return res, nil
}

func ListTopLikedPosts(db *DB, limit int) ([]LikePostRank, error) {
	return listTopLikedPostsByKind(db, PostKindPost, limit)
}

func ListTopLikedPages(db *DB, limit int) ([]LikePostRank, error) {
	return listTopLikedPostsByKind(db, PostKindPage, limit)
}

func ListTopLikedContents(db *DB, limit int) ([]LikeContentRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT p.id, p.title, p.slug, p.kind, COUNT(*) AS likes
		FROM `+string(TableLikes)+` AS l
		INNER JOIN `+string(TablePosts)+` AS p
			ON p.id = l.entity_id
		WHERE l.status = ?
			AND p.deleted_at IS NULL
			AND p.status = ?
		GROUP BY p.id, p.title, p.slug, p.kind, p.published_at
		ORDER BY likes DESC, p.published_at DESC, p.id DESC
		LIMIT ?`,
		LikeStatusActive, "published", limit,
	)
	if err != nil {
		return nil, WrapInternalErr("ListTopLikedContents.Query", err)
	}
	defer rows.Close()

	res := make([]LikeContentRank, 0, limit)
	for rows.Next() {
		var item LikeContentRank
		if err = rows.Scan(&item.PostID, &item.Title, &item.Slug, &item.Kind, &item.Likes); err != nil {
			return nil, WrapInternalErr("ListTopLikedContents.Scan", err)
		}
		res = append(res, item)
	}
	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("ListTopLikedContents.Rows", err)
	}

	return res, nil
}

func listTopLikedPostsByKind(db *DB, kind PostKind, limit int) ([]LikePostRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT p.id, p.title, p.slug, l.likes
		FROM (
			SELECT entity_id, COUNT(*) AS likes
			FROM `+string(TableLikes)+`
			WHERE status = ?
			GROUP BY entity_id
			ORDER BY likes DESC
			LIMIT ?
		) AS l
		INNER JOIN `+string(TablePosts)+` AS p
			ON p.id = l.entity_id
		WHERE p.deleted_at IS NULL
			AND p.kind = ?
			AND p.status = ?
		ORDER BY l.likes DESC, p.published_at DESC, p.id DESC`,
		LikeStatusActive, limit, kind, "published",
	)
	if err != nil {
		return nil, WrapInternalErr("listTopLikedPostsByKind.Query", err)
	}
	defer rows.Close()

	res := make([]LikePostRank, 0, limit)
	for rows.Next() {
		var item LikePostRank
		if err = rows.Scan(&item.PostID, &item.Title, &item.Slug, &item.Likes); err != nil {
			return nil, WrapInternalErr("listTopLikedPostsByKind.Scan", err)
		}
		res = append(res, item)
	}
	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("listTopLikedPostsByKind.Rows", err)
	}

	return res, nil
}

func listTopUVPostsByKind(db *DB, kind PostKind, limit int) ([]UVPostRank, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := db.Query(
		`SELECT p.id, p.title, p.slug, p.published_at, u.uv
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
		if err = rows.Scan(&item.ID, &item.Title, &item.Slug, &item.PublishedAt, &item.UV); err != nil {
			return nil, WrapInternalErr("listTopUVPostsByKind.Scan", err)
		}
		item.Kind = kind
		res = append(res, item)
	}
	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("listTopUVPostsByKind.Rows", err)
	}

	return res, nil
}

func UpsertEntityLike(db *DB, postID int64, visitorID string, status LikeStatus) error {
	if postID <= 0 {
		return errors.New("post_id is invalid")
	}
	if !status.IsValid() {
		return errors.New("like status is invalid")
	}

	visitorIDBytes, err := parseVisitorIDBytes(visitorID)
	if err != nil {
		return err
	}

	ts := now()
	_, err = db.Exec(
		`INSERT INTO `+string(TableLikes)+` (entity_id, visitor_id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(entity_id, visitor_id) DO UPDATE SET
			status = excluded.status,
			updated_at = excluded.updated_at`,
		postID, visitorIDBytes, status, ts, ts,
	)
	if err != nil {
		return WrapInternalErr("UpsertEntityLike", err)
	}

	return nil
}

func CountEntityLikes(db *DB, postID int64) (int, error) {
	if postID <= 0 {
		return 0, errors.New("post_id is invalid")
	}

	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*)
		FROM `+string(TableLikes)+`
		WHERE entity_id = ?
			AND status = ?`,
		postID, LikeStatusActive,
	).Scan(&count); err != nil {
		return 0, WrapInternalErr("CountEntityLikes", err)
	}
	return count, nil
}

func CountTotalLikes(db *DB) (int, error) {
	var count int
	if err := db.QueryRow(
		`SELECT COUNT(*)
		FROM `+string(TableLikes)+`
		WHERE status = ?`,
		LikeStatusActive,
	).Scan(&count); err != nil {
		return 0, WrapInternalErr("CountTotalLikes", err)
	}
	return count, nil
}

func IsEntityLikedByVisitor(db *DB, postID int64, visitorID string) (bool, error) {
	if postID <= 0 {
		return false, errors.New("post_id is invalid")
	}

	visitorIDBytes, err := parseVisitorIDBytes(visitorID)
	if err != nil {
		return false, err
	}

	var status int
	err = db.QueryRow(
		`SELECT status
		FROM `+string(TableLikes)+`
		WHERE entity_id = ?
			AND visitor_id = ?
		LIMIT 1`,
		postID, visitorIDBytes,
	).Scan(&status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, WrapInternalErr("IsEntityLikedByVisitor", err)
	}

	return status == int(LikeStatusActive), nil
}

// PostKind 文章类型
type PostKind int

const (
	PostKindPost PostKind = 0 // 文章
	PostKindPage PostKind = 1 // 页面
)

type Post struct {
	ID             int64
	Title          string
	Slug           string
	Content        string
	Status         string
	Kind           PostKind
	CommentEnabled int
	PublishedAt    int64 // 首次发布时间，0 表示未发布
	CreatedAt      int64
	UpdatedAt      int64
	DeletedAt      *int64
}

type PostWithRelation struct {
	Post     *Post
	Tags     []Tag
	Category *Category
}

type CommentStatus string

const (
	CommentStatusPending  CommentStatus = "pending"
	CommentStatusApproved CommentStatus = "approved"
	CommentStatusSpam     CommentStatus = "spam"
)

func (s CommentStatus) IsValid() bool {
	switch s {
	case CommentStatusPending, CommentStatusApproved, CommentStatusSpam:
		return true
	default:
		return false
	}
}

type Comment struct {
	ID          int64
	PostID      int64
	PostKind    PostKind
	PostPubAt   int64
	ParentID    int64
	Author      string
	AuthorEmail string
	AuthorURL   string
	AuthorIP    string
	VisitorID   string
	UserAgent   string
	Content     string
	Status      CommentStatus
	Type        string
	CreatedAt   int64
	UpdatedAt   int64
	DeletedAt   *int64

	PostTitle    string
	PostSlug     string
	ParentAuthor string
}

func buildCommentDuplicateIdentity(c *Comment) (string, []interface{}) {
	conditions := make([]string, 0, 3)
	args := make([]interface{}, 0, 6)

	visitorID := strings.TrimSpace(c.VisitorID)
	if visitorID != "" {
		conditions = append(conditions, "visitor_id = ?")
		args = append(args, visitorID)
	}

	author := strings.TrimSpace(c.Author)
	authorEmail := strings.TrimSpace(strings.ToLower(c.AuthorEmail))
	if authorEmail != "" {
		conditions = append(conditions, "(author = ? AND lower(author_email) = ?)")
		args = append(args, author, authorEmail)
	}

	authorIP := strings.TrimSpace(c.AuthorIP)
	if authorIP != "" {
		conditions = append(conditions, "(author = ? AND author_ip = ?)")
		args = append(args, author, authorIP)
	}

	if len(conditions) == 0 {
		return "1 = 0", nil
	}

	return "(" + strings.Join(conditions, " OR ") + ")", args
}

func CreateComment(db *DB, c *Comment) (int64, error) {
	if c.PostID <= 0 {
		return 0, errors.New("post_id is required")
	}
	c.Author = strings.TrimSpace(c.Author)
	if c.Author == "" {
		return 0, errors.New("author is required")
	}
	c.Content = strings.TrimSpace(c.Content)
	if c.Content == "" {
		return 0, errors.New("content is required")
	}
	if c.ParentID < 0 {
		c.ParentID = 0
	}
	if !c.Status.IsValid() {
		c.Status = CommentStatusPending
	}
	if c.Type == "" {
		c.Type = "comment"
	}

	userClause, userArgs := buildCommentDuplicateIdentity(c)
	duplicateArgs := make([]interface{}, 0, 2+len(userArgs))
	duplicateArgs = append(duplicateArgs, c.PostID, c.Content)
	duplicateArgs = append(duplicateArgs, userArgs...)

	query := `
		SELECT id
		FROM ` + string(TableComments) + `
		WHERE post_id = ?
			AND content = ?
			AND deleted_at IS NULL
			AND ` + userClause + `
		LIMIT 1
	`
	var duplicatedCommentID int64
	checkErr := db.QueryRow(query, duplicateArgs...).Scan(&duplicatedCommentID)
	if checkErr == nil {
		return 0, ErrDuplicateComment("CreateComment")
	}
	if !errors.Is(checkErr, sql.ErrNoRows) {
		return 0, WrapInternalErr("CreateComment.DuplicateCheck", checkErr)
	}

	id, err := Create(db, specComments, map[string]interface{}{
		"post_id":      c.PostID,
		"parent_id":    c.ParentID,
		"author":       c.Author,
		"author_email": c.AuthorEmail,
		"author_url":   c.AuthorURL,
		"author_ip":    c.AuthorIP,
		"visitor_id":   c.VisitorID,
		"user_agent":   c.UserAgent,
		"content":      c.Content,
		"status":       c.Status,
		"type":         c.Type,
		"created_at":   c.CreatedAt,
		"updated_at":   c.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateComment", err)
	}
	c.ID = id
	return id, nil
}

func GetCommentByID(db *DB, id int64) (*Comment, error) {
	result, err := Read(db, specComments, ReadOptions{
		SelectFields: "id, post_id, parent_id, author, author_email, author_url, author_ip, visitor_id, user_agent, content, status, type, created_at, updated_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		item, scanErr := scanComment(rows)
		if scanErr != nil {
			return nil, WrapInternalErr("GetCommentByID.Scan", scanErr)
		}
		return item, nil
	})
	if err != nil {
		return nil, WrapInternalErr("GetCommentByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return nil, ErrNotFound("GetCommentByID")
	}
	comment := item.(Comment)
	return &comment, nil
}

func ListPostComments(db *DB, postID int64, status CommentStatus) ([]Comment, error) {
	whereClause := "post_id=?"
	whereArgs := []interface{}{postID}
	if status != "" {
		whereClause = appendWhere(whereClause, "status=?")
		whereArgs = append(whereArgs, status)
	}

	results, err := Read(db, specComments, ReadOptions{
		SelectFields: "id, post_id, parent_id, author, author_email, author_url, author_ip, visitor_id, user_agent, content, status, type, created_at, updated_at, deleted_at",
		WhereClause:  whereClause,
		WhereArgs:    whereArgs,
		OrderBy:      "created_at ASC, id ASC",
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		item, scanErr := scanComment(rows)
		if scanErr != nil {
			return nil, WrapInternalErr("ListPostComments.Scan", scanErr)
		}
		return item, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListPostComments", err)
	}

	items := make([]Comment, len(results))
	for i, v := range results {
		items[i] = v.(Comment)
	}
	return items, nil
}

func ListApprovedPostComments(db *DB, postID int64) ([]Comment, error) {
	return ListPostComments(db, postID, CommentStatusApproved)
}

func CountPostComments(db *DB, postID int64, status CommentStatus) (int, error) {
	whereClause := "post_id=?"
	whereArgs := []interface{}{postID}
	if status != "" {
		whereClause = appendWhere(whereClause, "status=?")
		whereArgs = append(whereArgs, status)
	}
	return Count(db, specComments, whereClause, whereArgs)
}

func ListCommentsForDash(db *DB, status CommentStatus, limit, offset int) ([]Comment, int, error) {
	whereClause := "c.deleted_at IS NULL"
	whereArgs := make([]interface{}, 0, 2)
	if status != "" {
		whereClause += " AND c.status = ?"
		whereArgs = append(whereArgs, status)
	}

	totalSQL := `SELECT COUNT(*) FROM ` + string(TableComments) + ` c WHERE ` + whereClause
	var total int
	if err := db.QueryRow(totalSQL, whereArgs...).Scan(&total); err != nil {
		return nil, 0, WrapInternalErr("ListCommentsForDash.Count", err)
	}

	query := `
		SELECT
			c.id, c.post_id, c.parent_id, c.author, c.author_email, c.author_url, c.author_ip, c.visitor_id, c.user_agent, c.content, c.status, c.type, c.created_at, c.updated_at, c.deleted_at,
			COALESCE(p.kind, 0), COALESCE(p.published_at, 0), COALESCE(p.title, ''), COALESCE(p.slug, ''), COALESCE(pc.author, '')
		FROM ` + string(TableComments) + ` c
		LEFT JOIN ` + string(TablePosts) + ` p ON p.id = c.post_id
		LEFT JOIN ` + string(TableComments) + ` pc ON pc.id = c.parent_id
		WHERE ` + whereClause + `
		ORDER BY c.created_at DESC, c.id DESC
		LIMIT ? OFFSET ?
	`

	listArgs := append([]interface{}{}, whereArgs...)
	listArgs = append(listArgs, limit, offset)

	rows, err := db.Query(query, listArgs...)
	if err != nil {
		return nil, 0, WrapInternalErr("ListCommentsForDash.Query", err)
	}
	defer rows.Close()

	items := make([]Comment, 0)
	for rows.Next() {
		var (
			item      Comment
			deletedAt sql.NullInt64
		)
		if scanErr := rows.Scan(
			&item.ID,
			&item.PostID,
			&item.ParentID,
			&item.Author,
			&item.AuthorEmail,
			&item.AuthorURL,
			&item.AuthorIP,
			&item.VisitorID,
			&item.UserAgent,
			&item.Content,
			&item.Status,
			&item.Type,
			&item.CreatedAt,
			&item.UpdatedAt,
			&deletedAt,
			&item.PostKind,
			&item.PostPubAt,
			&item.PostTitle,
			&item.PostSlug,
			&item.ParentAuthor,
		); scanErr != nil {
			return nil, 0, WrapInternalErr("ListCommentsForDash.Scan", scanErr)
		}
		if deletedAt.Valid {
			item.DeletedAt = &deletedAt.Int64
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		return nil, 0, WrapInternalErr("ListCommentsForDash.Rows", err)
	}

	return items, total, nil
}

func UpdateCommentStatus(db *DB, id int64, status CommentStatus) error {
	if !status.IsValid() {
		return errors.New("invalid comment status")
	}

	if err := Update(db, specComments, id, map[string]interface{}{
		"status": status,
	}); err != nil {
		return WrapInternalErr("UpdateCommentStatus", err)
	}
	return nil
}

func SoftDeleteComment(db *DB, id int64) error {
	if err := Delete(db, specComments, id); err != nil {
		return WrapInternalErr("SoftDeleteComment", err)
	}
	return nil
}

func CreatePost(db *DB, p *Post) (int64, error) {
	if p.Status == "published" && p.PublishedAt == 0 {
		p.PublishedAt = now()
	}
	if p.CommentEnabled == 0 {
		p.CommentEnabled = 1
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

func GetPostByID(db *DB, id int64) (Post, error) {
	var p Post
	result, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "id=? AND status=? AND published_at>0",
		WhereArgs:    []interface{}{id, "published"},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		p2, err := scanPost(rows, true)
		if err != nil {
			return nil, WrapInternalErr("GetPostByID.Scan", err)
		}
		return p2, nil
	})
	if err != nil {
		return Post{}, WrapInternalErr("GetPostByID", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return Post{}, ErrNotFound("GetPostByID")
	}
	p = item.(Post)
	return p, nil
}

func GetPostByIDAnyStatus(db *DB, id int64) (Post, error) {
	var p Post
	result, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		p2, err := scanPost(rows, true)
		if err != nil {
			return nil, WrapInternalErr("GetPostByIDAnyStatus.Scan", err)
		}
		return p2, nil
	})
	if err != nil {
		return Post{}, WrapInternalErr("GetPostByIDAnyStatus", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return Post{}, ErrNotFound("GetPostByIDAnyStatus")
	}
	p = item.(Post)
	return p, nil
}

func UpdatePost(db *DB, p *Post) error {
	data := map[string]interface{}{
		"title":           p.Title,
		"content":         p.Content,
		"kind":            p.Kind,
		"comment_enabled": p.CommentEnabled,
	}
	if err := Update(db, specPosts, p.ID, data); err != nil {
		return err
	}
	return nil
}

func SetPostCommentEnabled(db *DB, id int64, enabled bool) error {
	commentEnabled := 0
	if enabled {
		commentEnabled = 1
	}
	if err := Update(db, specPosts, id, map[string]interface{}{
		"comment_enabled": commentEnabled,
	}); err != nil {
		return WrapInternalErr("SetPostCommentEnabled", err)
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
func CountEncryptedPostsByKind(db *DB) (int, error) {
	return Count(db, specEncryptedPosts, "", []interface{}{})
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
		pager.Page = config.DefaultPage
	}
	if pager.PageSize < 1 {
		pager.PageSize = config.DefaultPageSize
	}

	total, err := Count(db, specPosts, "status = ? AND kind = ?", []interface{}{"published", kind})
	if err != nil {
		logger.Error("[db] ListPublishedPosts Count: %v", err)
		return []Post{}
	}
	if kind == PostKindPage {
		pager.Page = 1
		pager.PageSize = 1024
	}
	offset := (pager.Page - 1) * pager.PageSize
	opts := ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
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
		logger.Error("[db] ListPublishedPosts Read: %v", err)
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
		SelectFields: "id, title, slug, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
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
		logger.Error("[db] ListPublishedPages Read: %v", err)
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
		opts.Pager.Page = config.DefaultPage
	}
	if opts.Pager.PageSize < 1 {
		opts.Pager.PageSize = config.DefaultPageSize
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
	selectFields := "id, title, slug, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at"
	if opts.WithContent {
		selectFields = "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at"
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

func queryInt64ListTx(tx *sql.Tx, op string, query string, args ...interface{}) ([]int64, error) {
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, WrapInternalErr(op+".Query", err)
	}
	defer rows.Close()

	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			return nil, WrapInternalErr(op+".Scan", err)
		}
		ids = append(ids, id)
	}
	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr(op+".RowsErr", err)
	}
	return ids, nil
}

func buildIDPlaceholders(ids []int64) (string, []interface{}) {
	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}
	return strings.Join(placeholders, ","), args
}

// CancelPostsByStatus 删除指定 status 的文章及其关联，并清理仅由这些文章引入且已无任何关联的标签/分类。
func CancelPostsByStatus(db *DB, status string) (int64, error) {
	status = strings.TrimSpace(status)
	if status == "" {
		return 0, errors.New("status is required")
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, WrapInternalErr("CancelPostsByStatus.Begin", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	postIDs, err := queryInt64ListTx(tx, "CancelPostsByStatus.ListPostIDs", `
		SELECT id
		FROM `+string(TablePosts)+`
		WHERE status = ? AND deleted_at IS NULL
	`, status)
	if err != nil {
		return 0, err
	}
	if len(postIDs) == 0 {
		if err = tx.Commit(); err != nil {
			return 0, WrapInternalErr("CancelPostsByStatus.CommitEmpty", err)
		}
		committed = true
		return 0, nil
	}

	postPlaceholders, postArgs := buildIDPlaceholders(postIDs)

	tagIDs, err := queryInt64ListTx(tx, "CancelPostsByStatus.ListTagIDs", `
		SELECT DISTINCT tag_id
		FROM `+string(TablePostTags)+`
		WHERE deleted_at IS NULL
		  AND post_id IN (`+postPlaceholders+`)
	`, postArgs...)
	if err != nil {
		return 0, err
	}

	categoryIDs, err := queryInt64ListTx(tx, "CancelPostsByStatus.ListCategoryIDs", `
		SELECT DISTINCT category_id
		FROM `+string(TablePostCategories)+`
		WHERE deleted_at IS NULL
		  AND post_id IN (`+postPlaceholders+`)
	`, postArgs...)
	if err != nil {
		return 0, err
	}

	if _, err = tx.Exec(`
		DELETE FROM `+string(TablePostTags)+`
		WHERE post_id IN (`+postPlaceholders+`)
	`, postArgs...); err != nil {
		return 0, WrapInternalErr("CancelPostsByStatus.DeletePostTags", err)
	}

	if _, err = tx.Exec(`
		DELETE FROM `+string(TablePostCategories)+`
		WHERE post_id IN (`+postPlaceholders+`)
	`, postArgs...); err != nil {
		return 0, WrapInternalErr("CancelPostsByStatus.DeletePostCategories", err)
	}

	res, err := tx.Exec(`
		DELETE FROM `+string(TablePosts)+`
		WHERE id IN (`+postPlaceholders+`)
	`, postArgs...)
	if err != nil {
		return 0, WrapInternalErr("CancelPostsByStatus.DeletePosts", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return 0, WrapInternalErr("CancelPostsByStatus.RowsAffected", err)
	}

	if len(tagIDs) > 0 {
		tagPlaceholders, tagArgs := buildIDPlaceholders(tagIDs)
		if _, err = tx.Exec(`
			DELETE FROM `+string(TableTags)+`
			WHERE id IN (`+tagPlaceholders+`)
			  AND deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM `+string(TablePostTags)+` pt
				WHERE pt.tag_id = `+string(TableTags)+`.id
				  AND pt.deleted_at IS NULL
			  )
		`, tagArgs...); err != nil {
			return 0, WrapInternalErr("CancelPostsByStatus.DeleteUnusedTags", err)
		}
	}

	if len(categoryIDs) > 0 {
		categoryPlaceholders, categoryArgs := buildIDPlaceholders(categoryIDs)
		if _, err = tx.Exec(`
			DELETE FROM `+string(TableCategories)+`
			WHERE id IN (`+categoryPlaceholders+`)
			  AND deleted_at IS NULL
			  AND NOT EXISTS (
				SELECT 1
				FROM `+string(TablePostCategories)+` pc
				WHERE pc.category_id = `+string(TableCategories)+`.id
				  AND pc.deleted_at IS NULL
			  )
			  AND NOT EXISTS (
				SELECT 1
				FROM `+string(TableCategories)+` child
				WHERE child.parent_id = `+string(TableCategories)+`.id
				  AND child.deleted_at IS NULL
			  )
		`, categoryArgs...); err != nil {
			return 0, WrapInternalErr("CancelPostsByStatus.DeleteUnusedCategories", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return 0, WrapInternalErr("CancelPostsByStatus.Commit", err)
	}
	committed = true
	return affected, nil
}

func ListDeletedPosts(db *DB) ([]Post, error) {
	results, err := ListDeletedRecords(db, TablePosts, "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at", "deleted_at DESC", func(rows *sql.Rows) (interface{}, error) {
		var p Post
		var deletedAt sql.NullInt64
		if err := rows.Scan(
			&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind, &p.CommentEnabled,
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
		SelectFields: "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
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
func GetPostByTitle(db *DB, title string) (Post, error) {
	var p Post
	result, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "title=? AND status=? AND published_at>0",
		WhereArgs:    []interface{}{title, "published"},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		p2, err := scanPost(rows, true)
		if err != nil {
			return nil, WrapInternalErr("GetPostByTitle.Scan", err)
		}
		return p2, nil
	})
	if err != nil {
		return Post{}, WrapInternalErr("GetPostByTitle", err)
	}
	item, ok := firstResult(result)
	if !ok {
		return Post{}, ErrNotFound("GetPostByTitle")
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
	return toPostWithRelation(db, err, p)
}
func GetPostByTitleWithRelation(db *DB, title string) (PostWithRelation, error) {
	p, err := GetPostByTitle(db, title)
	if err != nil {
		return PostWithRelation{}, err
	}
	return toPostWithRelation(db, err, p)
}

func GetPostByIDWithRelation(db *DB, id int64) (PostWithRelation, error) {
	p, err := GetPostByID(db, id)
	if err != nil {
		return PostWithRelation{}, err
	}
	return toPostWithRelation(db, err, p)
}

func toPostWithRelation(db *DB, err error, p Post) (PostWithRelation, error) {
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
			SelectFields: "id, title, slug, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
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
	selectFields := "id, title, slug, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at"
	if opts.WithContent {
		selectFields = "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at"
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
				&p.ID, &p.Title, &p.Slug, &p.Content, &p.Status, &p.Kind, &p.CommentEnabled,
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
				&p.ID, &p.Title, &p.Slug, &p.Status, &p.Kind, &p.CommentEnabled,
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
	if err := Update(db, specRedirects, r.ID, map[string]interface{}{
		"from_path": r.From,
		"to_path":   r.To,
		"status":    r.Status,
		"enabled":   r.Enabled,
	}); err != nil {
		return err
	}
	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableRedirects, TableOpUpdate)
	}
	return nil
}

func SoftDeleteRedirect(db *DB, id int64) error {
	if err := Delete(db, specRedirects, id); err != nil {
		return err
	}
	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableRedirects, TableOpDelete)
	}
	return nil
}

func HardDeleteRedirect(db *DB, id int64) error {
	return HardDelete(db, specRedirects, id)
}

func RestoreRedirect(db *DB, id int64) error {
	if err := Restore(db, specRedirects, id); err != nil {
		return err
	}
	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableRedirects, TableOpUpdate)
	}
	return nil
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
	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableRedirects, TableOpInsert)
	}
	return id, nil
}

type Setting struct {
	ID                 int64
	Kind               string
	SubKind            string
	Name               string
	Code               string
	Type               string
	Options            string // JSON string
	Attrs              string // JSON string
	Value              string
	DefaultOptionValue string // Default value for select/radio/checkbox when options are provided
	PrefixValue        string // Prefix text shown for prefix-field
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
	if err := normalizeSettingForWrite(s); err != nil {
		if err.Error() == "code is required" || err.Error() == "type is required" {
			return 0, err
		}
		return 0, WrapInternalErr("CreateSetting.normalize", err)
	}

	id, err := Create(db, specSettings, map[string]interface{}{
		"kind":                 s.Kind,
		"sub_kind":             strings.TrimSpace(s.SubKind),
		"name":                 s.Name,
		"code":                 s.Code,
		"type":                 s.Type,
		"options":              s.Options,
		"attrs":                s.Attrs,
		"value":                s.Value,
		"default_option_value": s.DefaultOptionValue,
		"prefix_value":         s.PrefixValue,
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

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpInsert)
	}

	return id, nil
}

func GetSettingByCode(db *DB, code string) (*Setting, error) {
	result, err := Read(db, specSettings, ReadOptions{
		SelectFields: "id, kind, sub_kind, name, code, type, options, attrs, value, default_option_value, prefix_value, description, sort, charset, author, keywords, reload, created_at, updated_at, deleted_at",
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
		SelectFields: "id, kind, sub_kind, name, code, type, options, attrs, value, default_option_value, prefix_value, description, sort, charset, author, keywords, reload, created_at, updated_at, deleted_at",
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
	subKindSortKey := buildSettingSubKindSortKeySQL()
	orderBy := buildSettingSubKindOrderSQL() + ", " + subKindSortKey + " ASC, sort ASC, id ASC"
	if kind != "" {
		whereClause = "kind=?"
		whereArgs = append(whereArgs, kind)
	} else {
		orderBy = buildSettingKindOrderSQL() + ", kind ASC, " + buildSettingSubKindOrderSQL() + ", " + subKindSortKey + " ASC, sort ASC, id ASC"
	}

	results, err := Read(db, specSettings, ReadOptions{
		SelectFields: "id, kind, sub_kind, name, code, type, options, attrs, value, default_option_value, prefix_value, description, sort, charset, author, keywords, reload, created_at, updated_at, deleted_at",
		WhereClause:  whereClause,
		OrderBy:      orderBy,
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

func buildSettingKindOrderSQL() string {
	parts := make([]string, 0, len(settingKindOrder)+2)
	parts = append(parts, "CASE kind")
	for idx, kind := range settingKindOrder {
		safeKind := strings.ReplaceAll(kind, "'", "''")
		parts = append(parts, fmt.Sprintf("WHEN '%s' THEN %d", safeKind, idx+1))
	}
	parts = append(parts, "ELSE 999 END")
	return strings.Join(parts, " ")
}

func buildSettingSubKindSortKeySQL() string {
	safeGeneral := strings.ReplaceAll(SettingSubKindGeneral, "'", "''")
	return fmt.Sprintf("COALESCE(NULLIF(sub_kind, ''), '%s')", safeGeneral)
}

func buildSettingSubKindOrderSQL() string {
	parts := []string{"CASE kind || ':' || " + buildSettingSubKindSortKeySQL()}
	rank := 1

	handledKinds := make(map[string]bool, len(settingSubKindOrder))
	for _, kind := range settingKindOrder {
		subKinds, ok := settingSubKindOrder[kind]
		if !ok {
			continue
		}
		handledKinds[kind] = true
		safeKind := strings.ReplaceAll(kind, "'", "''")
		for _, subKind := range subKinds {
			normalizedSubKind := strings.TrimSpace(subKind)
			if normalizedSubKind == "" {
				normalizedSubKind = SettingSubKindGeneral
			}
			safeSubKind := strings.ReplaceAll(normalizedSubKind, "'", "''")
			parts = append(parts, fmt.Sprintf("WHEN '%s:%s' THEN %d", safeKind, safeSubKind, rank))
			rank++
		}
	}

	for kind, subKinds := range settingSubKindOrder {
		if handledKinds[kind] {
			continue
		}
		safeKind := strings.ReplaceAll(kind, "'", "''")
		for _, subKind := range subKinds {
			normalizedSubKind := strings.TrimSpace(subKind)
			if normalizedSubKind == "" {
				normalizedSubKind = SettingSubKindGeneral
			}
			safeSubKind := strings.ReplaceAll(normalizedSubKind, "'", "''")
			parts = append(parts, fmt.Sprintf("WHEN '%s:%s' THEN %d", safeKind, safeSubKind, rank))
			rank++
		}
	}

	parts = append(parts, "ELSE 999 END")
	return strings.Join(parts, " ")
}

func ListAllSettings(db *DB) ([]Setting, error) {
	return ListSettingsByKind(db, "")
}

func UpdateSetting(db *DB, s *Setting) error {
	if err := normalizeSettingForWrite(s); err != nil {
		return WrapInternalErr("UpdateSetting.normalize", err)
	}

	err := Update(db, specSettings, s.ID, map[string]interface{}{
		"kind":                 s.Kind,
		"sub_kind":             strings.TrimSpace(s.SubKind),
		"name":                 s.Name,
		"type":                 s.Type,
		"options":              s.Options,
		"attrs":                s.Attrs,
		"value":                s.Value,
		"default_option_value": s.DefaultOptionValue,
		"prefix_value":         s.PrefixValue,
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
	setting, err := GetSettingByCode(db, code)
	if err != nil {
		return err
	}

	value, err = normalizeSettingValueForStorage(setting.Type, value)
	if err != nil {
		return WrapInternalErr("UpdateSettingByCode.normalize", err)
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

func CountSettings(db *DB) (int, error) {
	return Count(db, specSettings, "", nil)
}

func HasInstalledSettings(db *DB) (bool, error) {
	count, err := CountSettings(db)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func insertSettingTx(tx *sql.Tx, s Setting) error {
	_, err := tx.Exec(
		`INSERT INTO `+string(TableSettings)+` (
			kind, sub_kind, name, code, type, options, attrs, value, default_option_value,
			prefix_value, description, sort, charset, author, keywords, reload, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.Kind,
		strings.TrimSpace(s.SubKind),
		s.Name,
		s.Code,
		s.Type,
		s.Options,
		s.Attrs,
		s.Value,
		s.DefaultOptionValue,
		s.PrefixValue,
		s.Description,
		s.Sort,
		s.Charset,
		s.Author,
		s.Keywords,
		s.Reload,
		s.CreatedAt,
		s.UpdatedAt,
	)
	if err != nil {
		return WrapInternalErr("insertSettingTx", err)
	}
	return nil
}

func BootstrapDefaultSettings(db *DB, overrides map[string]string) error {
	tx, err := db.Begin()
	if err != nil {
		return WrapInternalErr("BootstrapDefaultSettings.Begin", err)
	}
	defer func() {
		if tx != nil {
			_ = tx.Rollback()
		}
	}()

	var count int
	row := tx.QueryRow(`SELECT COUNT(1) FROM ` + string(TableSettings) + ` WHERE deleted_at IS NULL`)
	if err = row.Scan(&count); err != nil {
		return WrapInternalErr("BootstrapDefaultSettings.Count", err)
	}
	if count > 0 {
		return errors.New("settings already initialized")
	}

	for _, defaultSetting := range DefaultSettings {
		setting := defaultSetting
		if overrides != nil {
			if value, ok := overrides[setting.Code]; ok {
				setting.Value = value
			}
		}
		if err = normalizeSettingForWrite(&setting); err != nil {
			return WrapInternalErr("BootstrapDefaultSettings.normalize", err)
		}
		if err = insertSettingTx(tx, setting); err != nil {
			return WrapInternalErr("BootstrapDefaultSettings.insert", err)
		}
	}

	if err = tx.Commit(); err != nil {
		return WrapInternalErr("BootstrapDefaultSettings.Commit", err)
	}
	tx = nil

	if OnDatabaseChanged != nil {
		OnDatabaseChanged(TableSettings, TableOpInsert)
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
	setting, err := GetSettingByCode(db, "dash_password")
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
		existing, err := GetSettingByCode(db, s.Code)
		if err == nil {
			if err := syncDefaultSettingMeta(db, existing, s); err != nil {
				return err
			}
			continue
		}
		if !IsErrNotFound(err) {
			return err
		}

		if _, err := CreateSetting(db, &s); err != nil {
			return err
		}
	}

	return nil
}

func syncDefaultSettingMeta(db *DB, existing *Setting, defaults Setting) error {
	if existing == nil {
		return nil
	}

	updateData := map[string]interface{}{}
	if existing.Kind != defaults.Kind {
		updateData["kind"] = defaults.Kind
	}
	if existing.SubKind != defaults.SubKind {
		updateData["sub_kind"] = defaults.SubKind
	}
	if existing.Type != defaults.Type {
		updateData["type"] = defaults.Type
	}
	if existing.Code == "dash_full_main_open" {
		// Keep existing instances aligned with the updated wording/semantics.
		if existing.Name != defaults.Name {
			updateData["name"] = defaults.Name
		}
		if existing.Description != defaults.Description {
			updateData["description"] = defaults.Description
		}
		if existing.Options != defaults.Options {
			updateData["options"] = defaults.Options
		}
		if existing.DefaultOptionValue != defaults.DefaultOptionValue {
			updateData["default_option_value"] = defaults.DefaultOptionValue
		}
	}

	if len(updateData) == 0 {
		return nil
	}

	if err := Update(db, specSettings, existing.ID, updateData); err != nil {
		return WrapInternalErr("syncDefaultSettingMeta", err)
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

// LoadRedirectsToMap 从 redirects 表加载 from_path -> redirect 映射，仅包含启用规则
func LoadRedirectsToMap(db *DB) (map[string]Redirect, error) {
	results, err := Read(db, specRedirects, ReadOptions{
		SelectFields: "id, from_path, to_path, COALESCE(status, 301), COALESCE(enabled, 1), created_at, updated_at, deleted_at",
		WhereClause:  "enabled=1",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        0,
	}, func(rows *sql.Rows) (interface{}, error) {
		r, err := scanRedirect(rows)
		if err != nil {
			return nil, WrapInternalErr("LoadRedirectsToMap.Scan", err)
		}
		return r, nil
	})
	if err != nil {
		return nil, WrapInternalErr("LoadRedirectsToMap", err)
	}

	redirectsMap := make(map[string]Redirect, len(results))
	for _, v := range results {
		redirect := v.(Redirect)
		redirectsMap[redirect.From] = redirect
	}
	return redirectsMap, nil
}

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

const (
	AssetKindImage  = "image"
	AssetKindBackup = "backup"
	AssetKindFile   = "file"
)

type Asset struct {
	ID                int64  `json:"id"`
	Kind              string `json:"kind"`
	Provider          string `json:"provider"`
	ProviderAssetID   string `json:"provider_asset_id"`
	ProviderDeleteKey string `json:"provider_delete_key"`
	FileURL           string `json:"file_url"`
	OriginalName      string `json:"original_name"`
	Remark            string `json:"remark"`
	SizeBytes         int64  `json:"size_bytes"`
	CreatedAt         int64  `json:"created_at"`
}

type AssetQueryOptions struct {
	Kind     string
	Provider string
	Limit    int
	Offset   int
}

func CreateAsset(db *DB, m *Asset) (int64, error) {
	if m == nil {
		return 0, WrapInternalErr("CreateAsset", fmt.Errorf("asset is nil"))
	}
	if strings.TrimSpace(m.Kind) == "" {
		m.Kind = AssetKindImage
	}
	if strings.TrimSpace(m.Provider) == "" {
		return 0, WrapInternalErr("CreateAsset", fmt.Errorf("provider is required"))
	}
	if strings.TrimSpace(m.ProviderAssetID) == "" {
		return 0, WrapInternalErr("CreateAsset", fmt.Errorf("provider_asset_id is required"))
	}
	if m.CreatedAt == 0 {
		m.CreatedAt = now()
	}

	id, err := Create(db, TableSpec{
		Name:         TableAssets,
		HasDeletedAt: false,
		HasCreatedAt: false,
		HasUpdatedAt: false,
	}, map[string]interface{}{
		"kind":                strings.TrimSpace(m.Kind),
		"provider":            strings.TrimSpace(m.Provider),
		"provider_asset_id":   strings.TrimSpace(m.ProviderAssetID),
		"provider_delete_key": strings.TrimSpace(m.ProviderDeleteKey),
		"file_url":            strings.TrimSpace(m.FileURL),
		"original_name":       strings.TrimSpace(m.OriginalName),
		"remark":              strings.TrimSpace(m.Remark),
		"size_bytes":          m.SizeBytes,
		"created_at":          m.CreatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateAsset", err)
	}

	m.ID = id
	return id, nil
}

func GetAssetByID(db *DB, id int64) (*Asset, error) {
	row := db.QueryRow(`
		SELECT id, kind, provider, provider_asset_id, provider_delete_key, file_url, original_name, remark, size_bytes, created_at
		FROM `+string(TableAssets)+`
		WHERE id = ?
	`, id)

	var m Asset
	if err := row.Scan(
		&m.ID,
		&m.Kind,
		&m.Provider,
		&m.ProviderAssetID,
		&m.ProviderDeleteKey,
		&m.FileURL,
		&m.OriginalName,
		&m.Remark,
		&m.SizeBytes,
		&m.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound("GetAssetByID")
		}
		return nil, WrapInternalErr("GetAssetByID", err)
	}

	return &m, nil
}

func GetAssetByProviderAssetID(db *DB, provider string, providerAssetID string) (*Asset, error) {
	row := db.QueryRow(`
		SELECT id, kind, provider, provider_asset_id, provider_delete_key, file_url, original_name, remark, size_bytes, created_at
		FROM `+string(TableAssets)+`
		WHERE provider = ? AND provider_asset_id = ?
		LIMIT 1
	`, strings.TrimSpace(provider), strings.TrimSpace(providerAssetID))

	var m Asset
	if err := row.Scan(
		&m.ID,
		&m.Kind,
		&m.Provider,
		&m.ProviderAssetID,
		&m.ProviderDeleteKey,
		&m.FileURL,
		&m.OriginalName,
		&m.Remark,
		&m.SizeBytes,
		&m.CreatedAt,
	); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound("GetAssetByProviderAssetID")
		}
		return nil, WrapInternalErr("GetAssetByProviderAssetID", err)
	}

	return &m, nil
}

func DeleteAsset(db *DB, id int64) error {
	if _, err := db.Exec(`DELETE FROM `+string(TableAssets)+` WHERE id = ?`, id); err != nil {
		return WrapInternalErr("DeleteAsset", err)
	}
	return nil
}

func CountAssets(db *DB, kind, provider string) (int, error) {
	whereClause, whereArgs := buildAssetWhereClause(kind, provider)
	query := `SELECT COUNT(*) FROM ` + string(TableAssets)
	if whereClause != "" {
		query += ` WHERE ` + whereClause
	}

	var total int
	if err := db.QueryRow(query, whereArgs...).Scan(&total); err != nil {
		return 0, WrapInternalErr("CountAssets", err)
	}
	return total, nil
}

func ListAssets(db *DB, opts AssetQueryOptions) ([]Asset, error) {
	whereClause, whereArgs := buildAssetWhereClause(opts.Kind, opts.Provider)
	query := `
		SELECT id, kind, provider, provider_asset_id, provider_delete_key, file_url, original_name, remark, size_bytes, created_at
		FROM ` + string(TableAssets)
	if whereClause != "" {
		query += ` WHERE ` + whereClause
	}
	query += ` ORDER BY created_at DESC, id DESC`

	if opts.Limit > 0 {
		query += ` LIMIT ? OFFSET ?`
		whereArgs = append(whereArgs, opts.Limit, opts.Offset)
	}

	rows, err := db.Query(query, whereArgs...)
	if err != nil {
		return nil, WrapInternalErr("ListAssets", err)
	}
	defer rows.Close()

	items := make([]Asset, 0)
	for rows.Next() {
		var m Asset
		if err = rows.Scan(
			&m.ID,
			&m.Kind,
			&m.Provider,
			&m.ProviderAssetID,
			&m.ProviderDeleteKey,
			&m.FileURL,
			&m.OriginalName,
			&m.Remark,
			&m.SizeBytes,
			&m.CreatedAt,
		); err != nil {
			return nil, WrapInternalErr("ListAssets.Scan", err)
		}
		items = append(items, m)
	}

	if err = rows.Err(); err != nil {
		return nil, WrapInternalErr("ListAssets.Rows", err)
	}

	return items, nil
}

func buildAssetWhereClause(kind, provider string) (string, []interface{}) {
	conditions := make([]string, 0, 2)
	args := make([]interface{}, 0, 2)

	if kind = strings.TrimSpace(kind); kind != "" {
		conditions = append(conditions, "kind = ?")
		args = append(args, kind)
	}
	if provider = strings.TrimSpace(provider); provider != "" {
		conditions = append(conditions, "provider = ?")
		args = append(args, provider)
	}

	if len(conditions) == 0 {
		return "", args
	}
	return strings.Join(conditions, " AND "), args
}

const (
	NotificationReceiverDash = "dash"

	NotificationLevelInfo    = "info"
	NotificationLevelWarning = "warning"
	NotificationLevelError   = "error"

	NotificationEventPostLike   = "post_like"
	NotificationEventComment    = "comment"
	NotificationEventTaskResult = "task_result"
	NotificationEventAppUpdate  = "app_update"
)

type Notification struct {
	ID             int64
	Receiver       string
	EventType      string
	Level          string
	Title          string
	Body           string
	AggregateKey   string
	AggregateCount int
	ReadAt         *int64
	CreatedAt      int64
	UpdatedAt      int64
}

func normalizeNotificationReceiver(receiver string) string {
	receiver = strings.TrimSpace(receiver)
	if receiver == "" {
		return NotificationReceiverDash
	}
	return receiver
}

func normalizeNotificationEventType(eventType string) string {
	switch strings.TrimSpace(eventType) {
	case NotificationEventPostLike:
		return NotificationEventPostLike
	case NotificationEventComment:
		return NotificationEventComment
	case NotificationEventTaskResult:
		return NotificationEventTaskResult
	case NotificationEventAppUpdate:
		return NotificationEventAppUpdate
	default:
		return ""
	}
}

func normalizeNotificationLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case NotificationLevelWarning:
		return NotificationLevelWarning
	case NotificationLevelError:
		return NotificationLevelError
	default:
		return NotificationLevelInfo
	}
}

func normalizeNotificationData(n *Notification) error {
	if n == nil {
		return errors.New("notification is nil")
	}

	n.Receiver = normalizeNotificationReceiver(n.Receiver)
	n.EventType = strings.TrimSpace(n.EventType)
	n.Level = normalizeNotificationLevel(n.Level)
	n.Title = strings.TrimSpace(n.Title)
	n.Body = strings.TrimSpace(n.Body)
	n.AggregateKey = strings.TrimSpace(n.AggregateKey)

	if n.EventType == "" {
		return errors.New("event_type is required")
	}
	if n.Title == "" {
		return errors.New("title is required")
	}
	if n.AggregateCount <= 0 {
		n.AggregateCount = 1
	}
	return nil
}

func CreateNotification(db *DB, n *Notification) (int64, error) {
	if err := normalizeNotificationData(n); err != nil {
		return 0, WrapInternalErr("CreateNotification", err)
	}

	if n.ReadAt != nil {
		readAt := *n.ReadAt
		n.ReadAt = &readAt
	}
	nowUnix := now()
	if n.CreatedAt <= 0 {
		n.CreatedAt = nowUnix
	}
	if n.UpdatedAt <= 0 {
		n.UpdatedAt = n.CreatedAt
	}

	id, err := Create(db, specNotifications, map[string]interface{}{
		"receiver":        n.Receiver,
		"event_type":      n.EventType,
		"level":           n.Level,
		"title":           n.Title,
		"body":            n.Body,
		"aggregate_key":   n.AggregateKey,
		"aggregate_count": n.AggregateCount,
		"read_at":         n.ReadAt,
		"created_at":      n.CreatedAt,
		"updated_at":      n.UpdatedAt,
	})
	if err != nil {
		return 0, WrapInternalErr("CreateNotification", err)
	}
	n.ID = id
	return id, nil
}

func CountNotifications(db *DB, receiver string) (int, error) {
	return CountNotificationsByEventType(db, receiver, "")
}

func CountNotificationsByEventType(db *DB, receiver string, eventType string) (int, error) {
	receiver = normalizeNotificationReceiver(receiver)
	eventType = normalizeNotificationEventType(eventType)
	if eventType == "" {
		return Count(db, specNotifications, "receiver = ?", []interface{}{receiver})
	}
	return Count(db, specNotifications, "receiver = ? AND event_type = ?", []interface{}{receiver, eventType})
}

func ListNotifications(db *DB, receiver string, limit, offset int) ([]Notification, error) {
	return ListNotificationsByEventType(db, receiver, "", limit, offset)
}

func ListNotificationsByEventType(db *DB, receiver string, eventType string, limit, offset int) ([]Notification, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	receiver = normalizeNotificationReceiver(receiver)
	eventType = normalizeNotificationEventType(eventType)

	whereClause := "receiver = ?"
	whereArgs := []interface{}{receiver}
	if eventType != "" {
		whereClause += " AND event_type = ?"
		whereArgs = append(whereArgs, eventType)
	}

	results, err := Read(db, specNotifications, ReadOptions{
		SelectFields: "id, receiver, event_type, level, title, body, aggregate_key, aggregate_count, read_at, created_at, updated_at",
		WhereClause:  whereClause,
		OrderBy:      "CASE WHEN read_at IS NULL THEN 0 ELSE 1 END ASC, updated_at DESC, id DESC",
		WhereArgs:    whereArgs,
		Limit:        limit,
		Offset:       offset,
	}, func(rows *sql.Rows) (interface{}, error) {
		n, scanErr := scanNotification(rows)
		if scanErr != nil {
			return nil, WrapInternalErr("ListNotifications.Scan", scanErr)
		}
		return n, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListNotifications", err)
	}

	items := make([]Notification, len(results))
	for i, result := range results {
		items[i] = result.(Notification)
	}
	return items, nil
}

func CountUnreadNotifications(db *DB, receiver string) (int, error) {
	return Count(db, specNotifications, "receiver = ? AND read_at IS NULL", []interface{}{normalizeNotificationReceiver(receiver)})
}

func MarkNotificationRead(db *DB, id int64, receiver string) error {
	if id <= 0 {
		return errors.New("id is invalid")
	}
	receiver = normalizeNotificationReceiver(receiver)
	nowUnix := now()
	res, err := db.Exec(
		`UPDATE `+string(TableNotifications)+`
		SET read_at = COALESCE(read_at, ?),
		    updated_at = ?
		WHERE id = ? AND receiver = ?`,
		nowUnix, nowUnix, id, receiver,
	)
	if err != nil {
		return WrapInternalErr("MarkNotificationRead", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return ErrNotFound("MarkNotificationRead")
	}
	return nil
}

func MarkAllNotificationsRead(db *DB, receiver string) (int64, error) {
	receiver = normalizeNotificationReceiver(receiver)
	nowUnix := now()
	res, err := db.Exec(
		`UPDATE `+string(TableNotifications)+`
		SET read_at = COALESCE(read_at, ?),
		    updated_at = ?
		WHERE receiver = ? AND read_at IS NULL`,
		nowUnix, nowUnix, receiver,
	)
	if err != nil {
		return 0, WrapInternalErr("MarkAllNotificationsRead", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
}

func DeleteNotification(db *DB, id int64, receiver string) error {
	if id <= 0 {
		return errors.New("id is invalid")
	}
	receiver = normalizeNotificationReceiver(receiver)
	res, err := db.Exec(
		`DELETE FROM `+string(TableNotifications)+` WHERE id = ? AND receiver = ? AND read_at IS NOT NULL`,
		id, receiver,
	)
	if err != nil {
		return WrapInternalErr("DeleteNotification", err)
	}
	affected, rowsErr := res.RowsAffected()
	if rowsErr != nil {
		return WrapInternalErr("DeleteNotification.RowsAffected", rowsErr)
	}
	if affected == 0 {
		return ErrNotFound("DeleteNotification")
	}
	return nil
}

func CreateOrBumpNotificationByAggregateKey(db *DB, n *Notification) (int64, error) {
	if err := normalizeNotificationData(n); err != nil {
		return 0, WrapInternalErr("CreateOrBumpNotificationByAggregateKey", err)
	}
	if n.AggregateKey == "" {
		return 0, WrapInternalErr("CreateOrBumpNotificationByAggregateKey", errors.New("aggregate_key is required"))
	}

	nowUnix := now()
	if n.CreatedAt <= 0 {
		n.CreatedAt = nowUnix
	}
	if n.UpdatedAt <= 0 {
		n.UpdatedAt = nowUnix
	}
	if n.UpdatedAt < n.CreatedAt {
		n.UpdatedAt = n.CreatedAt
	}

	if err := db.QueryRow(
		`INSERT INTO `+string(TableNotifications)+`
		(receiver, event_type, level, title, body, aggregate_key, aggregate_count, read_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, 1, NULL, ?, ?)
		ON CONFLICT(receiver, aggregate_key) WHERE aggregate_key <> ''
		DO UPDATE SET
			title = excluded.title,
			body = excluded.body,
			aggregate_count = `+string(TableNotifications)+`.aggregate_count + 1,
			read_at = NULL,
			updated_at = excluded.updated_at
		RETURNING id, aggregate_count, created_at, updated_at`,
		n.Receiver, n.EventType, n.Level, n.Title, n.Body, n.AggregateKey, n.CreatedAt, n.UpdatedAt,
	).Scan(&n.ID, &n.AggregateCount, &n.CreatedAt, &n.UpdatedAt); err != nil {
		return 0, WrapInternalErr("CreateOrBumpNotificationByAggregateKey.Upsert", err)
	}
	n.ReadAt = nil
	return n.ID, nil
}

func DeleteExpiredNotifications(db *DB, beforeUnix int64) (int64, error) {
	if beforeUnix <= 0 {
		return 0, nil
	}
	res, err := db.Exec(`DELETE FROM `+string(TableNotifications)+` WHERE updated_at < ?`, beforeUnix)
	if err != nil {
		return 0, WrapInternalErr("DeleteExpiredNotifications", err)
	}
	affected, _ := res.RowsAffected()
	return affected, nil
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

func ListTasksPaged(db *DB, limit, offset int) ([]Task, error) {
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}
	results, err := Read(db, specTasks, ReadOptions{
		SelectFields: "id, code, name, description, schedule, enabled, kind, last_run_at, last_status, created_at, updated_at, deleted_at",
		WhereClause:  "",
		OrderBy:      "",
		WhereArgs:    nil,
		Limit:        limit,
		Offset:       offset,
	}, func(rows *sql.Rows) (interface{}, error) {
		t, err := scanTask(rows)
		if err != nil {
			return nil, WrapInternalErr("ListTasksPaged.Scan", err)
		}
		return t, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListTasksPaged", err)
	}
	res := make([]Task, len(results))
	for i, v := range results {
		res[i] = v.(Task)
	}
	return res, nil
}

func ListTasks(db *DB) ([]Task, error) {
	return ListTasksPaged(db, 0, 0)
}

func CountTasks(db *DB) (int, error) {
	return Count(db, specTasks, "", nil)
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
		SelectFields: "id, task_code, status, message, started_at, finished_at, duration, created_at",
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

// WrapInternalErr 包装 SQL 等执行产生的 error：先 logger.Error 再返回带 label 的包装错误，便于上层区分
func WrapInternalErr(label string, err error) error {
	if err == nil {
		return nil
	}
	logger.Error("[db] %s: %v", label, err)
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

var errDuplicateCommentSentinel = errors.New("duplicate comment")

func ErrDuplicateComment(label string) error {
	return fmt.Errorf("%s: %w", label, errDuplicateCommentSentinel)
}

func IsErrDuplicateComment(err error) bool {
	return errors.Is(err, errDuplicateCommentSentinel)
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

func ListCategoriesPaged(db *DB, withPostCount bool, limit, offset int) ([]Category, error) {
	if limit < 0 {
		limit = 0
	}
	if offset < 0 {
		offset = 0
	}

	results, err := Read(db, specCategories, ReadOptions{
		SelectFields: "id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at",
		WhereClause:  "",
		OrderBy:      "sort ASC, id ASC",
		WhereArgs:    nil,
		Limit:        limit,
		Offset:       offset,
	}, func(rows *sql.Rows) (interface{}, error) {
		c, err := scanCategory(rows)
		if err != nil {
			return nil, WrapInternalErr("ListCategoriesPaged.Scan", err)
		}
		return c, nil
	})
	if err != nil {
		return nil, WrapInternalErr("ListCategoriesPaged", err)
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

func ListCategories(db *DB, withPostCount bool) ([]Category, error) {
	return ListCategoriesPaged(db, withPostCount, 0, 0)
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
		logger.Warn("failed to create directory: %v", err)
		return nil, errors.New("failed to create directory")
	}

	// 2. 临时文件
	tmpFile, err := os.Create(filepath.Join(dir, "__tmp__"))
	if err != nil {
		logger.Warn("failed to create temp file: %v", err)
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
