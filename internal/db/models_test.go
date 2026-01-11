package db

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()

	db := Open(Options{
		DSN: ":memory:",
	})

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	return db
}

func softDeleteByTable(db *DB, t *testing.T, table string, id int64) {
	t.Helper()
	_, err := db.Exec(
		`UPDATE `+table+` SET deleted_at=? WHERE id=? AND deleted_at IS NULL`,
		now(), id,
	)
	if err != nil {
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

	softDeleteByTable(db, t, "encrypted_posts", p.ID)

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

	softDeleteByTable(db, t, "tags", tag.ID)

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

	softDeleteByTable(db, t, "redirects", r.ID)

	// from_path 仍然唯一
	r2 := &Redirect{
		From: "/old",
		To:   "/another",
	}
	if err := CreateRedirect(db, r2); err == nil {
		t.Fatal("redirect from_path should remain unique after soft delete")
	}
}

func TestCronJobs(t *testing.T) {
	dbx := openTestDB(t)

	// ------------------------------
	// 1️⃣ 测试创建 CronJob
	job := &CronJob{
		Code:      "test_job",
		Name:      "测试任务",
		Schedule:  "* * * * *",
		Enabled:   1,
		CreatedAt: time.Now().Unix(),
		UpdatedAt: time.Now().Unix(),
	}

	if err := CreateCronJob(dbx, job); err != nil {
		t.Fatalf("failed to create cron job: %v", err)
	}
	if job.ID == 0 {
		t.Fatal("expected job.ID > 0")
	}

	// 2️⃣ 测试查询 CronJob
	jobs, err := ListCronJobs(dbx)
	if err != nil {
		t.Fatalf("failed to list cron jobs: %v", err)
	}
	if len(jobs) != 1 || jobs[0].Code != "test_job" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}

	// ------------------------------
	// 3️⃣ 测试创建 CronJobRun
	run := &CronJobRun{
		JobCode:    job.Code,
		RunID:      uuid.NewString(),
		Status:     "pending",
		Message:    "",
		StartedAt:  time.Now().Unix(),
		FinishedAt: time.Now().Unix(),
		Duration:   0,
		CreatedAt:  time.Now().Unix(),
	}
	if err := CreateCronJobRun(dbx, run); err != nil {
		t.Fatalf("failed to create cron job run: %v", err)
	}
	if run.ID == 0 {
		t.Fatal("expected run.ID > 0")
	}

	// 4️⃣ 测试查询 CronJobRun
	runs, err := ListCronJobRuns(dbx, job.Code, "", 10)
	if err != nil {
		t.Fatalf("failed to list cron job runs: %v", err)
	}
	if len(runs) != 1 || runs[0].Status != "pending" {
		t.Fatalf("unexpected runs: %+v", runs)
	}

	// 5️⃣ 测试更新 Run 状态
	run.Status = "success"
	run.Message = "ok"
	run.FinishedAt = time.Now().Unix()
	run.Duration = 123
	if err := UpdateCronJobRunStatus(dbx, run); err != nil {
		t.Fatalf("failed to update cron job run: %v", err)
	}

	updated, err := ListCronJobRuns(dbx, job.Code, "success", 10)
	if err != nil {
		t.Fatalf("failed to list cron job runs: %v", err)
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

	softDeleteByTable(db, t, "categories", c.ID)

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
