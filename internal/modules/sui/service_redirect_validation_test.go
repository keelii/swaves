package sui

import (
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/shared/share"
)

func newRedirectValidationTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "redirect-validation.sqlite")
	dbx := db.Open(db.Options{DSN: dbPath})
	t.Cleanup(func() {
		_ = dbx.Close()
	})

	if err := store.ReloadSettings(&store.GlobalStore{Model: dbx}); err != nil {
		t.Fatalf("reload settings failed: %v", err)
	}

	return dbx
}

func TestCreateRedirectServiceRejectsPublishedPostURLAsSource(t *testing.T) {
	dbx := newRedirectValidationTestDB(t)

	post := &db.Post{
		Title:       "Hello World",
		Slug:        "hello-world",
		Status:      "published",
		Kind:        db.PostKindPost,
		PublishedAt: 1735603200,
	}
	if _, err := db.CreatePost(dbx, post); err != nil {
		t.Fatalf("create published post failed: %v", err)
	}

	conflictPath := share.GetPostUrl(*post)
	err := CreateRedirectService(dbx, CreateRedirectInput{
		From:    conflictPath,
		To:      "/new-home",
		Status:  301,
		Enabled: 1,
	})
	if err == nil {
		t.Fatalf("expected conflict error when from path matches published post url")
	}
	if !strings.Contains(err.Error(), "来源路径与已发布内容地址冲突") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCreateRedirectServiceAllowsNonPostSourcePath(t *testing.T) {
	dbx := newRedirectValidationTestDB(t)

	post := &db.Post{
		Title:       "About Page",
		Slug:        "about",
		Status:      "published",
		Kind:        db.PostKindPage,
		PublishedAt: 1735603200,
	}
	if _, err := db.CreatePost(dbx, post); err != nil {
		t.Fatalf("create published page failed: %v", err)
	}

	err := CreateRedirectService(dbx, CreateRedirectInput{
		From:    "/old-about",
		To:      share.GetPostUrl(*post),
		Status:  302,
		Enabled: 1,
	})
	if err != nil {
		t.Fatalf("create redirect should succeed for non-conflicting from path: %v", err)
	}

	redirect, err := db.GetRedirectByFrom(dbx, "/old-about")
	if err != nil {
		t.Fatalf("expected redirect to be created: %v", err)
	}
	if redirect.To != share.GetPostUrl(*post) {
		t.Fatalf("unexpected redirect target: got %q want %q", redirect.To, share.GetPostUrl(*post))
	}
}

func TestCreateRedirectServiceRejectsPostSlugAsSource(t *testing.T) {
	dbx := newRedirectValidationTestDB(t)

	post := &db.Post{
		Title:  "Draft Route",
		Slug:   "draft-route",
		Status: "draft",
		Kind:   db.PostKindPost,
	}
	if _, err := db.CreatePost(dbx, post); err != nil {
		t.Fatalf("create draft post failed: %v", err)
	}

	err := CreateRedirectService(dbx, CreateRedirectInput{
		From:    "/draft-route",
		To:      "/new-draft-route",
		Status:  301,
		Enabled: 1,
	})
	if err == nil {
		t.Fatalf("expected conflict error when from path matches post slug")
	}
	if !strings.Contains(err.Error(), "来源路径与文章 slug 冲突") {
		t.Fatalf("unexpected error: %v", err)
	}
}
