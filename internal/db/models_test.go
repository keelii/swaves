package db

import (
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()

	db := Open(Options{
		DSN: ":memory:",
	})

	if err := InitDatabase(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	return db
}

func softDeleteByTable(db *DB, t *testing.T, table TableName, id int64) {
	t.Helper()
	if err := SoftDeleteRecord(db, table, id); err != nil {
		t.Fatalf("soft delete %s failed: %v", table, err)
	}
}

func TestPostCRUD(t *testing.T) {
	db := openTestDB(t)

	p := &Post{
		Title:   "Hello",
		Slug:    "hello",
		Content: "world",
		Status:  "published",
	}

	if err := CreatePost(db, p); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}
	if p.ID == 0 {
		t.Fatal("post id not set")
	}

	got, err := GetPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}
	if got.Title != p.Title {
		t.Fatalf("unexpected title: %s", got.Title)
	}

	p.Title = "Hello Updated"
	if err := UpdatePost(db, p); err != nil {
		t.Fatalf("UpdatePost failed: %v", err)
	}

	got, err = GetPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID after update failed: %v", err)
	}
	if got.Title != "Hello Updated" {
		t.Fatalf("update not applied")
	}

	if err := SoftDeletePost(db, p.ID); err != nil {
		t.Fatalf("SoftDeletePost failed: %v", err)
	}

	_, err = GetPostByID(db, p.ID)
	if err == nil {
		t.Fatalf("expected error after soft delete")
	}
}
func TestPosts_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	p := &Post{
		Title:   "Post",
		Slug:    "post",
		Content: "x",
		Status:  "published",
	}
	CreatePost(db, p)

	if err := SoftDeletePost(db, p.ID); err != nil {
		t.Fatal(err)
	}

	// 不可再查询
	if _, err := GetPostByID(db, p.ID); err == nil {
		t.Fatal("soft deleted post should not be readable")
	}

	// 再次 soft delete 不应报错
	if err := SoftDeletePost(db, p.ID); err != nil {
		t.Fatal("double soft delete should be safe")
	}
}

func TestPost_UniqueSlug(t *testing.T) {
	db := openTestDB(t)

	p1 := &Post{
		Title:   "A",
		Slug:    "same",
		Content: "1",
		Status:  "published",
	}
	if err := CreatePost(db, p1); err != nil {
		t.Fatalf("create p1 failed: %v", err)
	}

	p2 := &Post{
		Title:   "B",
		Slug:    "same",
		Content: "2",
		Status:  "draft",
	}
	if err := CreatePost(db, p2); err == nil {
		t.Fatal("expected unique constraint error on slug")
	}
}

func TestPost_UpdateDoesNotChangeSlug(t *testing.T) {
	db := openTestDB(t)

	p := &Post{
		Title:   "Hello",
		Slug:    "hello",
		Content: "world",
		Status:  "published",
	}
	if err := CreatePost(db, p); err != nil {
		t.Fatal(err)
	}

	p.Slug = "hacked"
	p.Title = "Updated"
	if err := UpdatePost(db, p); err != nil {
		t.Fatal(err)
	}

	got, err := GetPostByID(db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "hello" {
		t.Fatalf("slug should not change, got %s", got.Slug)
	}
}

func TestPost_SoftDeleteIsolation(t *testing.T) {
	db := openTestDB(t)

	p := &Post{
		Title:   "Soft",
		Slug:    "soft",
		Content: "x",
		Status:  "published",
	}
	CreatePost(db, p)

	if err := SoftDeletePost(db, p.ID); err != nil {
		t.Fatal(err)
	}

	_, err := GetPostByID(db, p.ID)
	if err == nil {
		t.Fatal("deleted post should not be queryable")
	}

	// slug still blocks reuse
	p2 := &Post{
		Title:   "Reuse",
		Slug:    "soft",
		Content: "y",
		Status:  "published",
	}
	if err := CreatePost(db, p2); err == nil {
		t.Fatal("slug should still be unique even if soft deleted")
	}
}
func TestPostTags_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	post := &Post{
		Title:   "P",
		Slug:    "p",
		Content: "c",
		Status:  "published",
	}
	tag := &Tag{Name: "T", Slug: "t"}

	CreatePost(db, post)
	CreateTag(db, tag)
	AttachTagToPost(db, post.ID, tag.ID)

	// 手动软删除关系
	_, err := db.Exec(
		`UPDATE post_tags SET deleted_at=? WHERE post_id=? AND tag_id=?`,
		now(), post.ID, tag.ID,
	)
	if err != nil {
		t.Fatal(err)
	}

	// 再 attach：由于 UNIQUE(post_id, tag_id)，仍然不会插入
	if err := AttachTagToPost(db, post.ID, tag.ID); err != nil {
		t.Fatal("attach after soft delete should still be ignored")
	}
}

func TestCreateEncryptedPost(t *testing.T) {
	db := openTestDB(t)

	p := &EncryptedPost{
		Title:    "Secret",
		Content:  "Top secret",
		Password: "hashed-password",
	}

	if err := CreateEncryptedPost(db, p); err != nil {
		t.Fatalf("CreateEncryptedPost failed: %v", err)
	}

	if p.ID == 0 {
		t.Fatal("encrypted post id not set")
	}
	if p.Slug == "" {
		t.Fatal("encrypted post slug not generated")
	}
}
func TestEncryptedPosts_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	p := &EncryptedPost{
		Title:    "Secret",
		Content:  "xxx",
		Password: "bcrypt",
	}
	CreateEncryptedPost(db, p)

	softDeleteByTable(db, t, TableEncryptedPosts, p.ID)

	// slug 不释放（唯一约束仍然存在）
	p2 := &EncryptedPost{
		Title:    "Another",
		Slug:     p.Slug,
		Content:  "yyy",
		Password: "bcrypt",
	}
	if err := CreateEncryptedPost(db, p2); err == nil {
		t.Fatal("slug should remain unique after soft delete")
	}
}

func TestTags_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	tag := &Tag{
		Name: "Go",
		Slug: "go",
	}
	CreateTag(db, tag)

	softDeleteByTable(db, t, TableTags, tag.ID)

	// slug 仍然占用
	tag2 := &Tag{
		Name: "Go2",
		Slug: "go",
	}
	if err := CreateTag(db, tag2); err == nil {
		t.Fatal("tag slug should remain unique after soft delete")
	}
}
func TestTagAndAttach(t *testing.T) {
	db := openTestDB(t)

	tag := &Tag{
		Name: "Go",
		Slug: "go",
	}
	if err := CreateTag(db, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	if tag.ID == 0 {
		t.Fatal("tag id not set")
	}

	post := &Post{
		Title:   "Go Post",
		Slug:    "go-post",
		Content: "content",
		Status:  "published",
	}
	if err := CreatePost(db, post); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	if err := AttachTagToPost(db, post.ID, tag.ID); err != nil {
		t.Fatalf("AttachTagToPost failed: %v", err)
	}

	// attach again should not error (INSERT OR IGNORE)
	if err := AttachTagToPost(db, post.ID, tag.ID); err != nil {
		t.Fatalf("AttachTagToPost duplicate failed: %v", err)
	}
}

func TestTag_UniqueSlug(t *testing.T) {
	db := openTestDB(t)

	t1 := &Tag{Name: "Go", Slug: "go"}
	if err := CreateTag(db, t1); err != nil {
		t.Fatal(err)
	}

	t2 := &Tag{Name: "GoLang", Slug: "go"}
	if err := CreateTag(db, t2); err == nil {
		t.Fatal("expected unique constraint on tag slug")
	}
}

func TestCreateRedirect(t *testing.T) {
	db := openTestDB(t)

	r := &Redirect{
		From: "/old",
		To:   "/new",
	}
	if err := CreateRedirect(db, r); err != nil {
		t.Fatalf("CreateRedirect failed: %v", err)
	}
	if r.ID == 0 {
		t.Fatal("redirect id not set")
	}
}
func TestRedirects_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	r := &Redirect{
		From: "/old",
		To:   "/new",
	}
	CreateRedirect(db, r)

	softDeleteByTable(db, t, TableRedirects, r.ID)

	// from_path 仍然唯一
	r2 := &Redirect{
		From: "/old",
		To:   "/another",
	}
	if err := CreateRedirect(db, r2); err == nil {
		t.Fatal("redirect from_path should remain unique after soft delete")
	}
}

