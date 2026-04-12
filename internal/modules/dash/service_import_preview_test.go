package dash

import (
	"path/filepath"
	"sort"
	"testing"
	"time"

	"swaves/internal/platform/db"
	"swaves/internal/shared/types"
)

func newImportPreviewTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "import-preview.sqlite")
	dbx := db.Open(db.Options{DSN: dbPath})
	t.Cleanup(func() {
		_ = dbx.Close()
	})
	return dbx
}

func mustStageImportingItem(t *testing.T, dbx *db.DB, item PreviewPostItem) PreviewPostItem {
	t.Helper()

	staged, err := ImportPreviewItemAsImportingService(dbx, item)
	if err != nil {
		t.Fatalf("stage importing item failed: %v", err)
	}
	return staged
}

func mustSaveImportingItem(t *testing.T, dbx *db.DB, item PreviewPostItem) PreviewPostItem {
	t.Helper()

	saved, err := SaveImportPreviewItemService(dbx, item)
	if err != nil {
		t.Fatalf("save importing item failed: %v", err)
	}
	return saved
}

func mustGetPostAnyStatus(t *testing.T, dbx *db.DB, postID int64) db.Post {
	t.Helper()

	post, err := db.GetPostByIDAnyStatus(dbx, postID)
	if err != nil {
		t.Fatalf("load post failed: %v", err)
	}
	return post
}

func tagNamesSorted(tags []db.Tag) []string {
	names := make([]string, 0, len(tags))
	for _, tag := range tags {
		names = append(names, tag.Name)
	}
	sort.Strings(names)
	return names
}

