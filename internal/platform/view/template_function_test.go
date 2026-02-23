package view

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/shared/helper"
	"testing"
)

func TestMacroIncludeRendersNestedTemplate(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{% import "macros.html" as ui %}{{ ui.row({"Value": "ok"}) }}`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "macros.html"), []byte(`{% macro row(ctx) %}{% with Value = ctx.Value, __root = ctx %}{% include "partial.html" %}{% endwith %}{% endmacro %}`), 0o644); err != nil {
		t.Fatalf("write macro template failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "partial.html"), []byte(`{{ Value }} / {{ __root.Value }}`), 0o644); err != nil {
		t.Fatalf("write partial template failed: %v", err)
	}

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page", map[string]any{}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok / ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestGlobalMacroNamespaceAvailableAcrossExtendsAndInclude(t *testing.T) {
	tempDir := t.TempDir()
	mustWriteTemplate := func(relativeName string, source string) {
		templatePath := filepath.Join(tempDir, relativeName)
		if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
			t.Fatalf("create template directory failed: %v", err)
		}
		if err := os.WriteFile(templatePath, []byte(source), 0o644); err != nil {
			t.Fatalf("write template %q failed: %v", relativeName, err)
		}
	}

	mustWriteTemplate("admin/macro/ui.html", `{% macro hi(label) %}{{ label }}{% endmacro %}`)
	mustWriteTemplate("admin/layout/base.html", `{% block body %}{% endblock %}`)
	mustWriteTemplate("admin/layout/layout.html", `{% extends "admin/layout/base.html" %}{% block body %}{% include "admin/include/actions.html" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplate("admin/include/actions.html", `{{ ui.hi("A") }}`)
	mustWriteTemplate("page.html", `{% extends "admin/layout/layout.html" %}{% block content %}{{ ui.hi("B") }}{% endblock %}`)

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page", map[string]any{}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "AB" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestExtendsAndIncludeSupportRootPathsWithoutLeadingSlash(t *testing.T) {
	tempDir := t.TempDir()
	mustWriteTemplate := func(relativeName string, source string) {
		templatePath := filepath.Join(tempDir, relativeName)
		if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
			t.Fatalf("create template directory failed: %v", err)
		}
		if err := os.WriteFile(templatePath, []byte(source), 0o644); err != nil {
			t.Fatalf("write template %q failed: %v", relativeName, err)
		}
	}

	mustWriteTemplate("admin/layout/base.html", `{% block body %}{% endblock %}`)
	mustWriteTemplate("admin/include/actions.html", `A`)
	mustWriteTemplate("admin/layout/layout.html", `{% extends "admin/layout/base.html" %}{% block body %}{% include "admin/include/actions.html" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplate("admin/categories_index.html", `{% extends "admin/layout/layout.html" %}{% block content %}B{% endblock %}`)

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "admin/categories_index", map[string]any{}); err != nil {
		t.Fatalf("render template failed: %v", err)
	}
	if got := out.String(); got != "AB" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestIncludeSupportsExplicitRelativeSubPath(t *testing.T) {
	tempDir := t.TempDir()
	mustWriteTemplate := func(relativeName string, source string) {
		templatePath := filepath.Join(tempDir, relativeName)
		if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
			t.Fatalf("create template directory failed: %v", err)
		}
		if err := os.WriteFile(templatePath, []byte(source), 0o644); err != nil {
			t.Fatalf("write template %q failed: %v", relativeName, err)
		}
	}

	mustWriteTemplate("admin/page.html", `{% include "./include/item.html" %}`)
	mustWriteTemplate("admin/include/item.html", `ok`)

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "admin/page", map[string]any{}); err != nil {
		t.Fatalf("render template failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestMapLookupHandlesMissingAndNumericKeys(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ PostCounts[12] }}|{{ PostCounts[99] or 0 }}`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page", map[string]any{
		"PostCounts": map[int64]int{12: 5},
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "5|0" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestRenderInjectsCSRFTokenInputGlobal(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`<form method="post">{{ _csrf_token() }}</form>`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page", map[string]any{
		"_csrf_token_value": `token<&>"'`,
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `<input type="hidden" name="_csrf_token"`) {
		t.Fatalf("expected csrf input in rendered output: %q", got)
	}
	if !strings.Contains(got, `value="token&lt;&amp;&gt;&#34;&#39;"`) {
		t.Fatalf("expected escaped csrf token value in rendered output: %q", got)
	}
	if strings.Contains(got, "&lt;input") {
		t.Fatalf("expected csrf input to be rendered as safe html: %q", got)
	}
}

func TestURLForSupportsKeywordArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ UrlFor('admin.posts.edit', id=PostID, tab='comments') }}`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	var capturedName string
	var capturedParams map[string]string
	var capturedQuery map[string]string
	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		capturedName = name
		capturedParams = params
		capturedQuery = query
		return "ok"
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page", map[string]any{"PostID": 12}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
	if capturedName != "admin.posts.edit" {
		t.Fatalf("captured route name = %q", capturedName)
	}
	if capturedParams["id"] != "12" || capturedParams["tab"] != "comments" {
		t.Fatalf("captured params = %#v", capturedParams)
	}
	if len(capturedQuery) != 0 {
		t.Fatalf("captured query = %#v, want empty", capturedQuery)
	}
}

func TestURLForKeywordArgumentsOverrideMapValues(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ UrlFor('admin.posts.edit', {"id": "1", "tab": "draft"}, id=PostID, tab='comments') }}`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	var capturedParams map[string]string
	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		capturedParams = params
		return "ok"
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page", map[string]any{"PostID": 9}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if capturedParams["id"] != "9" || capturedParams["tab"] != "comments" {
		t.Fatalf("captured params = %#v", capturedParams)
	}
}

func TestDecodeAnyToTypeReadsPostStruct(t *testing.T) {
	post, ok := helper.DecodeAnyToType[db.Post](db.Post{
		ID:          12,
		Kind:        db.PostKindPage,
		Slug:        "about",
		Title:       "About",
		PublishedAt: 1700000000,
	})
	if !ok {
		t.Fatalf("DecodeAnyToType should decode post struct payload")
	}
	if post.ID != 12 {
		t.Fatalf("post ID = %d, want 12", post.ID)
	}
	if post.Kind != db.PostKindPage {
		t.Fatalf("post kind = %d, want %d", post.Kind, db.PostKindPage)
	}
	if post.Slug != "about" {
		t.Fatalf("post slug = %q, want %q", post.Slug, "about")
	}
}
