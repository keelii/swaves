package dash

import (
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/shared/helper"
	"swaves/internal/shared/md"
	"swaves/internal/shared/pathutil"
	"swaves/internal/shared/redirect_rule"
	"swaves/internal/shared/share"
	"swaves/internal/shared/types"
	"time"
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
	Title          string
	Slug           string
	Content        string
	Status         string
	Kind           db.PostKind
	TagIDs         []int64
	CategoryID     int64
	CommentEnabled *bool
}

// UpdatePostAction 编辑文章时的操作：save=保存草稿，publish=发布，update=更新已发布文章
type UpdatePostAction string

const (
	UpdatePostActionSave    UpdatePostAction = "save"
	UpdatePostActionPublish UpdatePostAction = "publish"
	UpdatePostActionUpdate  UpdatePostAction = "update"
)

type UpdatePostInput struct {
	Title          string
	Content        string
	Status         string
	Kind           db.PostKind
	TagIDs         []int64
	CategoryID     int64
	CommentEnabled *bool
	Action         UpdatePostAction // save | publish | update
}

func GetAllTags(dbx *db.DB) ([]db.Tag, error) {
	rows, err := dbx.Query(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM ` + string(db.TableTags) + `
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

func findRedirectConflictsForPostSlug(dbx *db.DB, slug string) (string, string, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return "", "", nil
	}

	slugPath := pathutil.JoinAbsolute(share.GetBasePath(), slug)
	slugDirPrefix := slugPath + "/"

	redirects, _, err := db.ListRedirects(dbx, 0, 0)
	if err != nil {
		return "", "", err
	}

	for _, redirect := range redirects {
		fromPath := normalizeRedirectPath(redirect.From)
		if fromPath == slugPath {
			return fromPath, "", nil
		}
		if redirect_rule.HasPattern(fromPath) {
			rule, err := redirect_rule.Compile(fromPath, redirect.To)
			if err == nil {
				if _, matched := rule.Match(slugPath); matched {
					return fromPath, "", nil
				}
				if _, matched := rule.Match(pathutil.JoinAbsolute(slugPath, "__swaves_conflict_probe__")); matched {
					return "", fromPath, nil
				}
			}
		}
		if strings.HasPrefix(fromPath, slugDirPrefix) {
			return "", fromPath, nil
		}
	}

	return "", "", nil
}

func CreatePostService(dbx *db.DB, in CreatePostInput) (int64, error) {
	if in.Title == "" || in.Slug == "" {
		return 0, errors.New("title and slug required")
	}
	if !helper.IsSlug(in.Slug) {
		return 0, errSlugInvalid("001", in.Slug)
	}
	conflictSource, conflictDirectory, err := findRedirectConflictsForPostSlug(dbx, in.Slug)
	if err != nil {
		return 0, err
	}
	if conflictSource != "" {
		return 0, fmt.Errorf("文章 slug 与重定向来源冲突：%s", conflictSource)
	}
	if conflictDirectory != "" {
		return 0, fmt.Errorf("文章 slug 与重定向目录冲突：%s", conflictDirectory)
	}

	if strings.TrimSpace(in.Content) == "" {
		return 0, errors.New("内容不能为空")
	}

	p := &db.Post{
		Title:   in.Title,
		Slug:    in.Slug,
		Content: in.Content,
		Status:  in.Status,
		Kind:    in.Kind,
	}
	if _, err := db.CreatePost(dbx, p); err != nil {
		return 0, err
	}

	if in.CommentEnabled != nil && !*in.CommentEnabled {
		if err := db.SetPostCommentEnabled(dbx, p.ID, false); err != nil {
			return 0, err
		}
	}

	// 关联标签
	if len(in.TagIDs) > 0 {
		if err := db.SetPostTags(dbx, p.ID, in.TagIDs); err != nil {
			return 0, err
		}
	}

	// 关联分类（单选）
	if in.CategoryID > 0 {
		if err := db.SetPostCategory(dbx, p.ID, in.CategoryID); err != nil {
			return 0, err
		}
	}

	return p.ID, nil
}

func GetPostForEdit(dbx *db.DB, id int64) (*db.PostWithRelation, error) {
	post, err := db.GetPostByIDAnyStatus(dbx, id)
	if err != nil {
		return nil, err
	}

	tags, err := db.GetPostTags(dbx, id)

	if err != nil {
		return nil, err
	}

	return &db.PostWithRelation{
		Post: &post,
		Tags: tags,
	}, nil
}

func UpdatePostService(dbx *db.DB, id int64, in UpdatePostInput) error {
	p, err := db.GetPostByIDAnyStatus(dbx, id)
	if err != nil {
		return err
	}

	p.Title = in.Title
	p.Content = in.Content
	p.Kind = in.Kind
	if in.CommentEnabled != nil {
		if *in.CommentEnabled {
			p.CommentEnabled = 1
		} else {
			p.CommentEnabled = 0
		}
	}

	switch in.Action {
	case UpdatePostActionPublish:
		p.Status = "published"
		if p.PublishedAt == 0 {
			p.PublishedAt = time.Now().Unix()
		}
	case UpdatePostActionSave:
		p.Status = "draft"
		// 不修改 PublishedAt
	case UpdatePostActionUpdate:
		// 已发布状态仅更新内容，不修改 status、published_at
		p.Status = "published"
		// p.PublishedAt 保持原值
	default:
		if in.Status != "" {
			p.Status = in.Status
		} else {
			p.Status = "draft"
		}
	}

	if err := db.UpdatePost(dbx, &p); err != nil {
		return err
	}
	if in.Action == UpdatePostActionPublish {
		if err := db.PublishPost(dbx, p.ID); err != nil {
			return err
		}
	}

	// 更新标签关联
	if err := db.SetPostTags(dbx, id, in.TagIDs); err != nil {
		return err
	}

	// 更新分类关联（单选）
	if err := db.SetPostCategory(dbx, id, in.CategoryID); err != nil {
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

func ListTags(dbx *db.DB, pager *types.Pagination) ([]db.Tag, error) {
	var total int
	row := dbx.QueryRow(`SELECT COUNT(*) FROM ` + string(db.TableTags) + ` WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := dbx.Query(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM `+string(db.TableTags)+`
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

func CountTagsService(dbx *db.DB) (int, error) {
	return db.CountTags(dbx)
}

func generateSlug(name string) string {
	return helper.MakeSlug(name)
}

func isUniqueConstraintErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "unique constraint failed")
}

func findActiveTagBySlug(dbx *db.DB, slug string) (*db.Tag, error) {
	row := dbx.QueryRow(`
		SELECT id, name, slug, created_at, updated_at, deleted_at
		FROM `+string(db.TableTags)+`
		WHERE slug = ? AND deleted_at IS NULL
		LIMIT 1
	`, slug)

	var t db.Tag
	var deletedAt sql.NullInt64
	if err := row.Scan(
		&t.ID,
		&t.Name,
		&t.Slug,
		&t.CreatedAt,
		&t.UpdatedAt,
		&deletedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	if deletedAt.Valid {
		t.DeletedAt = &deletedAt.Int64
	}
	return &t, nil
}

func findActiveCategoryByNameOrSlug(dbx *db.DB, name string, slug string) (*db.Category, error) {
	row := dbx.QueryRow(`
		SELECT id, parent_id, name, slug, description, sort, created_at, updated_at, deleted_at
		FROM `+string(db.TableCategories)+`
		WHERE (name = ? OR slug = ?) AND deleted_at IS NULL
		LIMIT 1
	`, name, slug)

	var c db.Category
	var parentID sql.NullInt64
	var deletedAt sql.NullInt64
	if err := row.Scan(
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
		if errors.Is(err, sql.ErrNoRows) {
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

func CreateTagService(dbx *db.DB, in CreateTagInput) error {
	if in.Name == "" {
		return errors.New("name required")
	}

	slug := in.Slug
	if slug == "" {
		slug = generateSlug(in.Name)
	}
	slug = strings.Trim(slug, "-")
	if slug == "" {
		slug = "tag"
	}
	if !helper.IsSlug(slug) {
		return errSlugInvalid("002", slug)
	}

	t := &db.Tag{
		Name: in.Name,
		Slug: slug,
	}
	_, err := db.CreateTag(dbx, t)
	return err
}

// CreateTagByName 按名称创建或获取标签。postCreatedAt 为引用该标签的文章的创建时间（导入时传入，0 表示使用当前时间）。
func CreateTagByName(dbx *db.DB, name string, postCreatedAt int64) (*db.Tag, error) {
	if name == "" {
		return nil, errors.New("name required")
	}

	slug := strings.Trim(generateSlug(name), "-")
	if slug == "" {
		slug = "tag"
	}
	if !helper.IsSlug(slug) {
		return nil, errSlugInvalid("003", slug)
	}

	// 检查标签是否已存在
	existingTag, err := findActiveTagBySlug(dbx, slug)
	if err != nil {
		return nil, err
	}
	if existingTag != nil {
		if postCreatedAt > 0 && postCreatedAt < existingTag.CreatedAt {
			_ = db.UpdateTagCreatedAtIfEarlier(dbx, existingTag.ID, postCreatedAt)
			existingTag.CreatedAt = postCreatedAt
		}
		return existingTag, nil
	}

	createdAt := postCreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	t := &db.Tag{
		Name:      name,
		Slug:      slug,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if _, err := db.CreateTag(dbx, t); err != nil {
		if isUniqueConstraintErr(err) {
			existingTag, findErr := findActiveTagBySlug(dbx, slug)
			if findErr == nil && existingTag != nil {
				if postCreatedAt > 0 && postCreatedAt < existingTag.CreatedAt {
					_ = db.UpdateTagCreatedAtIfEarlier(dbx, existingTag.ID, postCreatedAt)
					existingTag.CreatedAt = postCreatedAt
				}
				return existingTag, nil
			}
		}
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
	if in.Slug != "" && !helper.IsSlug(in.Slug) {
		return errSlugInvalid("004", in.Slug)
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

func ListRedirects(dbx *db.DB, pager *types.Pagination) ([]db.Redirect, error) {
	offset := (pager.Page - 1) * pager.PageSize
	res, total, err := db.ListRedirects(dbx, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return res, nil
}

func CountRedirectsService(dbx *db.DB) (int, error) {
	return db.CountRedirects(dbx)
}

// checkRedirectCycle 检查是否存在循环重定向
func checkRedirectCycle(dbx *db.DB, fromPath, toPath string) error {
	if redirect_rule.HasPattern(fromPath) || redirect_rule.HasPattern(toPath) {
		if fromPath == toPath {
			return errors.New("from 和 to 不能相同")
		}
		return nil
	}

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
			if db.IsErrNotFound(err) {
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

func normalizeRedirectPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return pathutil.JoinAbsolute(path)
}

func extractPathSingleSlug(path string) string {
	path = strings.Trim(path, "/")
	if path == "" || strings.Contains(path, "/") {
		return ""
	}
	return path
}

func findPostRedirectSourceConflicts(dbx *db.DB, fromPath string) (string, string, error) {
	fromPath = normalizeRedirectPath(fromPath)
	if fromPath == "" {
		return "", "", nil
	}
	slugCandidate := extractPathSingleSlug(fromPath)
	var patternRule redirect_rule.Rule
	var patternEnabled bool
	if redirect_rule.HasPattern(fromPath) {
		rule, err := redirect_rule.Compile(fromPath, "/")
		if err != nil {
			return "", "", err
		}
		patternRule = rule
		patternEnabled = true
	}

	page := 1
	for {
		pager := types.Pagination{
			Page:     page,
			PageSize: 200,
		}
		posts, err := db.ListPosts(dbx, &db.PostQueryOptions{Pager: &pager})
		if err != nil {
			return "", "", err
		}

		for _, item := range posts {
			if item.Post == nil {
				continue
			}

			postSlug := strings.TrimSpace(item.Post.Slug)
			if slugCandidate != "" && postSlug == slugCandidate {
				return "", slugCandidate, nil
			}
			if patternEnabled && postSlug != "" {
				if _, matched := patternRule.Match(pathutil.JoinAbsolute(postSlug)); matched {
					return "", postSlug, nil
				}
			}

			if item.Post.Status == "published" {
				postURL := normalizeRedirectPath(share.GetPostUrl(*item.Post))
				if postURL == fromPath {
					return postURL, "", nil
				}
				if patternEnabled {
					if _, matched := patternRule.Match(postURL); matched {
						return postURL, "", nil
					}
				}
			}
		}

		if pager.Num == 0 || page >= pager.Num {
			break
		}
		page++
	}

	return "", "", nil
}

func CreateRedirectService(dbx *db.DB, in CreateRedirectInput) error {
	in, err := validateRedirectCreateInput(dbx, in)
	if err != nil {
		return err
	}

	r := &db.Redirect{
		From:    in.From,
		To:      in.To,
		Status:  in.Status,
		Enabled: in.Enabled,
	}
	_, err = db.CreateRedirect(dbx, r)
	return err
}

func GetRedirectForEdit(dbx *db.DB, id int64) (*db.Redirect, error) {
	return db.GetRedirectByID(dbx, id)
}

func UpdateRedirectService(dbx *db.DB, id int64, in UpdateRedirectInput) error {
	r, err := db.GetRedirectByID(dbx, id)
	if err != nil {
		return err
	}

	in, err = validateRedirectUpdateInput(dbx, id, in)
	if err != nil {
		return err
	}

	r.From = in.From
	r.To = in.To
	r.Status = in.Status
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

func ListEncryptedPosts(dbx *db.DB, pager *types.Pagination) ([]db.EncryptedPost, error) {
	var total int
	row := dbx.QueryRow(`SELECT COUNT(*) FROM ` + string(db.TableEncryptedPosts) + ` WHERE deleted_at IS NULL`)
	if err := row.Scan(&total); err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	rows, err := dbx.Query(`
		SELECT id, title, slug, content, password, expires_at, created_at, updated_at, deleted_at
		FROM `+string(db.TableEncryptedPosts)+`
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
	_, err := db.CreateEncryptedPost(dbx, p)
	return err
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

type CategorySelectOption struct {
	ID          int64
	Name        string
	DisplayName string
	Depth       int
}

func FormatCategorySelectLabel(name string, depth int) string {
	if depth <= 0 {
		return name
	}
	return strings.Repeat("　", depth) + name
}

func BuildCategorySelectOptions(list []db.Category) []CategorySelectOption {
	if len(list) == 0 {
		return []CategorySelectOption{}
	}

	categoryByID := make(map[int64]db.Category, len(list))
	childrenByParent := make(map[int64][]db.Category, len(list))
	roots := make([]db.Category, 0, len(list))

	for _, category := range list {
		categoryByID[category.ID] = category
	}

	for _, category := range list {
		parentID := category.ParentID
		if parentID == 0 || parentID == category.ID {
			roots = append(roots, category)
			continue
		}
		if _, ok := categoryByID[parentID]; !ok {
			roots = append(roots, category)
			continue
		}
		childrenByParent[parentID] = append(childrenByParent[parentID], category)
	}

	options := make([]CategorySelectOption, 0, len(list))
	visited := make(map[int64]bool, len(list))

	var walk func(category db.Category, depth int)
	walk = func(category db.Category, depth int) {
		if visited[category.ID] {
			return
		}
		visited[category.ID] = true
		options = append(options, CategorySelectOption{
			ID:          category.ID,
			Name:        category.Name,
			DisplayName: FormatCategorySelectLabel(category.Name, depth),
			Depth:       depth,
		})
		for _, child := range childrenByParent[category.ID] {
			walk(child, depth+1)
		}
	}

	for _, root := range roots {
		walk(root, 0)
	}
	for _, category := range list {
		if !visited[category.ID] {
			walk(category, 0)
		}
	}

	return options
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
	list, err := db.ListCategories(dbx, false)
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
	list, err := db.ListCategories(dbx, false)
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

func ListCategoriesService(dbx *db.DB, pager *types.Pagination) ([]db.Category, error) {
	if pager == nil {
		return db.ListCategories(dbx, false)
	}

	total, err := db.CountCategories(dbx)
	if err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	res, err := db.ListCategoriesPaged(dbx, false, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return res, nil
}

func CountCategoriesService(dbx *db.DB) (int, error) {
	return db.CountCategories(dbx)
}

func GetAllCategoriesFlat(dbx *db.DB) ([]db.Category, error) {
	return db.ListCategories(dbx, false)
}

func CreateCategoryService(dbx *db.DB, in CreateCategoryInput) error {
	if in.Name == "" {
		return errors.New("name required")
	}
	slug := in.Slug
	if slug != "" && !helper.IsSlug(slug) {
		return errSlugInvalid("005", slug)
	}

	c := &db.Category{
		ParentID:    in.ParentID,
		Name:        in.Name,
		Slug:        in.Slug,
		Description: in.Description,
		Sort:        in.Sort,
	}
	_, err := db.CreateCategory(dbx, c)
	return err
}

// CreateCategoryByName 按名称创建或获取分类（顶级）。postCreatedAt 为引用该分类的文章的创建时间（导入时传入，0 表示使用当前时间）。
func CreateCategoryByName(dbx *db.DB, name string, postCreatedAt int64) (*db.Category, error) {
	if name == "" {
		return nil, errors.New("name required")
	}

	slug := helper.MakeSlug(name)
	if slug == "" {
		slug = "category"
	}
	if !helper.IsSlug(slug) {
		return nil, errSlugInvalid("006", slug)
	}

	// 检查分类是否已存在（根据名称和 slug，由于是单选，只需要检查顶级分类中是否存在相同的 slug）
	existingCategory, err := findActiveCategoryByNameOrSlug(dbx, name, slug)
	if err != nil {
		return nil, err
	}
	if existingCategory != nil {
		if postCreatedAt > 0 && postCreatedAt < existingCategory.CreatedAt {
			_ = db.UpdateCategoryCreatedAtIfEarlier(dbx, existingCategory.ID, postCreatedAt)
			existingCategory.CreatedAt = postCreatedAt
		}
		return existingCategory, nil
	}

	createdAt := postCreatedAt
	if createdAt <= 0 {
		createdAt = time.Now().Unix()
	}
	c := &db.Category{
		Name:      name,
		Slug:      slug,
		CreatedAt: createdAt,
		UpdatedAt: createdAt,
	}
	if _, err := db.CreateCategory(dbx, c); err != nil {
		if isUniqueConstraintErr(err) {
			existingCategory, findErr := findActiveCategoryByNameOrSlug(dbx, name, slug)
			if findErr == nil && existingCategory != nil {
				if postCreatedAt > 0 && postCreatedAt < existingCategory.CreatedAt {
					_ = db.UpdateCategoryCreatedAtIfEarlier(dbx, existingCategory.ID, postCreatedAt)
					existingCategory.CreatedAt = postCreatedAt
				}
				return existingCategory, nil
			}
		}
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
	if in.Slug != "" && !helper.IsSlug(in.Slug) {
		return errSlugInvalid("007", in.Slug)
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
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		return nil, err
	}
	return db.ListAllSettings(dbx)
}

func GetSettingByCode(dbx *db.DB, code string) (*db.Setting, error) {
	return db.GetSettingByCode(dbx, code)
}

func GetSettingByID(dbx *db.DB, id int64) (*db.Setting, error) {
	return db.GetSettingByID(dbx, id)
}

func CreateSettingService(dbx *db.DB, s *db.Setting) error {
	_, err := db.CreateSetting(dbx, s)
	return err
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

func CountTrashPosts(dbx *db.DB) (int, error) {
	return db.CountDeletedPosts(dbx)
}

func GetTrashEncryptedPosts(dbx *db.DB) ([]db.EncryptedPost, error) {
	return db.ListDeletedEncryptedPosts(dbx)
}

func CountTrashEncryptedPosts(dbx *db.DB) (int, error) {
	return db.CountDeletedEncryptedPosts(dbx)
}

func GetTrashTags(dbx *db.DB) ([]db.Tag, error) {
	return db.ListDeletedTags(dbx)
}

func CountTrashTags(dbx *db.DB) (int, error) {
	return db.CountDeletedTags(dbx)
}

func GetTrashRedirects(dbx *db.DB) ([]db.Redirect, error) {
	return db.ListDeletedRedirects(dbx)
}

func CountTrashRedirects(dbx *db.DB) (int, error) {
	return db.CountDeletedRedirects(dbx)
}

func GetTrashCategories(dbx *db.DB) ([]db.Category, error) {
	return db.ListDeletedCategories(dbx)
}

func CountTrashCategories(dbx *db.DB) (int, error) {
	return db.CountDeletedCategories(dbx)
}

func RestorePostService(dbx *db.DB, id int64) error {
	return db.RestorePost(dbx, id)
}

func HardDeletePostService(dbx *db.DB, id int64) error {
	return db.HardDeletePost(dbx, id)
}

func RestoreEncryptedPostService(dbx *db.DB, id int64) error {
	return db.RestoreEncryptedPost(dbx, id)
}

func HardDeleteEncryptedPostService(dbx *db.DB, id int64) error {
	return db.HardDeleteEncryptedPost(dbx, id)
}

func RestoreTagService(dbx *db.DB, id int64) error {
	return db.RestoreTag(dbx, id)
}

func HardDeleteTagService(dbx *db.DB, id int64) error {
	return db.HardDeleteTag(dbx, id)
}

func RestoreRedirectService(dbx *db.DB, id int64) error {
	return db.RestoreRedirect(dbx, id)
}

func HardDeleteRedirectService(dbx *db.DB, id int64) error {
	return db.HardDeleteRedirect(dbx, id)
}

func RestoreCategoryService(dbx *db.DB, id int64) error {
	return db.RestoreCategory(dbx, id)
}

func HardDeleteCategoryService(dbx *db.DB, id int64) error {
	return db.HardDeleteCategory(dbx, id)
}

// Comments
func ListCommentsService(dbx *db.DB, status db.CommentStatus, pager *types.Pagination) ([]db.Comment, error) {
	offset := (pager.Page - 1) * pager.PageSize
	items, total, err := db.ListCommentsForDash(dbx, status, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize

	return items, nil
}

func CountCommentsService(dbx *db.DB, status db.CommentStatus) (int, error) {
	return db.CountComments(dbx, status)
}

func UpdateCommentStatusService(dbx *db.DB, id int64, status db.CommentStatus) error {
	return db.UpdateCommentStatus(dbx, id, status)
}

func DeleteCommentService(dbx *db.DB, id int64) error {
	return db.SoftDeleteComment(dbx, id)
}

// Tasks
type CreateTaskInput struct {
	Code        string
	Name        string
	Description string
	Schedule    string
	Enabled     bool
	Kind        db.TaskKind
}

func CreateTaskService(dbx *db.DB, in CreateTaskInput) error {
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

	task := &db.Task{
		Code:        in.Code,
		Name:        in.Name,
		Description: in.Description,
		Schedule:    in.Schedule,
		Enabled:     enabled,
		Kind:        in.Kind,
	}

	_, err := db.CreateTask(dbx, task)
	return err
}

type UpdateTaskInput struct {
	Code        string
	Name        string
	Description string
	Schedule    string
	Enabled     bool
	Kind        db.TaskKind
}

func UpdateTaskService(dbx *db.DB, id int64, in UpdateTaskInput) error {
	task, err := db.GetTaskByID(dbx, id)
	if err != nil {
		return err
	}

	// Code 不可修改，保持原有值
	task.Name = in.Name
	task.Description = in.Description
	task.Schedule = in.Schedule
	task.Kind = in.Kind
	if in.Enabled {
		task.Enabled = 1
	} else {
		task.Enabled = 0
	}

	return db.UpdateTask(dbx, task)
}

func ListTasksService(dbx *db.DB, pager *types.Pagination) ([]db.Task, error) {
	if pager == nil {
		return db.ListTasks(dbx)
	}

	total, err := db.CountTasks(dbx)
	if err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	res, err := db.ListTasksPaged(dbx, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return res, nil
}

func CountTasksService(dbx *db.DB) (int, error) {
	return db.CountTasks(dbx)
}

func GetTaskForEdit(dbx *db.DB, id int64) (*db.Task, error) {
	return db.GetTaskByID(dbx, id)
}

func DeleteTaskService(dbx *db.DB, id int64) error {
	task, err := db.GetTaskByID(dbx, id)
	if err != nil {
		return err
	}
	if task.Kind == db.TaskInternal {
		return errors.New("internal task cannot be deleted")
	}
	return db.SoftDeleteTask(dbx, id)
}

// TaskRuns
func ListTaskRunsService(dbx *db.DB, taskCode string, limit int) ([]db.TaskRun, error) {
	return db.ListTaskRuns(dbx, taskCode, "", limit)
}

func CreatePendingRunService(dbx *db.DB, taskCode string) error {
	now := time.Now().Unix()
	run := &db.TaskRun{
		TaskCode:   taskCode,
		Status:     "pending",
		Message:    "",
		StartedAt:  now, // pending 状态时，started_at 设置为当前时间
		FinishedAt: now, // pending 状态时，finished_at 设置为当前时间（满足 NOT NULL 约束）
		Duration:   0,   // pending 状态时，duration 为 0
	}
	_, err := db.CreateTaskRun(dbx, run)
	return err
}

func ListNotificationsService(dbx *db.DB, receiver string, eventType string, pager *types.Pagination) ([]db.Notification, error) {
	if pager == nil {
		return db.ListNotificationsByEventType(dbx, receiver, eventType, 20, 0)
	}

	total, err := db.CountNotificationsByEventType(dbx, receiver, eventType)
	if err != nil {
		return nil, err
	}

	offset := (pager.Page - 1) * pager.PageSize
	items, err := db.ListNotificationsByEventType(dbx, receiver, eventType, pager.PageSize, offset)
	if err != nil {
		return nil, err
	}

	pager.Total = total
	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	return items, nil
}

func CountUnreadNotificationsService(dbx *db.DB, receiver string) (int, error) {
	return db.CountUnreadNotifications(dbx, receiver)
}

func CountNotificationsByEventTypeService(dbx *db.DB, receiver string, eventType string) (int, error) {
	return db.CountNotificationsByEventType(dbx, receiver, eventType)
}

func GetLatestNotificationByEventTypeService(dbx *db.DB, receiver string, eventType string) (*db.Notification, error) {
	items, err := db.ListNotificationsByEventType(dbx, receiver, eventType, 1, 0)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func MarkNotificationReadService(dbx *db.DB, id int64, receiver string) error {
	return db.MarkNotificationRead(dbx, id, receiver)
}

func MarkAllNotificationsReadService(dbx *db.DB, receiver string) (int64, error) {
	return db.MarkAllNotificationsRead(dbx, receiver)
}

func DeleteNotificationService(dbx *db.DB, id int64, receiver string) error {
	return db.DeleteNotification(dbx, id, receiver)
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

type TagSource int

const (
	TagFromFrontmatter TagSource = iota // 从 frontmatter 指定字段（默认 tags）
	TagAutoCreate                       // 自动创建默认标签
	TagNone                             // 留空（不设置标签）
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
	TagSource      TagSource      // tag 来源
	TagField       string         // 如果 TagSource 是 TagFromFrontmatter，指定字段名（默认 "tags"）
}

// PreviewPostItem 预览页面的 post 数据
type PreviewPostItem struct {
	Index          int      // 索引（用于表单字段命名）
	PostID         int64    // 临时导入记录对应的 post ID
	Filename       string   // 原始文件名
	Title          string   // 标题
	Slug           string   // slug
	Content        string   // 内容（markdown）
	ContentPreview string   // 内容预览（用于导入预览表格展示）
	Status         string   // 状态
	Kind           string   // 类型： "0"=post, "1"=page
	CreatedAt      string   // 创建时间（格式化的时间字符串，用于显示）
	CreatedAtUnix  int64    // 创建时间（Unix 时间戳）
	Tags           string   // 标签（逗号分隔的字符串）
	TagsList       []string // 标签列表
	Category       string   // 分类名称
	Categories     string   // 分类列表（逗号分隔，首个默认关联文章）
}

const importPreviewContentLimit = 200
const importingPostStatus = "importing"

func buildImportContentPreview(content string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\n", " ")
	normalized = strings.Join(strings.Fields(normalized), " ")

	runes := []rune(normalized)
	if len(runes) <= importPreviewContentLimit {
		return normalized
	}
	return string(runes[:importPreviewContentLimit]) + "..."
}

func appendUniqueTrimmed(items []string, value string) []string {
	v := normalizeImportListValue(value)
	if v == "" {
		return items
	}
	for _, existing := range items {
		if existing == v {
			return items
		}
	}
	return append(items, v)
}

func normalizeImportListValue(raw string) string {
	raw = strings.Trim(strings.TrimSpace(raw), "\"'")
	return strings.TrimSpace(raw)
}

func splitAndNormalizeCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return []string{}
	}
	parts := strings.Split(value, ",")
	items := make([]string, 0, len(parts))
	for _, part := range parts {
		items = appendUniqueTrimmed(items, part)
	}
	return items
}

func normalizeImportTargetStatus(raw string) string {
	if strings.EqualFold(strings.TrimSpace(raw), "published") {
		return "published"
	}
	return "draft"
}

func normalizeImportKind(raw string) (db.PostKind, string) {
	if strings.TrimSpace(raw) == "1" {
		return db.PostKindPage, "1"
	}
	return db.PostKindPost, "0"
}

func parseImportCreatedAt(createdAtText string, createdAtUnix int64) int64 {
	if strings.TrimSpace(createdAtText) != "" {
		if t, err := time.Parse("2006-01-02 15:04:05", strings.TrimSpace(createdAtText)); err == nil {
			return t.Unix()
		}
	}
	if createdAtUnix > 0 {
		return createdAtUnix
	}
	return time.Now().Unix()
}

func formatImportCreatedAt(createdAtUnix int64) string {
	if createdAtUnix <= 0 {
		return ""
	}
	return time.Unix(createdAtUnix, 0).Format("2006-01-02 15:04:05")
}

func applyImportPreviewRelations(dbx *db.DB, postID int64, item PreviewPostItem, createdAt int64) {
	tagNames := splitAndNormalizeCSV(item.Tags)
	tagIDs := make([]int64, 0, len(tagNames))
	for _, tagName := range tagNames {
		tag, err := CreateTagByName(dbx, tagName, createdAt)
		if err == nil {
			tagIDs = append(tagIDs, tag.ID)
		}
	}
	if err := db.SetPostTags(dbx, postID, tagIDs); err != nil {
		// tag 关联失败不影响主流程
	}

	primaryCategory := normalizeImportListValue(item.Category)
	categoryNames := splitAndNormalizeCSV(item.Categories)
	if primaryCategory == "" && len(categoryNames) > 0 {
		primaryCategory = categoryNames[0]
	}
	categoryNames = appendUniqueTrimmed(categoryNames, primaryCategory)

	categoryIDByName := make(map[string]int64, len(categoryNames))
	for _, categoryName := range categoryNames {
		category, err := CreateCategoryByName(dbx, categoryName, createdAt)
		if err != nil {
			continue
		}
		categoryIDByName[categoryName] = category.ID
	}

	if primaryCategory != "" {
		if categoryID, ok := categoryIDByName[primaryCategory]; ok {
			if err := db.SetPostCategory(dbx, postID, categoryID); err != nil {
				// category 关联失败不影响主流程
			}
			return
		}
	}

	if err := db.SetPostCategory(dbx, postID, 0); err != nil {
		// category 关联失败不影响主流程
	}
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
func ParseImportFiles(files []ImportFile, slugSource SlugSource, slugField string, titleSource TitleSource, titleField string, titleLevel int, createdSource CreatedSource, createdField string, statusSource StatusSource, statusField string, categorySource CategorySource, categoryField string, tagSource TagSource, tagField string) ([]PreviewPostItem, error) {
	var items []PreviewPostItem
	var errList []string

	for i, file := range files {
		if file.Content == "" {
			errList = append(errList, file.Filename+": markdown content is required")
			continue
		}

		// 解析 markdown（导入预览不需要 TOC）
		result := md.ParseMarkdown(file.Content, false)

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
				slug = helper.MakeSlug(file.Filename)
			} else {
				slug = helper.MakeSlug(title)
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
				slug = helper.MakeSlug(title)
			}
		case SlugFromTitle:
			slug = helper.MakeSlug(title)
		default:
			slug = helper.MakeSlug(title)
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
		switch tagSource {
		case TagFromFrontmatter:
			// 从 frontmatter 指定字段获取
			fieldName := tagField
			if fieldName == "" {
				fieldName = "tags"
			}
			if val, ok := result.Meta[fieldName]; ok {
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
			}
			tagsStr = strings.Join(tagsList, ", ")
		case TagAutoCreate:
			// 自动创建默认标签（使用默认标签名称 "Default"）
			tagsList = []string{"Default"}
			tagsStr = "Default"
		case TagNone:
			// 留空，不设置标签
			tagsList = []string{}
			tagsStr = ""
		default:
			// 默认留空
			tagsList = []string{}
			tagsStr = ""
		}

		// 处理 category（单选，导入时默认关联第一个；其余分类也会创建）
		category := ""
		categoryList := make([]string, 0, 4)
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
					// 兼容单值或逗号分隔字符串
					categoryList = append(categoryList, splitAndNormalizeCSV(v)...)
				case []interface{}:
					for _, item := range v {
						if str, ok := item.(string); ok {
							categoryList = appendUniqueTrimmed(categoryList, str)
						}
					}
				}
			}
		case CategoryAutoCreate:
			// 自动创建默认分类（使用默认分类名称 "Default"）
			category = "Default"
			categoryList = append(categoryList, category)
		case CategoryNone:
			// 留空，不设置分类
			category = ""
		default:
			// 默认留空
			category = ""
		}
		if len(categoryList) > 0 && category == "" {
			category = categoryList[0]
		}
		categoryList = appendUniqueTrimmed(categoryList, category)
		categoryCSV := strings.Join(categoryList, ", ")

		items = append(items, PreviewPostItem{
			Index:          i,
			Filename:       file.Filename,
			Title:          title,
			Slug:           slug,
			Content:        content,
			ContentPreview: buildImportContentPreview(content),
			Status:         status,
			Kind:           "0", // 默认 post
			CreatedAt:      createdAtStr,
			CreatedAtUnix:  createdAt,
			Tags:           tagsStr,
			TagsList:       tagsList,
			Category:       category,
			Categories:     categoryCSV,
		})
	}

	if len(errList) > 0 {
		return items, errors.New(strings.Join(errList, "; "))
	}

	// 按 created_at 倒序（时间晚的在前）
	sort.Slice(items, func(i, j int) bool {
		return items[i].CreatedAtUnix > items[j].CreatedAtUnix
	})
	for i := range items {
		items[i].Index = i
	}

	return items, nil
}

// importSingleMarkdown 导入单个 markdown 文件
func importSingleMarkdown(dbx *db.DB, file ImportFile, slugSource SlugSource, slugField string) error {
	if file.Content == "" {
		return errors.New("markdown content is required")
	}

	// 解析 markdown（导入时不需要 TOC）
	result := md.ParseMarkdown(file.Content, false)

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
			slug = helper.MakeSlug(file.Filename)
		} else {
			// 如果文件名不存在，回退到从 title 生成
			slug = helper.MakeSlug(title)
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
			slug = helper.MakeSlug(title)
		}
	case SlugFromTitle:
		// 从 title 生成
		slug = helper.MakeSlug(title)
	default:
		// 默认从 title 生成
		slug = helper.MakeSlug(title)
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

	slug = strings.Trim(slug, "-")
	if slug == "" || !helper.IsSlug(slug) {
		slug = helper.MakeSlug(title)
		if slug == "" {
			slug = "post"
		}
		if !helper.IsSlug(slug) {
			return errSlugInvalid("008", slug)
		}
	}

	// 创建 post；导入时已发布则 published_at 默认为 created_at
	post := &db.Post{
		Title:     title,
		Slug:      slug,
		Content:   content,
		Status:    status,
		CreatedAt: createdAt,
		UpdatedAt: time.Now().Unix(),
	}
	if status == "published" {
		post.PublishedAt = createdAt
	}

	if _, err := db.CreatePost(dbx, post); err != nil {
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
				tag, err := CreateTagByName(dbx, tagName, post.CreatedAt)
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

func ListImportingPreviewItemsService(dbx *db.DB, pager *types.Pagination) ([]PreviewPostItem, error) {
	queryLimit := 0
	queryOffset := 0

	if pager != nil {
		row := dbx.QueryRow(`
			SELECT COUNT(1)
			FROM `+string(db.TablePosts)+`
			WHERE status = ? AND deleted_at IS NULL
		`, importingPostStatus)
		if err := row.Scan(&pager.Total); err != nil {
			return nil, err
		}
		if pager.PageSize > 0 {
			pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
			if pager.Page <= 0 {
				pager.Page = 1
			}
			if pager.Num > 0 && pager.Page > pager.Num {
				pager.Page = pager.Num
			}
			queryLimit = pager.PageSize
			queryOffset = (pager.Page - 1) * pager.PageSize
		}
	}

	querySQL := `
		SELECT id, title, slug, substr(content, 1, ?), kind, created_at, published_at
		FROM ` + string(db.TablePosts) + `
		WHERE status = ? AND deleted_at IS NULL
		ORDER BY created_at DESC, id DESC
	`
	queryArgs := []interface{}{importPreviewContentLimit * 4, importingPostStatus}
	if queryLimit > 0 {
		querySQL += " LIMIT ? OFFSET ?"
		queryArgs = append(queryArgs, queryLimit, queryOffset)
	}

	rows, err := dbx.Query(querySQL, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]PreviewPostItem, 0)
	for rows.Next() {
		var (
			item        PreviewPostItem
			content     string
			kind        db.PostKind
			publishedAt int64
		)
		if err := rows.Scan(
			&item.PostID,
			&item.Title,
			&item.Slug,
			&content,
			&kind,
			&item.CreatedAtUnix,
			&publishedAt,
		); err != nil {
			return nil, err
		}

		item.CreatedAt = formatImportCreatedAt(item.CreatedAtUnix)
		item.ContentPreview = buildImportContentPreview(content)
		if kind == db.PostKindPage {
			item.Kind = "1"
		} else {
			item.Kind = "0"
		}
		if publishedAt > 0 {
			item.Status = "published"
		} else {
			item.Status = "draft"
		}

		tags, err := db.GetPostTags(dbx, item.PostID)
		if err == nil {
			tagNames := make([]string, 0, len(tags))
			for _, tag := range tags {
				tagNames = appendUniqueTrimmed(tagNames, tag.Name)
			}
			item.Tags = strings.Join(tagNames, ", ")
		}

		if category, err := db.GetPostCategory(dbx, item.PostID); err == nil && category != nil {
			item.Category = normalizeImportListValue(category.Name)
			item.Categories = item.Category
		}

		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range items {
		items[i].Index = queryOffset + i
	}

	return items, nil
}

func ImportPreviewItemAsImportingService(dbx *db.DB, item PreviewPostItem) (PreviewPostItem, error) {
	item.Title = strings.TrimSpace(item.Title)
	if item.Title == "" {
		return item, errors.New("title is required")
	}

	createdAt := parseImportCreatedAt(item.CreatedAt, item.CreatedAtUnix)
	item.CreatedAtUnix = createdAt
	item.CreatedAt = formatImportCreatedAt(createdAt)

	kind, normalizedKind := normalizeImportKind(item.Kind)
	item.Kind = normalizedKind
	item.Status = normalizeImportTargetStatus(item.Status)

	slug := strings.Trim(item.Slug, "-")
	if slug == "" {
		slug = helper.MakeSlug(item.Title)
	}
	if slug == "" {
		slug = "post"
	}
	if !helper.IsSlug(slug) {
		return item, errSlugInvalid("017", item.Slug)
	}
	item.Slug = slug

	post := &db.Post{
		Title:     item.Title,
		Slug:      slug,
		Content:   item.Content,
		Status:    importingPostStatus,
		Kind:      kind,
		CreatedAt: createdAt,
		UpdatedAt: time.Now().Unix(),
	}
	if item.Status == "published" {
		post.PublishedAt = createdAt
	}

	id, err := db.CreatePost(dbx, post)
	if err != nil {
		if strings.TrimSpace(item.Filename) == "" {
			return item, err
		}
		return item, errors.New(item.Filename + ": " + err.Error())
	}

	item.PostID = id
	item.ContentPreview = buildImportContentPreview(item.Content)
	applyImportPreviewRelations(dbx, id, item, createdAt)
	return item, nil
}

func normalizeImportPreviewPersistFields(item PreviewPostItem, slugErrorCode string) (PreviewPostItem, db.PostKind, int64, int64, error) {
	title := strings.TrimSpace(item.Title)
	if title == "" {
		return item, db.PostKindPost, 0, 0, errors.New("title is required")
	}

	createdAt := parseImportCreatedAt(item.CreatedAt, item.CreatedAtUnix)
	status := normalizeImportTargetStatus(item.Status)
	kind, normalizedKind := normalizeImportKind(item.Kind)

	slug := strings.TrimSpace(strings.Trim(item.Slug, "-"))
	if slug == "" {
		slug = helper.MakeSlug(title)
	}
	if slug == "" {
		slug = "post"
	}
	if !helper.IsSlug(slug) {
		return item, kind, createdAt, 0, errSlugInvalid(slugErrorCode, item.Slug)
	}

	publishedAt := int64(0)
	if status == "published" {
		publishedAt = createdAt
		if publishedAt == 0 {
			publishedAt = time.Now().Unix()
		}
	}

	item.Title = title
	item.Slug = slug
	item.Status = status
	item.Kind = normalizedKind
	item.CreatedAtUnix = createdAt
	item.CreatedAt = formatImportCreatedAt(createdAt)

	return item, kind, createdAt, publishedAt, nil
}

func SaveImportPreviewItemService(dbx *db.DB, item PreviewPostItem) (PreviewPostItem, error) {
	if item.PostID <= 0 {
		return item, errors.New("post_id is required")
	}

	post, err := db.GetPostByIDAnyStatus(dbx, item.PostID)
	if err != nil {
		return item, err
	}
	if post.Status != importingPostStatus {
		return item, errors.New("record is not in importing status")
	}

	normalizedItem, kind, createdAt, publishedAt, err := normalizeImportPreviewPersistFields(item, "019")
	if err != nil {
		return item, err
	}
	normalizedItem.PostID = item.PostID
	if strings.TrimSpace(normalizedItem.Content) == "" {
		normalizedItem.Content = post.Content
	}

	if _, err = dbx.Exec(`
		UPDATE `+string(db.TablePosts)+`
		SET title = ?, slug = ?, content = ?, status = ?, kind = ?, created_at = ?, updated_at = ?, published_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, normalizedItem.Title, normalizedItem.Slug, normalizedItem.Content, importingPostStatus, kind, createdAt, time.Now().Unix(), publishedAt, item.PostID); err != nil {
		if strings.TrimSpace(item.Filename) == "" {
			return item, err
		}
		return item, errors.New(item.Filename + ": " + err.Error())
	}

	applyImportPreviewRelations(dbx, item.PostID, normalizedItem, createdAt)
	normalizedItem.ContentPreview = buildImportContentPreview(normalizedItem.Content)
	return normalizedItem, nil
}

func ConfirmImportPreviewItemService(dbx *db.DB, item PreviewPostItem) error {
	if item.PostID <= 0 {
		return errors.New("post_id is required")
	}

	post, err := db.GetPostByIDAnyStatus(dbx, item.PostID)
	if err != nil {
		return err
	}
	if post.Status != importingPostStatus {
		return errors.New("record is not in importing status")
	}

	normalizedItem, kind, createdAt, publishedAt, err := normalizeImportPreviewPersistFields(item, "018")
	if err != nil {
		return err
	}
	if strings.TrimSpace(normalizedItem.Content) == "" {
		normalizedItem.Content = post.Content
	}

	if _, err = dbx.Exec(`
		UPDATE `+string(db.TablePosts)+`
		SET title = ?, slug = ?, content = ?, status = ?, kind = ?, created_at = ?, updated_at = ?, published_at = ?
		WHERE id = ? AND deleted_at IS NULL
	`, normalizedItem.Title, normalizedItem.Slug, normalizedItem.Content, normalizedItem.Status, kind, createdAt, time.Now().Unix(), publishedAt, item.PostID); err != nil {
		if strings.TrimSpace(item.Filename) == "" {
			return err
		}
		return errors.New(item.Filename + ": " + err.Error())
	}

	normalizedItem.PostID = item.PostID
	applyImportPreviewRelations(dbx, item.PostID, normalizedItem, createdAt)
	return nil
}

type ImportConfirmAllResult struct {
	Total   int
	Success int
	Fail    int
	Errors  []string
}

func ConfirmAllImportingPreviewItemsService(dbx *db.DB) (ImportConfirmAllResult, error) {
	result := ImportConfirmAllResult{Errors: []string{}}

	items, err := ListImportingPreviewItemsService(dbx, nil)
	if err != nil {
		return result, err
	}
	result.Total = len(items)
	if result.Total == 0 {
		return result, nil
	}

	for _, item := range items {
		if err := ConfirmImportPreviewItemService(dbx, item); err != nil {
			result.Fail++
			title := strings.TrimSpace(item.Title)
			if title == "" {
				result.Errors = append(result.Errors, fmt.Sprintf("ID=%d: %v", item.PostID, err))
			} else {
				result.Errors = append(result.Errors, fmt.Sprintf("ID=%d(%s): %v", item.PostID, title, err))
			}
			continue
		}
		result.Success++
	}

	return result, nil
}

func CancelImportingPreviewItemsService(dbx *db.DB) (int64, error) {
	return db.CancelPostsByStatus(dbx, importingPostStatus)
}

func ImportPreviewItemService(dbx *db.DB, item PreviewPostItem) error {
	createdAt := parseImportCreatedAt(item.CreatedAt, item.CreatedAtUnix)

	item.Status = normalizeImportTargetStatus(item.Status)
	kind, _ := normalizeImportKind(item.Kind)

	slug := strings.Trim(item.Slug, "-")
	if slug == "" {
		slug = helper.MakeSlug(item.Title)
	}
	if slug == "" {
		slug = "post"
	}
	if !helper.IsSlug(slug) {
		return errSlugInvalid("009", item.Slug)
	}

	post := &db.Post{
		Title:     item.Title,
		Slug:      slug,
		Content:   item.Content,
		Status:    item.Status,
		Kind:      kind,
		CreatedAt: createdAt,
		UpdatedAt: time.Now().Unix(),
	}
	if item.Status == "published" {
		post.PublishedAt = createdAt
	}

	if _, err := db.CreatePost(dbx, post); err != nil {
		if strings.TrimSpace(item.Filename) == "" {
			return err
		}
		return errors.New(item.Filename + ": " + err.Error())
	}

	applyImportPreviewRelations(dbx, post.ID, item, createdAt)

	return nil
}

// ImportPreviewService 从预览数据导入到数据库
func ImportPreviewService(dbx *db.DB, items []PreviewPostItem) error {
	for _, item := range items {
		if err := ImportPreviewItemService(dbx, item); err != nil {
			return err
		}
	}
	return nil
}

func CountPost(dbx *db.DB) (int, int, int) {
	countPost, _ := db.CountPostsByKind(dbx, db.PostKindPost)
	countPage, _ := db.CountPostsByKind(dbx, db.PostKindPage)
	countEncryptedPost, _ := db.CountEncryptedPostsByKind(dbx)

	return countPost, countPage, countEncryptedPost
}