func TestSaveImportPreviewItemServicePersistsRowEdits(t *testing.T) {
	dbx := newImportPreviewTestDB(t)

	staged := mustStageImportingItem(t, dbx, PreviewPostItem{
		Title:      "Old Title",
		Slug:       "old-title",
		Content:    "old content",
		Status:     "draft",
		Kind:       "0",
		CreatedAt:  "2024-01-01 00:00:00",
		Tags:       "Old Tag",
		Category:   "Old Category",
		Categories: "Old Category",
	})

	targetCreatedAt := "2024-02-03 04:05:06"
	targetCreatedAtUnix, err := time.Parse("2006-01-02 15:04:05", targetCreatedAt)
	if err != nil {
		t.Fatalf("parse created_at failed: %v", err)
	}

	saved := mustSaveImportingItem(t, dbx, PreviewPostItem{
		PostID:     staged.PostID,
		Filename:   "demo.md",
		Title:      "  New Title  ",
		Slug:       "new-title",
		Content:    "new content",
		Status:     "published",
		Kind:       "1",
		CreatedAt:  targetCreatedAt,
		Tags:       "Tag A, Tag B",
		Category:   "Main Category",
		Categories: "Main Category, Extra Category",
	})

	if saved.Title != "New Title" {
		t.Fatalf("unexpected saved title: %q", saved.Title)
	}
	if saved.Slug != "new-title" {
		t.Fatalf("unexpected saved slug: %q", saved.Slug)
	}
	if saved.Status != "published" {
		t.Fatalf("unexpected saved status: %q", saved.Status)
	}
	if saved.Kind != "1" {
		t.Fatalf("unexpected saved kind: %q", saved.Kind)
	}

	post := mustGetPostAnyStatus(t, dbx, staged.PostID)
	if post.Status != importingPostStatus {
		t.Fatalf("post should stay in importing status after save, got %q", post.Status)
	}
	if post.Title != "New Title" {
		t.Fatalf("unexpected post title: %q", post.Title)
	}
	if post.Slug != "new-title" {
		t.Fatalf("unexpected post slug: %q", post.Slug)
	}
	if post.Kind != db.PostKindPage {
		t.Fatalf("unexpected post kind: %d", post.Kind)
	}
	if post.CreatedAt != targetCreatedAtUnix.Unix() {
		t.Fatalf("unexpected created_at: got=%d want=%d", post.CreatedAt, targetCreatedAtUnix.Unix())
	}
	if post.PublishedAt != targetCreatedAtUnix.Unix() {
		t.Fatalf("unexpected published_at: got=%d want=%d", post.PublishedAt, targetCreatedAtUnix.Unix())
	}

	tags, err := db.GetPostTags(dbx, staged.PostID)
	if err != nil {
		t.Fatalf("get post tags failed: %v", err)
	}
	names := tagNamesSorted(tags)
	if len(names) != 2 || names[0] != "Tag A" || names[1] != "Tag B" {
		t.Fatalf("unexpected tag names: %#v", names)
	}

	category, err := db.GetPostCategory(dbx, staged.PostID)
	if err != nil {
		t.Fatalf("get post category failed: %v", err)
	}
	if category == nil || category.Name != "Main Category" {
		t.Fatalf("unexpected category after save: %#v", category)
	}

	items, err := ListImportingPreviewItemsService(dbx, nil)
	if err != nil {
		t.Fatalf("list importing items failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("unexpected importing item count: %d", len(items))
	}
	if items[0].Status != "published" {
		t.Fatalf("unexpected importing preview status: %q", items[0].Status)
	}
	if items[0].Content != "" {
		t.Fatalf("expected importing list to omit full content")
	}
	if items[0].ContentPreview == "" {
		t.Fatal("expected importing list content preview")
	}
}

func TestConfirmAllImportingPreviewItemsServiceConfirmsAcrossPages(t *testing.T) {
	dbx := newImportPreviewTestDB(t)

	itemA := mustStageImportingItem(t, dbx, PreviewPostItem{Title: "Post A", Slug: "post-a", Content: "A", Status: "draft", Kind: "0", CreatedAt: "2024-01-01 00:00:00"})
	itemB := mustStageImportingItem(t, dbx, PreviewPostItem{Title: "Post B", Slug: "post-b", Content: "B", Status: "draft", Kind: "0", CreatedAt: "2024-01-02 00:00:00"})
	itemC := mustStageImportingItem(t, dbx, PreviewPostItem{Title: "Post C", Slug: "post-c", Content: "C", Status: "draft", Kind: "0", CreatedAt: "2024-01-03 00:00:00"})

	pager := &types.Pagination{Page: 1, PageSize: 1}
	pagedItems, err := ListImportingPreviewItemsService(dbx, pager)
	if err != nil {
		t.Fatalf("list paged importing items failed: %v", err)
	}
	if pager.Total != 3 {
		t.Fatalf("unexpected pager total: got=%d want=3", pager.Total)
	}
	if len(pagedItems) != 1 {
		t.Fatalf("unexpected paged result length: got=%d want=1", len(pagedItems))
	}

	mustSaveImportingItem(t, dbx, PreviewPostItem{
		PostID:    itemB.PostID,
		Title:     "Post B",
		Slug:      "post-b",
		Content:   "B updated",
		Status:    "published",
		Kind:      "0",
		CreatedAt: "2024-01-02 00:00:00",
	})

	result, err := ConfirmAllImportingPreviewItemsService(dbx)
	if err != nil {
		t.Fatalf("confirm all importing items failed: %v", err)
	}
	if result.Total != 3 {
		t.Fatalf("unexpected confirm total: got=%d want=3", result.Total)
	}
	if result.Success != 3 || result.Fail != 0 {
		t.Fatalf("unexpected confirm summary: success=%d fail=%d", result.Success, result.Fail)
	}

	remaining, err := ListImportingPreviewItemsService(dbx, nil)
	if err != nil {
		t.Fatalf("list remaining importing items failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected no importing items after confirm-all, got=%d", len(remaining))
	}

	postA := mustGetPostAnyStatus(t, dbx, itemA.PostID)
	if postA.Status != "draft" {
		t.Fatalf("unexpected post A status: %q", postA.Status)
	}

	postB := mustGetPostAnyStatus(t, dbx, itemB.PostID)
	if postB.Status != "published" {
		t.Fatalf("unexpected post B status: %q", postB.Status)
	}

	postC := mustGetPostAnyStatus(t, dbx, itemC.PostID)
	if postC.Status != "draft" {
		t.Fatalf("unexpected post C status: %q", postC.Status)
	}
}

func TestSaveImportPreviewItemServiceKeepsExistingContentWhenFormOmitsContent(t *testing.T) {
	dbx := newImportPreviewTestDB(t)

	staged := mustStageImportingItem(t, dbx, PreviewPostItem{
		Title:     "Keep Content",
		Slug:      "keep-content",
		Content:   "full body content",
		Status:    "draft",
		Kind:      "0",
		CreatedAt: "2024-01-01 00:00:00",
	})

	saved := mustSaveImportingItem(t, dbx, PreviewPostItem{
		PostID:    staged.PostID,
		Title:     "Keep Content Updated",
		Slug:      "keep-content-updated",
		Status:    "draft",
		Kind:      "0",
		CreatedAt: "2024-01-01 00:00:00",
	})
	if saved.Content != "full body content" {
		t.Fatalf("expected saved content to keep existing body, got %q", saved.Content)
	}

	post := mustGetPostAnyStatus(t, dbx, staged.PostID)
	if post.Content != "full body content" {
		t.Fatalf("expected post content unchanged, got %q", post.Content)
	}
}