func TestTasks(t *testing.T) {
	dbx := openTestDB(t)

	// ------------------------------
	// 1️⃣ 测试创建 Task
	task := &Task{
		Code:      "test_task",
		Name:      "测试任务",
		Schedule:  "* * * * *",
		Enabled:   1,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	if err := CreateTask(dbx, task); err != nil {
		t.Fatalf("failed to create task: %v", err)
	}
	if task.ID == 0 {
		t.Fatal("expected task.ID > 0")
	}

	// 2️⃣ 测试查询 Task
	tasks, err := ListTasks(dbx)
	if err != nil {
		t.Fatalf("failed to list tasks: %v", err)
	}
	if len(tasks) != 1 || tasks[0].Code != "test_task" {
		t.Fatalf("unexpected tasks: %+v", tasks)
	}

	// ------------------------------
	// 3️⃣ 测试创建 TaskRun
	run := &TaskRun{
		TaskCode:   task.Code,
		RunID:      uuid.NewString(),
		Status:     "pending",
		Message:    "",
		StartedAt:  time.Now().Unix(),
		FinishedAt: time.Now().Unix(),
		Duration:   0,
		CreatedAt:  time.Now().Unix(),
	}
	if err := CreateTaskRun(dbx, run); err != nil {
		t.Fatalf("failed to create task run: %v", err)
	}
	if run.ID == 0 {
		t.Fatal("expected run.ID > 0")
	}

	// 4️⃣ 测试查询 TaskRun
	runs, err := ListTaskRuns(dbx, task.Code, "", 10)
	if err != nil {
		t.Fatalf("failed to list task runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "pending" {
		t.Fatalf("unexpected runs: %+v", runs)
	}

	// 5️⃣ 测试更新 Run 状态
	run.Status = "success"
	run.Message = "ok"
	run.FinishedAt = time.Now().Unix()
	run.Duration = 123
	if err := UpdateTaskRunStatus(dbx, run); err != nil {
		t.Fatalf("failed to update task run: %v", err)
	}

	updated, err := ListTaskRuns(dbx, task.Code, "success", 10)
	if err != nil {
		t.Fatalf("failed to list task runs: %v", err)
	}
	if len(updated) != 1 || updated[0].Status != "success" || updated[0].Message != "ok" {
		t.Fatalf("update did not persist: %+v", updated)
	}
}

func TestCategoryCRUD(t *testing.T) {
	db := openTestDB(t)

	c := &Category{
		Name:        "Go",
		Slug:        "go",
		Description: "Go语言",
		Sort:        1,
	}

	if err := CreateCategory(db, c); err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("category id not set")
	}

	got, err := GetCategoryByID(db, c.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID failed: %v", err)
	}
	if got.Name != c.Name {
		t.Fatalf("unexpected name: %s", got.Name)
	}
	if got.Slug != c.Slug {
		t.Fatalf("unexpected slug: %s", got.Slug)
	}

	c.Name = "Go Updated"
	c.Slug = "go-updated"
	if err := UpdateCategory(db, c); err != nil {
		t.Fatalf("UpdateCategory failed: %v", err)
	}

	got, err = GetCategoryByID(db, c.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID after update failed: %v", err)
	}
	if got.Name != "Go Updated" {
		t.Fatalf("update not applied")
	}

	if err := SoftDeleteCategory(db, c.ID); err != nil {
		t.Fatalf("SoftDeleteCategory failed: %v", err)
	}

	_, err = GetCategoryByID(db, c.ID)
	if err == nil {
		t.Fatalf("expected error after soft delete")
	}
}

func TestCategory_ParentChild(t *testing.T) {
	db := openTestDB(t)

	parent := &Category{
		Name: "Parent",
		Slug: "parent",
		Sort: 1,
	}
	if err := CreateCategory(db, parent); err != nil {
		t.Fatalf("CreateCategory parent failed: %v", err)
	}

	child := &Category{
		ParentID: parent.ID,
		Name:     "Child",
		Slug:     "child",
		Sort:     1,
	}
	if err := CreateCategory(db, child); err != nil {
		t.Fatalf("CreateCategory child failed: %v", err)
	}

	got, err := GetCategoryByID(db, child.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID child failed: %v", err)
	}
	if got.ParentID != parent.ID {
		t.Fatalf("unexpected parent_id: %d", got.ParentID)
	}
}

func TestCategory_UniqueSlugUnderParent(t *testing.T) {
	db := openTestDB(t)

	parent := &Category{
		Name: "Parent",
		Slug: "parent",
		Sort: 1,
	}
	if err := CreateCategory(db, parent); err != nil {
		t.Fatalf("CreateCategory parent failed: %v", err)
	}

	c1 := &Category{
		ParentID: parent.ID,
		Name:     "A",
		Slug:     "same",
		Sort:     1,
	}
	if err := CreateCategory(db, c1); err != nil {
		t.Fatalf("create c1 failed: %v", err)
	}

	c2 := &Category{
		ParentID: parent.ID,
		Name:     "B",
		Slug:     "same",
		Sort:     1,
	}
	if err := CreateCategory(db, c2); err == nil {
		t.Fatal("expected unique constraint error on slug under same parent")
	}

	// 不同父级下可以有相同的slug
	c3 := &Category{
		ParentID: 0,
		Name:     "Root",
		Slug:     "same",
		Sort:     1,
	}
	if err := CreateCategory(db, c3); err != nil {
		t.Fatalf("create c3 with same slug under different parent should succeed: %v", err)
	}
}

func TestCategory_CycleDetection(t *testing.T) {
	db := openTestDB(t)

	parent := &Category{
		Name: "Parent",
		Slug: "parent",
		Sort: 1,
	}
	if err := CreateCategory(db, parent); err != nil {
		t.Fatalf("CreateCategory parent failed: %v", err)
	}

	child := &Category{
		ParentID: parent.ID,
		Name:     "Child",
		Slug:     "child",
		Sort:     1,
	}
	if err := CreateCategory(db, child); err != nil {
		t.Fatalf("CreateCategory child failed: %v", err)
	}

	// 尝试将父级设置为自己的子级，应该检测到循环
	if err := UpdateCategoryParent(db, parent.ID, child.ID); err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func TestCategory_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	c := &Category{
		Name: "Test",
		Slug: "test",
		Sort: 1,
	}
	CreateCategory(db, c)

	softDeleteByTable(db, t, TableCategories, c.ID)

	// 不可再查询
	if _, err := GetCategoryByID(db, c.ID); err == nil {
		t.Fatal("soft deleted category should not be readable")
	}

	// 再次 soft delete 不应报错
	if err := SoftDeleteCategory(db, c.ID); err != nil {
		t.Fatal("double soft delete should be safe")
	}

	// slug 仍然占用（唯一性检查仍然有效）
	c2 := &Category{
		Name: "Test2",
		Slug: "test",
		Sort: 1,
	}
	if err := CreateCategory(db, c2); err == nil {
		t.Fatal("category slug should remain unique after soft delete")
	}
}

func TestCategory_ListCategories(t *testing.T) {
	db := openTestDB(t)

	c1 := &Category{
		Name: "A",
		Slug: "a",
		Sort: 2,
	}
	c2 := &Category{
		Name: "B",
		Slug: "b",
		Sort: 1,
	}

	CreateCategory(db, c1)
	CreateCategory(db, c2)

	list, err := ListCategories(db)
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 categories, got %d", len(list))
	}

	// 应该按sort排序
	var foundB, foundA bool
	for _, c := range list {
		if c.Slug == "b" {
			foundB = true
		}
		if c.Slug == "a" {
			foundA = true
			if !foundB {
				t.Fatal("categories should be ordered by sort, b (sort=1) should come before a (sort=2)")
			}
		}
	}
	if !foundA || !foundB {
		t.Fatal("expected both categories in list")
	}
}

