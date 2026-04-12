package app

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
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

func newInstalledTestApp(t *testing.T, sqliteName string) SwavesApp {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), sqliteName)
	prepareInstalledAppDB(t, dbPath)
	swv := NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
	})
	t.Cleanup(func() {
		swv.Shutdown()
	})
	return swv
}

func setProductionFlags(t *testing.T, isProduction bool) {
	t.Helper()

	originalIsProduction := config.IsProduction
	originalIsNotProduction := config.IsNotProduction
	config.IsProduction = isProduction
	config.IsNotProduction = !isProduction
	t.Cleanup(func() {
		config.IsProduction = originalIsProduction
		config.IsNotProduction = originalIsNotProduction
	})
}

func TestImportParseItemRouteRespondsForPostAndGet(t *testing.T) {
	swv := newInstalledTestApp(t, "test.sqlite")

	cookieKV, loginCSRFToken := loadDashLoginForm(t, swv, "")
	loginResp, err := postDashLogin(t, swv, "", cookieKV, loginCSRFToken, "dash")
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	if loginResp.StatusCode < 300 || loginResp.StatusCode >= 400 {
		t.Fatalf("expected login redirect status, got %d", loginResp.StatusCode)
	}

	cookieHeader := strings.TrimSpace(loginResp.Header.Get("Set-Cookie"))
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
	importPageBody, err := io.ReadAll(importPageResp.Body)
	if err != nil {
		t.Fatalf("read import page failed: %v", err)
	}
	csrfToken := extractCSRFTokenFromBody(t, importPageBody)

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

func TestDashLoginRateLimitInProduction(t *testing.T) {
	swv := newInstalledTestApp(t, "login-rate-limit.sqlite")
	middleware.DashLoginRateLimitResetAll()
	setProductionFlags(t, true)

	cookieKV, csrfToken := loadDashLoginForm(t, swv, "198.51.100.10:12345")

	for i := 0; i < 9; i++ {
		resp, err := postDashLogin(t, swv, "198.51.100.10:12345", cookieKV, csrfToken, "wrong-password")
		if err != nil {
			t.Fatalf("login post %d failed: %v", i+1, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("login post %d status = %d, want %d", i+1, resp.StatusCode, fiber.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read login post %d failed: %v", i+1, err)
		}
		if !strings.Contains(string(body), "Invalid password") {
			t.Fatalf("expected invalid password message in attempt %d, got: %s", i+1, string(body))
		}
	}

	resp, err := postDashLogin(t, swv, "198.51.100.10:12345", cookieKV, csrfToken, "wrong-password")
	if err != nil {
		t.Fatalf("rate-limited login post failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("rate-limited login post status = %d, want %d", resp.StatusCode, fiber.StatusTooManyRequests)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read rate-limited login post failed: %v", err)
	}
	if !strings.Contains(string(body), "今日登录访问次数已达上限") {
		t.Fatalf("expected rate-limit message in body, got: %s", string(body))
	}
}

func TestAppTrustsLoopbackProxyHeaderForClientIP(t *testing.T) {
	swv := newInstalledTestApp(t, "proxy-ip.sqlite")

	appConfig := swv.App.Config()
	if got := appConfig.ProxyHeader; got != fiber.HeaderXForwardedFor {
		t.Fatalf("ProxyHeader = %q, want %q", got, fiber.HeaderXForwardedFor)
	}
	if !appConfig.TrustProxy {
		t.Fatal("TrustProxy = false, want true")
	}
	if !appConfig.TrustProxyConfig.Loopback {
		t.Fatal("TrustProxyConfig.Loopback = false, want true")
	}
	if !appConfig.EnableIPValidation {
		t.Fatal("EnableIPValidation = false, want true")
	}
}

func TestDashLoginRateLimitSkippedOutsideProduction(t *testing.T) {
	swv := newInstalledTestApp(t, "login-rate-limit-dev.sqlite")
	middleware.DashLoginRateLimitResetAll()
	setProductionFlags(t, false)

	cookieKV, csrfToken := loadDashLoginForm(t, swv, "198.51.100.11:12345")

	for i := 0; i < 11; i++ {
		resp, err := postDashLogin(t, swv, "198.51.100.11:12345", cookieKV, csrfToken, "wrong-password")
		if err != nil {
			t.Fatalf("login post %d failed: %v", i+1, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("login post %d status = %d, want %d", i+1, resp.StatusCode, fiber.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read login post %d failed: %v", i+1, err)
		}
		if !strings.Contains(string(body), "Invalid password") {
			t.Fatalf("expected invalid password message in attempt %d, got: %s", i+1, string(body))
		}
	}
}

func TestDashLoginRateLimitResetsAfterSuccessfulLogin(t *testing.T) {
	swv := newInstalledTestApp(t, "login-rate-limit-reset.sqlite")
	middleware.DashLoginRateLimitResetAll()
	setProductionFlags(t, true)

	cookieKV, csrfToken := loadDashLoginForm(t, swv, "198.51.100.12:12345")

	for i := 0; i < 9; i++ {
		resp, err := postDashLogin(t, swv, "198.51.100.12:12345", cookieKV, csrfToken, "wrong-password")
		if err != nil {
			t.Fatalf("pre-reset login post %d failed: %v", i+1, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("pre-reset login post %d status = %d, want %d", i+1, resp.StatusCode, fiber.StatusOK)
		}
	}

	successResp, err := postDashLogin(t, swv, "198.51.100.12:12345", cookieKV, csrfToken, "dash")
	if err != nil {
		t.Fatalf("successful login failed: %v", err)
	}
	if successResp.StatusCode < 300 || successResp.StatusCode >= 400 {
		t.Fatalf("successful login status = %d, want redirect", successResp.StatusCode)
	}

	cookieKV, csrfToken = loadDashLoginForm(t, swv, "198.51.100.12:12345")

	for i := 0; i < 9; i++ {
		resp, err := postDashLogin(t, swv, "198.51.100.12:12345", cookieKV, csrfToken, "wrong-password")
		if err != nil {
			t.Fatalf("post-reset login post %d failed: %v", i+1, err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Fatalf("post-reset login post %d status = %d, want %d", i+1, resp.StatusCode, fiber.StatusOK)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read post-reset login post %d failed: %v", i+1, err)
		}
		if !strings.Contains(string(body), "Invalid password") {
			t.Fatalf("expected invalid password message in post-reset attempt %d, got: %s", i+1, string(body))
		}
	}

	limitedResp, err := postDashLogin(t, swv, "198.51.100.12:12345", cookieKV, csrfToken, "wrong-password")
	if err != nil {
		t.Fatalf("post-reset rate-limited login post failed: %v", err)
	}
	if limitedResp.StatusCode != fiber.StatusTooManyRequests {
		t.Fatalf("post-reset rate-limited login post status = %d, want %d", limitedResp.StatusCode, fiber.StatusTooManyRequests)
	}
}

func loadDashLoginForm(t *testing.T, swv SwavesApp, remoteAddr string) (string, string) {
	t.Helper()

	req := httptest.NewRequest("GET", "/dash/login", nil)
	req.RemoteAddr = remoteAddr

	resp, err := swv.App.Test(req)
	if err != nil {
		t.Fatalf("login page request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected login page status: %d", resp.StatusCode)
	}

	cookieHeader := strings.TrimSpace(resp.Header.Get("Set-Cookie"))
	if cookieHeader == "" {
		t.Fatal("expected login page response to set session cookie")
	}
	cookieKV := strings.SplitN(cookieHeader, ";", 2)[0]
	if cookieKV == "" {
		t.Fatal("expected valid session cookie")
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read login page failed: %v", err)
	}

	return cookieKV, extractCSRFTokenFromBody(t, body)
}

func postDashLogin(t *testing.T, swv SwavesApp, remoteAddr string, cookieKV string, csrfToken string, password string) (*http.Response, error) {
	t.Helper()

	form := url.Values{}
	form.Set("_csrf_token", csrfToken)
	form.Set("password", password)

	req := httptest.NewRequest("POST", "/dash/login", strings.NewReader(form.Encode()))
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", cookieKV)

	return swv.App.Test(req)
}

func extractCSRFTokenFromBody(t *testing.T, body []byte) string {
	t.Helper()

	matches := regexp.MustCompile(`name="_csrf_token" value="([^"]+)"`).FindStringSubmatch(string(body))
	if len(matches) < 2 || strings.TrimSpace(matches[1]) == "" {
		t.Fatal("expected csrf token in response body")
	}
	return strings.TrimSpace(matches[1])
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
