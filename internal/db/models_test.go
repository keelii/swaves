package db

import (
	"database/sql"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
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

func TestCreateSettings(t *testing.T) {
	db := openTestDB(t)

	c := &Settings{
		Name:              "My Blog",
		Language:          "zh-CN",
		Timezone:          "Asia/Shanghai",
		PostSlugPattern:   "/{yyyy}/{MM}/{dd}/{slug}",
		TagSlugPattern:    "/tags/{slug}",
		TagsPattern:       ",",
		GiscusConfig:      `{"repo":"a/b"}`,
		GA4ID:             "G-XXXX",
		AdminPasswordHash: "raw_password",
	}

	if err := CreateSettings(db, c); err != nil {
		t.Fatalf("CreateSettings failed: %v", err)
	}
	if c.ID == 0 {
		t.Fatal("settings id not set")
	}
}
func TestSettings_SoftDelete(t *testing.T) {
	db := openTestDB(t)

	c := &Settings{
		Name:              "Blog",
		Language:          "en",
		Timezone:          "UTC",
		PostSlugPattern:   "/{slug}",
		TagSlugPattern:    "/tags/{slug}",
		TagsPattern:       ",",
		AdminPasswordHash: "raw_password",
	}
	CreateSettings(db, c)

	softDeleteByTable(db, t, "settings", c.ID)

	// 允许再次创建（settings 无唯一约束）
	c2 := &Settings{
		Name:              "Blog2",
		Language:          "zh",
		Timezone:          "Asia/Shanghai",
		PostSlugPattern:   "/{yyyy}/{slug}",
		TagSlugPattern:    "/t/{slug}",
		TagsPattern:       "|",
		AdminPasswordHash: "raw_password",
	}
	if err := CreateSettings(db, c2); err != nil {
		t.Fatal(err)
	}
}

func GetSettingsForTest(db *DB) (*Settings, error) {
	row := db.QueryRow(`
		SELECT
			id,
			name,
			language,
			timezone,
			post_slug_pattern,
			tag_slug_pattern,
			tags_pattern,
			giscus_config,
			ga4_id,
			admin_password_hash,
			created_at,
			updated_at,
			deleted_at
		FROM settings
		WHERE deleted_at IS NULL
		ORDER BY id ASC
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

func TestCreateSettings_AdminPasswordIsBcrypt(t *testing.T) {
	db := openTestDB(t)
	defer db.Close()

	db.Exec(`DELETE FROM settings where deleted_at IS NULL`)
	//_, err := GetSettingsForTest(db)

	cfg := &Settings{
		Name:              "swaves",
		Language:          "zh-CN",
		Timezone:          "Asia/Shanghai",
		PostSlugPattern:   "/{yyyy}/{MM}/{dd}/{name}",
		TagSlugPattern:    "/tags/{name}",
		TagsPattern:       "/tags",
		AdminPasswordHash: "plain-password",
	}

	if err := CreateSettings(db, cfg); err != nil {
		t.Fatalf("CreateSettings failed: %v", err)
	}

	got, err := GetSettingsForTest(db)
	if err != nil {
		t.Fatalf("GetSettingsForTest failed: %v", err)
	}

	if got.AdminPasswordHash == "plain-password" {
		t.Fatal("admin password should not be stored as plaintext")
	}

	if err := bcrypt.CompareHashAndPassword(
		[]byte(got.AdminPasswordHash),
		[]byte("plain-password"),
	); err != nil {
		t.Fatalf("bcrypt compare failed: %v", err)
	}
}

func TestCreateAndListCronJobs(t *testing.T) {
	dbx := openTestDB(t)

	job := &CronJob{
		Name:     "test_job",
		Schedule: "*/5 * * * *",
		Enabled:  true,
	}

	if err := CreateCronJob(dbx, job); err != nil {
		t.Fatalf("CreateCronJob failed: %v", err)
	}

	if job.ID == 0 {
		t.Fatal("job ID not set")
	}

	list, err := ListCronJobs(dbx)
	if err != nil {
		t.Fatalf("ListCronJobs failed: %v", err)
	}

	if len(list) != 1 {
		t.Fatalf("expected 1 job, got %d", len(list))
	}

	if list[0].Name != "test_job" {
		t.Fatalf("unexpected job name: %s", list[0].Name)
	}
}

func TestCreateCronJobLogSuccess(t *testing.T) {
	dbx := openTestDB(t)

	job := &CronJob{
		Name:     "success_job",
		Schedule: "* * * * *",
		Enabled:  true,
	}
	if err := CreateCronJob(dbx, job); err != nil {
		t.Fatal(err)
	}

	start := time.Now().Unix()
	end := start + 2

	log := &CronJobLog{
		JobID:      job.ID,
		Status:     "success",
		Message:    "ok",
		StartedAt:  start,
		FinishedAt: end,
		Duration:   end - start,
	}

	if err := CreateCronJobLog(dbx, log); err != nil {
		t.Fatalf("CreateCronJobLog failed: %v", err)
	}

	if log.ID == 0 {
		t.Fatal("log ID not set")
	}
	if log.RunID == "" {
		t.Fatal("run_id should be generated")
	}
}

func TestCreateCronJobLogErrorUpdatesJob(t *testing.T) {
	dbx := openTestDB(t)

	job := &CronJob{
		Name:     "error_job",
		Schedule: "* * * * *",
		Enabled:  true,
	}
	if err := CreateCronJob(dbx, job); err != nil {
		t.Fatal(err)
	}

	start := time.Now().Unix()
	end := start + 1

	log := &CronJobLog{
		JobID:      job.ID,
		Status:     "error",
		Message:    "failed",
		StartedAt:  start,
		FinishedAt: end,
		Duration:   end - start,
	}

	if err := CreateCronJobLog(dbx, log); err != nil {
		t.Fatal(err)
	}

	// 重新查询 job
	jobs, err := ListCronJobs(dbx)
	if err != nil {
		t.Fatal(err)
	}

	if jobs[0].LastErrorAt == nil {
		t.Fatal("LastErrorAt should be set")
	}
}