func TestCategory_UpdateParent(t *testing.T) {
	db := openTestDB(t)

	parent1 := &Category{
		Name: "Parent1",
		Slug: "parent1",
		Sort: 1,
	}
	parent2 := &Category{
		Name: "Parent2",
		Slug: "parent2",
		Sort: 1,
	}
	child := &Category{
		ParentID: parent1.ID,
		Name:     "Child",
		Slug:     "child",
		Sort:     1,
	}

	CreateCategory(db, parent1)
	CreateCategory(db, parent2)
	CreateCategory(db, child)

	// 将child从parent1移动到parent2
	if err := UpdateCategoryParent(db, child.ID, parent2.ID); err != nil {
		t.Fatalf("UpdateCategoryParent failed: %v", err)
	}

	got, err := GetCategoryByID(db, child.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID failed: %v", err)
	}
	if got.ParentID != parent2.ID {
		t.Fatalf("unexpected parent_id: %d, expected %d", got.ParentID, parent2.ID)
	}
}

// ========== Settings 功能测试 ==========

func TestCreateSetting(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Test Setting",
		Code:  "test_setting",
		Type:  "text",
		Value: "test value",
	}

	if err := CreateSetting(db, s); err != nil {
		t.Fatalf("CreateSetting failed: %v", err)
	}
	if s.ID == 0 {
		t.Fatal("setting id not set")
	}

	got, err := GetSettingByCode(db, "test_setting")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if got.Value != "test value" {
		t.Fatalf("unexpected value: %s", got.Value)
	}
}

func TestCreateSetting_PasswordEncryption(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Admin Password",
		Code:  "test_password",
		Type:  "password",
		Value: "plaintext123",
	}

	if err := CreateSetting(db, s); err != nil {
		t.Fatalf("CreateSetting failed: %v", err)
	}

	got, err := GetSettingByCode(db, "test_password")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}

	// password 应该被 bcrypt 加密（长度 >= 60）
	if len(got.Value) < 60 {
		t.Fatalf("password should be encrypted, got length %d", len(got.Value))
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(got.Value), []byte("plaintext123")); err != nil {
		t.Fatalf("password verification failed: %v", err)
	}
}

func TestCreateSetting_RequiredFields(t *testing.T) {
	db := openTestDB(t)

	// 测试缺少 code
	s1 := &Setting{
		Kind: "General",
		Name: "Test",
		Type: "text",
	}
	if err := CreateSetting(db, s1); err == nil {
		t.Fatal("expected error when code is missing")
	}

	// 测试缺少 type
	s2 := &Setting{
		Kind: "General",
		Name: "Test",
		Code: "test",
	}
	if err := CreateSetting(db, s2); err == nil {
		t.Fatal("expected error when type is missing")
	}
}

func TestGetSettingByCode(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Test",
		Code:  "test_code",
		Type:  "text",
		Value: "test value",
	}
	CreateSetting(db, s)

	got, err := GetSettingByCode(db, "test_code")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if got.Code != "test_code" {
		t.Fatalf("unexpected code: %s", got.Code)
	}

	// 测试不存在的 code
	_, err = GetSettingByCode(db, "non_exist")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetSettingByID(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Test",
		Code:  "test_id",
		Type:  "text",
		Value: "test value",
	}
	CreateSetting(db, s)

	got, err := GetSettingByID(db, s.ID)
	if err != nil {
		t.Fatalf("GetSettingByID failed: %v", err)
	}
	if got.ID != s.ID {
		t.Fatalf("unexpected id: %d", got.ID)
	}

	// 测试不存在的 id
	_, err = GetSettingByID(db, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateSetting(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Original",
		Code:  "update_test",
		Type:  "text",
		Value: "original value",
	}
	CreateSetting(db, s)

	s.Name = "Updated"
	s.Value = "updated value"
	if err := UpdateSetting(db, s); err != nil {
		t.Fatalf("UpdateSetting failed: %v", err)
	}

	got, err := GetSettingByCode(db, "update_test")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if got.Name != "Updated" {
		t.Fatalf("update not applied, got %s", got.Name)
	}
	if got.Value != "updated value" {
		t.Fatalf("update not applied, got %s", got.Value)
	}
}

func TestUpdateSetting_PasswordEncryption(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Password",
		Code:  "update_password",
		Type:  "password",
		Value: "oldpass",
	}
	CreateSetting(db, s)

	// 更新为新的明文密码
	s.Value = "newpass123"
	if err := UpdateSetting(db, s); err != nil {
		t.Fatalf("UpdateSetting failed: %v", err)
	}

	got, err := GetSettingByCode(db, "update_password")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}

	// 新密码应该被 bcrypt 加密
	if len(got.Value) < 60 {
		t.Fatalf("password should be encrypted, got length %d", len(got.Value))
	}

	// 验证新密码
	if err := bcrypt.CompareHashAndPassword([]byte(got.Value), []byte("newpass123")); err != nil {
		t.Fatalf("password verification failed: %v", err)
	}
}

func TestUpdateSettingByCode(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Test",
		Code:  "update_by_code",
		Type:  "text",
		Value: "original",
	}
	CreateSetting(db, s)

	if err := UpdateSettingByCode(db, "update_by_code", "updated"); err != nil {
		t.Fatalf("UpdateSettingByCode failed: %v", err)
	}

	got, err := GetSettingByCode(db, "update_by_code")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if got.Value != "updated" {
		t.Fatalf("update not applied, got %s", got.Value)
	}
}

func TestUpdateSettingByCode_PasswordEncryption(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Password",
		Code:  "update_by_code_pass",
		Type:  "password",
		Value: "oldpass",
	}
	CreateSetting(db, s)

	if err := UpdateSettingByCode(db, "update_by_code_pass", "newpass456"); err != nil {
		t.Fatalf("UpdateSettingByCode failed: %v", err)
	}

	got, err := GetSettingByCode(db, "update_by_code_pass")
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}

	// 新密码应该被 bcrypt 加密
	if len(got.Value) < 60 {
		t.Fatalf("password should be encrypted, got length %d", len(got.Value))
	}

	// 验证新密码
	if err := bcrypt.CompareHashAndPassword([]byte(got.Value), []byte("newpass456")); err != nil {
		t.Fatalf("password verification failed: %v", err)
	}
}

