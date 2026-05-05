package store

import (
	"path/filepath"
	"strings"
	"testing"

	"swaves/internal/platform/db"
)

func TestReloadRedirectsReturnsErrorOnInvalidPatternRule(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "redirects.sqlite")})
	t.Cleanup(func() {
		_ = dbx.Close()
	})

	if _, err := db.CreateRedirect(dbx, &db.Redirect{
		From:    "/posts/{slug",
		To:      "/archive",
		Status:  301,
		Enabled: 1,
	}); err != nil {
		t.Fatalf("create redirect failed: %v", err)
	}

	err := ReloadRedirects(&GlobalStore{Model: dbx})
	if err == nil {
		t.Fatal("expected invalid redirect rule error")
	}
	if !strings.Contains(err.Error(), "/posts/{slug") {
		t.Fatalf("unexpected error: %v", err)
	}
}
