package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeThemeTransferPayloadSupportsMapAndStringFiles(t *testing.T) {
	payload, err := decodeThemeTransferPayload([]byte(`{
"name": "demo",
"code": "demo",
"files": {
"themes/demo/home.html": "home",
"site/include/nav.html": "nav"
}
}`))
	if err != nil {
		t.Fatalf("decodeThemeTransferPayload(map) failed: %v", err)
	}
	if payload.Files["themes/demo/home.html"] != "home" {
		t.Fatalf("unexpected map payload files: %+v", payload.Files)
	}

	payload, err = decodeThemeTransferPayload([]byte(`{
"name": "demo",
"code": "demo",
"files": "{\"themes/demo/home.html\":\"home\",\"site/include/nav.html\":\"nav\"}"
}`))
	if err != nil {
		t.Fatalf("decodeThemeTransferPayload(string) failed: %v", err)
	}
	if payload.Files["site/include/nav.html"] != "nav" {
		t.Fatalf("unexpected string payload files: %+v", payload.Files)
	}
}

func TestNormalizeExtractFilePath(t *testing.T) {
	cases := []struct {
		raw  string
		want string
	}{
		{raw: "home.html", want: "home.html"},
		{raw: "themes/demo/home.html", want: "home.html"},
		{raw: "site/home.html", want: "home.html"},
		{raw: "site/include/nav.html", want: "include/nav.html"},
	}

	for _, tc := range cases {
		got, err := normalizeExtractFilePath(tc.raw)
		if err != nil {
			t.Fatalf("normalizeExtractFilePath(%q) failed: %v", tc.raw, err)
		}
		if got != tc.want {
			t.Fatalf("normalizeExtractFilePath(%q) = %q, want %q", tc.raw, got, tc.want)
		}
	}
}

func TestExtractThemeFilesWritesAndOverwrites(t *testing.T) {
	root := t.TempDir()
	themeCode := "demo"
	themeRoot := filepath.Join(root, themeCode)
	if err := os.MkdirAll(filepath.Join(themeRoot, "include"), 0o755); err != nil {
		t.Fatalf("mkdir include failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(themeRoot, "home.html"), []byte("old-home"), 0o644); err != nil {
		t.Fatalf("write existing home failed: %v", err)
	}

	files := map[string]string{
		"themes/demo/home.html":        "new-home",
		"themes/demo/include/nav.html": "new-nav",
		"themes/demo/asset.css":        "body{}",
	}
	var warn bytes.Buffer
	written, skipped, err := extractThemeFiles(files, root, themeCode, &warn)
	if err != nil {
		t.Fatalf("extractThemeFiles failed: %v", err)
	}
	if written != 2 || skipped != 1 {
		t.Fatalf("written/skipped = %d/%d, want 2/1", written, skipped)
	}
	if warn.Len() == 0 {
		t.Fatal("expected warning for non-html file")
	}

	home, err := os.ReadFile(filepath.Join(themeRoot, "home.html"))
	if err != nil {
		t.Fatalf("read home.html failed: %v", err)
	}
	if string(home) != "new-home" {
		t.Fatalf("home.html = %q, want new-home", string(home))
	}
	nav, err := os.ReadFile(filepath.Join(themeRoot, "include", "nav.html"))
	if err != nil {
		t.Fatalf("read include/nav.html failed: %v", err)
	}
	if string(nav) != "new-nav" {
		t.Fatalf("include/nav.html = %q, want new-nav", string(nav))
	}
}

func TestExtractThemeFilesRejectsPathTraversal(t *testing.T) {
	root := t.TempDir()
	_, _, err := extractThemeFiles(map[string]string{
		"themes/demo/../evil.html": "oops",
	}, root, "demo", nil)
	if err == nil {
		t.Fatal("expected path traversal to be rejected")
	}

	_, _, err = extractThemeFiles(map[string]string{
		"site/include/../../evil.html": "oops",
	}, root, "demo", nil)
	if err == nil {
		t.Fatal("expected path traversal with parent segments to be rejected")
	}
}

func TestResolveThemeCodeUsesCodeAndFallsBackToName(t *testing.T) {
	code, err := resolveThemeCode(&themeTransferPayload{Code: "demo", Name: "name"})
	if err != nil {
		t.Fatalf("resolveThemeCode(code) failed: %v", err)
	}
	if code != "demo" {
		t.Fatalf("resolveThemeCode(code) = %q, want demo", code)
	}

	code, err = resolveThemeCode(&themeTransferPayload{Name: "demo-from-name"})
	if err != nil {
		t.Fatalf("resolveThemeCode(name fallback) failed: %v", err)
	}
	if code != "demo-from-name" {
		t.Fatalf("resolveThemeCode(name fallback) = %q, want demo-from-name", code)
	}

	if _, err := resolveThemeCode(&themeTransferPayload{Code: "../bad"}); err == nil {
		t.Fatal("expected invalid theme code to fail")
	}
}