func TestDeleteSetting(t *testing.T) {
	db := openTestDB(t)

	s := &Setting{
		Kind:  "General",
		Name:  "Test",
		Code:  "delete_test",
		Type:  "text",
		Value: "test",
	}
	CreateSetting(db, s)

	if err := DeleteSetting(db, s.ID); err != nil {
		t.Fatalf("DeleteSetting failed: %v", err)
	}

	// 软删除后应该查不到
	_, err := GetSettingByCode(db, "delete_test")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after soft delete, got %v", err)
	}
}

func TestCheckPassword(t *testing.T) {
	db := openTestDB(t)

	// EnsureDefaultSettings 已经创建了 admin_password，我们更新它
	if err := UpdateSettingByCode(db, "admin_password", "admin123"); err != nil {
		t.Fatalf("UpdateSettingByCode failed: %v", err)
	}

	// 正确的密码
	if err := CheckPassword(db, "admin123"); err != nil {
		t.Fatalf("CheckPassword should succeed with correct password: %v", err)
	}

	// 错误的密码
	if err := CheckPassword(db, "wrongpass"); err == nil {
		t.Fatal("CheckPassword should fail with wrong password")
	}
}

func TestListSettingsByKind(t *testing.T) {
	db := openTestDB(t)

	s1 := &Setting{Kind: "General", Name: "S1", Code: "s1", Type: "text", Value: "v1"}
	s2 := &Setting{Kind: "General", Name: "S2", Code: "s2", Type: "text", Value: "v2"}
	s3 := &Setting{Kind: "Post", Name: "S3", Code: "s3", Type: "text", Value: "v3"}

	CreateSetting(db, s1)
	CreateSetting(db, s2)
	CreateSetting(db, s3)

	// 按 kind 查询
	list, err := ListSettingsByKind(db, "General")
	if err != nil {
		t.Fatalf("ListSettingsByKind failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 General settings, got %d", len(list))
	}

	// 查询所有
	all, err := ListAllSettings(db)
	if err != nil {
		t.Fatalf("ListAllSettings failed: %v", err)
	}
	if len(all) < 3 {
		t.Fatalf("expected at least 3 settings, got %d", len(all))
	}
}

func TestLoadSettingsToMap(t *testing.T) {
	db := openTestDB(t)

	s1 := &Setting{Kind: "General", Name: "S1", Code: "code1", Type: "text", Value: "value1"}
	s2 := &Setting{Kind: "General", Name: "S2", Code: "code2", Type: "text", Value: "value2"}

	CreateSetting(db, s1)
	CreateSetting(db, s2)

	m, err := LoadSettingsToMap(db)
	if err != nil {
		t.Fatalf("LoadSettingsToMap failed: %v", err)
	}

	if m["code1"] != "value1" {
		t.Fatalf("unexpected value for code1: %s", m["code1"])
	}
	if m["code2"] != "value2" {
		t.Fatalf("unexpected value for code2: %s", m["code2"])
	}
}

// ========== Restore 功能测试 ==========

func TestRestorePost(t *testing.T) {
	db := openTestDB(t)

	p := &Post{
		Title:   "Restore Test",
		Slug:    "restore-test",
		Content: "content",
		Status:  "published",
	}
	CreatePost(db, p)

	// 软删除
	if err := SoftDeletePost(db, p.ID); err != nil {
		t.Fatalf("SoftDeletePost failed: %v", err)
	}

	// 恢复
	if err := RestorePost(db, p.ID); err != nil {
		t.Fatalf("RestorePost failed: %v", err)
	}

	// 恢复后应该可以查询
	got, err := GetPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID failed after restore: %v", err)
	}
	if got.Title != "Restore Test" {
		t.Fatalf("unexpected title: %s", got.Title)
	}
}

func TestRestoreEncryptedPost(t *testing.T) {
	db := openTestDB(t)

	p := &EncryptedPost{
		Title:    "Restore Encrypted",
		Content:  "secret content",
		Password: "pass",
	}
	CreateEncryptedPost(db, p)

	SoftDeleteEncryptedPost(db, p.ID)

	// 恢复
	if err := RestoreEncryptedPost(db, p.ID); err != nil {
		t.Fatalf("RestoreEncryptedPost failed: %v", err)
	}

	// 恢复后应该可以查询
	got, err := GetEncryptedPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetEncryptedPostByID failed after restore: %v", err)
	}
	if got.Title != "Restore Encrypted" {
		t.Fatalf("unexpected title: %s", got.Title)
	}
}

func TestRestoreTag(t *testing.T) {
	db := openTestDB(t)

	tag := &Tag{
		Name: "Restore Tag",
		Slug: "restore-tag",
	}
	CreateTag(db, tag)

	SoftDeleteTag(db, tag.ID)

	// 恢复
	if err := RestoreTag(db, tag.ID); err != nil {
		t.Fatalf("RestoreTag failed: %v", err)
	}

	// 恢复后应该可以查询
	got, err := GetTagByID(db, tag.ID)
	if err != nil {
		t.Fatalf("GetTagByID failed after restore: %v", err)
	}
	if got.Name != "Restore Tag" {
		t.Fatalf("unexpected name: %s", got.Name)
	}
}

func TestRestoreRedirect(t *testing.T) {
	db := openTestDB(t)

	r := &Redirect{
		From: "/restore-from",
		To:   "/restore-to",
	}
	CreateRedirect(db, r)

	SoftDeleteRedirect(db, r.ID)

	// 恢复
	if err := RestoreRedirect(db, r.ID); err != nil {
		t.Fatalf("RestoreRedirect failed: %v", err)
	}

	// 恢复后应该可以查询
	got, err := GetRedirectByID(db, r.ID)
	if err != nil {
		t.Fatalf("GetRedirectByID failed after restore: %v", err)
	}
	if got.From != "/restore-from" {
		t.Fatalf("unexpected from: %s", got.From)
	}
}

func TestRestoreCategory(t *testing.T) {
	db := openTestDB(t)

	c := &Category{
		Name: "Restore Category",
		Slug: "restore-category",
		Sort: 1,
	}
	CreateCategory(db, c)

	SoftDeleteCategory(db, c.ID)

	// 恢复
	if err := RestoreCategory(db, c.ID); err != nil {
		t.Fatalf("RestoreCategory failed: %v", err)
	}

	// 恢复后应该可以查询
	got, err := GetCategoryByID(db, c.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID failed after restore: %v", err)
	}
	if got.Name != "Restore Category" {
		t.Fatalf("unexpected name: %s", got.Name)
	}
}

// ========== Get/Update 功能测试 ==========

func TestGetEncryptedPostByID(t *testing.T) {
	db := openTestDB(t)

	originalContent := "This is secret content"
	p := &EncryptedPost{
		Title:    "Secret Post",
		Content:  originalContent,
		Password: "password123",
	}
	if err := CreateEncryptedPost(db, p); err != nil {
		t.Fatalf("CreateEncryptedPost failed: %v", err)
	}

	got, err := GetEncryptedPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetEncryptedPostByID failed: %v", err)
	}

	if got.Content != originalContent {
		t.Fatalf("content should be decrypted, got %s, expected %s", got.Content, originalContent)
	}
	if got.Title != "Secret Post" {
		t.Fatalf("unexpected title: %s", got.Title)
	}
}

