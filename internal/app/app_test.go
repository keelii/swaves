package app

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

func TestImportParseItemRouteRespondsForPostAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
	})
	defer swv.Shutdown()

	form := url.Values{}
	form.Set("password", "admin")
	loginPageReq := httptest.NewRequest("GET", "/admin/login", nil)
	loginPageResp, err := swv.App.Test(loginPageReq)
	if err != nil {
		t.Fatalf("login page request failed: %v", err)
	}
	if loginPageResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected login page status: %d", loginPageResp.StatusCode)
	}

	cookieHeader := strings.TrimSpace(loginPageResp.Header.Get("Set-Cookie"))
	if cookieHeader == "" {
		t.Fatalf("expected login page response to set session cookie")
	}
	cookieKV := strings.SplitN(cookieHeader, ";", 2)[0]
	if cookieKV == "" {
		t.Fatalf("expected valid session cookie")
	}

	loginPageBody, _ := io.ReadAll(loginPageResp.Body)
	matches := regexp.MustCompile(`name="_csrf_token" value="([^"]+)"`).FindStringSubmatch(string(loginPageBody))
	if len(matches) < 2 || strings.TrimSpace(matches[1]) == "" {
		t.Fatalf("expected csrf token in login form")
	}
	form.Set("_csrf_token", strings.TrimSpace(matches[1]))

	loginReq := httptest.NewRequest("POST", "/admin/login", strings.NewReader(form.Encode()))
	loginReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	loginReq.Header.Set("Cookie", cookieKV)

	loginResp, err := swv.App.Test(loginReq)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode < 300 || loginResp.StatusCode >= 400 {
		t.Fatalf("expected login redirect status, got %d", loginResp.StatusCode)
	}

	cookieHeader = strings.TrimSpace(loginResp.Header.Get("Set-Cookie"))
	if cookieHeader == "" {
		t.Fatalf("expected login response to set session cookie")
	}
	cookieKV = strings.SplitN(cookieHeader, ";", 2)[0]
	if cookieKV == "" {
		t.Fatalf("expected valid session cookie")
	}

	importPageReq := httptest.NewRequest("GET", "/admin/import", nil)
	importPageReq.Header.Set("Cookie", cookieKV)
	importPageResp, err := swv.App.Test(importPageReq)
	if err != nil {
		t.Fatalf("import page request failed: %v", err)
	}
	if importPageResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected import page status: %d", importPageResp.StatusCode)
	}
	importPageBody, _ := io.ReadAll(importPageResp.Body)
	metaMatches := regexp.MustCompile(`name="_csrf_token" value="([^"]+)"`).FindStringSubmatch(string(importPageBody))
	if len(metaMatches) < 2 || strings.TrimSpace(metaMatches[1]) == "" {
		t.Fatalf("expected csrf token field in import page")
	}
	csrfToken := strings.TrimSpace(metaMatches[1])

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "post.md")
	if err != nil {
		t.Fatalf("create form file failed: %v", err)
	}
	if _, err := part.Write([]byte("# title\n\ncontent")); err != nil {
		t.Fatalf("write markdown content failed: %v", err)
	}
	if err := writer.WriteField("title_source", "markdown"); err != nil {
		t.Fatalf("write title source failed: %v", err)
	}
	if err := writer.WriteField("slug_source", "filename"); err != nil {
		t.Fatalf("write slug source failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	postReq := httptest.NewRequest("POST", "/admin/import/parse-item", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.Header.Set("Cookie", cookieKV)
	postReq.Header.Set("X-CSRF-Token", csrfToken)
	postResp, err := swv.App.Test(postReq)
	if err != nil {
		t.Fatalf("post parse-item request failed: %v", err)
	}
	if postResp.StatusCode == fiber.StatusNotFound {
		t.Fatalf("expected post /admin/import/parse-item route to exist")
	}

	getReq := httptest.NewRequest("GET", "/admin/import/parse-item", nil)
	getReq.Header.Set("Cookie", cookieKV)
	getResp, err := swv.App.Test(getReq)
	if err != nil {
		t.Fatalf("get parse-item request failed: %v", err)
	}
	if getResp.StatusCode != fiber.StatusMethodNotAllowed {
		t.Fatalf("unexpected get /admin/import/parse-item status: %d", getResp.StatusCode)
	}
}
