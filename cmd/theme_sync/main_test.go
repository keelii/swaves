package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadLocalThemeFiles(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "home.html"), []byte("home"), 0o644); err != nil {
		t.Fatalf("write home.html failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "post.html"), []byte("post"), 0o644); err != nil {
		t.Fatalf("write post.html failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("skip"), 0o644); err != nil {
		t.Fatalf("write README.md failed: %v", err)
	}

	files, err := loadLocalThemeFiles(dir)
	if err != nil {
		t.Fatalf("loadLocalThemeFiles failed: %v", err)
	}
	if files["home.html"] != "home" {
		t.Fatalf("home.html = %q, want home", files["home.html"])
	}
	if files["post.html"] != "post" {
		t.Fatalf("post.html = %q, want post", files["post.html"])
	}
	if _, ok := files["README.md"]; ok {
		t.Fatal("expected non-html file to be ignored")
	}
}

func TestResolveThemeCurrentFile(t *testing.T) {
	files := map[string]string{
		"layout_main.html": "layout",
		"home.html":        "home",
		"post.html":        "post",
	}

	if got := resolveThemeCurrentFile(files, "post.html"); got != "post.html" {
		t.Fatalf("resolveThemeCurrentFile(existing) = %q, want post.html", got)
	}
	if got := resolveThemeCurrentFile(files, "missing.html"); got != "home.html" {
		t.Fatalf("resolveThemeCurrentFile(fallback home) = %q, want home.html", got)
	}

	delete(files, "home.html")
	if got := resolveThemeCurrentFile(files, ""); got != "layout_main.html" {
		t.Fatalf("resolveThemeCurrentFile(sorted fallback) = %q, want layout_main.html", got)
	}
}
