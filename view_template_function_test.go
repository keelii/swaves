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
