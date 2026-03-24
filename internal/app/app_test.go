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
	"strconv"
	"strings"
	"testing"

	"swaves/internal/platform/db"
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

func prepareInstalledAppDB(t *testing.T, dbPath string) {
	t.Helper()

	model := db.Open(db.Options{DSN: dbPath})
	if err := db.EnsureDefaultSettings(model); err != nil {
		_ = model.Close()
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	if err := model.Close(); err != nil {
		t.Fatalf("close prepared db failed: %v", err)
	}
}

func TestImportParseItemRouteRespondsForPostAndGet(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.sqlite")
	prepareInstalledAppDB(t, dbPath)
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

func TestInstallFlowRedirectsThenInitializesSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "install.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile:    dbPath,
		AdminPassword: mustHashPassword(t, "runtime-secret"),
		ListenAddr:    ":0",
		AppName:       "swaves-test",
	})
	defer swv.Shutdown()

	homeResp := requestControllerP0(t, swv, fiber.MethodGet, "/", nil, "", nil)
	if homeResp.StatusCode < 300 || homeResp.StatusCode >= 400 {
		t.Fatalf("expected install redirect status, got %d", homeResp.StatusCode)
	}
	if location := strings.TrimSpace(homeResp.Header.Get("Location")); location != "/install" {
		t.Fatalf("unexpected install redirect location: %q", location)
	}

	csrfToken, cookieKV, _ := fetchCSRFToken(
		t,
		swv,
		"/install",
		"",
		`name="setting_dash_password"`,
		"完成安装",
	)

	form := url.Values{}
	form.Set("_csrf_token", csrfToken)
	form.Set("setting_dash_password", "install-secret")

	installResp := requestControllerP0(t, swv, fiber.MethodPost, "/install", form, cookieKV, nil)
	body := assertTemplateRendered(t, installResp, fiber.StatusOK, "安装完成", "进入当前后台")
	if !strings.Contains(body, "runtime-secret") {
		// sanity check only: response should not leak runtime secret
	} else {
		t.Fatal("install success page should not expose runtime secret")
	}

	count, err := db.CountSettings(swv.Store.Model)
	if err != nil {
		t.Fatalf("CountSettings failed: %v", err)
	}
	if count != len(db.DefaultSettings) {
		t.Fatalf("unexpected installed settings count: got=%d want=%d", count, len(db.DefaultSettings))
	}
	if err := db.CheckPassword(swv.Store.Model, "install-secret"); err != nil {
		t.Fatalf("CheckPassword should use installed password before restart: %v", err)
	}

	installAgainResp := requestControllerP0(t, swv, fiber.MethodGet, "/install", nil, cookieKV, nil)
	if installAgainResp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("expected /install to be unavailable after install, got %d", installAgainResp.StatusCode)
	}

	loginToken, loginCookieKV, _ := fetchCSRFToken(
		t,
		swv,
		"/dash/login",
		"",
		`<h1 class="auth-title">登录管理后台</h1>`,
	)
	loginForm := url.Values{}
	loginForm.Set("_csrf_token", loginToken)
	loginForm.Set("password", "install-secret")
	loginResp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/login", loginForm, loginCookieKV, nil)
	if loginResp.StatusCode < 300 || loginResp.StatusCode >= 400 {
		t.Fatalf("expected login redirect after install, got %d", loginResp.StatusCode)
	}
}

func TestInstallPageOnlyShowsKeySettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "install-fields.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile:    dbPath,
		AdminPassword: mustHashPassword(t, "runtime-secret"),
		ListenAddr:    ":0",
		AppName:       "swaves-test",
	})
	defer swv.Shutdown()

	_, _, body := fetchCSRFToken(
		t,
		swv,
		"/install",
		"",
		`name="setting_site_name"`,
		`name="setting_dash_password"`,
		"完成安装",
	)

	if strings.Contains(body, `name="setting_editor_font_size"`) {
		t.Fatal("install page should not expose editor settings")
	}
	if strings.Contains(body, `name="setting_editor_font_family"`) {
		t.Fatal("install page should not expose editor font settings")
	}
	if !strings.Contains(body, `name="setting_page_url_prefix"`) || !strings.Contains(body, `data-prefix-source-code="base_path"`) {
		t.Fatal("install page should wire prefix-field sync metadata for routed prefixes")
	}
}

func TestInstallFlowShowsRestartNoteForReloadSettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "install-restart.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile:    dbPath,
		AdminPassword: mustHashPassword(t, "runtime-secret"),
		ListenAddr:    ":0",
		AppName:       "swaves-test",
	})
	defer swv.Shutdown()

	csrfToken, cookieKV, _ := fetchCSRFToken(
		t,
		swv,
		"/install",
		"",
		`name="setting_dash_password"`,
	)

	form := url.Values{}
	form.Set("_csrf_token", csrfToken)
	form.Set("setting_dash_password", "install-secret")
	form.Set("setting_dash_path", "/console")
	form.Set("setting_backup_local_interval_min", strconv.Itoa(1440))

	installResp := requestControllerP0(t, swv, fiber.MethodPost, "/install", form, cookieKV, nil)
	assertTemplateRendered(
		t,
		installResp,
		fiber.StatusOK,
		"请先重启应用",
		"管理后台路径",
	)
}
