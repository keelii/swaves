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
	if err := view.Render(&out, "page.html", map[string]any{}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok / ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestMacroImportAvailableAcrossExtendsAndInclude(t *testing.T) {
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

	mustWriteTemplate("dash/layout/base.html", `{% block body %}{% endblock %}`)
	mustWriteTemplate("dash/include/ui.html", `{% macro hi(label) %}{{ label }}{% endmacro %}`)
	mustWriteTemplate("dash/layout/layout.html", `{% extends "dash/layout/base.html" %}{% import "dash/include/ui.html" as ui %}{% block body %}{% include "dash/include/actions.html" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplate("dash/include/actions.html", `{% import "dash/include/ui.html" as ui %}{{ ui.hi("A") }}`)
	mustWriteTemplate("page.html", `{% extends "dash/layout/layout.html" %}{% import "dash/include/ui.html" as ui %}{% block content %}{{ ui.hi("B") }}{% endblock %}`)

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page.html", map[string]any{}); err != nil {
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

	mustWriteTemplate("dash/layout/base.html", `{% block body %}{% endblock %}`)
	mustWriteTemplate("dash/include/actions.html", `A`)
	mustWriteTemplate("dash/layout/layout.html", `{% extends "dash/layout/base.html" %}{% block body %}{% include "dash/include/actions.html" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplate("dash/categories_index.html", `{% extends "dash/layout/layout.html" %}{% block content %}B{% endblock %}`)

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "dash/categories_index.html", map[string]any{}); err != nil {
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

	mustWriteTemplate("dash/page.html", `{% include "./include/item.html" %}`)
	mustWriteTemplate("dash/include/item.html", `ok`)

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return name
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "dash/page.html", map[string]any{}); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{
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
	if err := view.Render(&out, "page.html", map[string]any{
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

func TestSafeFilterRendersRawHTML(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ ContentHTML|safe }}`), 0o644); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{
		"ContentHTML": "<ol class=\"toc-list\"><li>x</li></ol>",
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, `<ol class="toc-list">`) {
		t.Fatalf("expected safe html to remain raw, got: %q", got)
	}
	if strings.Contains(got, "&lt;ol") {
		t.Fatalf("expected safe html to bypass escaping: %q", got)
	}
}

func TestUrlIsUsesRouteNameFromContext(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{% if UrlIs("dash.posts.edit") %}active{% else %}inactive{% endif %}|{% if UrlIs("dash.home") %}home{% else %}not-home{% endif %}`),
		0o644,
	); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{
		"RouteName": "dash.posts.edit",
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "active|not-home" {
		t.Fatalf("unexpected render output: %q", got)
	}
}

func TestUrlIsRejectsMultipleArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{% if UrlIs("dash.posts.list", "dash.posts.edit") %}active{% else %}inactive{% endif %}`),
		0o644,
	); err != nil {
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
	err := view.Render(&out, "page.html", map[string]any{
		"RouteName": "dash.posts.edit",
	})
	if err == nil {
		t.Fatal("expected render error when UrlIs receives multiple arguments")
	}
	if !strings.Contains(err.Error(), "UrlIs requires exactly one route name argument") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestURLForSupportsKeywordArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ UrlFor('dash.posts.edit', id=PostID, tab='comments') }}`), 0o644); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{"PostID": 12}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
	if capturedName != "dash.posts.edit" {
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
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ UrlFor('dash.posts.edit', {"id": "1", "tab": "draft"}, id=PostID, tab='comments') }}`), 0o644); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{"PostID": 9}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if capturedParams["id"] != "9" || capturedParams["tab"] != "comments" {
		t.Fatalf("captured params = %#v", capturedParams)
	}
}

func TestURLForDropsBlankParamsAndQuery(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{{ UrlFor('dash.posts.list', {"kind": Kind, "q": Search, "tag": "", "category": Category}, {"status": Status, "mode": ""}) }}`),
		0o644,
	); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	var capturedParams map[string]string
	var capturedQuery map[string]string
	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		capturedParams = params
		capturedQuery = query
		return "ok"
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page.html", map[string]any{
		"Kind":     "0",
		"Search":   "   ",
		"Category": "9",
		"Status":   "published",
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
	if capturedParams["kind"] != "0" || capturedParams["category"] != "9" {
		t.Fatalf("captured params = %#v", capturedParams)
	}
	if _, ok := capturedParams["q"]; ok {
		t.Fatalf("captured params should not include empty q: %#v", capturedParams)
	}
	if _, ok := capturedParams["tag"]; ok {
		t.Fatalf("captured params should not include empty tag: %#v", capturedParams)
	}
	if capturedQuery["status"] != "published" {
		t.Fatalf("captured query = %#v", capturedQuery)
	}
	if _, ok := capturedQuery["mode"]; ok {
		t.Fatalf("captured query should not include empty mode: %#v", capturedQuery)
	}
}

func TestURLForDropsBlankKeywordArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{{ UrlFor('dash.posts.edit', {"id": "1", "tab": "draft"}, id=PostID, tab='') }}`),
		0o644,
	); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{"PostID": 7}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
	if capturedParams["id"] != "7" {
		t.Fatalf("captured params = %#v", capturedParams)
	}
	if _, ok := capturedParams["tab"]; ok {
		t.Fatalf("captured params should not include empty tab: %#v", capturedParams)
	}
}

func TestPagerURLUsesContextRouteAndQuery(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{{ PagerURL(3) }}`),
		0o644,
	); err != nil {
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
	if err := view.Render(&out, "page.html", map[string]any{
		"RouteName": "dash.posts.list",
		"Query": map[string]string{
			"kind": "0",
			"q":    "demo",
		},
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
	if capturedName != "dash.posts.list" {
		t.Fatalf("captured route name = %q", capturedName)
	}
	if capturedParams != nil && len(capturedParams) != 0 {
		t.Fatalf("captured params = %#v, want empty", capturedParams)
	}
	if capturedQuery["kind"] != "0" || capturedQuery["q"] != "demo" || capturedQuery["page"] != "3" {
		t.Fatalf("captured query = %#v", capturedQuery)
	}
}

func TestPagerURLUsesExplicitRouteAndQuery(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{{ PagerURL(2, "dash.comments.list", {"status":"pending", "pageSize":"20"}) }}`),
		0o644,
	); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	var capturedName string
	var capturedQuery map[string]string
	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		capturedName = name
		capturedQuery = query
		return "ok"
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	if err := view.Render(&out, "page.html", map[string]any{
		"RouteName": "dash.posts.list",
		"Query": map[string]string{
			"kind": "1",
		},
	}); err != nil {
		t.Fatalf("render page failed: %v", err)
	}
	if got := out.String(); got != "ok" {
		t.Fatalf("unexpected render output: %q", got)
	}
	if capturedName != "dash.comments.list" {
		t.Fatalf("captured route name = %q", capturedName)
	}
	if capturedQuery["status"] != "pending" || capturedQuery["pageSize"] != "20" || capturedQuery["page"] != "2" {
		t.Fatalf("captured query = %#v", capturedQuery)
	}
	if _, ok := capturedQuery["kind"]; ok {
		t.Fatalf("captured query should not include context kind: %#v", capturedQuery)
	}
}

func TestPagerURLRejectsKeywordArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{{ PagerURL(page=2) }}`),
		0o644,
	); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	view := newMiniJinjaView(tempDir, false)
	registerViewFunc(view.env, func(name string, params map[string]string, query map[string]string) string {
		return "ok"
	})
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	var out bytes.Buffer
	err := view.Render(&out, "page.html", map[string]any{})
	if err == nil {
		t.Fatal("expected render error when PagerURL receives keyword arguments")
	}
	if !strings.Contains(err.Error(), "PagerURL does not support keyword arguments") {
		t.Fatalf("unexpected error: %v", err)
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
