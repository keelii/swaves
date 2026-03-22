package app

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
	"golang.org/x/crypto/bcrypt"
)

func mustHashPassword(t *testing.T, raw string) string {
	t.Helper()

	hashed, err := bcrypt.GenerateFromPassword([]byte(raw), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password failed: %v", err)
	}
	return string(hashed)
}

func TestImportParseItemRouteRespondsForPostAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile:    dbPath,
		AdminPassword: mustHashPassword(t, "dash"),
		ListenAddr:    ":0",
		AppName:       "swaves-test",
	})
	defer swv.Shutdown()

	form := url.Values{}
	form.Set("password", "dash")
	loginPageReq := httptest.NewRequest("GET", "/dash/login", nil)
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

	loginReq := httptest.NewRequest("POST", "/dash/login", strings.NewReader(form.Encode()))
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

	importPageReq := httptest.NewRequest("GET", "/dash/import", nil)
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

	postReq := httptest.NewRequest("POST", "/dash/import/parse-item", &body)
	postReq.Header.Set("Content-Type", writer.FormDataContentType())
	postReq.Header.Set("Cookie", cookieKV)
	postReq.Header.Set("X-CSRF-Token", csrfToken)
	postResp, err := swv.App.Test(postReq)
	if err != nil {
		t.Fatalf("post parse-item request failed: %v", err)
	}
	if postResp.StatusCode == fiber.StatusNotFound {
		t.Fatalf("expected post /dash/import/parse-item route to exist")
	}

	getReq := httptest.NewRequest("GET", "/dash/import/parse-item", nil)
	getReq.Header.Set("Cookie", cookieKV)
	getResp, err := swv.App.Test(getReq)
	if err != nil {
		t.Fatalf("get parse-item request failed: %v", err)
	}
	if getResp.StatusCode != fiber.StatusMethodNotAllowed {
		t.Fatalf("unexpected get /dash/import/parse-item status: %d", getResp.StatusCode)
	}
}

func TestResolveProjectPathFindsFromNestedWorkingDir(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "repo")
	templatesDir := filepath.Join(projectRoot, "web", "templates")
	nestedDir := filepath.Join(projectRoot, "web", "static", "seditor")

	if err := os.MkdirAll(templatesDir, 0o755); err != nil {
		t.Fatalf("create templates dir failed: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("create nested dir failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	if err := os.Chdir(nestedDir); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}

	got := resolveProjectPath(filepath.Join("web", "templates"))
	want := templatesDir
	gotInfo, err := os.Stat(got)
	if err != nil {
		t.Fatalf("stat resolved path failed: %v", err)
	}
	wantInfo, err := os.Stat(want)
	if err != nil {
		t.Fatalf("stat expected path failed: %v", err)
	}
	if !os.SameFile(gotInfo, wantInfo) {
		t.Fatalf("unexpected resolved template path: got %q want %q", got, want)
	}
}
