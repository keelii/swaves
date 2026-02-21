package main

import (
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestPrepareTemplateBindingAddsSystemNamespaces(t *testing.T) {
	original := fiber.Map{
		"Title":     "Dashboard",
		"RouteName": "admin.home",
		"UrlPath":   "/admin",
		"Query": map[string]string{
			"page": "2",
		},
		"IsLogin": true,
		"ReqID":   "req-1",
	}

	prepared := prepareTemplateBinding(original)

	if got := prepared["Title"]; got != "Dashboard" {
		t.Fatalf("expected Title preserved, got %#v", got)
	}

	ctx, ok := prepared["_ctx"].(templateContextMeta)
	if !ok {
		t.Fatalf("expected _ctx type templateContextMeta, got %T", prepared["_ctx"])
	}
	if ctx.RouteName != "admin.home" {
		t.Fatalf("unexpected route name: %q", ctx.RouteName)
	}
	if ctx.Path != "/admin" {
		t.Fatalf("unexpected path: %q", ctx.Path)
	}
	if ctx.ReqID != "req-1" {
		t.Fatalf("unexpected req id: %q", ctx.ReqID)
	}
	if got := ctx.Query["page"]; got != "2" {
		t.Fatalf("unexpected query page: %q", got)
	}

	auth, ok := prepared["_auth"].(templateAuthMeta)
	if !ok {
		t.Fatalf("expected _auth type templateAuthMeta, got %T", prepared["_auth"])
	}
	if !auth.IsLogin {
		t.Fatalf("expected _auth.IsLogin to be true")
	}

	site, ok := prepared["_site"].(templateSiteMeta)
	if !ok {
		t.Fatalf("expected _site type templateSiteMeta, got %T", prepared["_site"])
	}
	if site.Settings == nil {
		t.Fatalf("expected _site.Settings to be initialized")
	}

	if _, exists := original["_ctx"]; exists {
		t.Fatalf("original map should not be mutated with _ctx")
	}
}

func TestPrepareTemplateBindingNormalizesBoolAndQuery(t *testing.T) {
	prepared := prepareTemplateBinding(fiber.Map{
		"Path":    "/monitor",
		"Query":   map[string]interface{}{"page": 3, "kind": "uv"},
		"IsLogin": "1",
	})

	ctx, ok := prepared["_ctx"].(templateContextMeta)
	if !ok {
		t.Fatalf("expected _ctx type templateContextMeta, got %T", prepared["_ctx"])
	}
	if ctx.Path != "/monitor" {
		t.Fatalf("unexpected path: %q", ctx.Path)
	}
	if got := ctx.Query["page"]; got != "3" {
		t.Fatalf("unexpected query page: %q", got)
	}
	if got := ctx.Query["kind"]; got != "uv" {
		t.Fatalf("unexpected query kind: %q", got)
	}

	auth, ok := prepared["_auth"].(templateAuthMeta)
	if !ok {
		t.Fatalf("expected _auth type templateAuthMeta, got %T", prepared["_auth"])
	}
	if !auth.IsLogin {
		t.Fatalf("expected _auth.IsLogin true for string value 1")
	}
}
