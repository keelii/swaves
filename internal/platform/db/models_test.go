package db

import (
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"swaves/internal/shared/types"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()

	dsn := filepath.Join(t.TempDir(), "data.sqlite")
	db := Open(Options{DSN: dsn})
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func uniqueValue(prefix string) string {
	return fmt.Sprintf("%s_%s", prefix, strings.ReplaceAll(uuid.NewString(), "-", ""))
}

func mustCreatePost(t *testing.T, db *DB, status string, kind PostKind, publishedAt int64) *Post {
	t.Helper()
	slug := uniqueValue("post")
	p := &Post{
		Title:       slug,
		Slug:        slug,
		Content:     "content-" + slug,
		Status:      status,
		Kind:        kind,
		PublishedAt: publishedAt,
	}
	if _, err := CreatePost(db, p); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}
	return p
}

func mustCreateTag(t *testing.T, db *DB, slug string) *Tag {
	t.Helper()
	tag := &Tag{Name: slug, Slug: slug}
	if _, err := CreateTag(db, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	return tag
}

func mustCreateCategory(t *testing.T, db *DB, parentID int64, slug string) *Category {
	t.Helper()
	c := &Category{ParentID: parentID, Name: slug, Slug: slug, Sort: 1}
	if _, err := CreateCategory(db, c); err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	return c
}

func TestGenericCRUDFlow(t *testing.T) {
	db := openTestDB(t)
	slug := uniqueValue("generic")

	id, err := Create(db, specPosts, map[string]interface{}{
		"title":        "Generic",
		"slug":         slug,
		"content":      "body",
		"status":       "draft",
		"kind":         PostKindPost,
		"published_at": int64(0),
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	cnt, err := Count(db, specPosts, "id=?", []interface{}{id})
	if err != nil {
		t.Fatalf("Count failed: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected count 1, got %d", cnt)
	}

	results, err := Read(db, specPosts, ReadOptions{
		SelectFields: "id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
		WhereClause:  "id=?",
		WhereArgs:    []interface{}{id},
		Limit:        1,
	}, func(rows *sql.Rows) (interface{}, error) {
		return scanPost(rows, true)
	})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 row, got %d", len(results))
	}

	if err := Update(db, specPosts, id, map[string]interface{}{"title": "Generic2"}); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	p, err := GetPostByIDAnyStatus(db, id)
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}
	if p.Title != "Generic2" {
		t.Fatalf("expected updated title, got %s", p.Title)
	}

	if err := Delete(db, specPosts, id); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
	cnt, err = Count(db, specPosts, "id=?", []interface{}{id})
	if err != nil {
		t.Fatalf("Count after delete failed: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected count 0 after soft delete, got %d", cnt)
	}

	delRows, err := ListDeletedRecords(db, TablePosts,
		"id, title, slug, content, status, kind, comment_enabled, created_at, updated_at, published_at, deleted_at",
		"",
		func(rows *sql.Rows) (interface{}, error) {
			return scanPost(rows, true)
		},
	)
	if err != nil {
		t.Fatalf("ListDeletedRecords failed: %v", err)
	}
	if len(delRows) != 1 {
		t.Fatalf("expected 1 deleted row, got %d", len(delRows))
	}

	if err := Restore(db, specPosts, id); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
	cnt, err = Count(db, specPosts, "id=?", []interface{}{id})
	if err != nil {
		t.Fatalf("Count after restore failed: %v", err)
	}
	if cnt != 1 {
		t.Fatalf("expected count 1 after restore, got %d", cnt)
	}

	if err := HardDelete(db, specPosts, id); err != nil {
		t.Fatalf("HardDelete failed: %v", err)
	}
	cnt, err = Count(db, specPosts, "id=?", []interface{}{id})
	if err != nil {
		t.Fatalf("Count after hard delete failed: %v", err)
	}
	if cnt != 0 {
		t.Fatalf("expected count 0 after hard delete, got %d", cnt)
	}
}

func TestValidateWhereArgsAndHelpers(t *testing.T) {
	if err := validateWhereArgs("", nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := validateWhereArgs("", []interface{}{1}); err == nil {
		t.Fatal("expected error for args without where")
	}
	if err := validateWhereArgs("a=? AND b=?", []interface{}{1, 2}); err != nil {
		t.Fatalf("unexpected mismatch: %v", err)
	}
	if err := validateWhereArgs("a=?", []interface{}{1, 2}); err == nil {
		t.Fatal("expected mismatch error")
	}

	if got := appendWhere("", "a=1"); got != "a=1" {
		t.Fatalf("unexpected appendWhere result: %s", got)
	}
	if got := appendWhere("a=1", "b=2"); got != "a=1 AND b=2" {
		t.Fatalf("unexpected appendWhere result: %s", got)
	}

	data := map[string]interface{}{"created_at": int64(0)}
	ensureTimeField(data, "created_at")
	if data["created_at"].(int64) == 0 {
		t.Fatal("ensureTimeField should set timestamp")
	}

	if _, ok := firstResult(nil); ok {
		t.Fatal("firstResult should be empty")
	}
	if v, ok := firstResult([]interface{}{123}); !ok || v.(int) != 123 {
		t.Fatal("firstResult should return first item")
	}
}

func TestErrorHelpers(t *testing.T) {
	nf := ErrNotFound("x")
	if !IsErrNotFound(nf) {
		t.Fatal("ErrNotFound should satisfy IsErrNotFound")
	}

	in := ErrInternalError("x")
	if !IsErrInternalError(in) {
		t.Fatal("ErrInternalError should satisfy IsErrInternalError")
	}

	wrapped := WrapInternalErr("x", errors.New("boom"))
	if !IsErrInternalError(wrapped) {
		t.Fatal("WrapInternalErr should satisfy IsErrInternalError")
	}
}

func TestPostVisibilityAndPublish(t *testing.T) {
	db := openTestDB(t)

	draft := mustCreatePost(t, db, "draft", PostKindPost, 0)
	published := mustCreatePost(t, db, "published", PostKindPost, 0)

	if _, err := GetPostBySlug(db, published.Slug); err != nil {
		t.Fatalf("GetPostByIST published failed: %v", err)
	}
	if _, err := GetPostBySlug(db, draft.Slug); !IsErrNotFound(err) {
		t.Fatalf("draft should not be visible by slug, got: %v", err)
	}
	if _, err := GetPostByID(db, draft.ID); !IsErrNotFound(err) {
		t.Fatalf("draft should not be visible by id, got: %v", err)
	}

	if err := PublishPost(db, draft.ID); err != nil {
		t.Fatalf("PublishPost failed: %v", err)
	}
	postByID, err := GetPostByIDAnyStatus(db, draft.ID)
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}
	if postByID.Status != "published" || postByID.PublishedAt == 0 {
		t.Fatalf("draft should be published now: %+v", postByID)
	}
	if _, err := GetPostBySlug(db, draft.Slug); err != nil {
		t.Fatalf("published draft should be visible now: %v", err)
	}
}

func TestListPublishedPostsAndPages(t *testing.T) {
	db := openTestDB(t)

	_ = mustCreatePost(t, db, "published", PostKindPost, 0)
	_ = mustCreatePost(t, db, "draft", PostKindPost, 0)
	_ = mustCreatePost(t, db, "published", PostKindPage, 0)

	var nilPager *types.Pagination
	posts := ListPublishedPosts(db, PostKindPost, nilPager)
	if len(posts) == 0 {
		t.Fatal("expected published posts")
	}

	pager := &types.Pagination{Page: 1, PageSize: 10}
	pages := ListPublishedPosts(db, PostKindPage, pager)
	if len(pages) == 0 {
		t.Fatal("expected published pages")
	}
	if pager.Page != 1 || pager.PageSize != 1024 {
		t.Fatalf("page kind should force pager, got %+v", pager)
	}

	listPages := ListPublishedPages(db)
	if len(listPages) == 0 {
		t.Fatal("ListPublishedPages should return rows")
	}
	for _, p := range listPages {
		if p.Kind != PostKindPage {
			t.Fatalf("expected page kind, got %v", p.Kind)
		}
	}
}

func TestListPostsFiltersAndRelations(t *testing.T) {
	db := openTestDB(t)

	p1 := mustCreatePost(t, db, "published", PostKindPost, 0)
	p2 := mustCreatePost(t, db, "published", PostKindPost, 0)

	tag1 := mustCreateTag(t, db, uniqueValue("tag1"))
	tag2 := mustCreateTag(t, db, uniqueValue("tag2"))
	cat1 := mustCreateCategory(t, db, 0, uniqueValue("cat1"))
	cat2 := mustCreateCategory(t, db, 0, uniqueValue("cat2"))

	if err := AttachTagToPost(db, p1.ID, tag1.ID); err != nil {
		t.Fatalf("AttachTagToPost failed: %v", err)
	}
	if err := AttachTagToPost(db, p2.ID, tag2.ID); err != nil {
		t.Fatalf("AttachTagToPost failed: %v", err)
	}
	if err := SetPostCategory(db, p1.ID, cat1.ID); err != nil {
		t.Fatalf("SetPostCategory failed: %v", err)
	}
	if err := SetPostCategory(db, p2.ID, cat2.ID); err != nil {
		t.Fatalf("SetPostCategory failed: %v", err)
	}

	all, err := ListPosts(db, nil)
	if err != nil {
		t.Fatalf("ListPosts nil opts failed: %v", err)
	}
	if len(all) < 2 {
		t.Fatalf("expected >=2 posts, got %d", len(all))
	}

	byTag, err := ListPostsByTag(db, &PostQueryOptions{
		TagID: tag1.ID,
		Pager: &types.Pagination{Page: 1, PageSize: 20},
	})
	if err != nil {
		t.Fatalf("ListPostsByTag failed: %v", err)
	}
	if len(byTag) != 1 || byTag[0].Post.ID != p1.ID {
		t.Fatalf("unexpected byTag result: %+v", byTag)
	}
	if len(byTag[0].Tags) == 0 {
		t.Fatal("expected related tags")
	}

	byCategory, err := ListPostsByCategory(db, &PostQueryOptions{
		CategoryID: cat2.ID,
		Pager:      &types.Pagination{Page: 1, PageSize: 20},
	})
	if err != nil {
		t.Fatalf("ListPostsByCategory failed: %v", err)
	}
	if len(byCategory) != 1 || byCategory[0].Post.ID != p2.ID {
		t.Fatalf("unexpected byCategory result: %+v", byCategory)
	}
	if byCategory[0].Category == nil || byCategory[0].Category.ID != cat2.ID {
		t.Fatalf("expected category %d", cat2.ID)
	}
}

func TestListPostsBySearch(t *testing.T) {
	db := openTestDB(t)
	needle := uniqueValue("needle")

	p1 := mustCreatePost(t, db, "draft", PostKindPost, 0)
	p1.Title = "title-" + needle
	if err := UpdatePost(db, p1); err != nil {
		t.Fatalf("UpdatePost failed: %v", err)
	}

	p2 := mustCreatePost(t, db, "draft", PostKindPost, 0)
	if err := Update(db, specPosts, p2.ID, map[string]interface{}{"slug": "slug-" + needle}); err != nil {
		t.Fatalf("Update slug failed: %v", err)
	}

	res, err := ListPostsBySearch(db, &PostQueryOptions{Pager: &types.Pagination{Page: 1, PageSize: 10}}, needle)
	if err != nil {
		t.Fatalf("ListPostsBySearch failed: %v", err)
	}
	if len(res) < 2 {
		t.Fatalf("expected >=2 search results, got %d", len(res))
	}

	empty, err := ListPostsBySearch(db, &PostQueryOptions{Pager: &types.Pagination{Page: 1, PageSize: 10}}, "   ")
	if err != nil {
		t.Fatalf("empty query should not error: %v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("empty query should return empty, got %d", len(empty))
	}
}

func TestPostCommentEnabledSwitch(t *testing.T) {
	db := openTestDB(t)

	p := mustCreatePost(t, db, "draft", PostKindPost, 0)

	created, err := GetPostByIDAnyStatus(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID failed: %v", err)
	}
	if created.CommentEnabled != 1 {
		t.Fatalf("new post should default comment_enabled=1, got %d", created.CommentEnabled)
	}

	if err := SetPostCommentEnabled(db, p.ID, false); err != nil {
		t.Fatalf("SetPostCommentEnabled(false) failed: %v", err)
	}
	disabled, err := GetPostByIDAnyStatus(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID after disable failed: %v", err)
	}
	if disabled.CommentEnabled != 0 {
		t.Fatalf("expected comment_enabled=0, got %d", disabled.CommentEnabled)
	}

	disabled.Title = disabled.Title + "-updated"
	disabled.CommentEnabled = 1
	if err := UpdatePost(db, &disabled); err != nil {
		t.Fatalf("UpdatePost failed: %v", err)
	}

	enabled, err := GetPostByIDAnyStatus(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostByID after update failed: %v", err)
	}
	if enabled.CommentEnabled != 1 {
		t.Fatalf("expected comment_enabled=1 after update, got %d", enabled.CommentEnabled)
	}
}

func TestGetPrevNextPost(t *testing.T) {
	db := openTestDB(t)

	_ = mustCreatePost(t, db, "published", PostKindPost, 100)
	mid := mustCreatePost(t, db, "published", PostKindPost, 200)
	_ = mustCreatePost(t, db, "published", PostKindPage, 250)
	_ = mustCreatePost(t, db, "draft", PostKindPost, 260)
	_ = mustCreatePost(t, db, "published", PostKindPost, 300)

	prev, next, err := GetPrevNextPost(db, mid.PublishedAt)
	if err != nil {
		t.Fatalf("GetPrevNextPost failed: %v", err)
	}
	if prev == nil || prev.PublishedAt != 100 {
		t.Fatalf("unexpected prev: %+v", prev)
	}
	if next == nil || next.PublishedAt != 300 {
		t.Fatalf("unexpected next: %+v", next)
	}
}

func TestPostDeleteRestoreAndDeletedList(t *testing.T) {
	db := openTestDB(t)
	p := mustCreatePost(t, db, "published", PostKindPost, 0)

	if err := SoftDeletePost(db, p.ID); err != nil {
		t.Fatalf("SoftDeletePost failed: %v", err)
	}
	if _, err := GetPostByID(db, p.ID); !IsErrNotFound(err) {
		t.Fatalf("expected not found after soft delete, got %v", err)
	}

	dels, err := ListDeletedPosts(db)
	if err != nil {
		t.Fatalf("ListDeletedPosts failed: %v", err)
	}
	if len(dels) == 0 {
		t.Fatal("expected deleted posts")
	}

	if err := RestorePost(db, p.ID); err != nil {
		t.Fatalf("RestorePost failed: %v", err)
	}
	if _, err := GetPostByID(db, p.ID); err != nil {
		t.Fatalf("GetPostByID after restore failed: %v", err)
	}
}

func TestTagLifecycleAndCounts(t *testing.T) {
	db := openTestDB(t)
	slug := uniqueValue("tag")
	tag := mustCreateTag(t, db, slug)

	if _, err := GetTagBySlug(db, slug); err != nil {
		t.Fatalf("GetTagBySlug failed: %v", err)
	}
	if _, err := GetTagByID(db, tag.ID); err != nil {
		t.Fatalf("GetTagByID failed: %v", err)
	}

	tag.Name = "updated"
	if err := UpdateTag(db, tag); err != nil {
		t.Fatalf("UpdateTag failed: %v", err)
	}

	p := mustCreatePost(t, db, "published", PostKindPost, 0)
	if err := AttachTagToPost(db, p.ID, tag.ID); err != nil {
		t.Fatalf("AttachTagToPost failed: %v", err)
	}

	counts, err := CountPostsByTags(db, []int64{tag.ID, 999999})
	if err != nil {
		t.Fatalf("CountPostsByTags failed: %v", err)
	}
	if counts[tag.ID] != 1 || counts[999999] != 0 {
		t.Fatalf("unexpected counts: %+v", counts)
	}

	nowTs := time.Now().Unix()
	if err := UpdateTagCreatedAtIfEarlier(db, tag.ID, nowTs-100); err != nil {
		t.Fatalf("UpdateTagCreatedAtIfEarlier failed: %v", err)
	}
	got, err := GetTagByID(db, tag.ID)
	if err != nil {
		t.Fatalf("GetTagByID failed: %v", err)
	}
	if got.CreatedAt != nowTs-100 {
		t.Fatalf("created_at should be updated to earlier, got %d", got.CreatedAt)
	}

	if err := SoftDeleteTag(db, tag.ID); err != nil {
		t.Fatalf("SoftDeleteTag failed: %v", err)
	}
	if _, err := GetTagByID(db, tag.ID); !IsErrNotFound(err) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	if err := RestoreTag(db, tag.ID); err != nil {
		t.Fatalf("RestoreTag failed: %v", err)
	}
	if _, err := GetTagByID(db, tag.ID); err != nil {
		t.Fatalf("GetTagByID after restore failed: %v", err)
	}
}

func TestSetPostTags(t *testing.T) {
	db := openTestDB(t)
	p := mustCreatePost(t, db, "published", PostKindPost, 0)
	t1 := mustCreateTag(t, db, uniqueValue("t1"))
	t2 := mustCreateTag(t, db, uniqueValue("t2"))
	t3 := mustCreateTag(t, db, uniqueValue("t3"))

	if err := SetPostTags(db, p.ID, []int64{t1.ID, t2.ID}); err != nil {
		t.Fatalf("SetPostTags failed: %v", err)
	}
	tags, err := GetPostTags(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags, got %d", len(tags))
	}

	if err := SetPostTags(db, p.ID, []int64{t2.ID, t3.ID}); err != nil {
		t.Fatalf("SetPostTags update failed: %v", err)
	}
	tags, err = GetPostTags(db, p.ID)
	if err != nil {
		t.Fatalf("GetPostTags failed: %v", err)
	}
	if len(tags) != 2 {
		t.Fatalf("expected 2 tags after update, got %d", len(tags))
	}
	found := map[int64]bool{}
	for _, tg := range tags {
		found[tg.ID] = true
	}
	if !found[t2.ID] || !found[t3.ID] || found[t1.ID] {
		t.Fatalf("unexpected tags after update: %+v", found)
	}
}

func TestCategoryLifecycleAndRules(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateCategory(db, &Category{ParentID: 999999, Name: "x", Slug: uniqueValue("bad")}); err == nil {
		t.Fatal("expected parent not exists error")
	}

	parent := mustCreateCategory(t, db, 0, uniqueValue("parent"))
	child := mustCreateCategory(t, db, parent.ID, uniqueValue("child"))

	if !strings.Contains(parent.Slug, "parent") {
		t.Fatal("unexpected parent slug")
	}
	if got, err := GetCategoryByID(db, child.ID); err != nil || got.ParentID != parent.ID {
		t.Fatalf("GetCategoryByID child mismatch: %+v, err=%v", got, err)
	}
	if got, err := GetCategoryBySlug(db, child.Slug); err != nil || got.ID != child.ID {
		t.Fatalf("GetCategoryBySlug mismatch: %+v, err=%v", got, err)
	}

	if err := UpdateCategoryParent(db, parent.ID, child.ID); err == nil {
		t.Fatal("expected cycle detection error")
	}

	dup := &Category{ParentID: parent.ID, Name: "dup", Slug: child.Slug}
	if _, err := CreateCategory(db, dup); err == nil {
		t.Fatal("expected duplicate slug error under same parent")
	}

	// soft delete 后 slug 仍被占用（create/update 一致）
	if err := SoftDeleteCategory(db, child.ID); err != nil {
		t.Fatalf("SoftDeleteCategory failed: %v", err)
	}
	if _, err := CreateCategory(db, &Category{ParentID: parent.ID, Name: "dup2", Slug: child.Slug}); err == nil {
		t.Fatal("expected duplicate slug error even when sibling is soft deleted")
	}

	sibling := mustCreateCategory(t, db, parent.ID, uniqueValue("sibling"))
	sibling.Slug = child.Slug
	if err := UpdateCategory(db, sibling); err == nil {
		t.Fatal("expected UpdateCategory duplicate slug error against soft deleted sibling")
	}

	if err := RestoreCategory(db, child.ID); err != nil {
		t.Fatalf("RestoreCategory failed: %v", err)
	}
	if _, err := GetCategoryByID(db, child.ID); err != nil {
		t.Fatalf("GetCategoryByID after restore failed: %v", err)
	}

	if err := UpdateCategoryCreatedAtIfEarlier(db, child.ID, 100); err != nil {
		t.Fatalf("UpdateCategoryCreatedAtIfEarlier failed: %v", err)
	}
	gotChild, err := GetCategoryByID(db, child.ID)
	if err != nil {
		t.Fatalf("GetCategoryByID failed: %v", err)
	}
	if gotChild.CreatedAt != 100 {
		t.Fatalf("expected created_at updated to 100, got %d", gotChild.CreatedAt)
	}

	dels, err := ListDeletedCategories(db)
	if err != nil {
		t.Fatalf("ListDeletedCategories failed: %v", err)
	}
	if len(dels) != 0 {
		t.Fatalf("expected no deleted categories now, got %d", len(dels))
	}
}

func TestCategoryPostAssociations(t *testing.T) {
	db := openTestDB(t)
	post := mustCreatePost(t, db, "published", PostKindPost, 0)
	cat1 := mustCreateCategory(t, db, 0, uniqueValue("cat1"))
	cat2 := mustCreateCategory(t, db, 0, uniqueValue("cat2"))

	if err := AttachCategoryToPost(db, post.ID, cat1.ID); err != nil {
		t.Fatalf("AttachCategoryToPost failed: %v", err)
	}
	if err := AttachCategoryToPost(db, post.ID, cat2.ID); err != nil {
		t.Fatalf("AttachCategoryToPost second failed: %v", err)
	}

	got, err := GetPostCategory(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostCategory failed: %v", err)
	}
	if got == nil || got.ID != cat2.ID {
		t.Fatalf("expected current category %d, got %+v", cat2.ID, got)
	}

	var activeCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM `+string(TablePostCategories)+` WHERE post_id=? AND deleted_at IS NULL`, post.ID).Scan(&activeCount); err != nil {
		t.Fatalf("count active category relation failed: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("expected single active category relation, got %d", activeCount)
	}

	if err := SetPostCategory(db, post.ID, 0); err != nil {
		t.Fatalf("SetPostCategory clear failed: %v", err)
	}
	got, err = GetPostCategory(db, post.ID)
	if err != nil {
		t.Fatalf("GetPostCategory failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil category after clear, got %+v", got)
	}

	if err := SetPostCategory(db, post.ID, cat1.ID); err != nil {
		t.Fatalf("SetPostCategory assign failed: %v", err)
	}
	if err := SetPostCategory(db, post.ID, cat1.ID); err != nil {
		t.Fatalf("SetPostCategory idempotent failed: %v", err)
	}
}

func TestListCategoriesWithPostCount(t *testing.T) {
	db := openTestDB(t)
	post := mustCreatePost(t, db, "published", PostKindPost, 0)
	cat := mustCreateCategory(t, db, 0, uniqueValue("countcat"))
	if err := SetPostCategory(db, post.ID, cat.ID); err != nil {
		t.Fatalf("SetPostCategory failed: %v", err)
	}

	list, err := ListCategories(db, true)
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	found := false
	for _, c := range list {
		if c.ID == cat.ID {
			found = true
			if c.PostCount != 1 {
				t.Fatalf("expected post count 1, got %d", c.PostCount)
			}
		}
	}
	if !found {
		t.Fatal("category not found in list")
	}
}

func TestEncryptedPostLifecycle(t *testing.T) {
	db := openTestDB(t)
	exp := time.Now().Unix() + 600
	ep := &EncryptedPost{
		Title:     "secret",
		Slug:      uniqueValue("enc"),
		Content:   "hello secret",
		Password:  "pwd",
		ExpiresAt: &exp,
	}
	if _, err := CreateEncryptedPost(db, ep); err != nil {
		t.Fatalf("CreateEncryptedPost failed: %v", err)
	}

	var raw string
	if err := db.QueryRow(`SELECT content FROM `+string(TableEncryptedPosts)+` WHERE id=?`, ep.ID).Scan(&raw); err != nil {
		t.Fatalf("query raw encrypted content failed: %v", err)
	}
	if raw == ep.Content {
		t.Fatal("encrypted content should not equal plaintext")
	}

	got, err := GetEncryptedPostByID(db, ep.ID)
	if err != nil {
		t.Fatalf("GetEncryptedPostByID failed: %v", err)
	}
	if got.Content != "hello secret" {
		t.Fatalf("decrypted content mismatch: %s", got.Content)
	}

	ep.Content = "new secret"
	if err := UpdateEncryptedPost(db, ep); err != nil {
		t.Fatalf("UpdateEncryptedPost failed: %v", err)
	}
	got, err = GetEncryptedPostByID(db, ep.ID)
	if err != nil {
		t.Fatalf("GetEncryptedPostByID failed: %v", err)
	}
	if got.Content != "new secret" {
		t.Fatalf("expected updated content, got %s", got.Content)
	}

	past := time.Now().Unix() - 10
	expired := &EncryptedPost{Title: "expired", Slug: uniqueValue("enc_expired"), Content: "x", Password: "p", ExpiresAt: &past}
	if _, err := CreateEncryptedPost(db, expired); err != nil {
		t.Fatalf("CreateEncryptedPost expired failed: %v", err)
	}
	affected, err := SoftDeleteExpiredEncryptedPosts(db, time.Now().Unix())
	if err != nil {
		t.Fatalf("SoftDeleteExpiredEncryptedPosts failed: %v", err)
	}
	if affected == 0 {
		t.Fatal("expected at least one expired encrypted post soft deleted")
	}

	dels, err := ListDeletedEncryptedPosts(db)
	if err != nil {
		t.Fatalf("ListDeletedEncryptedPosts failed: %v", err)
	}
	if len(dels) == 0 {
		t.Fatal("expected deleted encrypted posts")
	}

	if err := RestoreEncryptedPost(db, expired.ID); err != nil {
		t.Fatalf("RestoreEncryptedPost failed: %v", err)
	}
	if _, err := GetEncryptedPostByID(db, expired.ID); err != nil {
		t.Fatalf("GetEncryptedPostByID after restore failed: %v", err)
	}
}

func TestRedirectLifecycle(t *testing.T) {
	db := openTestDB(t)
	r := &Redirect{From: "/old-" + uniqueValue("r"), To: "/new"}
	if _, err := CreateRedirect(db, r); err != nil {
		t.Fatalf("CreateRedirect failed: %v", err)
	}
	if r.Status != 301 || r.Enabled != 1 {
		t.Fatalf("redirect defaults mismatch: %+v", r)
	}

	if _, err := GetRedirectByID(db, r.ID); err != nil {
		t.Fatalf("GetRedirectByID failed: %v", err)
	}
	if _, err := GetRedirectByFrom(db, r.From); err != nil {
		t.Fatalf("GetRedirectByFrom failed: %v", err)
	}

	r.To = "/newer"
	r.Status = 302
	r.Enabled = 0
	if err := UpdateRedirect(db, r); err != nil {
		t.Fatalf("UpdateRedirect failed: %v", err)
	}
	got, err := GetRedirectByID(db, r.ID)
	if err != nil {
		t.Fatalf("GetRedirectByID failed: %v", err)
	}
	if got.To != "/newer" || got.Status != 302 || got.Enabled != 0 {
		t.Fatalf("unexpected redirect after update: %+v", got)
	}

	list, total, err := ListRedirects(db, 10, 0)
	if err != nil {
		t.Fatalf("ListRedirects failed: %v", err)
	}
	if total == 0 || len(list) == 0 {
		t.Fatalf("expected redirects, total=%d len=%d", total, len(list))
	}

	if err := SoftDeleteRedirect(db, r.ID); err != nil {
		t.Fatalf("SoftDeleteRedirect failed: %v", err)
	}
	if _, err := GetRedirectByID(db, r.ID); !IsErrNotFound(err) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	dels, err := ListDeletedRedirects(db)
	if err != nil {
		t.Fatalf("ListDeletedRedirects failed: %v", err)
	}
	if len(dels) == 0 {
		t.Fatal("expected deleted redirects")
	}

	if err := RestoreRedirect(db, r.ID); err != nil {
		t.Fatalf("RestoreRedirect failed: %v", err)
	}
	if _, err := GetRedirectByID(db, r.ID); err != nil {
		t.Fatalf("GetRedirectByID after restore failed: %v", err)
	}
}

func TestSettingsLifecycleAndPassword(t *testing.T) {
	db := openTestDB(t)

	if _, err := CreateSetting(db, &Setting{Type: "text"}); err == nil {
		t.Fatal("expected code required error")
	}
	if _, err := CreateSetting(db, &Setting{Code: uniqueValue("s")}); err == nil {
		t.Fatal("expected type required error")
	}

	code := uniqueValue("setting")
	s := &Setting{Code: code, Type: "text", Name: "n", Value: "v", Kind: "Custom"}
	if _, err := CreateSetting(db, s); err != nil {
		t.Fatalf("CreateSetting failed: %v", err)
	}
	if _, err := GetSettingByCode(db, code); err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if _, err := GetSettingByID(db, s.ID); err != nil {
		t.Fatalf("GetSettingByID failed: %v", err)
	}

	s.Value = "v2"
	if err := UpdateSetting(db, s); err != nil {
		t.Fatalf("UpdateSetting failed: %v", err)
	}
	if err := UpdateSettingByCode(db, code, "v3"); err != nil {
		t.Fatalf("UpdateSettingByCode failed: %v", err)
	}
	got, err := GetSettingByCode(db, code)
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if got.Value != "v3" {
		t.Fatalf("expected updated value v3, got %s", got.Value)
	}

	pwdCode := uniqueValue("pwd")
	pwdSetting := &Setting{Code: pwdCode, Type: "password", Name: "pwd", Value: "abc123"}
	if _, err := CreateSetting(db, pwdSetting); err != nil {
		t.Fatalf("Create password setting failed: %v", err)
	}
	gotPwd, err := GetSettingByCode(db, pwdCode)
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if len(gotPwd.Value) < 60 {
		t.Fatalf("expected bcrypt hash, got %s", gotPwd.Value)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(gotPwd.Value), []byte("abc123")); err != nil {
		t.Fatalf("hash compare failed: %v", err)
	}

	if err := UpdateSettingByCode(db, pwdCode, "def456"); err != nil {
		t.Fatalf("UpdateSettingByCode password failed: %v", err)
	}
	gotPwd, err = GetSettingByCode(db, pwdCode)
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(gotPwd.Value), []byte("def456")); err != nil {
		t.Fatalf("updated hash compare failed: %v", err)
	}

	if err := UpdateSettingByCode(db, "admin_password", "secret-123"); err != nil {
		t.Fatalf("UpdateSettingByCode admin_password failed: %v", err)
	}
	if err := CheckPassword(db, "secret-123"); err != nil {
		t.Fatalf("CheckPassword should pass: %v", err)
	}
	if err := CheckPassword(db, "wrong"); err == nil {
		t.Fatal("CheckPassword should fail for wrong password")
	}

	m, err := LoadSettingsToMap(db)
	if err != nil {
		t.Fatalf("LoadSettingsToMap failed: %v", err)
	}
	if _, ok := m[code]; !ok {
		t.Fatalf("custom setting %s not found in map", code)
	}

	if err := DeleteSetting(db, s.ID); err != nil {
		t.Fatalf("DeleteSetting failed: %v", err)
	}
	if _, err := GetSettingByCode(db, code); !IsErrNotFound(err) {
		t.Fatalf("expected not found after delete, got %v", err)
	}
	if err := Restore(db, specSettings, s.ID); err != nil {
		t.Fatalf("Restore setting failed: %v", err)
	}
	if _, err := GetSettingByCode(db, code); err != nil {
		t.Fatalf("GetSettingByCode after restore failed: %v", err)
	}

	if err := EnsureDefaultSettings(db); err != nil {
		t.Fatalf("EnsureDefaultSettings should be idempotent: %v", err)
	}
}

func TestListSettingsOrder(t *testing.T) {
	db := openTestDB(t)

	settingA := &Setting{
		Code:  uniqueValue("setting_order"),
		Kind:  "OrderKind",
		Name:  "A",
		Type:  "text",
		Value: "a",
		Sort:  20,
	}
	if _, err := CreateSetting(db, settingA); err != nil {
		t.Fatalf("CreateSetting A failed: %v", err)
	}

	settingB := &Setting{
		Code:  uniqueValue("setting_order"),
		Kind:  "OrderKind",
		Name:  "B",
		Type:  "text",
		Value: "b",
		Sort:  10,
	}
	if _, err := CreateSetting(db, settingB); err != nil {
		t.Fatalf("CreateSetting B failed: %v", err)
	}

	settingC := &Setting{
		Code:  uniqueValue("setting_order"),
		Kind:  "OrderKind",
		Name:  "C",
		Type:  "text",
		Value: "c",
		Sort:  10,
	}
	if _, err := CreateSetting(db, settingC); err != nil {
		t.Fatalf("CreateSetting C failed: %v", err)
	}

	orderedByKind, err := ListSettingsByKind(db, "OrderKind")
	if err != nil {
		t.Fatalf("ListSettingsByKind failed: %v", err)
	}
	if len(orderedByKind) != 3 {
		t.Fatalf("expected 3 settings, got %d", len(orderedByKind))
	}
	if orderedByKind[0].Code != settingB.Code || orderedByKind[1].Code != settingC.Code || orderedByKind[2].Code != settingA.Code {
		t.Fatalf(
			"unexpected kind order: got [%s, %s, %s], want [%s, %s, %s]",
			orderedByKind[0].Code,
			orderedByKind[1].Code,
			orderedByKind[2].Code,
			settingB.Code,
			settingC.Code,
			settingA.Code,
		)
	}

	kindRank := func(kind string) int {
		for idx, item := range settingKindOrder {
			if item == kind {
				return idx + 1
			}
		}
		return 999
	}
	subKindRank := func(kind string, subKind string) int {
		normalizedSubKind := strings.TrimSpace(subKind)
		if normalizedSubKind == "" {
			normalizedSubKind = SettingSubKindGeneral
		}
		items, ok := settingSubKindOrder[kind]
		if !ok {
			return 999
		}
		for idx, item := range items {
			if normalizedSubKind == item {
				return idx + 1
			}
		}
		return 999
	}
	normalizedSubKind := func(raw string) string {
		value := strings.TrimSpace(raw)
		if value == "" {
			return SettingSubKindGeneral
		}
		return value
	}

	all, err := ListAllSettings(db)
	if err != nil {
		t.Fatalf("ListAllSettings failed: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("expected settings")
	}

	prevRank := -1
	prevSubKindRank := -1
	prevKind := ""
	prevSubKind := ""
	prevSort := int64(0)
	for i, s := range all {
		rank := kindRank(s.Kind)
		subKind := normalizedSubKind(s.SubKind)
		subRank := subKindRank(s.Kind, s.SubKind)
		if i == 0 {
			prevRank = rank
			prevSubKindRank = subRank
			prevKind = s.Kind
			prevSubKind = subKind
			prevSort = s.Sort
			continue
		}

		if rank < prevRank {
			t.Fatalf("kind order not stable at index %d: rank %d < prev %d", i, rank, prevRank)
		}
		if rank == prevRank && s.Kind < prevKind {
			t.Fatalf("kind order not stable at index %d: kind %s < prev %s", i, s.Kind, prevKind)
		}
		if rank == prevRank && s.Kind == prevKind && subRank < prevSubKindRank {
			t.Fatalf("sub kind rank not stable at index %d: rank %d < prev %d (kind=%s)", i, subRank, prevSubKindRank, s.Kind)
		}
		if rank == prevRank && s.Kind == prevKind && subRank == prevSubKindRank && subKind < prevSubKind {
			t.Fatalf("sub kind order not stable at index %d: sub_kind %s < prev %s (kind=%s)", i, subKind, prevSubKind, s.Kind)
		}
		if rank == prevRank && s.Kind == prevKind && subRank == prevSubKindRank && subKind == prevSubKind && s.Sort < prevSort {
			t.Fatalf("sort order not stable at index %d: sort %d < prev %d (kind=%s sub_kind=%s)", i, s.Sort, prevSort, s.Kind, subKind)
		}

		prevRank = rank
		prevSubKindRank = subRank
		prevKind = s.Kind
		prevSubKind = subKind
		prevSort = s.Sort
	}
}

func TestListSettingsByKindSubKindOrder(t *testing.T) {
	db := openTestDB(t)

	seeCode := uniqueValue("setting_subkind_see")
	imagekitCode := uniqueValue("setting_subkind_imagekit")
	s3Code := uniqueValue("setting_subkind_s3")
	generalCode := uniqueValue("setting_subkind_general")

	items := []*Setting{
		{Code: seeCode, Kind: SettingKindThirdPartyServices, SubKind: SettingSubKindSEE, Name: "see", Type: "text", Value: "1", Sort: 5},
		{Code: imagekitCode, Kind: SettingKindThirdPartyServices, SubKind: SettingSubKindImageKit, Name: "imagekit", Type: "text", Value: "1", Sort: 5},
		{Code: s3Code, Kind: SettingKindThirdPartyServices, SubKind: SettingSubKindS3, Name: "s3", Type: "text", Value: "1", Sort: 5},
		{Code: generalCode, Kind: SettingKindThirdPartyServices, SubKind: "", Name: "general", Type: "text", Value: "1", Sort: 5},
	}
	for _, item := range items {
		if _, err := CreateSetting(db, item); err != nil {
			t.Fatalf("CreateSetting(%s) failed: %v", item.Code, err)
		}
	}

	list, err := ListSettingsByKind(db, SettingKindThirdPartyServices)
	if err != nil {
		t.Fatalf("ListSettingsByKind failed: %v", err)
	}

	got := make([]string, 0, 4)
	targetCodes := map[string]bool{
		seeCode:      true,
		imagekitCode: true,
		s3Code:       true,
		generalCode:  true,
	}
	for _, item := range list {
		if targetCodes[item.Code] {
			got = append(got, item.Code)
		}
	}

	want := []string{generalCode, seeCode, imagekitCode, s3Code}
	if len(got) != len(want) {
		t.Fatalf("unexpected matched settings length: got=%d want=%d codes=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected sub kind order at index=%d got=%s want=%s full=%v", i, got[i], want[i], got)
		}
	}
}

func TestEnsureDefaultSettingsSyncKind(t *testing.T) {
	db := openTestDB(t)

	siteURL, err := GetSettingByCode(db, "site_url")
	if err != nil {
		t.Fatalf("GetSettingByCode(site_url) failed: %v", err)
	}

	if err := Update(db, specSettings, siteURL.ID, map[string]interface{}{
		"kind": "LegacyKind",
	}); err != nil {
		t.Fatalf("Update setting metadata failed: %v", err)
	}

	if err := EnsureDefaultSettings(db); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}

	updated, err := GetSettingByCode(db, "site_url")
	if err != nil {
		t.Fatalf("GetSettingByCode(site_url) after ensure failed: %v", err)
	}
	if updated.Kind != SettingKindSiteBasics {
		t.Fatalf("expected site_url kind %s, got %s", SettingKindSiteBasics, updated.Kind)
	}
}

func TestEnsureDefaultSettingsSyncSubKind(t *testing.T) {
	db := openTestDB(t)

	s3Endpoint, err := GetSettingByCode(db, "s3_api_endpoint")
	if err != nil {
		t.Fatalf("GetSettingByCode(s3_api_endpoint) failed: %v", err)
	}

	if err := Update(db, specSettings, s3Endpoint.ID, map[string]interface{}{
		"sub_kind": "legacy_sub_kind",
	}); err != nil {
		t.Fatalf("Update setting metadata failed: %v", err)
	}

	if err := EnsureDefaultSettings(db); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}

	updated, err := GetSettingByCode(db, "s3_api_endpoint")
	if err != nil {
		t.Fatalf("GetSettingByCode(s3_api_endpoint) after ensure failed: %v", err)
	}
	if updated.SubKind != SettingSubKindS3 {
		t.Fatalf("expected s3_api_endpoint sub_kind %s, got %s", SettingSubKindS3, updated.SubKind)
	}
}

func TestBuiltinPrefixFieldSettings(t *testing.T) {
	db := openTestDB(t)

	basePath, err := GetSettingByCode(db, "base_path")
	if err != nil {
		t.Fatalf("GetSettingByCode(base_path) failed: %v", err)
	}
	if basePath.Type != "prefix-field" {
		t.Fatalf("expected base_path type=prefix-field, got %s", basePath.Type)
	}
	if basePath.PrefixValue != "/" {
		t.Fatalf("expected base_path prefix_value=/, got %s", basePath.PrefixValue)
	}

	prefixCodes := []string{
		"page_url_prefix",
		"rss_path",
		"post_url_prefix",
		"category_url_prefix",
		"tag_url_prefix",
	}
	for _, code := range prefixCodes {
		setting, err := GetSettingByCode(db, code)
		if err != nil {
			t.Fatalf("GetSettingByCode(%s) failed: %v", code, err)
		}
		if setting.Type != "prefix-field" {
			t.Fatalf("expected %s type=prefix-field, got %s", code, setting.Type)
		}
		if setting.PrefixValue != "/" {
			t.Fatalf("expected %s prefix_value=/, got %s", code, setting.PrefixValue)
		}
	}
}

func TestSettingPrefixValueLifecycle(t *testing.T) {
	db := openTestDB(t)

	code := uniqueValue("prefix_setting")
	s := &Setting{
		Code:        code,
		Kind:        "Custom",
		Name:        "Prefix Setting",
		Type:        "prefix-field",
		Value:       "/posts",
		PrefixValue: "全局路径前缀",
	}
	if _, err := CreateSetting(db, s); err != nil {
		t.Fatalf("CreateSetting failed: %v", err)
	}

	got, err := GetSettingByCode(db, code)
	if err != nil {
		t.Fatalf("GetSettingByCode failed: %v", err)
	}
	if got.PrefixValue != "全局路径前缀" {
		t.Fatalf("expected prefix value 全局路径前缀, got %s", got.PrefixValue)
	}

	got.PrefixValue = "base_path"
	if err := UpdateSetting(db, got); err != nil {
		t.Fatalf("UpdateSetting failed: %v", err)
	}

	got2, err := GetSettingByCode(db, code)
	if err != nil {
		t.Fatalf("GetSettingByCode after update failed: %v", err)
	}
	if got2.PrefixValue != "base_path" {
		t.Fatalf("expected prefix value base_path after update, got %s", got2.PrefixValue)
	}
}

func TestEnsureDefaultCategories(t *testing.T) {
	db := openTestDB(t)
	type expectedCategory struct {
		name        string
		slug        string
		parentSlug  string
		description string
		sort        int64
	}

	expected := []expectedCategory{
		{name: "生活", slug: "life", description: "与技术主线无直接关系的个人生活内容。", sort: 1},
		{name: "文娱", slug: "entertainment", parentSlug: "life", description: "音乐、电影、剧集、游戏及相关文化内容。", sort: 2},
		{name: "阅读", slug: "reading", parentSlug: "life", description: "读书笔记、文学随笔与阅读思考。", sort: 3},
		{name: "工作", slug: "work", description: "与职业实践、团队协作、工作方式相关内容。", sort: 4},
		{name: "职业", slug: "career", parentSlug: "work", description: "职业成长、管理协作、流程方法与职场经验。", sort: 5},
		{name: "技术", slug: "technology", description: "技术内容总入口，涵盖编程与软件工程实践。", sort: 6},
		{name: "编程", slug: "programming", parentSlug: "technology", description: "代码实现、底层原理与工程技巧。", sort: 7},
		{name: "编程语言", slug: "programming-languages", parentSlug: "programming", description: "语言特性、范式对比与生态实践。", sort: 8},
		{name: "操作系统", slug: "operating-systems", parentSlug: "programming", description: "Linux、macOS、Windows 与进程、内存、IO 等系统机制。", sort: 9},
		{name: "工具与效率", slug: "tools-productivity", parentSlug: "programming", description: "IDE、CLI、自动化与开发效率优化。", sort: 10},
		{name: "软件开发", slug: "software-development", parentSlug: "technology", description: "从需求到上线的架构、测试、发布与维护实践。", sort: 11},
		{name: "技术观点", slug: "tech-opinions", parentSlug: "technology", description: "技术趋势、行业观察与观点评论。", sort: 12},
		{name: "科技", slug: "tech", description: "消费科技与新品体验内容总入口。", sort: 13},
		{name: "发布与动态", slug: "tech-news", parentSlug: "tech", description: "发布会、新品发布与科技行业动态。", sort: 14},
		{name: "产品体验", slug: "product-hands-on", parentSlug: "tech", description: "设备开箱、上手评测与长期使用体验。", sort: 15},
		{name: "选购建议", slug: "buying-guides", parentSlug: "tech", description: "产品对比、选购建议与购买避坑。", sort: 16},
	}

	categories, err := ListCategories(db, false)
	if err != nil {
		t.Fatalf("ListCategories failed: %v", err)
	}
	if len(categories) != len(expected) {
		t.Fatalf("expected %d default categories, got %d", len(expected), len(categories))
	}
	for i, c := range categories {
		wantSort := int64(i + 1)
		if c.Sort != wantSort {
			t.Fatalf("category list order mismatch at index %d: got sort=%d want sort=%d", i, c.Sort, wantSort)
		}
	}

	bySlug := make(map[string]Category, len(categories))
	for _, c := range categories {
		bySlug[c.Slug] = c
	}

	for _, seed := range expected {
		c, ok := bySlug[seed.slug]
		if !ok {
			t.Fatalf("default category missing: %s", seed.slug)
		}
		if c.Name != seed.name {
			t.Fatalf("name mismatch for %s: got %s want %s", seed.slug, c.Name, seed.name)
		}
		if c.Description != seed.description {
			t.Fatalf("description mismatch for %s: got %s want %s", seed.slug, c.Description, seed.description)
		}
		if c.Sort != seed.sort {
			t.Fatalf("sort mismatch for %s: got %d want %d", seed.slug, c.Sort, seed.sort)
		}
		if seed.parentSlug == "" {
			if c.ParentID != 0 {
				t.Fatalf("root category %s parent should be 0, got %d", seed.slug, c.ParentID)
			}
			continue
		}
		parent, ok := bySlug[seed.parentSlug]
		if !ok {
			t.Fatalf("parent missing for %s: %s", seed.slug, seed.parentSlug)
		}
		if c.ParentID != parent.ID {
			t.Fatalf("parent mismatch for %s: got %d want %d", seed.slug, c.ParentID, parent.ID)
		}
	}

}

func TestHttpErrorLogLifecycle(t *testing.T) {
	db := openTestDB(t)

	logItem := &HttpErrorLog{
		ReqID:     uniqueValue("req"),
		ClientIP:  "127.0.0.1",
		Method:    "GET",
		Path:      "/err",
		Status:    500,
		UserAgent: "ua",
	}
	if _, err := CreateHttpErrorLog(db, logItem); err != nil {
		t.Fatalf("CreateHttpErrorLog failed: %v", err)
	}
	if logItem.CreatedAt == 0 || logItem.ExpiredAt != logItem.CreatedAt+7*24*60*60 {
		t.Fatalf("unexpected created/expired timestamps: %+v", logItem)
	}

	logs, err := ListHttpErrorLogs(db, 10, 0)
	if err != nil {
		t.Fatalf("ListHttpErrorLogs failed: %v", err)
	}
	if len(logs) == 0 {
		t.Fatal("expected logs")
	}

	before, err := CountHttpErrorLogs(db)
	if err != nil {
		t.Fatalf("CountHttpErrorLogs failed: %v", err)
	}
	if err := DeleteHttpErrorLog(db, logItem.ID); err != nil {
		t.Fatalf("DeleteHttpErrorLog failed: %v", err)
	}
	after, err := CountHttpErrorLogs(db)
	if err != nil {
		t.Fatalf("CountHttpErrorLogs failed: %v", err)
	}
	if after != before-1 {
		t.Fatalf("count should decrease by 1, before=%d after=%d", before, after)
	}
}

func TestTaskAndTaskRunLifecycle(t *testing.T) {
	db := openTestDB(t)

	task := &Task{
		Code:        uniqueValue("task"),
		Name:        "task",
		Description: "desc",
		Schedule:    "@every 1m",
		Enabled:     1,
		Kind:        TaskUser,
	}
	if _, err := CreateTask(db, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}
	if task.ID == 0 {
		t.Fatal("task id should be set")
	}

	if _, err := GetTaskByID(db, task.ID); err != nil {
		t.Fatalf("GetTaskByID failed: %v", err)
	}
	if _, err := GetTaskByCode(db, task.Code); err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}

	task.Name = "task-updated"
	task.Enabled = 0
	if err := UpdateTask(db, task); err != nil {
		t.Fatalf("UpdateTask failed: %v", err)
	}

	runAt := time.Now().Unix()
	if err := UpdateTaskStatus(db, task.Code, "success", runAt); err != nil {
		t.Fatalf("UpdateTaskStatus failed: %v", err)
	}
	if err := UpdateTaskStatus(db, uniqueValue("missing-task"), "ok", runAt); err != nil {
		t.Fatalf("UpdateTaskStatus on missing task should not fail: %v", err)
	}

	refreshed, err := GetTaskByCode(db, task.Code)
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if refreshed.LastStatus != "success" || refreshed.LastRunAt == nil || *refreshed.LastRunAt != runAt {
		t.Fatalf("unexpected task status fields: %+v", refreshed)
	}

	run := &TaskRun{
		TaskCode:   task.Code,
		Status:     "running",
		Message:    "start",
		StartedAt:  runAt,
		FinishedAt: runAt,
		Duration:   0,
	}
	if _, err := CreateTaskRun(db, run); err != nil {
		t.Fatalf("CreateTaskRun failed: %v", err)
	}
	if run.ID == 0 {
		t.Fatalf("run id field not set: %+v", run)
	}

	runs, err := ListTaskRuns(db, task.Code, "", 10)
	if err != nil {
		t.Fatalf("ListTaskRuns failed: %v", err)
	}
	if len(runs) == 0 {
		t.Fatal("expected task runs")
	}

	run.Status = "success"
	run.Message = "done"
	run.FinishedAt = runAt + 5
	run.Duration = 5
	if err := UpdateTaskRunStatus(db, run); err != nil {
		t.Fatalf("UpdateTaskRunStatus failed: %v", err)
	}
	updatedTask, err := GetTaskByCode(db, task.Code)
	if err != nil {
		t.Fatalf("GetTaskByCode failed: %v", err)
	}
	if updatedTask.LastStatus != "success" {
		t.Fatalf("expected task last status success, got %s", updatedTask.LastStatus)
	}

	if err := SoftDeleteTask(db, task.ID); err != nil {
		t.Fatalf("SoftDeleteTask failed: %v", err)
	}
	if _, err := GetTaskByID(db, task.ID); !IsErrNotFound(err) {
		t.Fatalf("expected not found after soft delete, got %v", err)
	}
}

func TestGetPostBySlugWithRelation(t *testing.T) {
	db := openTestDB(t)
	p := mustCreatePost(t, db, "published", PostKindPost, 0)
	tag := mustCreateTag(t, db, uniqueValue("tag-rel"))
	cat := mustCreateCategory(t, db, 0, uniqueValue("cat-rel"))
	if err := AttachTagToPost(db, p.ID, tag.ID); err != nil {
		t.Fatalf("AttachTagToPost failed: %v", err)
	}
	if err := SetPostCategory(db, p.ID, cat.ID); err != nil {
		t.Fatalf("SetPostCategory failed: %v", err)
	}

	got, err := GetPostBySlugWithRelation(db, p.Slug)
	if err != nil {
		t.Fatalf("GetPostBySlugWithRelation failed: %v", err)
	}
	if got.Post == nil || got.Post.ID != p.ID {
		t.Fatalf("unexpected post: %+v", got.Post)
	}
	if len(got.Tags) != 1 || got.Tags[0].ID != tag.ID {
		t.Fatalf("unexpected tags: %+v", got.Tags)
	}
	if got.Category == nil || got.Category.ID != cat.ID {
		t.Fatalf("unexpected category: %+v", got.Category)
	}
}

func TestExportSQLiteWithHash(t *testing.T) {
	db := openTestDB(t)

	exportDir := t.TempDir()
	res, err := ExportSQLiteWithHash(db, exportDir)
	if err != nil {
		t.Fatalf("ExportSQLiteWithHash failed: %v", err)
	}
	if res == nil || len(res.Hash) != 64 {
		t.Fatalf("unexpected export result: %+v", res)
	}
	if _, err := os.Stat(res.File); err != nil {
		t.Fatalf("export file not found: %v", err)
	}

	_, err = ExportSQLiteWithHash(db, exportDir)
	if err == nil {
		t.Fatal("expected no-change export error")
	}
	if !strings.Contains(err.Error(), "无需重复导出") {
		t.Fatalf("unexpected second export error: %v", err)
	}
}

func mustInsertUVRow(t *testing.T, db *DB, entityType UVEntityType, entityID int64, visitorID string, ts int64) {
	t.Helper()
	if visitorID == "" {
		t.Fatal("visitorID is required")
	}
	if _, err := db.Exec(
		`INSERT INTO `+string(TableUVUnique)+` (entity_type, entity_id, visitor_id, first_seen_at, last_seen_at) VALUES (?, ?, ?, ?, ?)`,
		entityType, entityID, []byte(visitorID), ts, ts,
	); err != nil {
		t.Fatalf("insert uv row failed: %v", err)
	}
}

func testVisitorID(seed byte) string {
	raw := make([]byte, UVVisitorIDBytes)
	for i := range raw {
		raw[i] = seed + byte(i)
	}
	return base64.RawURLEncoding.EncodeToString(raw)
}

func TestCountDistinctVisitorsBetween(t *testing.T) {
	db := openTestDB(t)

	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-a", 100)
	mustInsertUVRow(t, db, UVEntityPost, 101, "visitor-a", 100)
	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-b", 150)
	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-c", 250)

	count, err := CountDistinctVisitorsBetween(db, 0, 200)
	if err != nil {
		t.Fatalf("CountDistinctVisitorsBetween failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 distinct visitors in [0,200), got %d", count)
	}

	count, err = CountDistinctVisitorsBetween(db, 200, 300)
	if err != nil {
		t.Fatalf("CountDistinctVisitorsBetween failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 distinct visitor in [200,300), got %d", count)
	}
}

func TestListDistinctVisitorsByBucket(t *testing.T) {
	db := openTestDB(t)

	start := int64(100)
	end := int64(280)
	bucket := int64(60)

	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-a", 110)
	mustInsertUVRow(t, db, UVEntityPost, 201, "visitor-a", 115) // same visitor, same bucket
	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-b", 170)
	mustInsertUVRow(t, db, UVEntityCategory, 301, "visitor-c", 170)
	mustInsertUVRow(t, db, UVEntityTag, 401, "visitor-b", 190) // duplicate visitor in bucket 1
	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-d", 230)

	buckets, err := ListDistinctVisitorsByBucket(db, start, end, bucket)
	if err != nil {
		t.Fatalf("ListDistinctVisitorsByBucket failed: %v", err)
	}
	if len(buckets) != 3 {
		t.Fatalf("expected 3 buckets, got %d: %+v", len(buckets), buckets)
	}

	expected := map[int]int{
		0: 1,
		1: 2,
		2: 1,
	}

	for _, item := range buckets {
		if got := expected[item.BucketIndex]; got != item.UV {
			t.Fatalf("bucket %d expected uv=%d got=%d", item.BucketIndex, got, item.UV)
		}
	}
}

func TestCountActiveVisitors(t *testing.T) {
	db := openTestDB(t)

	nowTs := time.Now().Unix()
	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-a", nowTs-10*60)
	mustInsertUVRow(t, db, UVEntityPost, 100, "visitor-a", nowTs-5*60)
	mustInsertUVRow(t, db, UVEntitySite, 0, "visitor-b", nowTs-40*60)

	count30m, err := CountActiveVisitors(db, 30*60)
	if err != nil {
		t.Fatalf("CountActiveVisitors 30m failed: %v", err)
	}
	if count30m != 1 {
		t.Fatalf("expected 1 active visitor in 30m, got %d", count30m)
	}

	count60m, err := CountActiveVisitors(db, 60*60)
	if err != nil {
		t.Fatalf("CountActiveVisitors 60m failed: %v", err)
	}
	if count60m != 2 {
		t.Fatalf("expected 2 active visitors in 60m, got %d", count60m)
	}

	countDefault, err := CountActiveVisitors(db, 0)
	if err != nil {
		t.Fatalf("CountActiveVisitors default failed: %v", err)
	}
	if countDefault != 2 {
		t.Fatalf("expected 2 active visitors with default window, got %d", countDefault)
	}
}

func TestListTopUVContents(t *testing.T) {
	db := openTestDB(t)

	post := mustCreatePost(t, db, "published", PostKindPost, time.Now().Unix()-300)
	page := mustCreatePost(t, db, "published", PostKindPage, time.Now().Unix()-200)
	draft := mustCreatePost(t, db, "draft", PostKindPost, 0)
	deleted := mustCreatePost(t, db, "published", PostKindPost, time.Now().Unix()-100)
	if err := SoftDeletePost(db, deleted.ID); err != nil {
		t.Fatalf("SoftDeletePost failed: %v", err)
	}

	mustInsertUVRow(t, db, UVEntityPost, post.ID, "uv-post-1", 100)
	mustInsertUVRow(t, db, UVEntityPost, post.ID, "uv-post-2", 110)
	mustInsertUVRow(t, db, UVEntityPost, post.ID, "uv-post-3", 120)

	mustInsertUVRow(t, db, UVEntityPost, page.ID, "uv-page-1", 130)
	mustInsertUVRow(t, db, UVEntityPost, page.ID, "uv-page-2", 140)

	mustInsertUVRow(t, db, UVEntityPost, draft.ID, "uv-draft-1", 150)
	mustInsertUVRow(t, db, UVEntityPost, draft.ID, "uv-draft-2", 160)
	mustInsertUVRow(t, db, UVEntityPost, deleted.ID, "uv-deleted-1", 170)

	rows, err := ListTopUVContents(db, 10)
	if err != nil {
		t.Fatalf("ListTopUVContents failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected 2 ranked contents, got %d: %+v", len(rows), rows)
	}

	if rows[0].ID != post.ID || rows[0].UV != 3 || rows[0].Kind != PostKindPost || rows[0].PublishedAt != post.PublishedAt {
		t.Fatalf("unexpected top uv content row 0: %+v", rows[0])
	}
	if rows[1].ID != page.ID || rows[1].UV != 2 || rows[1].Kind != PostKindPage || rows[1].PublishedAt != page.PublishedAt {
		t.Fatalf("unexpected top uv content row 1: %+v", rows[1])
	}
}

func TestLikeStateAndTopLikedContents(t *testing.T) {
	db := openTestDB(t)

	post := mustCreatePost(t, db, "published", PostKindPost, time.Now().Unix()-300)
	page := mustCreatePost(t, db, "published", PostKindPage, time.Now().Unix()-200)
	draft := mustCreatePost(t, db, "draft", PostKindPost, 0)

	v1 := testVisitorID(1)
	v2 := testVisitorID(2)
	v3 := testVisitorID(3)
	v4 := testVisitorID(4)
	v5 := testVisitorID(5)

	if err := UpsertEntityLike(db, post.ID, v1, LikeStatusActive); err != nil {
		t.Fatalf("UpsertEntityLike post v1 failed: %v", err)
	}
	if err := UpsertEntityLike(db, post.ID, v2, LikeStatusActive); err != nil {
		t.Fatalf("UpsertEntityLike post v2 failed: %v", err)
	}
	if err := UpsertEntityLike(db, page.ID, v3, LikeStatusActive); err != nil {
		t.Fatalf("UpsertEntityLike page v3 failed: %v", err)
	}
	if err := UpsertEntityLike(db, page.ID, v4, LikeStatusInactive); err != nil {
		t.Fatalf("UpsertEntityLike page v4 inactive failed: %v", err)
	}
	if err := UpsertEntityLike(db, draft.ID, v5, LikeStatusActive); err != nil {
		t.Fatalf("UpsertEntityLike draft v5 failed: %v", err)
	}

	postLikes, err := CountEntityLikes(db, post.ID)
	if err != nil {
		t.Fatalf("CountEntityLikes post failed: %v", err)
	}
	if postLikes != 2 {
		t.Fatalf("expected post likes 2, got %d", postLikes)
	}

	pageLikes, err := CountEntityLikes(db, page.ID)
	if err != nil {
		t.Fatalf("CountEntityLikes page failed: %v", err)
	}
	if pageLikes != 1 {
		t.Fatalf("expected page likes 1, got %d", pageLikes)
	}

	liked, err := IsEntityLikedByVisitor(db, post.ID, v1)
	if err != nil {
		t.Fatalf("IsEntityLikedByVisitor post v1 failed: %v", err)
	}
	if !liked {
		t.Fatal("expected post v1 liked=true")
	}

	liked, err = IsEntityLikedByVisitor(db, page.ID, v4)
	if err != nil {
		t.Fatalf("IsEntityLikedByVisitor page v4 failed: %v", err)
	}
	if liked {
		t.Fatal("expected page v4 liked=false for inactive status")
	}

	totalLikes, err := CountTotalLikes(db)
	if err != nil {
		t.Fatalf("CountTotalLikes failed: %v", err)
	}
	if totalLikes != 4 {
		t.Fatalf("expected total likes 4, got %d", totalLikes)
	}

	topLiked, err := ListTopLikedContents(db, 10)
	if err != nil {
		t.Fatalf("ListTopLikedContents failed: %v", err)
	}
	if len(topLiked) != 2 {
		t.Fatalf("expected 2 ranked liked contents, got %d: %+v", len(topLiked), topLiked)
	}

	if topLiked[0].PostID != post.ID || topLiked[0].Likes != 2 || topLiked[0].Kind != PostKindPost {
		t.Fatalf("unexpected top liked row 0: %+v", topLiked[0])
	}
	if topLiked[1].PostID != page.ID || topLiked[1].Likes != 1 || topLiked[1].Kind != PostKindPage {
		t.Fatalf("unexpected top liked row 1: %+v", topLiked[1])
	}

	for _, row := range topLiked {
		if row.PostID == draft.ID {
			t.Fatalf("draft should not appear in top liked contents: %+v", row)
		}
	}

	if err := UpsertEntityLike(db, post.ID, "bad_visitor", LikeStatusActive); err == nil {
		t.Fatal("expected invalid visitor id error")
	}
}