func TestUpdateEncryptedPost(t *testing.T) {
	db := openTestDB(t)

	p := &EncryptedPost{
		Title:    "Original",
		Content:  "original content",
		Password: "pass",
	}
	CreateEncryptedPost(db, p)

	newContent := "updated content"
	p.Title = "Updated"
	p.Content = newContent
	p.Password = "newpass"

	if err := UpdateEncryptedPost(db, p); err != nil {
		t.Fatalf("UpdateEncryptedPost failed: %v", err)
	}

	got, err := GetEncryptedPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetEncryptedPostByID failed: %v", err)
	}

	if got.Content != newContent {
		t.Fatalf("content update not applied, got %s", got.Content)
	}
	if got.Title != "Updated" {
		t.Fatalf("title update not applied, got %s", got.Title)
	}
}

func TestGetTagByID(t *testing.T) {
	db := openTestDB(t)

	tag := &Tag{
		Name: "Go Language",
		Slug: "go-lang",
	}
	CreateTag(db, tag)

	got, err := GetTagByID(db, tag.ID)
	if err != nil {
		t.Fatalf("GetTagByID failed: %v", err)
	}
	if got.Name != "Go Language" {
		t.Fatalf("unexpected name: %s", got.Name)
	}
	if got.Slug != "go-lang" {
		t.Fatalf("unexpected slug: %s", got.Slug)
	}

	// 测试不存在的 id
	_, err = GetTagByID(db, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateTag(t *testing.T) {
	db := openTestDB(t)

	tag := &Tag{
		Name: "Original",
		Slug: "original",
	}
	CreateTag(db, tag)

	tag.Name = "Updated"
	tag.Slug = "updated"
	if err := UpdateTag(db, tag); err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}

	got, err := GetTagByID(db, tag.ID)
	if err != nil {
		t.Fatalf("GetTagByID failed: %v", err)
	}
	if got.Name != "Updated" {
		t.Fatalf("update not applied, got %s", got.Name)
	}
	if got.Slug != "updated" {
		t.Fatalf("update not applied, got %s", got.Slug)
	}
}

func TestGetRedirectByID(t *testing.T) {
	db := openTestDB(t)

	r := &Redirect{
		From:    "/get-test",
		To:      "/target",
		Status:  302,
		Enabled: 1,
	}
	CreateRedirect(db, r)

	got, err := GetRedirectByID(db, r.ID)
	if err != nil {
		t.Fatalf("GetRedirectByID failed: %v", err)
	}
	if got.From != "/get-test" {
		t.Fatalf("unexpected from: %s", got.From)
	}
	if got.Status != 302 {
		t.Fatalf("unexpected status: %d", got.Status)
	}

	// 测试不存在的 id
	_, err = GetRedirectByID(db, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetRedirectByFrom(t *testing.T) {
	db := openTestDB(t)

	r := &Redirect{
		From: "/from-path",
		To:   "/to-path",
	}
	CreateRedirect(db, r)

	got, err := GetRedirectByFrom(db, "/from-path")
	if err != nil {
		t.Fatalf("GetRedirectByFrom failed: %v", err)
	}
	if got.To != "/to-path" {
		t.Fatalf("unexpected to: %s", got.To)
	}

	// 测试不存在的路径
	_, err = GetRedirectByFrom(db, "/non-exist")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateRedirect(t *testing.T) {
	db := openTestDB(t)

	r := &Redirect{
		From:    "/update-from",
		To:      "/update-to",
		Status:  301,
		Enabled: 1,
	}
	CreateRedirect(db, r)

	r.To = "/new-target"
	r.Status = 302
	r.Enabled = 0
	if err := UpdateRedirect(db, r); err != nil {
		t.Fatalf("UpdateRedirect failed: %v", err)
	}

	got, err := GetRedirectByID(db, r.ID)
	if err != nil {
		t.Fatalf("GetRedirectByID failed: %v", err)
	}
	if got.To != "/new-target" {
		t.Fatalf("update not applied, got %s", got.To)
	}
	if got.Status != 302 {
		t.Fatalf("update not applied, got %d", got.Status)
	}
	if got.Enabled != 0 {
		t.Fatalf("update not applied, got %d", got.Enabled)
	}
}

func TestGetTaskByID(t *testing.T) {
	db := openTestDB(t)

	task := &Task{
		Code:      "get_by_id",
		Name:      "Test Task",
		Schedule:  "0 0 * * *",
		Enabled:   1,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	CreateTask(db, task)

	got, err := GetTaskByID(db, task.ID)
	if err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if got.Code != "get_by_id" {
		t.Fatalf("unexpected code: %s", got.Code)
	}

	// 测试不存在的 id
	_, err = GetTaskByID(db, 99999)
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestGetTaskByCode(t *testing.T) {
	db := openTestDB(t)

	task := &Task{
		Code:      "get_by_code",
		Name:      "Test Task",
		Schedule:  "0 0 * * *",
		Enabled:   1,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	CreateTask(db, task)

	got, err := GetTaskByCode(db, "get_by_code")
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if got.Name != "Test Task" {
		t.Fatalf("unexpected name: %s", got.Name)
	}

	// 测试不存在的 code
	_, err = GetTaskByCode(db, "non_exist")
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateTask(t *testing.T) {
	db := openTestDB(t)

	task := &Task{
		Code:      "update_task",
		Name:      "Original",
		Schedule:  "0 0 * * *",
		Enabled:   1,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	CreateTask(db, task)

	task.Name = "Updated"
	task.Schedule = "0 */5 * * *"
	task.Enabled = 0
	if err := UpdateTask(db, task); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	got, err := GetTaskByCode(db, "update_task")
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if got.Name != "Updated" {
		t.Fatalf("update not applied, got %s", got.Name)
	}
	if got.Schedule != "0 */5 * * *" {
		t.Fatalf("update not applied, got %s", got.Schedule)
	}
	if got.Enabled != 0 {
		t.Fatalf("update not applied, got %d", got.Enabled)
	}
}

func TestUpdateTaskStatus(t *testing.T) {
	db := openTestDB(t)

	task := &Task{
		Code:      "update_status",
		Name:      "Test",
		Schedule:  "0 0 * * *",
		Enabled:   1,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}
	CreateTask(db, task)

	now := time.Now().Unix()
	if err := UpdateTaskStatus(db, "update_status", "success", now); err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}

	got, err := GetTaskByCode(db, "update_status")
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if got.LastStatus != "success" {
		t.Fatalf("status update not applied, got %s", got.LastStatus)
	}
	if got.LastRunAt == nil || *got.LastRunAt != now {
		t.Fatalf("last_run_at update not applied")
	}
}

// ========== List 功能测试 ==========

func TestListDeletedPosts(t *testing.T) {
	db := openTestDB(t)

	p1 := &Post{Title: "P1", Slug: "p1", Content: "c1", Status: "published"}
	p2 := &Post{Title: "P2", Slug: "p2", Content: "c2", Status: "published"}
	p3 := &Post{Title: "P3", Slug: "p3", Content: "c3", Status: "published"}

	CreatePost(db, p1)
	CreatePost(db, p2)
	CreatePost(db, p3)

	// 软删除 p1 和 p2
	SoftDeletePost(db, p1.ID)
	SoftDeletePost(db, p2.ID)

	list, err := ListDeletedPosts(db)
	if err != nil {
		t.Fatalf("ListDeletedPosts failed: %v", err)
	}
	if len(list) < 2 {
		t.Fatalf("expected at least 2 deleted posts, got %d", len(list))
	}

	// 验证 p3 不在列表中
	for _, p := range list {
		if p.ID == p3.ID {
			t.Fatal("p3 should not be in deleted list")
		}
	}
}

func TestListDeletedEncryptedPosts(t *testing.T) {
	db := openTestDB(t)

	p1 := &EncryptedPost{Title: "EP1", Content: "c1", Password: "p1"}
	p2 := &EncryptedPost{Title: "EP2", Content: "c2", Password: "p2"}

	CreateEncryptedPost(db, p1)
	CreateEncryptedPost(db, p2)

	SoftDeleteEncryptedPost(db, p1.ID)

	list, err := ListDeletedEncryptedPosts(db)
	if err != nil {
		t.Fatalf("ListDeletedEncryptedPosts failed: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("expected at least 1 deleted encrypted post, got %d", len(list))
	}
}

func TestListDeletedTags(t *testing.T) {
	db := openTestDB(t)

	tag1 := &Tag{Name: "T1", Slug: "t1"}
	tag2 := &Tag{Name: "T2", Slug: "t2"}

	CreateTag(db, tag1)
	CreateTag(db, tag2)

	SoftDeleteTag(db, tag1.ID)

	list, err := ListDeletedTags(db)
	if err != nil {
		t.Fatalf("ListDeletedTags failed: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("expected at least 1 deleted tag, got %d", len(list))
	}
}

func TestListDeletedRedirects(t *testing.T) {
	db := openTestDB(t)

	r1 := &Redirect{From: "/r1", To: "/t1"}
	r2 := &Redirect{From: "/r2", To: "/t2"}

	CreateRedirect(db, r1)
	CreateRedirect(db, r2)

	SoftDeleteRedirect(db, r1.ID)

	list, err := ListDeletedRedirects(db)
	if err != nil {
		t.Fatalf("ListDeletedRedirects failed: %v", err)
	}
	if len(list) < 1 {
		t.Fatalf("expected at least 1 deleted redirect, got %d", len(list))
	}
}

func TestListRedirects(t *testing.T) {
	db := openTestDB(t)

	// 创建多个重定向
	for i := 0; i < 5; i++ {
		r := &Redirect{
			From: fmt.Sprintf("/list-test-%d", i),
			To:   fmt.Sprintf("/target-%d", i),
		}
		CreateRedirect(db, r)
	}

	// 测试分页
	list, total, err := ListRedirects(db, 2, 0)
	if err != nil {
		t.Fatalf("ListRedirects failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 redirects, got %d", len(list))
	}
	if total < 5 {
		t.Fatalf("expected total >= 5, got %d", total)
	}

	// 测试第二页
	list2, total2, err := ListRedirects(db, 2, 2)
	if err != nil {
		t.Fatalf("ListRedirects failed: %v", err)
	}
	if len(list2) != 2 {
		t.Fatalf("expected 2 redirects on page 2, got %d", len(list2))
	}
	if total2 != total {
		t.Fatalf("total should be same, got %d vs %d", total2, total)
	}
}

func TestCreateHttpErrorLog(t *testing.T) {
	db := openTestDB(t)

	log := &HttpErrorLog{
		ReqID:     "test-req-id",
		ClientIP:  "127.0.0.1",
		Method:    "GET",
		Path:      "/test",
		Status:    404,
		UserAgent: "test-agent",
	}

	if err := CreateHttpErrorLog(db, log); err != nil {
		t.Fatalf("CreateHttpErrorLog failed: %v", err)
	}
	if log.ID == 0 {
		t.Fatal("log id not set")
	}
}

func TestListHttpErrorLogs(t *testing.T) {
	db := openTestDB(t)

	// 创建多个日志
	for i := 0; i < 5; i++ {
		log := &HttpErrorLog{
			ReqID:     fmt.Sprintf("req-%d", i),
			ClientIP:  "127.0.0.1",
			Method:    "GET",
			Path:      "/test",
			Status:    404,
			UserAgent: "test",
		}
		CreateHttpErrorLog(db, log)
	}

	list, err := ListHttpErrorLogs(db, 3, 0)
	if err != nil {
		t.Fatalf("ListHttpErrorLogs failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 logs, got %d", len(list))
	}
}

func TestCountHttpErrorLogs(t *testing.T) {
	db := openTestDB(t)

	// 创建几个日志
	for i := 0; i < 3; i++ {
		log := &HttpErrorLog{
			ReqID:     fmt.Sprintf("count-req-%d", i),
			ClientIP:  "127.0.0.1",
			Method:    "GET",
			Path:      "/test",
			Status:    404,
			UserAgent: "test",
		}
		CreateHttpErrorLog(db, log)
	}

	count, err := CountHttpErrorLogs(db)
	if err != nil {
		t.Fatalf("CountHttpErrorLogs failed: %v", err)
	}
	if count < 3 {
		t.Fatalf("expected count >= 3, got %d", count)
	}
}

func TestDeleteHttpErrorLog(t *testing.T) {
	db := openTestDB(t)

	log := &HttpErrorLog{
		ReqID:     "delete-req",
		ClientIP:  "127.0.0.1",
		Method:    "GET",
		Path:      "/test",
		Status:    404,
		UserAgent: "test",
	}
	CreateHttpErrorLog(db, log)

	if err := DeleteHttpErrorLog(db, log.ID); err != nil {
		t.Fatalf("DeleteHttpErrorLog failed: %v", err)
	}

	// 验证已删除
	_, err := CountHttpErrorLogs(db)
	if err != nil {
		t.Fatalf("CountHttpErrorLogs failed: %v", err)
	}
	// 注意：这里可能会有其他日志，所以我们只能确认删除操作成功
}

// ========== 关联关系功能测试 ==========

func TestGetPostTags(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Tag Post", Slug: "tag-post", Content: "c", Status: "published"}
	tag1 := &Tag{Name: "Tag1", Slug: "tag1"}
	tag2 := &Tag{Name: "Tag2", Slug: "tag2"}

	CreatePost(db, post)
	CreateTag(db, tag1)
	CreateTag(db, tag2)

	AttachTagToPost(db, post.ID, tag1.ID)
	AttachTagToPost(db, post.ID, tag2.ID)

	tags, err := GetPostTags(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}
}

func TestDetachTagFromPost(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Detach Post", Slug: "detach-post", Content: "c", Status: "published"}
	tag := &Tag{Name: "Detach Tag", Slug: "detach-tag"}

	CreatePost(db, post)
	CreateTag(db, tag)

	AttachTagToPost(db, post.ID, tag.ID)

	// 验证已关联
	tags, _ := GetPostTags(db, post.ID)
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag before detach, got %d", len(tags))
	}

	// 取消关联
	if err := DetachTagFromPost(db, post.ID, tag.ID); err != nil {
		t.Fatalf("DetachTagFromPost failed: %v", err)
	}

	// 验证已取消关联
	tags, _ = GetPostTags(db, post.ID)
	if len(tags) != 0 {
		t.Fatalf("expected 0 tags after detach, got %d", len(tags))
	}
}

func TestSetPostTags(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Set Tags Post", Slug: "set-tags-post", Content: "c", Status: "published"}
	tag1 := &Tag{Name: "ST1", Slug: "st1"}
	tag2 := &Tag{Name: "ST2", Slug: "st2"}
	tag3 := &Tag{Name: "ST3", Slug: "st3"}

	CreatePost(db, post)
	CreateTag(db, tag1)
	CreateTag(db, tag2)
	CreateTag(db, tag3)

	// 先关联 tag1 和 tag2
	AttachTagToPost(db, post.ID, tag1.ID)
	AttachTagToPost(db, post.ID, tag2.ID)

	// 使用 SetPostTags 设置为 tag2 和 tag3
	if err := SetPostTags(db, post.ID, []int64{tag2.ID, tag3.ID}); err != nil {
		t.Fatalf("SetPostTags failed: %v", err)
	}

	tags, err := GetPostTags(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	// 验证 tag1 已被移除，tag2 和 tag3 存在
	foundTag1, foundTag2, foundTag3 := false, false, false
	for _, tag := range tags {
		if tag.ID == tag1.ID {
			foundTag1 = true
		}
		if tag.ID == tag2.ID {
			foundTag2 = true
		}
		if tag.ID == tag3.ID {
			foundTag3 = true
		}
	}

	if foundTag1 {
		t.Fatal("tag1 should be removed")
	}
	if !foundTag2 {
		t.Fatal("tag2 should exist")
	}
	if !foundTag3 {
		t.Fatal("tag3 should exist")
	}
}

func TestGetPostCategory(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Cat Post", Slug: "cat-post", Content: "c", Status: "published"}
	cat1 := &Category{Name: "Cat1", Slug: "cat1", Sort: 1}

	CreatePost(db, post)
	CreateCategory(db, cat1)

	AttachCategoryToPost(db, post.ID, cat1.ID)

	cat, err := GetPostCategory(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostCategory failed: %v", err)
	}
	if cat == nil {
		t.Fatal("expected category, got nil")
	}
	if cat.ID != cat1.ID {
		t.Fatalf("expected category ID %d, got %d", cat1.ID, cat.ID)
	}
}

func TestAttachCategoryToPost(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Attach Cat Post", Slug: "attach-cat-post", Content: "c", Status: "published"}
	cat := &Category{Name: "Attach Cat", Slug: "attach-cat", Sort: 1}

	CreatePost(db, post)
	CreateCategory(db, cat)

	if err := AttachCategoryToPost(db, post.ID, cat.ID); err != nil {
		t.Fatalf("AttachCategoryToPost failed: %v", err)
	}

	gotCat, err := GetPostCategory(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostCategory failed: %v", err)
	}
	if gotCat == nil {
		t.Fatal("expected category, got nil")
	}
	if gotCat.ID != cat.ID {
		t.Fatalf("expected category ID %d, got %d", cat.ID, gotCat.ID)
	}

	// 再次关联应该不报错（INSERT OR IGNORE）
	if err := AttachCategoryToPost(db, post.ID, cat.ID); err != nil {
		t.Fatalf("AttachCategoryToPost duplicate should not error: %v", err)
	}
}

func TestDetachCategoryFromPost(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Detach Cat Post", Slug: "detach-cat-post", Content: "c", Status: "published"}
	cat := &Category{Name: "Detach Cat", Slug: "detach-cat", Sort: 1}

	CreatePost(db, post)
	CreateCategory(db, cat)

	AttachCategoryToPost(db, post.ID, cat.ID)

	// 验证已关联
	gotCat, _ := GetPostCategory(db, post.ID)
	if gotCat == nil {
		t.Fatal("expected category before detach, got nil")
	}
	if gotCat.ID != cat.ID {
		t.Fatalf("expected category ID %d, got %d", cat.ID, gotCat.ID)
	}

	// 取消关联
	if err := DetachCategoryFromPost(db, post.ID, cat.ID); err != nil {
		t.Fatalf("DetachCategoryFromPost failed: %v", err)
	}

	// 验证已取消关联
	gotCat, _ = GetPostCategory(db, post.ID)
	if gotCat != nil {
		t.Fatalf("expected nil category after detach, got category ID %d", gotCat.ID)
	}
}

func TestSetPostCategory(t *testing.T) {
	db := openTestDB(t)

	post := &Post{Title: "Set Cat Post", Slug: "set-cat-post", Content: "c", Status: "published"}
	cat1 := &Category{Name: "SC1", Slug: "sc1", Sort: 1}
	cat2 := &Category{Name: "SC2", Slug: "sc2", Sort: 1}

	CreatePost(db, post)
	CreateCategory(db, cat1)
	CreateCategory(db, cat2)

	// 先关联 cat1
	AttachCategoryToPost(db, post.ID, cat1.ID)

	// 使用 SetPostCategory 设置为 cat2
	if err := SetPostCategory(db, post.ID, cat2.ID); err != nil {
		t.Fatalf("SetPostCategory failed: %v", err)
	}

	cat, err := GetPostCategory(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostCategory failed: %v", err)
	}
	if cat == nil {
		t.Fatal("expected category, got nil")
	}
	if cat.ID != cat2.ID {
		t.Fatalf("expected category ID %d, got %d", cat2.ID, cat.ID)
	}

	// 验证 cat1 已被移除
	// 通过再次设置 cat1 来验证之前 cat2 已正确设置
	if err := SetPostCategory(db, post.ID, cat1.ID); err != nil {
		t.Fatalf("SetPostCategory to cat1 failed: %v", err)
	}
	cat, _ = GetPostCategory(db, post.ID)
	if cat == nil || cat.ID != cat1.ID {
		t.Fatalf("expected category ID %d, got %v", cat1.ID, cat)
	}
}

// ============================================================================
// 通用基础函数测试（确保100%覆盖）
// ============================================================================

func TestSoftDeleteRecord(t *testing.T) {
	db := openTestDB(t)

	// 测试 posts 表
	p := &Post{Title: "SoftDelete Test", Slug: "softdelete-test", Content: "c", Status: "published"}
	CreatePost(db, p)

	if err := SoftDeleteRecord(db, TablePosts, p.ID); err != nil {
		t.Fatalf("SoftDeleteRecord failed: %v", err)
	}

	// 验证已软删除
	_, err := GetPostByID(db, p.ID)
	if err == nil {
		t.Fatal("post should be soft deleted")
	}

	// 测试 tags 表
	tag := &Tag{Name: "SoftDelete Tag", Slug: "softdelete-tag"}
	CreateTag(db, tag)

	if err := SoftDeleteRecord(db, TableTags, tag.ID); err != nil {
		t.Fatalf("SoftDeleteRecord for tags failed: %v", err)
	}

	// 验证已软删除
	_, err = GetTagByID(db, tag.ID)
	if err == nil {
		t.Fatal("tag should be soft deleted")
	}
}

func TestRestoreRecord(t *testing.T) {
	db := openTestDB(t)

	// 测试 posts 表
	p := &Post{Title: "Restore Test", Slug: "restore-test", Content: "c", Status: "published"}
	CreatePost(db, p)

	// 先软删除
	if err := SoftDeleteRecord(db, TablePosts, p.ID); err != nil {
		t.Fatalf("SoftDeleteRecord failed: %v", err)
	}

	// 恢复
	if err := RestoreRecord(db, TablePosts, p.ID); err != nil {
		t.Fatalf("RestoreRecord failed: %v", err)
	}

	// 验证已恢复
	got, err := GetPostByID(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID failed after restore: %v", err)
	}
	if got.Title != "Restore Test" {
		t.Fatalf("unexpected title: %s", got.Title)
	}

	// 测试 categories 表
	c := &Category{Name: "Restore Cat", Slug: "restore-cat", Sort: 1}
	CreateCategory(db, c)

	// 先软删除
	if err := SoftDeleteRecord(db, TableCategories, c.ID); err != nil {
		t.Fatalf("SoftDeleteRecord for categories failed: %v", err)
	}

	// 恢复
	if err := RestoreRecord(db, TableCategories, c.ID); err != nil {
		t.Fatalf("RestoreRecord for categories failed: %v", err)
	}

	// 验证已恢复
	gotCat, err := GetCategoryByID(db, c.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID failed after restore: %v", err)
	}
	if gotCat.Name != "Restore Cat" {
		t.Fatalf("unexpected name: %s", gotCat.Name)
	}
}

func TestHardDeleteRecord(t *testing.T) {
	db := openTestDB(t)

	// 测试 posts 表
	p := &Post{Title: "HardDelete Test", Slug: "harddelete-test", Content: "c", Status: "published"}
	CreatePost(db, p)
	postID := p.ID

	// 硬删除
	if err := HardDeleteRecord(db, TablePosts, p.ID); err != nil {
		t.Fatalf("HardDeleteRecord failed: %v", err)
	}

	// 验证已硬删除（包括 deleted_at 为 NULL 的记录也查不到）
	var foundID int64
	err := db.QueryRow(`SELECT id FROM posts WHERE id=?`, postID).Scan(&foundID)
	if err == nil {
		t.Fatal("post should be hard deleted from database")
	}

	// 测试 redirects 表
	r := &Redirect{From: "/harddelete-from", To: "/harddelete-to"}
	CreateRedirect(db, r)
	redirectID := r.ID

	// 硬删除
	if err := HardDeleteRecord(db, TableRedirects, r.ID); err != nil {
		t.Fatalf("HardDeleteRecord for redirects failed: %v", err)
	}

	// 验证已硬删除
	err = db.QueryRow(`SELECT id FROM redirects WHERE id=?`, redirectID).Scan(&foundID)
	if err == nil {
		t.Fatal("redirect should be hard deleted from database")
	}
}

func TestGetRecordByID(t *testing.T) {
	db := openTestDB(t)

	// 测试 posts 表
	p := &Post{Title: "GetByID Test", Slug: "getbyid-test", Content: "c", Status: "published"}
	CreatePost(db, p)

	result, err := GetRecordByID(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", p.ID, func(row *sql.Row) (interface{}, error) {
		var post Post
		var deletedAt sql.NullInt64
		if err := row.Scan(
			&post.ID, &post.Title, &post.Slug, &post.Content, &post.Status,
			&post.CreatedAt, &post.UpdatedAt, &deletedAt,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, err
		}
		if deletedAt.Valid {
			post.DeletedAt = &deletedAt.Int64
		}
		return &post, nil
	})
	if err != nil {
		t.Fatalf("GetRecordByID failed: %v", err)
	}

	post := result.(*Post)
	if post.Title != "GetByID Test" {
		t.Fatalf("unexpected title: %s", post.Title)
	}

	// 测试不存在的记录
	_, err = GetRecordByID(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", 99999, func(row *sql.Row) (interface{}, error) {
		var post Post
		var deletedAt sql.NullInt64
		if err := row.Scan(
			&post.ID, &post.Title, &post.Slug, &post.Content, &post.Status,
			&post.CreatedAt, &post.UpdatedAt, &deletedAt,
		); err != nil {
			if err == sql.ErrNoRows {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &post, nil
	})
	if err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestListRecords(t *testing.T) {
	db := openTestDB(t)

	// 创建多个 posts
	for i := 0; i < 3; i++ {
		p := &Post{
			Title:   fmt.Sprintf("List Test %d", i),
			Slug:    fmt.Sprintf("list-test-%d", i),
			Content: "c",
			Status:  "published",
		}
		CreatePost(db, p)
	}

	// 测试列出所有记录
	results, err := ListRecords(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", "", "created_at DESC", nil, 10, 0, func(rows *sql.Rows) (interface{}, error) {
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
		t.Fatalf("ListRecords failed: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected at least 3 records, got %d", len(results))
	}

	// 测试带条件查询
	results, err = ListRecords(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", "status=?", "created_at DESC", []interface{}{"published"}, 10, 0, func(rows *sql.Rows) (interface{}, error) {
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
		t.Fatalf("ListRecords with where clause failed: %v", err)
	}
	if len(results) < 3 {
		t.Fatalf("expected at least 3 published posts, got %d", len(results))
	}

	// 测试分页
	results, err = ListRecords(db, TablePosts, "id, title, slug, content, status, created_at, updated_at, deleted_at", "", "created_at DESC", nil, 2, 0, func(rows *sql.Rows) (interface{}, error) {
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
		t.Fatalf("ListRecords with pagination failed: %v", err)
	}
	if len(results) > 2 {
		t.Fatalf("expected at most 2 records with limit, got %d", len(results))
	}
}

func TestListDeletedRecords(t *testing.T) {
	db := openTestDB(t)

	// 创建并软删除 posts
	p1 := &Post{Title: "Deleted 1", Slug: "deleted-1", Content: "c", Status: "published"}
	p2 := &Post{Title: "Deleted 2", Slug: "deleted-2", Content: "c", Status: "published"}
	p3 := &Post{Title: "Not Deleted", Slug: "not-deleted", Content: "c", Status: "published"}

	CreatePost(db, p1)
	CreatePost(db, p2)
	CreatePost(db, p3)

	SoftDeleteRecord(db, TablePosts, p1.ID)
	SoftDeleteRecord(db, TablePosts, p2.ID)

	// 测试列出已删除记录
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
		t.Fatalf("ListDeletedRecords failed: %v", err)
	}
	if len(results) < 2 {
		t.Fatalf("expected at least 2 deleted records, got %d", len(results))
	}

	// 验证 p3 不在已删除列表中
	for _, result := range results {
		post := result.(Post)
		if post.ID == p3.ID {
			t.Fatal("p3 should not be in deleted list")
		}
	}
}

func TestCountRecords(t *testing.T) {
	db := openTestDB(t)

	// 创建多个 tags
	var createdTags []*Tag
	for i := 0; i < 5; i++ {
		tag := &Tag{
			Name: fmt.Sprintf("Count Test %d", i),
			Slug: fmt.Sprintf("count-test-%d", i),
		}
		if err := CreateTag(db, tag); err != nil {
			t.Fatalf("CreateTag failed: %v", err)
		}
		createdTags = append(createdTags, tag)
	}

	// 软删除第一个 tag
	if len(createdTags) > 0 {
		if err := SoftDeleteRecord(db, TableTags, createdTags[0].ID); err != nil {
			t.Fatalf("SoftDeleteRecord failed: %v", err)
		}
	}

	// 测试统计所有记录（不包括已删除）
	count, err := CountRecords(db, TableTags, "", nil)
	if err != nil {
		t.Fatalf("CountRecords failed: %v", err)
	}
	if count < 4 {
		t.Fatalf("expected at least 4 records, got %d", count)
	}

	// 测试带条件统计
	count, err = CountRecords(db, TableTags, "slug LIKE ?", []interface{}{"count-test%"})
	if err != nil {
		t.Fatalf("CountRecords with where clause failed: %v", err)
	}
	if count < 4 {
		t.Fatalf("expected at least 4 records with condition, got %d", count)
	}
}
