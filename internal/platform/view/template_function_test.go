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

func mustWriteTemplateFile(t *testing.T, root string, relativeName string, source string) {
	t.Helper()

	templatePath := filepath.Join(root, relativeName)
	if err := os.MkdirAll(filepath.Dir(templatePath), 0o755); err != nil {
		t.Fatalf("create template directory failed: %v", err)
	}
	if err := os.WriteFile(templatePath, []byte(source), 0o644); err != nil {
		t.Fatalf("write template %q failed: %v", relativeName, err)
	}
}

func mustLoadMiniJinjaTestView(t *testing.T, root string, resolver func(name string, params map[string]string, query map[string]string) string) *FiberView {
	t.Helper()

	if resolver == nil {
		resolver = func(name string, params map[string]string, query map[string]string) string {
			return name
		}
	}

	view := newMiniJinjaView(root, false)
	registerViewFunc(view.env, resolver)
	if err := view.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}
	return view
}

func TestMacroIncludeRendersNestedTemplate(t *testing.T) {
	tempDir := t.TempDir()
	mustWriteTemplateFile(t, tempDir, "page.html", `{% import "macros.html" as ui %}{{ ui.row({"Value": "ok"}) }}`)
	mustWriteTemplateFile(t, tempDir, "macros.html", `{% macro row(ctx) %}{% with Value = ctx.Value, __root = ctx %}{% include "partial.html" %}{% endwith %}{% endmacro %}`)
	mustWriteTemplateFile(t, tempDir, "partial.html", `{{ Value }} / {{ __root.Value }}`)

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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
	mustWriteTemplateFile(t, tempDir, "dash/layout/base.html", `{% block body %}{% endblock %}`)
	mustWriteTemplateFile(t, tempDir, "dash/include/ui.html", `{% macro hi(label) %}{{ label }}{% endmacro %}`)
	mustWriteTemplateFile(t, tempDir, "dash/layout/layout.html", `{% extends "dash/layout/base.html" %}{% import "dash/include/ui.html" as ui %}{% block body %}{% include "dash/include/actions.html" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplateFile(t, tempDir, "dash/include/actions.html", `{% import "dash/include/ui.html" as ui %}{{ ui.hi("A") }}`)
	mustWriteTemplateFile(t, tempDir, "page.html", `{% extends "dash/layout/layout.html" %}{% import "dash/include/ui.html" as ui %}{% block content %}{{ ui.hi("B") }}{% endblock %}`)

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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
	mustWriteTemplateFile(t, tempDir, "dash/layout/base.html", `{% block body %}{% endblock %}`)
	mustWriteTemplateFile(t, tempDir, "dash/include/actions.html", `A`)
	mustWriteTemplateFile(t, tempDir, "dash/layout/layout.html", `{% extends "dash/layout/base.html" %}{% block body %}{% include "dash/include/actions.html" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplateFile(t, tempDir, "dash/categories_index.html", `{% extends "dash/layout/layout.html" %}{% block content %}B{% endblock %}`)

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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
	mustWriteTemplateFile(t, tempDir, "dash/page.html", `{% include "./include/item.html" %}`)
	mustWriteTemplateFile(t, tempDir, "dash/include/item.html", `ok`)

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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

func TestCompactNumberFilter(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ Total|compactNumber }}`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

	cases := []struct {
		name  string
		total any
		want  string
	}{
		{name: "small", total: 999, want: "999"},
		{name: "one_thousand", total: 1000, want: "1k"},
		{name: "mid_thousand", total: 1500, want: "1.5k"},
		{name: "ten_thousand", total: 10500, want: "11k"},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := view.Render(&out, "page.html", map[string]any{
				"Total": item.total,
			}); err != nil {
				t.Fatalf("render page failed: %v", err)
			}
			if got := out.String(); got != item.want {
				t.Fatalf("compactNumber = %q, want %q", got, item.want)
			}
		})
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

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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

	view := mustLoadMiniJinjaTestView(t, tempDir, nil)

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
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedName = name
		capturedParams = params
		capturedQuery = query
		return "ok"
	})

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
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedParams = params
		return "ok"
	})

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
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedParams = params
		capturedQuery = query
		return "ok"
	})

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
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedParams = params
		return "ok"
	})

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
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedName = name
		capturedParams = params
		capturedQuery = query
		return "ok"
	})

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
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedName = name
		capturedQuery = query
		return "ok"
	})

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

func TestPagerURLAllowsKeywordArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(tempDir, "page.html"),
		[]byte(`{{ PagerURL(page=2, routeName="dash.comments.list", status="pending", pageSize=20) }}`),
		0o644,
	); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}

	var capturedName string
	var capturedQuery map[string]string
	view := mustLoadMiniJinjaTestView(t, tempDir, func(name string, params map[string]string, query map[string]string) string {
		capturedName = name
		capturedQuery = query
		return "resolved"
	})

	var out bytes.Buffer
	err := view.Render(&out, "page.html", map[string]any{})
	if err != nil {
		t.Fatalf("render failed: %v", err)
	}
	if strings.TrimSpace(out.String()) != "resolved" {
		t.Fatalf("unexpected render output: %q", out.String())
	}
	if capturedName != "dash.comments.list" {
		t.Fatalf("captured route name = %q", capturedName)
	}
	if capturedQuery["status"] != "pending" || capturedQuery["pageSize"] != "20" || capturedQuery["page"] != "2" {
		t.Fatalf("captured query = %#v", capturedQuery)
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
