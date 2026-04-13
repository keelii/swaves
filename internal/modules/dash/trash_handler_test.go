package dash

import (
	"path/filepath"
	"testing"

	"swaves/internal/platform/db"
)

func TestGetTrashTabCounts(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "trash.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	post := &db.Post{
		Title:   "trash-post",
		Slug:    "trash-post",
		Content: "content",
		Status:  "published",
		Kind:    db.PostKindPost,
	}
	if _, err := db.CreatePost(dbx, post); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}
	if err := db.SoftDeletePost(dbx, post.ID); err != nil {
		t.Fatalf("SoftDeletePost failed: %v", err)
	}

	encryptedPost := &db.EncryptedPost{
		Title:    "trash-encrypted-post",
		Slug:     "trash-encrypted-post",
		Content:  "secret",
		Password: "password",
	}
	if _, err := db.CreateEncryptedPost(dbx, encryptedPost); err != nil {
		t.Fatalf("CreateEncryptedPost failed: %v", err)
	}
	if err := db.SoftDeleteEncryptedPost(dbx, encryptedPost.ID); err != nil {
		t.Fatalf("SoftDeleteEncryptedPost failed: %v", err)
	}

	tag := &db.Tag{Name: "trash-tag", Slug: "trash-tag"}
	if _, err := db.CreateTag(dbx, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}
	if err := db.SoftDeleteTag(dbx, tag.ID); err != nil {
		t.Fatalf("SoftDeleteTag failed: %v", err)
	}

	category := &db.Category{Name: "trash-category", Slug: "trash-category"}
	if _, err := db.CreateCategory(dbx, category); err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}
	if err := db.SoftDeleteCategory(dbx, category.ID); err != nil {
		t.Fatalf("SoftDeleteCategory failed: %v", err)
	}

	redirect := &db.Redirect{From: "/trash-from", To: "/trash-to", Enabled: 1}
	if _, err := db.CreateRedirect(dbx, redirect); err != nil {
		t.Fatalf("CreateRedirect failed: %v", err)
	}
	if err := db.SoftDeleteRedirect(dbx, redirect.ID); err != nil {
		t.Fatalf("SoftDeleteRedirect failed: %v", err)
	}

	tabCounts, err := getTrashTabCounts(&Handler{Model: dbx})
	if err != nil {
		t.Fatalf("getTrashTabCounts failed: %v", err)
	}

	want := map[string]int{
		"posts":           1,
		"encrypted-posts": 1,
		"tags":            1,
		"categories":      1,
		"redirects":       1,
	}
	for key, expected := range want {
		if got := tabCounts[key]; got != expected {
			t.Fatalf("tabCounts[%q] = %d, want %d", key, got, expected)
		}
	}
}

func TestGetRecordTabCounts(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "records.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	baseCounts, err := getRecordTabCounts(dbx)
	if err != nil {
		t.Fatalf("getRecordTabCounts baseline failed: %v", err)
	}

	category := &db.Category{Name: "record-category", Slug: "record-category"}
	if _, err := db.CreateCategory(dbx, category); err != nil {
		t.Fatalf("CreateCategory failed: %v", err)
	}

	tag := &db.Tag{Name: "record-tag", Slug: "record-tag"}
	if _, err := db.CreateTag(dbx, tag); err != nil {
		t.Fatalf("CreateTag failed: %v", err)
	}

	task := &db.Task{
		Code:        "record-task",
		Name:        "Record Task",
		Description: "record task description",
		Schedule:    "@every 1h",
		Enabled:     1,
		Kind:        db.TaskUser,
	}
	if _, err := db.CreateTask(dbx, task); err != nil {
		t.Fatalf("CreateTask failed: %v", err)
	}

	redirect := &db.Redirect{From: "/record-from", To: "/record-to", Enabled: 1}
	if _, err := db.CreateRedirect(dbx, redirect); err != nil {
		t.Fatalf("CreateRedirect failed: %v", err)
	}

	theme := &db.Theme{
		Name:        "record-theme",
		Code:        "record-theme",
		Description: "record theme",
		Author:      "tester",
		Files:       `{"site/home.html":"<h1>theme</h1>"}`,
		CurrentFile: "site/home.html",
		Status:      "draft",
		Version:     1,
	}
	if _, err := db.CreateTheme(dbx, theme); err != nil {
		t.Fatalf("CreateTheme failed: %v", err)
	}

	tabCounts, err := getRecordTabCounts(dbx)
	if err != nil {
		t.Fatalf("getRecordTabCounts failed: %v", err)
	}

	want := map[string]int{
		"categories": baseCounts["categories"] + 1,
		"tags":       baseCounts["tags"] + 1,
		"tasks":      baseCounts["tasks"] + 1,
		"redirects":  baseCounts["redirects"] + 1,
		"themes":     baseCounts["themes"] + 1,
	}
	for key, expected := range want {
		if got := tabCounts[key]; got != expected {
			t.Fatalf("tabCounts[%q] = %d, want %d", key, got, expected)
		}
	}
}

func TestGetCommentTabCounts(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "comments.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	post := &db.Post{
		Title:   "comment-post",
		Slug:    "comment-post",
		Content: "content",
		Status:  "published",
		Kind:    db.PostKindPost,
	}
	if _, err := db.CreatePost(dbx, post); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}

	comments := []db.Comment{
		{
			PostID:      post.ID,
			Author:      "Pending User",
			AuthorEmail: "pending@example.com",
			AuthorIP:    "127.0.0.1",
			Content:     "pending comment",
			Status:      db.CommentStatusPending,
		},
		{
			PostID:      post.ID,
			Author:      "Approved User",
			AuthorEmail: "approved@example.com",
			AuthorIP:    "127.0.0.2",
			Content:     "approved comment",
			Status:      db.CommentStatusApproved,
		},
		{
			PostID:      post.ID,
			Author:      "Spam User",
			AuthorEmail: "spam@example.com",
			AuthorIP:    "127.0.0.3",
			Content:     "spam comment",
			Status:      db.CommentStatusSpam,
		},
	}
	for _, item := range comments {
		comment := item
		if _, err := db.CreateComment(dbx, &comment); err != nil {
			t.Fatalf("CreateComment failed: %v", err)
		}
	}

	tabCounts, err := getCommentTabCounts(dbx)
	if err != nil {
		t.Fatalf("getCommentTabCounts failed: %v", err)
	}

	want := map[string]int{
		"all":      3,
		"pending":  1,
		"approved": 1,
		"spam":     1,
	}
	for key, expected := range want {
		if got := tabCounts[key]; got != expected {
			t.Fatalf("tabCounts[%q] = %d, want %d", key, got, expected)
		}
	}
}
