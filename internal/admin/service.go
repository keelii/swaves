package admin

import (
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"swaves/internal/md"
	"swaves/internal/middleware"
	"time"

	"swaves/internal/db"

	slg "github.com/gosimple/slug"
)

var ErrInvalidPassword = errors.New("invalid password")

type Service struct {
	DB *db.DB
}

func NewService(db *db.DB) *Service {
	return &Service{DB: db}
}

func (a *Service) CheckPassword(raw string) error {
	return db.CheckPassword(a.DB, raw)
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

func generateSlug(name string) string {
	// 简单的 slug 生成：转换为小写，替换空格为连字符
	slug := strings.ToLower(name)
	slug = strings.ReplaceAll(slug, " ", "-")
	// 移除其他特殊字符，只保留字母、数字和连字符
	var result strings.Builder
	for _, r := range slug {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			result.WriteRune(r)
		}
	}
	return result.String()
}

func CreateTagService(dbx *db.DB, in CreateTagInput) error {
	if in.Name == "" {
		return errors.New("name required")
	}

	slug := in.Slug
	if slug == "" {
		slug = generateSlug(in.Name)
	}

	t := &db.Tag{
		Name: in.Name,
		Slug: slug,
	}
	return db.CreateTag(dbx, t)
}

func CreateTagByName(dbx *db.DB, name string) (*db.Tag, error) {
	if name == "" {
		return nil, errors.New("name required")
	}

	// 检查标签是否已存在
	slug := generateSlug(name)
	rows, err := dbx.Query(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM tags
		WHERE slug = ? AND deleted_at IS NULL
		LIMIT 1
	`, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
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
		return &t, nil
	}

	// 如果不存在，创建新标签
	t := &db.Tag{
		Name: name,
		Slug: slug,
	}
	if err := db.CreateTag(dbx, t); err != nil {
		return nil, err
	}
	return t, nil
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

// Settings
func ListSettingsByCategory(dbx *db.DB, category string) ([]db.Setting, error) {
	return db.ListSettingsByCategory(dbx, category)
}

func ListAllSettings(dbx *db.DB) ([]db.Setting, error) {
	return db.ListAllSettings(dbx)
}

func GetSettingByCode(dbx *db.DB, code string) (*db.Setting, error) {
	return db.GetSettingByCode(dbx, code)
}

func GetSettingByID(dbx *db.DB, id int64) (*db.Setting, error) {
	return db.GetSettingByID(dbx, id)
}

func CreateSettingService(dbx *db.DB, s *db.Setting) error {
	return db.CreateSetting(dbx, s)
}

func UpdateSettingService(dbx *db.DB, s *db.Setting) error {
	return db.UpdateSetting(dbx, s)
}

func UpdateSettingValueService(dbx *db.DB, code string, value string) error {
	return db.UpdateSettingByCode(dbx, code, value)
}

func DeleteSettingService(dbx *db.DB, id int64) error {
	return db.DeleteSetting(dbx, id)
}

// Trash
func GetTrashPosts(dbx *db.DB) ([]db.Post, error) {
	return db.ListDeletedPosts(dbx)
}

func GetTrashEncryptedPosts(dbx *db.DB) ([]db.EncryptedPost, error) {
	return db.ListDeletedEncryptedPosts(dbx)
}

func GetTrashTags(dbx *db.DB) ([]db.Tag, error) {
	return db.ListDeletedTags(dbx)
}

func GetTrashRedirects(dbx *db.DB) ([]db.Redirect, error) {
	return db.ListDeletedRedirects(dbx)
}

func RestorePostService(dbx *db.DB, id int64) error {
	return db.RestorePost(dbx, id)
}

func RestoreEncryptedPostService(dbx *db.DB, id int64) error {
	return db.RestoreEncryptedPost(dbx, id)
}

func RestoreTagService(dbx *db.DB, id int64) error {
	return db.RestoreTag(dbx, id)
}

func RestoreRedirectService(dbx *db.DB, id int64) error {
	return db.RestoreRedirect(dbx, id)
}

// HttpErrorLogs
func ListHttpErrorLogs(dbx *db.DB, pager *middleware.Pagination) ([]db.HttpErrorLog, error) {
	var total int
	total, err := db.CountHttpErrorLogs(dbx)
	if err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	logs, err := db.ListHttpErrorLogs(dbx, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return logs, nil
}

func DeleteHttpErrorLogService(dbx *db.DB, id int64) error {
	return db.DeleteHttpErrorLog(dbx, id)
}

// CronJobs
type CreateCronJobInput struct {
	Name        string
	Description string
	Schedule    string
	Enabled     bool
}

func CreateCronJobService(dbx *db.DB, in CreateCronJobInput) error {
	if in.Name == "" {
		return errors.New("name required")
	}
	if in.Schedule == "" {
		return errors.New("schedule required")
	}

	job := &db.CronJob{
		Name:        in.Name,
		Description: in.Description,
		Schedule:    in.Schedule,
		Enabled:     in.Enabled,
	}

	return db.CreateCronJob(dbx, job)
}

func ListCronJobs(dbx *db.DB) ([]db.CronJob, error) {
	return db.ListCronJobs(dbx)
}

func GetCronJobForEdit(dbx *db.DB, id int64) (*db.CronJob, error) {
	return db.GetCronJobByID(dbx, id)
}

// CronJobLogs
func ListCronJobLogs(dbx *db.DB, jobID int64, limit int) ([]*db.CronJobLog, error) {
	return db.ListCronJobLogs(dbx, jobID, limit)
}

// Import/Export
type SlugSource int

const (
	SlugFromFilename    SlugSource = iota // 从文件名
	SlugFromFrontmatter                   // 从 frontmatter 指定字段（默认 slug）
	SlugFromTitle                         // 从 title 生成
)

type ImportFile struct {
	Filename string // 文件名（不含扩展名）
	Content  string // markdown 内容
}

type ImportMarkdownInput struct {
	Files      []ImportFile
	SlugSource SlugSource // slug 来源
	SlugField  string     // 如果 SlugSource 是 SlugFromFrontmatter，指定字段名（默认 "slug"）
}

// importSingleMarkdown 导入单个 markdown 文件
func importSingleMarkdown(dbx *db.DB, file ImportFile, slugSource SlugSource, slugField string) error {
	if file.Content == "" {
		return errors.New("markdown content is required")
	}

	// 解析 markdown
	result := md.ParseMarkdown(file.Content)

	// 从 meta 中提取信息
	title := ""
	if val, ok := result.Meta["title"]; ok {
		if str, ok := val.(string); ok {
			title = str
		}
	}
	if title == "" {
		return errors.New("title is required in frontmatter")
	}

	// 根据 slug 来源确定 slug
	slug := ""
	switch slugSource {
	case SlugFromFilename:
		// 从文件名生成 slug
		if file.Filename != "" {
			slug = slg.Make(file.Filename)
		} else {
			// 如果文件名不存在，回退到从 title 生成
			slug = slg.Make(title)
		}
	case SlugFromFrontmatter:
		// 从 frontmatter 指定字段获取
		fieldName := slugField
		if fieldName == "" {
			fieldName = "slug"
		}
		if val, ok := result.Meta[fieldName]; ok {
			if str, ok := val.(string); ok {
				slug = str
			}
		}
		// 如果 frontmatter 中没有找到，回退到从 title 生成
		if slug == "" {
			slug = slg.Make(title)
		}
	case SlugFromTitle:
		// 从 title 生成
		slug = slg.Make(title)
	default:
		// 默认从 title 生成
		slug = slg.Make(title)
	}

	status := "draft"
	if val, ok := result.Meta["status"]; ok {
		if str, ok := val.(string); ok {
			status = str
		}
	}

	// 获取 HTML 内容作为 post content
	content := result.Markdown

	// 解析 date 字段并转换为东8区时间戳
	// 格式固定为: 2011-05-12T02:20:04.000Z
	var createdAt int64
	if val, ok := result.Meta["date"]; ok {
		if dateStr, ok := val.(string); ok && dateStr != "" {
			t, err := time.Parse("2006-01-02T15:04:05.000Z", dateStr)
			if err == nil {
				// 加载东8区时区
				loc, _ := time.LoadLocation("Asia/Shanghai")
				// 将 UTC 时间转换为东8区时间戳
				createdAt = t.In(loc).Unix()
			}
		}
	}

	// 如果没有解析到 date 或解析失败，使用当前时间
	if createdAt == 0 {
		createdAt = time.Now().Unix()
	}

	// 创建 post
	post := &db.Post{
		Title:     title,
		Slug:      slug,
		Content:   content,
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: time.Now().Unix(),
	}

	if err := db.CreatePost(dbx, post); err != nil {
		return err
	}

	// 处理 tags（如果有）
	if val, ok := result.Meta["tags"]; ok {
		var tagNames []string
		switch v := val.(type) {
		case string:
			// 如果是字符串，可能是逗号分隔的
			if v != "" {
				tagNames = strings.Split(v, ",")
			}
		case []interface{}:
			// 如果是数组
			for _, item := range v {
				if str, ok := item.(string); ok {
					tagNames = append(tagNames, str)
				}
			}
		}

		// 创建或获取 tags 并关联
		var tagIDs []int64
		for _, tagName := range tagNames {
			tagName = strings.TrimSpace(tagName)
			if tagName != "" {
				tag, err := CreateTagByName(dbx, tagName)
				if err == nil {
					tagIDs = append(tagIDs, tag.ID)
				}
			}
		}

		if len(tagIDs) > 0 {
			if err := db.SetPostTags(dbx, post.ID, tagIDs); err != nil {
				// tag 关联失败不影响主流程
			}
		}
	}

	return nil
}

func ImportMarkdownService(dbx *db.DB, in ImportMarkdownInput) error {
	if len(in.Files) == 0 {
		return errors.New("no files to import")
	}

	var errList []string
	for _, file := range in.Files {
		if err := importSingleMarkdown(dbx, file, in.SlugSource, in.SlugField); err != nil {
			errList = append(errList, file.Filename+": "+err.Error())
		}
	}

	if len(errList) > 0 {
		return errors.New(strings.Join(errList, "; "))
	}

	return nil
}
