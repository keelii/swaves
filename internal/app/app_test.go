package app

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

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
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
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
	sourceContent := "# title\n\n" + strings.Repeat("content ", 700)
	if _, err := part.Write([]byte(sourceContent)); err != nil {
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
	if postResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected post /dash/import/parse-item status: %d", postResp.StatusCode)
	}

	parseBodyBytes, err := io.ReadAll(postResp.Body)
	if err != nil {
		t.Fatalf("read parse-item response failed: %v", err)
	}

	var parseResult struct {
		OK   bool `json:"ok"`
		Item struct {
			ContentPreview string `json:"content_preview"`
		} `json:"item"`
	}
	if err := json.Unmarshal(parseBodyBytes, &parseResult); err != nil {
		t.Fatalf("decode parse-item response failed: %v", err)
	}
	if !parseResult.OK {
		t.Fatal("expected parse-item response ok=true")
	}
	if parseResult.Item.ContentPreview == "" {
		t.Fatal("expected parse-item response content preview")
	}

	var parseBody map[string]any
	if err := json.Unmarshal(parseBodyBytes, &parseBody); err != nil {
		t.Fatalf("decode parse-item response map failed: %v", err)
	}
	itemMap, ok := parseBody["item"].(map[string]any)
	if !ok {
		t.Fatalf("expected item object in parse-item response")
	}
	if _, exists := itemMap["content"]; exists {
		t.Fatal("expected parse-item response to omit content")
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

	legacyPostReq := httptest.NewRequest("POST", "/dash/import", strings.NewReader(url.Values{
		"_csrf_token": []string{csrfToken},
	}.Encode()))
	legacyPostReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	legacyPostReq.Header.Set("Cookie", cookieKV)
	legacyPostResp, err := swv.App.Test(legacyPostReq)
	if err != nil {
		t.Fatalf("legacy post import request failed: %v", err)
	}
	if legacyPostResp.StatusCode != fiber.StatusMethodNotAllowed {
		t.Fatalf("unexpected post /dash/import status: %d", legacyPostResp.StatusCode)
	}
}

func TestResolveProjectPathFindsFromNestedWorkingDir(t *testing.T) {
	projectRoot := filepath.Join(t.TempDir(), "repo")
	templatesDir := filepath.Join(projectRoot, "web", "templates")
	nestedDir := filepath.Join(projectRoot, "web", "seditor")

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
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
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
	if installResp.StatusCode < 300 || installResp.StatusCode >= 400 {
		t.Fatalf("expected install redirect status, got %d", installResp.StatusCode)
	}
	if location := strings.TrimSpace(installResp.Header.Get("Location")); location != "/dash" {
		t.Fatalf("unexpected install redirect location: %q", location)
	}
	cookieKV = mergeCookieKV(cookieKV, installResp)
	if cookieKV == "" {
		t.Fatal("expected install response to set logged-in session cookie")
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

	dashHomeResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash", nil, cookieKV, nil)
	assertTemplateRendered(t, dashHomeResp, fiber.StatusOK)
	if strings.Contains(strings.TrimSpace(dashHomeResp.Header.Get("Location")), "/dash/login") {
		t.Fatal("install should log into dash automatically")
	}
}

func TestInstallPageOnlyShowsKeySettings(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "install-fields.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
	})
	defer swv.Shutdown()

	_, _, body := fetchCSRFToken(
		t,
		swv,
		"/install",
		"",
		`name="setting_site_name"`,
		`name="setting_author"`,
		`name="setting_dash_password"`,
		"完成安装",
	)

	if strings.Contains(body, `name="setting_editor_font_size"`) {
		t.Fatal("install page should not expose editor settings")
	}
	if strings.Contains(body, `name="setting_editor_font_family"`) {
		t.Fatal("install page should not expose editor font settings")
	}
	if strings.Contains(body, `name="setting_language"`) {
		t.Fatal("install page should not expose language settings")
	}
	if strings.Contains(body, `name="setting_timezone"`) {
		t.Fatal("install page should not expose timezone settings")
	}
	if strings.Contains(body, `id="setting_author_email"`) {
		t.Fatal("install page should not expose author email settings")
	}
	if strings.Contains(body, `id="setting_site_title"`) {
		t.Fatal("install page should not expose site title settings")
	}
	if !strings.Contains(body, `name="setting_site_desc"`) {
		t.Fatal("install page should expose site description setting")
	}
	if strings.Contains(body, `id="setting_site_url"`) {
		t.Fatal("install page should not expose site url setting")
	}
	if strings.Contains(body, `id="setting_asset_default_provider"`) {
		t.Fatal("install page should not expose asset provider settings")
	}
	if strings.Contains(body, `id="setting_page_url_prefix"`) {
		t.Fatal("install page should not expose page url prefix settings")
	}
	if strings.Contains(body, `id="setting_post_url_prefix"`) {
		t.Fatal("install page should not expose post url prefix setting")
	}
	if strings.Contains(body, `id="setting_post_url_name"`) {
		t.Fatal("install page should not expose post url name setting")
	}
	if strings.Contains(body, `id="setting_post_url_ext"`) {
		t.Fatal("install page should not expose post url ext setting")
	}
	if strings.Contains(body, `id="setting_base_path"`) {
		t.Fatal("install page should not expose base path setting")
	}
	if got := strings.Count(body, `class="install-sep"`); got != 1 {
		t.Fatalf("install page should render 1 separator, got %d", got)
	}
	if got := strings.Count(body, `class="install-sep-label"`); got != 1 {
		t.Fatalf("install page should render 1 separator label, got %d", got)
	}
	if !strings.Contains(body, `class="install-sep-label">后台</span>`) {
		t.Fatal("install page should render backend separator label")
	}
	if !strings.Contains(body, `id="install-post-url-preview"`) {
		t.Fatal("install page should render post url preview alert")
	}

	expectedOrder := []string{
		`name="setting_site_name"`,
		`name="setting_site_desc"`,
		`name="setting_author"`,
		`name="setting_dash_path"`,
		`name="setting_dash_password"`,
	}
	lastIndex := -1
	for _, marker := range expectedOrder {
		currentIndex := strings.Index(body, marker)
		if currentIndex < 0 {
			t.Fatalf("install page should contain %s", marker)
		}
		if currentIndex <= lastIndex {
			t.Fatalf("install page field order mismatch at %s", marker)
		}
		lastIndex = currentIndex
	}
}

func TestInstallPagePrefillsSiteURLFromCurrentPageAddress(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "install-site-url.sqlite")
	swv := NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
	})
	defer swv.Shutdown()

	resp := requestControllerP0(t, swv, fiber.MethodGet, "http://127.0.0.1:4321/install", nil, "", nil)
	body := assertTemplateRendered(t, resp, fiber.StatusOK, `文章 URL 样例`)
	if strings.Contains(body, `id="setting_site_url"`) {
		t.Fatalf("install page should not expose site_url input, body=%q", body)
	}
	if !strings.Contains(body, `文章 URL 样例`) {
		t.Fatalf("install page should show post url preview title, body=%q", body)
	}
	if !strings.Contains(body, `http:&#x2f;&#x2f;127.0.0.1:4321&#x2f;2024&#x2f;01&#x2f;02&#x2f;hello-world`) {
		t.Fatalf("install page should prefill post url preview from current page address, body=%q", body)
	}
}
