package admin

import (
	"database/sql"
	"errors"
	"sort"
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
	Title       string
	Slug        string
	Content     string
	Status      string
	TagIDs      []int64
	CategoryIDs []int64
}

type UpdatePostInput struct {
	Title       string
	Content     string
	Status      string
	TagIDs      []int64
	CategoryIDs []int64
}

type PostWithTags struct {
	Post       *db.Post
	Tags       []db.Tag
	Categories []db.Category
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

		// 获取该 post 的 categories
		categories, err := db.GetPostCategories(dbx, p.ID)
		if err != nil {
			return nil, err
		}

		res = append(res, PostWithTags{
			Post:       &p,
			Tags:       tags,
			Categories: categories,
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

	if strings.TrimSpace(in.Content) == "" {
		return errors.New("content is required")
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

	// 关联分类
	if len(in.CategoryIDs) > 0 {
		if err := db.SetPostCategories(dbx, p.ID, in.CategoryIDs); err != nil {
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

	// 更新分类关联
	if err := db.SetPostCategories(dbx, id, in.CategoryIDs); err != nil {
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
	From    string
	To      string
	Status  int
	Enabled int
}

type UpdateRedirectInput struct {
	From    string
	To      string
	Status  int
	Enabled int
}

func ListRedirects(dbx *db.DB, pager *middleware.Pagination) ([]db.Redirect, error) {
	offset := (pager.Page - 1) * pager.PageSize
	res, total, err := db.ListRedirects(dbx, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}

// checkRedirectCycle 检查是否存在循环重定向
func checkRedirectCycle(dbx *db.DB, fromPath, toPath string) error {
	visited := make(map[string]bool)
	current := toPath
	maxDepth := 100 // 防止无限循环，设置最大深度

	for i := 0; i < maxDepth; i++ {
		// 如果当前路径就是要创建的 from 路径，说明添加新的重定向后会形成循环
		if current == fromPath {
			return errors.New("detected redirect cycle: adding this redirect would create a loop")
		}

		// 如果已经访问过这个路径，说明在现有重定向链中存在循环
		// 虽然不是直接回到 fromPath，但为了避免复杂的循环情况，也应该报错
		if visited[current] {
			return errors.New("detected redirect cycle in existing redirects")
		}

		// 标记当前路径为已访问
		visited[current] = true

		// 查找是否有从 current 路径的重定向
		redirect, err := db.GetRedirectByFrom(dbx, current)
		if err != nil {
			if err == db.ErrNotFound {
				// 没有找到重定向，链到此结束，没有循环
				return nil
			}
			// 其他错误，返回
			return err
		}

		current = redirect.To
	}

	return errors.New("detected redirect cycle: exceeded maximum redirect chain length")
}

func CreateRedirectService(dbx *db.DB, in CreateRedirectInput) error {
	if in.From == "" || in.To == "" {
		return errors.New("from and to required")
	}

	// 检查 from 和 to 是否相同
	if in.From == in.To {
		return errors.New("from 和 to 不能相同")
	}

	if in.Status == 0 {
		in.Status = 301 // default
	}

	// 检查是否存在循环重定向
	if err := checkRedirectCycle(dbx, in.From, in.To); err != nil {
		return err
	}

	if in.Status == 0 {
		in.Status = 301 // default
	}
	if in.Enabled == 0 {
		in.Enabled = 1 // default
	}

	r := &db.Redirect{
		From:    in.From,
		To:      in.To,
		Status:  in.Status,
		Enabled: in.Enabled,
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
	if in.Status > 0 {
		r.Status = in.Status
	} else {
		r.Status = 301 // default
	}
	r.Enabled = in.Enabled

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
	if in.Title == "" {
		return errors.New("title is required")
	}

	if strings.TrimSpace(in.Content) == "" {
		return errors.New("content is required")
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

// Categories
type CategoryNode struct {
	Category db.Category
	Children []*CategoryNode `json:"children"`
}

func BuildCategoryTree(list []db.Category) []*CategoryNode {
	nodeMap := make(map[int64]*CategoryNode)
	var roots []*CategoryNode

	// 1. 先把所有节点建出来
	for _, c := range list {
		nodeMap[c.ID] = &CategoryNode{
			Category: c,
			Children: make([]*CategoryNode, 0),
		}
	}

	// 2. 组装父子关系
	for _, node := range nodeMap {
		if node.Category.ParentID == 0 {
			roots = append(roots, node)
			continue
		}
		if parent, ok := nodeMap[node.Category.ParentID]; ok {
			parent.Children = append(parent.Children, node)
		}
	}

	return roots
}

func SortCategoryTree(nodes []*CategoryNode) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Category.Sort < nodes[j].Category.Sort
	})

	for _, n := range nodes {
		if len(n.Children) > 0 {
			SortCategoryTree(n.Children)
		}
	}
}
func GetCategoryTree(dbx *db.DB) ([]db.Category, []*CategoryNode, error) {
	list, err := db.ListCategories(dbx)
	if err != nil {
		return nil, nil, err
	}
	roots := BuildCategoryTree(list)
	SortCategoryTree(roots)
	return list, roots, nil
}

func HasCycle(all map[int64]*db.Category, nodeID int64, newParentID int64) bool {
	cur := newParentID
	for cur != 0 {
		if cur == nodeID {
			return true
		}
		parent, ok := all[cur]
		if !ok {
			break
		}
		cur = parent.ParentID
	}
	return false
}

func UpdateCategoryParentService(dbx *db.DB, id int64, newParentID int64) error {
	list, err := db.ListCategories(dbx)
	if err != nil {
		return err
	}

	m := make(map[int64]*db.Category)
	for i := range list {
		m[list[i].ID] = &list[i]
	}

	if HasCycle(m, id, newParentID) {
		return errors.New("category cycle detected")
	}

	return db.UpdateCategoryParent(dbx, id, newParentID)
}

// Category Service Functions
type CreateCategoryInput struct {
	ParentID    int64
	Name        string
	Slug        string
	Description string
	Sort        int64
}

type UpdateCategoryInput struct {
	ParentID    int64
	Name        string
	Slug        string
	Description string
	Sort        int64
}

func ListCategoriesService(dbx *db.DB) ([]db.Category, error) {
	return db.ListCategories(dbx)
}

func GetAllCategoriesFlat(dbx *db.DB) ([]db.Category, error) {
	return db.ListCategories(dbx)
}

func CreateCategoryService(dbx *db.DB, in CreateCategoryInput) error {
	if in.Name == "" {
		return errors.New("name required")
	}

	c := &db.Category{
		ParentID:    in.ParentID,
		Name:        in.Name,
		Slug:        in.Slug,
		Description: in.Description,
		Sort:        in.Sort,
	}
	return db.CreateCategory(dbx, c)
}

func CreateCategoryByName(dbx *db.DB, name string) (*db.Category, error) {
	if name == "" {
		return nil, errors.New("name required")
	}

	// 生成 slug
	slug := slg.Make(name)

	// 检查分类是否已存在（根据名称和 slug，由于是单选，只需要检查顶级分类中是否存在相同的 slug）
	rows, err := dbx.Query(`
		SELECT id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at
		FROM categories
		WHERE (name = ? OR slug = ?) AND deleted_at IS NULL
		LIMIT 1
	`, name, slug)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		var c db.Category
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
		return &c, nil
	}

	// 如果不存在，创建新分类（作为顶级分类）
	c := &db.Category{
		Name: name,
		Slug: slug,
	}
	if err := db.CreateCategory(dbx, c); err != nil {
		return nil, err
	}
	return c, nil
}

func GetCategoryForEdit(dbx *db.DB, id int64) (*db.Category, error) {
	return db.GetCategoryByID(dbx, id)
}

func UpdateCategoryService(dbx *db.DB, id int64, in UpdateCategoryInput) error {
	c, err := db.GetCategoryByID(dbx, id)
	if err != nil {
		return err
	}

	c.ParentID = in.ParentID
	c.Name = in.Name
	c.Slug = in.Slug
	c.Description = in.Description
	c.Sort = in.Sort

	return db.UpdateCategory(dbx, c)
}

func DeleteCategoryService(dbx *db.DB, id int64) error {
	return db.SoftDeleteCategory(dbx, id)
}

// Settings
func ListSettingsByKind(dbx *db.DB, kind string) ([]db.Setting, error) {
	return db.ListSettingsByKind(dbx, kind)
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
	Code        string
	Name        string
	Description string
	Schedule    string
	Enabled     bool
}

func CreateCronJobService(dbx *db.DB, in CreateCronJobInput) error {
	if in.Code == "" {
		return errors.New("code is required")
	}
	if in.Name == "" {
		return errors.New("name is required")
	}
	if in.Schedule == "" {
		return errors.New("schedule is required")
	}

	enabled := 0
	if in.Enabled {
		enabled = 1
	}

	job := &db.CronJob{
		Code:        in.Code,
		Name:        in.Name,
		Description: in.Description,
		Schedule:    in.Schedule,
		Enabled:     enabled,
	}

	return db.CreateCronJob(dbx, job)
}

type UpdateCronJobInput struct {
	Code        string
	Name        string
	Description string
	Schedule    string
	Enabled     bool
}

func UpdateCronJobService(dbx *db.DB, id int64, in UpdateCronJobInput) error {
	job, err := db.GetCronJobByID(dbx, id)
	if err != nil {
		return err
	}

	// Code 不可修改，保持原有值
	job.Name = in.Name
	job.Description = in.Description
	job.Schedule = in.Schedule
	if in.Enabled {
		job.Enabled = 1
	} else {
		job.Enabled = 0
	}

	return db.UpdateCronJob(dbx, job)
}

func ListCronJobsService(dbx *db.DB) ([]db.CronJob, error) {
	return db.ListCronJobs(dbx)
}

func GetCronJobForEdit(dbx *db.DB, id int64) (*db.CronJob, error) {
	return db.GetCronJobByID(dbx, id)
}

func DeleteCronJobService(dbx *db.DB, id int64) error {
	return db.SoftDeleteCronJob(dbx, id)
}

// CronJobRuns
func ListCronJobRunsService(dbx *db.DB, jobCode string, limit int) ([]db.CronJobRun, error) {
	return db.ListCronJobRuns(dbx, jobCode, "", limit)
}

func CreatePendingRunService(dbx *db.DB, jobCode string) error {
	now := time.Now().Unix()
	run := &db.CronJobRun{
		JobCode:    jobCode,
		Status:     "pending",
		Message:    "",
		StartedAt:  now, // pending 状态时，started_at 设置为当前时间
		FinishedAt: now, // pending 状态时，finished_at 设置为当前时间（满足 NOT NULL 约束）
		Duration:   0,   // pending 状态时，duration 为 0
	}
	return db.CreateCronJobRun(dbx, run)
}

// Import/Export
type SlugSource int

const (
	SlugFromFilename    SlugSource = iota // 从文件名
	SlugFromFrontmatter                   // 从 frontmatter 指定字段（默认 slug）
	SlugFromTitle                         // 从 title 生成
)

type TitleSource int

const (
	TitleFromFilename    TitleSource = iota // 从文件名
	TitleFromFrontmatter                    // 从 frontmatter 指定字段（默认 title）
	TitleFromMarkdown                       // 从 markdown 标题中提取（H1/H2/H3）
)

type CreatedSource int

const (
	CreatedFromFrontmatter CreatedSource = iota // 从 frontmatter 指定字段（默认 date）
	CreatedFromFileTime                         // 从文件创建时间（实际上使用当前时间，因为 HTTP 上传无法获取文件创建时间）
)

type StatusSource int

const (
	StatusFromFrontmatter StatusSource = iota // 从 frontmatter 指定字段（默认 status）
	StatusAllDraft                            // 全部 draft
	StatusAllPublished                        // 全部 published
)

type CategorySource int

const (
	CategoryFromFrontmatter CategorySource = iota // 从 frontmatter 指定字段（默认 category）
	CategoryAutoCreate                            // 自动创建默认分类
	CategoryNone                                  // 留空（不设置分类）
)

type ImportFile struct {
	Filename string // 文件名（不含扩展名）
	Content  string // markdown 内容
}

type ImportMarkdownInput struct {
	Files          []ImportFile
	SlugSource     SlugSource     // slug 来源
	SlugField      string         // 如果 SlugSource 是 SlugFromFrontmatter，指定字段名（默认 "slug"）
	TitleSource    TitleSource    // title 来源
	TitleField     string         // 如果 TitleSource 是 TitleFromFrontmatter，指定字段名（默认 "title"）
	TitleLevel     int            // 如果 TitleSource 是 TitleFromMarkdown，指定标题级别（1/2/3）
	CreatedSource  CreatedSource  // created_at 来源
	CreatedField   string         // 如果 CreatedSource 是 CreatedFromFrontmatter，指定字段名（默认 "date"）
	StatusSource   StatusSource   // status 来源
	StatusField    string         // 如果 StatusSource 是 StatusFromFrontmatter，指定字段名（默认 "status"）
	CategorySource CategorySource // category 来源
	CategoryField  string         // 如果 CategorySource 是 CategoryFromFrontmatter，指定字段名（默认 "category"）
}

// PreviewPostItem 预览页面的 post 数据
type PreviewPostItem struct {
	Index         int      // 索引（用于表单字段命名）
	Filename      string   // 原始文件名
	Title         string   // 标题
	Slug          string   // slug
	Content       string   // 内容（markdown）
	Status        string   // 状态
	CreatedAt     string   // 创建时间（格式化的时间字符串，用于显示）
	CreatedAtUnix int64    // 创建时间（Unix 时间戳）
	Tags          string   // 标签（逗号分隔的字符串）
	TagsList      []string // 标签列表
	Category      string   // 分类名称
}

// extractTitleFromMarkdown 从 markdown 内容中提取指定级别的标题
func extractTitleFromMarkdown(content string, level int) string {
	lines := strings.Split(content, "\n")
	prefix := strings.Repeat("#", level)
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix+" ") {
			// 提取标题文本（去除 # 前缀和空格）
			title := strings.TrimSpace(strings.TrimPrefix(line, prefix))
			if title != "" {
				return title
			}
		}
	}
	return ""
}

// ParseImportFiles 解析导入文件但不入库，返回预览数据
func ParseImportFiles(files []ImportFile, slugSource SlugSource, slugField string, titleSource TitleSource, titleField string, titleLevel int, createdSource CreatedSource, createdField string, statusSource StatusSource, statusField string, categorySource CategorySource, categoryField string) ([]PreviewPostItem, error) {
	var items []PreviewPostItem
	var errList []string

	for i, file := range files {
		if file.Content == "" {
			errList = append(errList, file.Filename+": markdown content is required")
			continue
		}

		// 解析 markdown
		result := md.ParseMarkdown(file.Content)

		// 根据 title 来源确定 title
		title := ""
		switch titleSource {
		case TitleFromFilename:
			// 从文件名生成 title（去除扩展名，作为标题）
			if file.Filename != "" {
				title = file.Filename
			} else {
				// 如果文件名不存在，尝试从 frontmatter 获取
				if val, ok := result.Meta["title"]; ok {
					if str, ok := val.(string); ok {
						title = str
					}
				}
			}
		case TitleFromFrontmatter:
			// 从 frontmatter 指定字段获取
			fieldName := titleField
			if fieldName == "" {
				fieldName = "title"
			}
			if val, ok := result.Meta[fieldName]; ok {
				if str, ok := val.(string); ok {
					title = str
				}
			}
		case TitleFromMarkdown:
			// 从 markdown 标题中提取
			if titleLevel < 1 || titleLevel > 3 {
				titleLevel = 1 // 默认使用 H1
			}
			title = extractTitleFromMarkdown(result.Markdown, titleLevel)
			// 如果没找到，尝试从 frontmatter 获取
			if title == "" {
				if val, ok := result.Meta["title"]; ok {
					if str, ok := val.(string); ok {
						title = str
					}
				}
			}
		default:
			// 默认从 frontmatter 的 title 字段获取
			if val, ok := result.Meta["title"]; ok {
				if str, ok := val.(string); ok {
					title = str
				}
			}
		}

		if title == "" {
			errList = append(errList, file.Filename+": title is required")
			continue
		}

		// 根据 slug 来源确定 slug
		slug := ""
		switch slugSource {
		case SlugFromFilename:
			if file.Filename != "" {
				slug = slg.Make(file.Filename)
			} else {
				slug = slg.Make(title)
			}
		case SlugFromFrontmatter:
			fieldName := slugField
			if fieldName == "" {
				fieldName = "slug"
			}
			if val, ok := result.Meta[fieldName]; ok {
				if str, ok := val.(string); ok {
					slug = str
				}
			}
			if slug == "" {
				slug = slg.Make(title)
			}
		case SlugFromTitle:
			slug = slg.Make(title)
		default:
			slug = slg.Make(title)
		}

		// 根据 status 来源确定 status
		status := "draft"
		switch statusSource {
		case StatusFromFrontmatter:
			// 从 frontmatter 指定字段获取
			fieldName := statusField
			//if fieldName == "" {
			//	fieldName = "draft"
			//}
			if _, ok := result.Meta[fieldName]; ok {
				status = "draft"
			} else {
				status = "published"
			}
			// 如果没找到或为空，使用默认值 draft
			//if status == "" {
			//	status = "draft"
			//}
		case StatusAllDraft:
			// 全部 draft
			status = "draft"
		case StatusAllPublished:
			// 全部 published
			status = "published"
		default:
			// 默认从 frontmatter 的 status 字段获取
			if val, ok := result.Meta["status"]; ok {
				if str, ok := val.(string); ok && str != "" {
					status = str
				}
			}
			if status == "" {
				status = "draft"
			}
		}

		// 获取 Markdown 内容
		content := result.Markdown

		// 根据 created_at 来源确定创建时间
		var createdAt int64
		var createdAtStr string
		switch createdSource {
		case CreatedFromFrontmatter:
			// 从 frontmatter 指定字段获取
			fieldName := createdField
			if fieldName == "" {
				fieldName = "date"
			}
			if val, ok := result.Meta[fieldName]; ok {
				if dateStr, ok := val.(string); ok && dateStr != "" {
					t, err := time.Parse("2006-01-02T15:04:05.000Z", dateStr)
					if err == nil {
						loc, _ := time.LoadLocation("Asia/Shanghai")
						createdAt = t.In(loc).Unix()
						// 格式化为显示字符串 (YYYY-MM-DD HH:MM:SS)
						createdAtStr = t.In(loc).Format("2006-01-02 15:04:05")
					}
				}
			}
		case CreatedFromFileTime:
			// 使用当前时间（HTTP 上传无法获取文件创建时间）
			createdAt = time.Now().Unix()
			createdAtStr = time.Now().Format("2006-01-02 15:04:05")
		default:
			// 默认从 frontmatter 的 date 字段获取
			if val, ok := result.Meta["date"]; ok {
				if dateStr, ok := val.(string); ok && dateStr != "" {
					t, err := time.Parse("2006-01-02T15:04:05.000Z", dateStr)
					if err == nil {
						loc, _ := time.LoadLocation("Asia/Shanghai")
						createdAt = t.In(loc).Unix()
						createdAtStr = t.In(loc).Format("2006-01-02 15:04:05")
					}
				}
			}
		}
		// 如果都取不到，使用当前时间
		if createdAt == 0 {
			createdAt = time.Now().Unix()
			createdAtStr = time.Now().Format("2006-01-02 15:04:05")
		}

		// 处理 tags
		var tagsList []string
		var tagsStr string
		if val, ok := result.Meta["tags"]; ok {
			switch v := val.(type) {
			case string:
				if v != "" {
					tagsList = strings.Split(v, ",")
					for i, tag := range tagsList {
						tagsList[i] = strings.TrimSpace(tag)
					}
				}
			case []interface{}:
				for _, item := range v {
					if str, ok := item.(string); ok {
						tagsList = append(tagsList, strings.TrimSpace(str))
					}
				}
			}
			tagsStr = strings.Join(tagsList, ", ")
		}

		// 处理 category（单选）
		category := ""
		switch categorySource {
		case CategoryFromFrontmatter:
			// 从 frontmatter 指定字段获取
			fieldName := categoryField
			if fieldName == "" {
				fieldName = "category"
			}
			if val, ok := result.Meta[fieldName]; ok {
				switch v := val.(type) {
				case string:
					// 如果是字符串，直接使用
					category = strings.TrimSpace(v)
				case []interface{}:
					// 如果是数组，使用第一个元素
					if len(v) > 0 {
						if str, ok := v[0].(string); ok {
							category = strings.TrimSpace(str)
						}
					}
				}
			}
		case CategoryAutoCreate:
			// 自动创建默认分类（使用默认分类名称 "Default"）
			category = "Default"
		case CategoryNone:
			// 留空，不设置分类
			category = ""
		default:
			// 默认留空
			category = ""
		}

		items = append(items, PreviewPostItem{
			Index:         i,
			Filename:      file.Filename,
			Title:         title,
			Slug:          slug,
			Content:       content,
			Status:        status,
			CreatedAt:     createdAtStr,
			CreatedAtUnix: createdAt,
			Tags:          tagsStr,
			TagsList:      tagsList,
			Category:      category,
		})
	}

	if len(errList) > 0 {
		return items, errors.New(strings.Join(errList, "; "))
	}

	return items, nil
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

// ImportPreviewService 从预览数据导入到数据库
func ImportPreviewService(dbx *db.DB, items []PreviewPostItem) error {
	for _, item := range items {
		// 解析时间字符串为时间戳
		createdAt := item.CreatedAtUnix
		if item.CreatedAt != "" {
			// 尝试解析时间字符串 "2006-01-02 15:04:05"
			if t, err := time.Parse("2006-01-02 15:04:05", item.CreatedAt); err == nil {
				createdAt = t.Unix()
			}
		}
		if createdAt == 0 {
			createdAt = time.Now().Unix()
		}

		// 创建 post
		post := &db.Post{
			Title:     item.Title,
			Slug:      item.Slug,
			Content:   item.Content,
			Status:    item.Status,
			CreatedAt: createdAt,
			UpdatedAt: time.Now().Unix(),
		}

		if err := db.CreatePost(dbx, post); err != nil {
			return errors.New(item.Filename + ": " + err.Error())
		}

		// 处理 tags
		if item.Tags != "" {
			var tagIDs []int64
			tagNames := strings.Split(item.Tags, ",")
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

		// 处理 category（单选）
		if item.Category != "" {
			category, err := CreateCategoryByName(dbx, item.Category)
			if err == nil {
				var categoryIDs []int64
				categoryIDs = append(categoryIDs, category.ID)
				if err := db.SetPostCategories(dbx, post.ID, categoryIDs); err != nil {
					// category 关联失败不影响主流程
				}
			}
		}
	}

	return nil
}
