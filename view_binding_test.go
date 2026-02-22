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

	prepared, err := prepareTemplateBinding(original)
	if err != nil {
		t.Fatalf("prepare binding failed: %v", err)
	}

	if got := prepared["Title"]; got != "Dashboard" {
		t.Fatalf("expected Title preserved, got %#v", got)
	}

	req, ok := prepared["Req"].(templateReqMeta)
	if !ok {
		t.Fatalf("expected Req type templateReqMeta, got %T", prepared["Req"])
	}
	if req.RouteName != "admin.home" {
		t.Fatalf("unexpected route name: %q", req.RouteName)
	}
	if req.Path != "/admin" {
		t.Fatalf("unexpected path: %q", req.Path)
	}
	if req.ReqID != "req-1" {
		t.Fatalf("unexpected req id: %q", req.ReqID)
	}
	if got := req.Query["page"]; got != "2" {
		t.Fatalf("unexpected query page: %q", got)
	}

	auth, ok := prepared["Auth"].(templateAuthMeta)
	if !ok {
		t.Fatalf("expected Auth type templateAuthMeta, got %T", prepared["Auth"])
	}
	if !auth.IsLogin {
		t.Fatalf("expected Auth.IsLogin to be true")
	}

	site, ok := prepared["Site"].(templateSiteMeta)
	if !ok {
		t.Fatalf("expected Site type templateSiteMeta, got %T", prepared["Site"])
	}
	if site.Settings == nil {
		t.Fatalf("expected Site.Settings to be initialized")
	}

	rootCtxRaw, exists := prepared[internalRootContextKey]
	if !exists {
		t.Fatalf("expected %s in prepared binding", internalRootContextKey)
	}
	rootCtx, ok := rootCtxRaw.(map[string]any)
	if !ok {
		t.Fatalf("expected %s type map[string]any, got %T", internalRootContextKey, rootCtxRaw)
	}
	if got := rootCtx["Title"]; got != "Dashboard" {
		t.Fatalf("expected %s.Title preserved, got %#v", internalRootContextKey, got)
	}

	if _, exists := original["Req"]; exists {
		t.Fatalf("original map should not be mutated with Req")
	}
}

func TestPrepareTemplateBindingNormalizesBoolAndQuery(t *testing.T) {
	prepared, err := prepareTemplateBinding(fiber.Map{
		"Path":    "/monitor",
		"Query":   map[string]interface{}{"page": 3, "kind": "uv"},
		"IsLogin": "1",
	})
	if err != nil {
		t.Fatalf("prepare binding failed: %v", err)
	}

	req, ok := prepared["Req"].(templateReqMeta)
	if !ok {
		t.Fatalf("expected Req type templateReqMeta, got %T", prepared["Req"])
	}
	if req.Path != "/monitor" {
		t.Fatalf("unexpected path: %q", req.Path)
	}
	if got := req.Query["page"]; got != "3" {
		t.Fatalf("unexpected query page: %q", got)
	}
	if got := req.Query["kind"]; got != "uv" {
		t.Fatalf("unexpected query kind: %q", got)
	}

	auth, ok := prepared["Auth"].(templateAuthMeta)
	if !ok {
		t.Fatalf("expected Auth type templateAuthMeta, got %T", prepared["Auth"])
	}
	if !auth.IsLogin {
		t.Fatalf("expected Auth.IsLogin true for string value 1")
	}
}

func TestPrepareTemplateBindingRejectsReservedKeys(t *testing.T) {
	_, err := prepareTemplateBinding(fiber.Map{
		"Req": "not allowed",
	})
	if err == nil {
		t.Fatalf("expected reserved key collision error")
	}
}
