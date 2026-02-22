package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestMacroIncludeRendersNestedTemplate(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{% import "macros" as ui %}{{ ui.row(dict("Value", "ok")) }}`), 0o644); err != nil {
		t.Fatalf("write page template failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tempDir, "macros.html"), []byte(`{% macro row(ctx) %}{% with Value = ctx.Value, __root = ctx %}{% include "partial" %}{% endwith %}{% endmacro %}`), 0o644); err != nil {
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
	mustWriteTemplate("admin/layout/layout.html", `{% extends "/admin/layout/base" %}{% block body %}{% include "/admin/include/actions" %}{% block content %}{% endblock %}{% endblock %}`)
	mustWriteTemplate("admin/include/actions.html", `{{ ui.hi("A") }}`)
	mustWriteTemplate("page.html", `{% extends "/admin/layout/layout" %}{% block content %}{{ ui.hi("B") }}{% endblock %}`)

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

func TestURLForSupportsKeywordArguments(t *testing.T) {
	tempDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ url_for("admin.posts.edit", id=PostID, tab="comments") }}`), 0o644); err != nil {
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
	if err := os.WriteFile(filepath.Join(tempDir, "page.html"), []byte(`{{ url_for("admin.posts.edit", dict("id", "1", "tab", "draft"), id=PostID, tab="comments") }}`), 0o644); err != nil {
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
