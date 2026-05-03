package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/md"
	"swaves/internal/shared/types"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

var csrfTokenPattern = regexp.MustCompile(`name="_csrf_token" value="([^"]+)"`)
var postEditPathPattern = regexp.MustCompile(`/dash/posts/([0-9]+)/edit`)

func newControllerP0TestApp(t *testing.T) SwavesApp {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "controller-p0.sqlite")
	prepareInstalledAppDB(t, dbPath)
	middleware.DashLoginRateLimitResetAll()
	return NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
	})
}

func withControllerP0TemplateReload(t *testing.T, enabled bool) {
	t.Helper()

	original := config.TemplateReload
	config.TemplateReload = enabled
	t.Cleanup(func() {
		config.TemplateReload = original
	})
}

func withControllerP0WorkingDir(t *testing.T, dir string) {
	t.Helper()

	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd failed: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir(%s) failed: %v", dir, err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatalf("restore working directory failed: %v", err)
		}
	})
}

func requestControllerP0(
	t *testing.T,
	swv SwavesApp,
	method string,
	path string,
	form url.Values,
	cookieKV string,
	headers map[string]string,
) *http.Response {
	t.Helper()

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	requestPath := path
	host := ""
	if parsed, err := url.Parse(path); err == nil && parsed.Scheme != "" && parsed.Host != "" {
		requestPath = parsed.RequestURI()
		host = parsed.Host
	}
	req := httptest.NewRequest(method, requestPath, body)
	if host != "" {
		req.Host = host
	}
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	if cookieKV != "" {
		req.Header.Set("Cookie", cookieKV)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	resp, err := swv.App.Test(req)
	if err != nil {
		t.Fatalf("request failed: method=%s path=%s err=%v", method, path, err)
	}
	return resp
}

func readResponseBody(t *testing.T, resp *http.Response) string {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body failed: %v", err)
	}
	return string(body)
}

func assertTemplateRendered(
	t *testing.T,
	resp *http.Response,
	wantStatus int,
	markers ...string,
) string {
	t.Helper()

	if resp.StatusCode != wantStatus {
		t.Fatalf("unexpected status: got=%d want=%d", resp.StatusCode, wantStatus)
	}

	body := readResponseBody(t, resp)
	if strings.TrimSpace(body) == "" {
		t.Fatalf("template response body should not be empty")
	}
	if strings.Contains(body, "Internal Server Error") {
		t.Fatalf("template render appears failed, body includes internal error")
	}
	for _, marker := range markers {
		if marker == "" {
			continue
		}
		if !strings.Contains(body, marker) {
			preview := body
			if len(preview) > 600 {
				preview = preview[:600]
			}
			t.Fatalf("template marker missing: %q body_preview=%q", marker, preview)
		}
	}
	return body
}

func responseCookieKV(resp *http.Response) string {
	setCookie := strings.TrimSpace(resp.Header.Get("Set-Cookie"))
	if setCookie == "" {
		return ""
	}
	return strings.SplitN(setCookie, ";", 2)[0]
}

func mergeCookieKV(current string, resp *http.Response) string {
	next := responseCookieKV(resp)
	if next != "" {
		return next
	}
	return current
}

func extractCSRFToken(body []byte) string {
	matches := csrfTokenPattern.FindStringSubmatch(string(body))
	if len(matches) < 2 {
		return ""
	}
	return strings.TrimSpace(matches[1])
}

func fetchCSRFToken(t *testing.T, swv SwavesApp, path string, cookieKV string, markers ...string) (string, string, string) {
	t.Helper()

	resp := requestControllerP0(t, swv, fiber.MethodGet, path, nil, cookieKV, nil)
	baseMarkers := []string{
		`name="_csrf_token"`,
	}
	baseMarkers = append(baseMarkers, markers...)
	bodyText := assertTemplateRendered(t, resp, fiber.StatusOK, baseMarkers...)
	cookieKV = mergeCookieKV(cookieKV, resp)

	token := extractCSRFToken([]byte(bodyText))
	if token == "" {
		t.Fatalf("csrf token missing: path=%s", path)
	}
	return token, cookieKV, bodyText
}

func loginAsDash(t *testing.T, swv SwavesApp) string {
	t.Helper()

	csrfToken, cookieKV, _ := fetchCSRFToken(
		t,
		swv,
		"/dash/login",
		"",
		`<h1 class="auth-title">登录管理后台</h1>`,
		`name="password"`,
	)
	form := url.Values{}
	form.Set("password", "dash")
	form.Set("_csrf_token", csrfToken)
	resp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/login", form, cookieKV, nil)
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected login redirect status, got %d", resp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, resp)
	if cookieKV == "" {
		t.Fatalf("expected login session cookie")
	}
	return cookieKV
}

func TestDashControllerP0_TrashPageRendersTrashScripts(t *testing.T) {
	swv := newControllerP0TestApp(t)
	t.Cleanup(func() {
		swv.Shutdown()
	})

	post := &db.Post{
		Title:   "trash-script-post",
		Slug:    "trash-script-post",
		Content: "content",
		Status:  "published",
		Kind:    db.PostKindPost,
	}
	if _, err := db.CreatePost(swv.Store.Model, post); err != nil {
		t.Fatalf("CreatePost failed: %v", err)
	}
	if err := db.SoftDeletePost(swv.Store.Model, post.ID); err != nil {
		t.Fatalf("SoftDeletePost failed: %v", err)
	}

	cookieKV := loginAsDash(t, swv)
	resp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/trash?type=posts", nil, cookieKV, nil)
	assertTemplateRendered(
		t,
		resp,
		fiber.StatusOK,
		`data-role="multiselect-batch-restore"`,
		`data-batch-restore-url=`,
		`batch-restore`,
		`trash-empty-confirm-dialog`,
		`bindBatchAction(batchRestore, {`,
		`successIDsKey: 'restored_ids'`,
	)
}

func TestDashControllerP0_ProtectedRouteRequiresLogin(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	resp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/posts", nil, "", nil)
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected redirect for unauthenticated route, got %d", resp.StatusCode)
	}
	location := strings.TrimSpace(resp.Header.Get("Location"))
	want := "/dash/login?returnUrl=%2Fdash%2Fposts"
	if location != want {
		t.Fatalf("unexpected redirect location: got=%q want=%q", location, want)
	}
}

func TestDashControllerP0_LoginReturnURLValidation(t *testing.T) {
	t.Run("allow internal return url", func(t *testing.T) {
		swv := newControllerP0TestApp(t)
		defer swv.Shutdown()

		csrfToken, cookieKV, _ := fetchCSRFToken(
			t,
			swv,
			"/dash/login",
			"",
			`<h1 class="auth-title">登录管理后台</h1>`,
			`name="password"`,
		)
		form := url.Values{}
		form.Set("password", "dash")
		form.Set("returnUrl", "/dash/posts")
		form.Set("_csrf_token", csrfToken)

		resp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/login", form, cookieKV, nil)
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			t.Fatalf("expected login redirect status, got %d", resp.StatusCode)
		}
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location != "/dash/posts" {
			t.Fatalf("unexpected safe return redirect: got=%q want=%q", location, "/dash/posts")
		}
	})

	t.Run("reject unsafe return url", func(t *testing.T) {
		swv := newControllerP0TestApp(t)
		defer swv.Shutdown()

		csrfToken, cookieKV, _ := fetchCSRFToken(
			t,
			swv,
			"/dash/login",
			"",
			`<h1 class="auth-title">登录管理后台</h1>`,
			`name="password"`,
		)
		form := url.Values{}
		form.Set("password", "dash")
		form.Set("returnUrl", "//evil.test/phish")
		form.Set("_csrf_token", csrfToken)

		resp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/login", form, cookieKV, nil)
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			t.Fatalf("expected login redirect status, got %d", resp.StatusCode)
		}
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if strings.Contains(location, "evil.test") {
			t.Fatalf("unsafe return url should be rejected, got location=%q", location)
		}
		if !strings.HasPrefix(location, "/dash") {
			t.Fatalf("fallback redirect should stay in dash namespace, got=%q", location)
		}
	})
}

func TestDashControllerP0_ThemeModeSettingAppliesToRenderedPage(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	csrfToken, cookieKV, _ := fetchCSRFToken(
		t,
		swv,
		"/dash/posts/new",
		cookieKV,
		`data-role="toggle-theme"`,
	)

	form := url.Values{}
	form.Set("code", "mode")
	form.Set("value", "dark")

	resp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/api/settings/dash-ui", form, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected theme mode update status: %d body=%q", resp.StatusCode, readResponseBody(t, resp))
	}

	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			Code  string `json:"code"`
			Value string `json:"value"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(readResponseBody(t, resp)), &payload); err != nil {
		t.Fatalf("parse theme mode update response failed: %v", err)
	}
	if !payload.OK || payload.Data.Code != "mode" || payload.Data.Value != "dark" {
		t.Fatalf("unexpected theme mode update payload: %+v", payload)
	}

	modeSetting, err := db.GetSettingByCode(swv.Store.Model, "mode")
	if err != nil {
		t.Fatalf("GetSettingByCode(mode) failed: %v", err)
	}
	if modeSetting.Value != "dark" {
		t.Fatalf("expected mode setting updated to dark, got %q", modeSetting.Value)
	}

	renderResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/posts/new", nil, cookieKV, nil)
	renderBody := assertTemplateRendered(t, renderResp, fiber.StatusOK, `data-role="toggle-theme"`)
	if !strings.Contains(renderBody, `data-theme="dark"`) {
		t.Fatalf("expected rendered page to use dark theme, body=%q", renderBody)
	}
	if strings.Contains(renderBody, "sui.demo_theme_mode") {
		t.Fatalf("theme rendering should not depend on demo localStorage key")
	}
}

func TestDashControllerP0_MenuCollapsedSettingAppliesToRenderedPage(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	csrfToken, cookieKV, _ := fetchCSRFToken(
		t,
		swv,
		"/dash/posts/new",
		cookieKV,
		`data-role="toggle-menu-collapse"`,
	)

	form := url.Values{}
	form.Set("code", "dash_full_main_open")
	form.Set("value", "1")

	resp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/api/settings/dash-ui", form, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected menu collapsed update status: %d body=%q", resp.StatusCode, readResponseBody(t, resp))
	}

	menuSetting, err := db.GetSettingByCode(swv.Store.Model, "dash_full_main_open")
	if err != nil {
		t.Fatalf("GetSettingByCode(dash_full_main_open) failed: %v", err)
	}
	if menuSetting.Value != "1" {
		t.Fatalf("expected menu collapsed setting updated to 1, got %q", menuSetting.Value)
	}

	renderResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/posts/new", nil, cookieKV, nil)
	renderBody := assertTemplateRendered(t, renderResp, fiber.StatusOK, `data-role="toggle-menu-collapse"`)
	if !strings.Contains(renderBody, `class="app-window menu-collapsed"`) {
		t.Fatalf("expected rendered page to keep menu collapsed, body=%q", renderBody)
	}
}

func TestDashControllerP0_MobileRequestAddsBodyClass(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	resp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/posts/new", nil, cookieKV, map[string]string{
		"User-Agent": "Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.0 Mobile/15E148 Safari/604.1",
	})
	body := assertTemplateRendered(t, resp, fiber.StatusOK, `id="post-title"`)
	if !strings.Contains(body, `<body class="is-mobile"`) {
		t.Fatalf("expected mobile body class, body=%q", body)
	}
}

func TestDashControllerP0_PostLifecycle(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	createPath := "/dash/posts/new"

	baseForm := url.Values{}
	baseForm.Set("title", "P0 Controller Post")
	baseForm.Set("slug", fmt.Sprintf("p0-controller-%d", time.Now().UnixNano()))
	baseForm.Set("content", "controller p0 test content")
	baseForm.Set("action", "publish")
	baseForm.Set("kind", "0")
	baseForm.Set("comment_enabled", "1")

	resp := requestControllerP0(t, swv, fiber.MethodPost, createPath, baseForm, cookieKV, nil)
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected create post forbidden without csrf, got %d", resp.StatusCode)
	}

	csrfToken, cookieKV, newPageBody := fetchCSRFToken(
		t,
		swv,
		createPath,
		cookieKV,
		`id="post-title"`,
		`id="post-slug"`,
		`id="post-content"`,
		`id="form" method="post"`,
	)
	if !strings.Contains(newPageBody, `name="action" value="publish"`) {
		t.Fatalf("new post page should include publish action")
	}
	if !strings.Contains(newPageBody, `class="post-editor-layout"`) {
		t.Fatalf("new post page should render shared editor layout shell")
	}
	resp = requestControllerP0(t, swv, fiber.MethodPost, createPath, baseForm, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected create post redirect 303, got %d", resp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, resp)

	location := strings.TrimSpace(resp.Header.Get("Location"))
	matches := postEditPathPattern.FindStringSubmatch(location)
	if len(matches) < 2 {
		t.Fatalf("create post redirect should point to edit page, got %q", location)
	}
	postID, err := strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		t.Fatalf("parse post id from redirect failed: %v", err)
	}

	createdPost, err := db.GetPostBySlug(swv.Store.Model, baseForm.Get("slug"))
	if err != nil {
		t.Fatalf("created post not found by slug: %v", err)
	}
	if createdPost.ID != postID {
		t.Fatalf("created post id mismatch: got=%d want=%d", createdPost.ID, postID)
	}
	if createdPost.Status != "published" {
		t.Fatalf("created post status = %q, want published", createdPost.Status)
	}
	if createdPost.DeletedAt != nil {
		t.Fatalf("created post should not be deleted")
	}

	editPath := fmt.Sprintf("/dash/posts/%d/edit", postID)
	csrfToken, cookieKV, editPageBody := fetchCSRFToken(
		t,
		swv,
		editPath,
		cookieKV,
		`id="post-title"`,
		`name="action" value="update"`,
		baseForm.Get("title"),
	)
	if !strings.Contains(editPageBody, baseForm.Get("slug")) {
		t.Fatalf("edit page should include post slug")
	}
	if !strings.Contains(editPageBody, `class="post-editor-layout"`) {
		t.Fatalf("edit page should render shared editor layout shell")
	}
	updateForm := url.Values{}
	updateForm.Set("title", "P0 Controller Post Updated")
	updateForm.Set("content", "controller p0 test content updated")
	updateForm.Set("action", "update")
	updateForm.Set("kind", "0")
	updateForm.Set("status", "published")
	updateForm.Set("comment_enabled", "1")

	resp = requestControllerP0(t, swv, fiber.MethodPost, editPath, updateForm, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if resp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected update post redirect 303, got %d", resp.StatusCode)
	}

	updatedPost, err := db.GetPostByIDAnyStatus(swv.Store.Model, postID)
	if err != nil {
		t.Fatalf("updated post not found: %v", err)
	}
	if updatedPost.Title != updateForm.Get("title") {
		t.Fatalf("updated post title mismatch: got=%q want=%q", updatedPost.Title, updateForm.Get("title"))
	}
	if updatedPost.Status != "published" {
		t.Fatalf("updated post status = %q, want published", updatedPost.Status)
	}

	csrfToken, cookieKV, updatedEditPageBody := fetchCSRFToken(
		t,
		swv,
		editPath,
		cookieKV,
		`id="post-title"`,
		updateForm.Get("title"),
		`name="action" value="update"`,
	)
	if !strings.Contains(updatedEditPageBody, "post-editor-word-count") {
		t.Fatalf("edit page should include editor status toolbar")
	}
	deletePath := fmt.Sprintf("/dash/posts/%d/delete", postID)
	resp = requestControllerP0(t, swv, fiber.MethodPost, deletePath, url.Values{}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected delete post redirect, got %d", resp.StatusCode)
	}

	deletedPosts, err := db.ListDeletedPosts(swv.Store.Model)
	if err != nil {
		t.Fatalf("list deleted posts failed: %v", err)
	}
	foundDeleted := false
	for _, item := range deletedPosts {
		if item.ID != postID {
			continue
		}
		foundDeleted = true
		if item.DeletedAt == nil {
			t.Fatalf("expected deleted post deleted_at to be set")
		}
		break
	}
	if !foundDeleted {
		t.Fatalf("expected deleted post id=%d to appear in trash list", postID)
	}
}

func TestDashControllerP0_ThemeLifecycle(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	defaultTheme, err := db.GetThemeByCode(swv.Store.Model, db.DefaultThemeCode)
	if err != nil {
		t.Fatalf("GetThemeByCode(default theme) failed: %v", err)
	}

	entryResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/themes/default", nil, cookieKV, nil)
	if entryResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected theme entry redirect 303, got %d", entryResp.StatusCode)
	}
	entryLocation := strings.TrimSpace(entryResp.Header.Get("Location"))
	entryURL, err := url.Parse(entryLocation)
	if err != nil {
		t.Fatalf("parse theme entry redirect failed: %v", err)
	}
	if entryURL.Path != fmt.Sprintf("/dash/themes/%d", defaultTheme.ID) {
		t.Fatalf("unexpected theme entry redirect path: %q", entryLocation)
	}
	if entryURL.Query().Get("file") != "home.html" {
		t.Fatalf("unexpected theme entry redirect file query: %q", entryLocation)
	}

	listResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/themes", nil, cookieKV, nil)
	assertTemplateRendered(t, listResp, fiber.StatusOK, "主题", "新建主题", "tuft")

	csrfToken, cookieKV, newPageBody := fetchCSRFToken(
		t,
		swv,
		"/dash/themes/new",
		cookieKV,
		`id="theme-name"`,
		`id="theme-code"`,
		`id="form" method="post"`,
	)
	if !strings.Contains(newPageBody, `name="description"`) {
		t.Fatalf("theme new page should include description field")
	}

	createForm := url.Values{}
	createForm.Set("name", "Controller Theme")
	createForm.Set("code", fmt.Sprintf("controller-theme-%d", time.Now().UnixNano()))
	createForm.Set("author", "keelii")
	createForm.Set("description", "theme from controller p0 test")
	createForm.Set("_csrf_token", csrfToken)

	createResp := requestControllerP0(t, swv, fiber.MethodPost, "/dash/themes", createForm, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if createResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected create theme redirect 303, got %d", createResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, createResp)

	location := strings.TrimSpace(createResp.Header.Get("Location"))
	assertTemplateRendered(t, requestControllerP0(t, swv, fiber.MethodGet, location, nil, cookieKV, nil), fiber.StatusOK, "主题已创建。", `id="theme-current-content"`)
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse create theme redirect failed: %v", err)
	}
	if redirectURL.Path == "" || !strings.HasPrefix(redirectURL.Path, "/dash/themes/") {
		t.Fatalf("unexpected create theme redirect path: %q", location)
	}
	themeID, err := strconv.ParseInt(strings.TrimPrefix(redirectURL.Path, "/dash/themes/"), 10, 64)
	if err != nil {
		t.Fatalf("parse theme id from redirect failed: %v", err)
	}
	if redirectURL.Query().Get("file") != "home.html" {
		t.Fatalf("expected create theme redirect file query to be home.html, got %q", redirectURL.Query().Get("file"))
	}

	createdTheme, err := db.GetThemeByID(swv.Store.Model, themeID)
	if err != nil {
		t.Fatalf("GetThemeByID(created) failed: %v", err)
	}
	if createdTheme.Name != createForm.Get("name") || createdTheme.Author != createForm.Get("author") {
		t.Fatalf("unexpected created theme: %+v", createdTheme)
	}
	var createdFiles map[string]string
	if err := json.Unmarshal([]byte(createdTheme.Files), &createdFiles); err != nil {
		t.Fatalf("decode created theme files failed: %v", err)
	}
	var defaultFiles map[string]string
	if err := json.Unmarshal([]byte(defaultTheme.Files), &defaultFiles); err != nil {
		t.Fatalf("decode default theme files failed: %v", err)
	}
	if createdFiles["home.html"] == "" {
		t.Fatal("expected created theme files to include home.html")
	}
	if createdFiles["post.html"] == "" {
		t.Fatal("expected created theme files to include post.html")
	}
	if createdFiles["layout_main.html"] != defaultFiles["layout_main.html"] {
		t.Fatal("expected created theme to copy default theme layout")
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	copyResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/copy", themeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if copyResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected copy theme redirect 303, got %d", copyResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, copyResp)
	copyLocation := strings.TrimSpace(copyResp.Header.Get("Location"))
	copyURL, err := url.Parse(copyLocation)
	if err != nil {
		t.Fatalf("parse copy theme redirect failed: %v", err)
	}
	copiedThemeID, err := strconv.ParseInt(strings.TrimPrefix(copyURL.Path, "/dash/themes/"), 10, 64)
	if err != nil {
		t.Fatalf("parse copied theme id from redirect failed: %v", err)
	}
	copiedTheme, err := db.GetThemeByID(swv.Store.Model, copiedThemeID)
	if err != nil {
		t.Fatalf("GetThemeByID(copied) failed: %v", err)
	}
	if copiedTheme.Name != "Controller Theme 副本" {
		t.Fatalf("unexpected copied theme name: %q", copiedTheme.Name)
	}
	if copiedTheme.Code != createForm.Get("code")+"-copy" {
		t.Fatalf("unexpected copied theme code: %q", copiedTheme.Code)
	}
	if copiedTheme.Files != createdTheme.Files || copiedTheme.CurrentFile != createdTheme.CurrentFile {
		t.Fatalf("copied theme should keep files/current_file: %+v", copiedTheme)
	}
	if copiedTheme.IsCurrent != 0 || copiedTheme.IsBuiltin != 0 {
		t.Fatalf("copied theme should not be current/builtin: %+v", copiedTheme)
	}

	exportResp := requestControllerP0(t, swv, fiber.MethodGet, fmt.Sprintf("/dash/themes/%d/export", themeID), nil, cookieKV, nil)
	if exportResp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected export theme status 200, got %d", exportResp.StatusCode)
	}
	if got := strings.TrimSpace(exportResp.Header.Get("Content-Type")); !strings.Contains(got, "application/json") {
		t.Fatalf("unexpected export content-type: %q", got)
	}
	exportBody, err := io.ReadAll(exportResp.Body)
	if err != nil {
		t.Fatalf("read export theme body failed: %v", err)
	}
	var exportedTheme struct {
		Name  string            `json:"name"`
		Code  string            `json:"code"`
		Files map[string]string `json:"files"`
	}
	if err := json.Unmarshal(exportBody, &exportedTheme); err != nil {
		t.Fatalf("decode export theme payload failed: %v", err)
	}
	if exportedTheme.Code != createdTheme.Code || exportedTheme.Files["home.html"] == "" {
		t.Fatalf("unexpected exported theme payload: %+v", exportedTheme)
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	var importBody bytes.Buffer
	importWriter := multipart.NewWriter(&importBody)
	importPart, err := importWriter.CreateFormFile("file", "theme.json")
	if err != nil {
		t.Fatalf("create import form file failed: %v", err)
	}
	if _, err := importPart.Write(exportBody); err != nil {
		t.Fatalf("write import json failed: %v", err)
	}
	if err := importWriter.Close(); err != nil {
		t.Fatalf("close import writer failed: %v", err)
	}
	importReq := httptest.NewRequest(fiber.MethodPost, "/dash/themes/import", &importBody)
	importReq.Header.Set("Content-Type", importWriter.FormDataContentType())
	importReq.Header.Set("Cookie", cookieKV)
	importReq.Header.Set("X-CSRF-Token", csrfToken)
	importResp, err := swv.App.Test(importReq)
	if err != nil {
		t.Fatalf("import theme request failed: %v", err)
	}
	if importResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected import theme redirect 303, got %d", importResp.StatusCode)
	}
	importLocation := strings.TrimSpace(importResp.Header.Get("Location"))
	importURL, err := url.Parse(importLocation)
	if err != nil {
		t.Fatalf("parse import theme redirect failed: %v", err)
	}
	importedThemeID, err := strconv.ParseInt(strings.TrimPrefix(importURL.Path, "/dash/themes/"), 10, 64)
	if err != nil {
		t.Fatalf("parse imported theme id failed: %v", err)
	}
	importedTheme, err := db.GetThemeByID(swv.Store.Model, importedThemeID)
	if err != nil {
		t.Fatalf("GetThemeByID(imported) failed: %v", err)
	}
	if importedTheme.Name != "Controller Theme 副本 2" {
		t.Fatalf("unexpected imported theme name: %q", importedTheme.Name)
	}
	if importedTheme.Code != createForm.Get("code")+"-copy-2" {
		t.Fatalf("unexpected imported theme code: %q", importedTheme.Code)
	}
	if importedTheme.Files == "" || importedTheme.CurrentFile != "home.html" {
		t.Fatalf("unexpected imported theme payload: %+v", importedTheme)
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	deleteResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/delete", copiedThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if deleteResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected delete theme redirect 303, got %d", deleteResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, deleteResp)
	if _, err := db.GetThemeByID(swv.Store.Model, copiedThemeID); !db.IsErrNotFound(err) {
		t.Fatalf("expected copied theme deleted, got err=%v", err)
	}
	deletedThemes, err := db.ListDeletedThemes(swv.Store.Model)
	if err != nil {
		t.Fatalf("ListDeletedThemes failed: %v", err)
	}
	foundDeletedTheme := false
	for _, item := range deletedThemes {
		if item.ID != copiedThemeID {
			continue
		}
		foundDeletedTheme = true
		if item.DeletedAt == nil {
			t.Fatalf("expected deleted theme deleted_at to be set")
		}
		break
	}
	if !foundDeletedTheme {
		t.Fatalf("expected deleted theme id=%d to appear in trash list", copiedThemeID)
	}

	trashResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/trash?type=themes", nil, cookieKV, nil)
	assertTemplateRendered(t, trashResp, fiber.StatusOK, "已删除主题列表", copiedTheme.Name, copiedTheme.Code)

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/trash?type=themes", cookieKV, "trash-table")
	restoreResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/trash/themes/%d/restore", copiedThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
		"Referer":      "/dash/trash?type=themes",
	})
	if restoreResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected restore theme redirect 303, got %d", restoreResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, restoreResp)
	restoredCopiedTheme, err := db.GetThemeByID(swv.Store.Model, copiedThemeID)
	if err != nil {
		t.Fatalf("GetThemeByID(restored copied theme) failed: %v", err)
	}
	if restoredCopiedTheme.DeletedAt != nil {
		t.Fatalf("expected restored copied theme deleted_at to be nil, got %+v", restoredCopiedTheme.DeletedAt)
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	deleteAgainResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/delete", copiedThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if deleteAgainResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected second delete theme redirect 303, got %d", deleteAgainResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, deleteAgainResp)

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/trash?type=themes", cookieKV, "trash-table")
	hardDeleteResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/trash/themes/%d/delete", copiedThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
		"Referer":      "/dash/trash?type=themes",
	})
	if hardDeleteResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected hard delete theme redirect 303, got %d", hardDeleteResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, hardDeleteResp)
	if _, err := db.GetThemeByID(swv.Store.Model, copiedThemeID); !db.IsErrNotFound(err) {
		t.Fatalf("expected copied theme hard deleted, got err=%v", err)
	}
	deletedThemes, err = db.ListDeletedThemes(swv.Store.Model)
	if err != nil {
		t.Fatalf("ListDeletedThemes after hard delete failed: %v", err)
	}
	for _, item := range deletedThemes {
		if item.ID == copiedThemeID {
			t.Fatalf("expected copied theme to be removed from trash after hard delete")
		}
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	deleteCurrentResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/delete", defaultTheme.ID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if deleteCurrentResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected delete current theme redirect 303, got %d", deleteCurrentResp.StatusCode)
	}
	currentThemeStill, err := db.GetThemeByID(swv.Store.Model, defaultTheme.ID)
	if err != nil {
		t.Fatalf("GetThemeByID(default current) failed: %v", err)
	}
	if currentThemeStill.IsCurrent != 1 {
		t.Fatalf("current theme should not be deleted: %+v", currentThemeStill)
	}

	editPath := fmt.Sprintf("/dash/themes/%d?file=home.html", themeID)
	csrfToken, cookieKV, editPageBody := fetchCSRFToken(
		t,
		swv,
		editPath,
		cookieKV,
		`id="theme-current-content"`,
	)
	if !strings.Contains(editPageBody, "暂无文章") {
		t.Fatalf("theme edit page should show current file content")
	}

	updateForm := url.Values{}
	updateForm.Set("name", "Controller Theme Updated")
	updateForm.Set("author", "tester")
	updateForm.Set("status", "published")
	updateForm.Set("description", "updated description")
	updateForm.Set("current_file", "home.html")
	updateForm.Set("current_content", "<h1>updated theme</h1>")
	updateForm.Set("version", strconv.FormatInt(createdTheme.Version, 10))
	updateForm.Set("_csrf_token", csrfToken)

	updateResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d", themeID), updateForm, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if updateResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected update theme redirect 303, got %d", updateResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, updateResp)

	updatedTheme, err := db.GetThemeByID(swv.Store.Model, themeID)
	if err != nil {
		t.Fatalf("GetThemeByID(updated) failed: %v", err)
	}
	if updatedTheme.Name != updateForm.Get("name") || updatedTheme.Author != updateForm.Get("author") {
		t.Fatalf("unexpected updated theme metadata: %+v", updatedTheme)
	}
	if updatedTheme.CurrentFile != "home.html" || updatedTheme.Version != createdTheme.Version+1 {
		t.Fatalf("unexpected updated theme state: %+v", updatedTheme)
	}
	if updatedTheme.Status != "published" {
		t.Fatalf("updated theme status = %q, want published", updatedTheme.Status)
	}
	var updatedFiles map[string]string
	if err := json.Unmarshal([]byte(updatedTheme.Files), &updatedFiles); err != nil {
		t.Fatalf("decode updated theme files failed: %v", err)
	}
	if updatedFiles["home.html"] != "<h1>updated theme</h1>" {
		t.Fatalf("updated home.html content mismatch: %q", updatedFiles["home.html"])
	}
	if updatedFiles["post.html"] == "" {
		t.Fatal("expected update to keep other theme files")
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(
		t,
		swv,
		fmt.Sprintf("/dash/themes/%d?file=home.html", themeID),
		cookieKV,
		`theme-new-file-trigger`,
	)
	createFileForm := url.Values{}
	createFileForm.Set("name", updatedTheme.Name)
	createFileForm.Set("author", updatedTheme.Author)
	createFileForm.Set("status", updatedTheme.Status)
	createFileForm.Set("description", updatedTheme.Description)
	createFileForm.Set("current_file", "home.html")
	createFileForm.Set("current_content", updatedFiles["home.html"])
	createFileForm.Set("new_file_path", "inc_footer.html")
	createFileForm.Set("version", strconv.FormatInt(updatedTheme.Version, 10))
	createFileForm.Set("_csrf_token", csrfToken)

	createFileResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d", themeID), createFileForm, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if createFileResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected create theme file redirect 303, got %d", createFileResp.StatusCode)
	}

	themeAfterNewFile, err := db.GetThemeByID(swv.Store.Model, themeID)
	if err != nil {
		t.Fatalf("GetThemeByID(after new file) failed: %v", err)
	}
	var filesAfterNewFile map[string]string
	if err := json.Unmarshal([]byte(themeAfterNewFile.Files), &filesAfterNewFile); err != nil {
		t.Fatalf("decode files after new file failed: %v", err)
	}
	if _, ok := filesAfterNewFile["inc_footer.html"]; !ok {
		t.Fatal("expected newly created theme file to exist")
	}
	if themeAfterNewFile.CurrentFile != "inc_footer.html" {
		t.Fatalf("expected current file switch to new file, got %q", themeAfterNewFile.CurrentFile)
	}

	draftThemeID, err := db.CreateTheme(swv.Store.Model, &db.Theme{
		Name:        "Draft Theme",
		Code:        fmt.Sprintf("draft-theme-%d", time.Now().UnixNano()),
		Description: "draft",
		Author:      "tester",
		Files:       `{"home.html":"<h1>draft</h1>"}`,
		CurrentFile: "home.html",
		Status:      "draft",
		Version:     1,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("CreateTheme(draft) failed: %v", err)
	}

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	draftSetCurrentResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/set-current", draftThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if draftSetCurrentResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected draft set current redirect 303, got %d", draftSetCurrentResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, draftSetCurrentResp)
	currentAfterDraftAttempt, err := db.GetCurrentTheme(swv.Store.Model)
	if err != nil {
		t.Fatalf("GetCurrentTheme after draft attempt failed: %v", err)
	}
	if currentAfterDraftAttempt.ID == draftThemeID {
		t.Fatalf("draft theme should not become current: %+v", currentAfterDraftAttempt)
	}
	draftListResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/themes", nil, cookieKV, nil)
	assertTemplateRendered(t, draftListResp, fiber.StatusOK, "草稿主题不能应用为当前主题。", "Draft Theme")

	secondThemeID, err := db.CreateTheme(swv.Store.Model, &db.Theme{
		Name:        "Second Theme",
		Code:        fmt.Sprintf("second-theme-%d", time.Now().UnixNano()),
		Description: "second",
		Author:      "tester",
		Files:       `{"home.html":"<h1>second</h1>"}`,
		CurrentFile: "home.html",
		Status:      "published",
		Version:     1,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	})
	if err != nil {
		t.Fatalf("CreateTheme(second) failed: %v", err)
	}

	withControllerP0WorkingDir(t, t.TempDir())

	csrfToken, cookieKV, _ = fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	setCurrentResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/set-current", secondThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if setCurrentResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected set current theme redirect 303, got %d", setCurrentResp.StatusCode)
	}

	currentTheme, err := db.GetCurrentTheme(swv.Store.Model)
	if err != nil {
		t.Fatalf("GetCurrentTheme failed: %v", err)
	}
	if currentTheme.ID != secondThemeID || currentTheme.IsCurrent != 1 {
		t.Fatalf("unexpected current theme after switch attempt in reload mode: %+v", currentTheme)
	}

	updatedListResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/themes", nil, cookieKV, nil)
	assertTemplateRendered(t, updatedListResp, fiber.StatusOK, "当前主题已更新；当前运行实例未自动重载，请手动重启服务后生效。", "Second Theme", "Controller Theme Updated")
}

func TestDashControllerP0_ThemeSetCurrentShowsManualRestartNoticeWhenHotReloadUnavailable(t *testing.T) {
	withControllerP0TemplateReload(t, false)

	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	nowUnix := time.Now().Unix()
	nextThemeID, err := db.CreateTheme(swv.Store.Model, &db.Theme{
		Name:        "Manual Restart Theme",
		Code:        fmt.Sprintf("manual-restart-theme-%d", nowUnix),
		Description: "requires manual restart",
		Author:      "tester",
		Files:       `{"home.html":"<h1>manual restart</h1>"}`,
		CurrentFile: "home.html",
		Status:      "published",
		Version:     1,
		CreatedAt:   nowUnix,
		UpdatedAt:   nowUnix,
	})
	if err != nil {
		t.Fatalf("CreateTheme failed: %v", err)
	}

	withControllerP0WorkingDir(t, t.TempDir())

	csrfToken, cookieKV, _ := fetchCSRFToken(t, swv, "/dash/themes", cookieKV, "themes-table")
	setCurrentResp := requestControllerP0(t, swv, fiber.MethodPost, fmt.Sprintf("/dash/themes/%d/set-current", nextThemeID), url.Values{
		"_csrf_token": []string{csrfToken},
	}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
	})
	if setCurrentResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("expected set current theme redirect 303, got %d", setCurrentResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, setCurrentResp)

	currentTheme, err := db.GetCurrentTheme(swv.Store.Model)
	if err != nil {
		t.Fatalf("GetCurrentTheme failed: %v", err)
	}
	if currentTheme.ID != nextThemeID {
		t.Fatalf("unexpected current theme after switch without active runtime: %+v", currentTheme)
	}

	listResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/themes", nil, cookieKV, nil)
	assertTemplateRendered(t, listResp, fiber.StatusOK, "当前主题已更新；当前运行实例未自动重载，请手动重启服务后生效。", "Manual Restart Theme")
}

func TestDashControllerP0_DeletePostKeepsCurrentListQuery(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	nowUnix := time.Now().Unix()
	postID, err := db.CreatePost(swv.Store.Model, &db.Post{
		Title:     fmt.Sprintf("P0 Keep Query Page %d", nowUnix),
		Slug:      fmt.Sprintf("p0-keep-query-page-%d", nowUnix),
		Content:   "delete keep query",
		Status:    "published",
		Kind:      db.PostKindPage,
		CreatedAt: nowUnix,
		UpdatedAt: nowUnix,
	})
	if err != nil {
		t.Fatalf("create page post failed: %v", err)
	}

	csrfToken, cookieKV, _ := fetchCSRFToken(t, swv, "/dash/posts?kind=1&page=2&q=keep-query", cookieKV)
	deletePath := fmt.Sprintf("/dash/posts/%d/delete", postID)
	resp := requestControllerP0(t, swv, fiber.MethodPost, deletePath, url.Values{}, cookieKV, map[string]string{
		"X-CSRF-Token": csrfToken,
		"Referer":      "/dash/posts?kind=1&page=2&q=keep-query",
	})
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected delete post redirect, got %d", resp.StatusCode)
	}

	location := strings.TrimSpace(resp.Header.Get("Location"))
	redirectURL, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parse redirect location failed: %v location=%q", err, location)
	}
	if redirectURL.Path != "/dash/posts" {
		t.Fatalf("delete redirect should stay on posts list, got path=%q", redirectURL.Path)
	}
	query := redirectURL.Query()
	if query.Get("kind") != "1" {
		t.Fatalf("delete redirect should keep kind query, got kind=%q location=%q", query.Get("kind"), location)
	}
	if query.Get("page") != "2" {
		t.Fatalf("delete redirect should keep page query, got page=%q location=%q", query.Get("page"), location)
	}
	if query.Get("q") != "keep-query" {
		t.Fatalf("delete redirect should keep search query, got q=%q location=%q", query.Get("q"), location)
	}
}

func TestDashControllerP0_PageListCreateButtonAndNewPageDefaultKind(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)

	listResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/posts?kind=1", nil, cookieKV, nil)
	listBody := assertTemplateRendered(t, listResp, fiber.StatusOK, "新建页面")
	if !strings.Contains(listBody, `posts&#x2f;new?kind=1`) {
		t.Fatalf("page list should link create button to kind=1 new page, body=%q", listBody)
	}

	newResp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/posts/new?kind=1", nil, cookieKV, nil)
	newBody := assertTemplateRendered(t, newResp, fiber.StatusOK, `name="kind" value="1"`)
	if !strings.Contains(newBody, "New Page") {
		t.Fatalf("page create view should render page title, body=%q", newBody)
	}
	if strings.Contains(newBody, `name="kind" value="0"`) {
		t.Fatalf("page create view should not fall back to post kind, body=%q", newBody)
	}
}

func TestDashControllerP0_PostEditKeepsServerRenderedTOC(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	nowUnix := time.Now().Unix()
	content := "# 控制器测试目录\n\n## 二级标题\n\n正文"
	postID, err := db.CreatePost(swv.Store.Model, &db.Post{
		Title:     fmt.Sprintf("P0 TOC Post %d", nowUnix),
		Slug:      fmt.Sprintf("p0-toc-post-%d", nowUnix),
		Content:   content,
		Status:    "published",
		Kind:      db.PostKindPost,
		CreatedAt: nowUnix,
		UpdatedAt: nowUnix,
	})
	if err != nil {
		t.Fatalf("create post failed: %v", err)
	}

	body := assertTemplateRendered(t, requestControllerP0(t, swv, fiber.MethodGet, fmt.Sprintf("/dash/posts/%d/edit", postID), nil, cookieKV, nil), fiber.StatusOK, `data-role="post-editor-toc-body"`)
	wantTOC := md.ParseMarkdownTOC(content)
	if !strings.Contains(body, wantTOC) {
		t.Fatalf("post edit page should include server-rendered toc, body=%q want=%q", body, wantTOC)
	}
}

func TestDashControllerP0_EncryptedPostEditKeepsServerRenderedTOC(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)
	content := "# 加密目录\n\n## 二级标题\n\n正文"
	expiresAt := time.Now().Add(2 * time.Hour).Unix()
	postID, err := db.CreateEncryptedPost(swv.Store.Model, &db.EncryptedPost{
		Title:     fmt.Sprintf("encrypted-toc-%d", time.Now().UnixNano()),
		Content:   content,
		Password:  "123456",
		ExpiresAt: &expiresAt,
	})
	if err != nil {
		t.Fatalf("create encrypted post failed: %v", err)
	}

	body := assertTemplateRendered(t, requestControllerP0(t, swv, fiber.MethodGet, fmt.Sprintf("/dash/encrypted-posts/%d/edit", postID), nil, cookieKV, nil), fiber.StatusOK, `data-role="post-editor-toc-body"`)
	wantTOC := md.ParseMarkdownTOC(content)
	if !strings.Contains(body, wantTOC) {
		t.Fatalf("encrypted edit page should include server-rendered toc, body=%q want=%q", body, wantTOC)
	}
}

func TestDashControllerP0_EncryptedPostEditorPages(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)

	_, cookieKV, newPageBody := fetchCSRFToken(
		t,
		swv,
		"/dash/encrypted-posts/new",
		cookieKV,
		`id="post-title"`,
		`id="post-content"`,
		`id="encrypted-password"`,
		`id="encrypted-expiry-option"`,
		`id="post-editor-word-count"`,
	)
	if !strings.Contains(newPageBody, `data-seditor-command="bold"`) {
		t.Fatalf("encrypted new page should include seditor toolbar actions")
	}
	if !strings.Contains(newPageBody, `class="post-editor-layout"`) {
		t.Fatalf("encrypted new page should render shared editor layout shell")
	}
	if strings.Contains(newPageBody, `class="post-editor-layout-meta"`) {
		t.Fatalf("encrypted new page should keep editor body aligned with normal post layout")
	}

	expiresAt := time.Now().Add(2 * time.Hour).Unix()
	encryptedPost := &db.EncryptedPost{
		Title:     fmt.Sprintf("encrypted-p0-%d", time.Now().UnixNano()),
		Content:   "secret content",
		Password:  "123456",
		ExpiresAt: &expiresAt,
	}
	postID, err := db.CreateEncryptedPost(swv.Store.Model, encryptedPost)
	if err != nil {
		t.Fatalf("create encrypted post failed: %v", err)
	}

	editPath := fmt.Sprintf("/dash/encrypted-posts/%d/edit", postID)
	_, cookieKV, editPageBody := fetchCSRFToken(
		t,
		swv,
		editPath,
		cookieKV,
		`id="post-title"`,
		`id="post-content"`,
		`id="encrypted-password"`,
		`id="encrypted-expiry-option"`,
		`id="encrypted-expiry-custom"`,
		`id="post-editor-word-count"`,
	)
	if !strings.Contains(editPageBody, encryptedPost.Slug) {
		t.Fatalf("encrypted edit page should include slug display")
	}
	if !strings.Contains(editPageBody, `value="custom" selected`) {
		t.Fatalf("encrypted edit page should preselect custom expiry for existing expires_at")
	}
	if !strings.Contains(editPageBody, `class="post-editor-layout"`) {
		t.Fatalf("encrypted edit page should render shared editor layout shell")
	}
	if strings.Contains(editPageBody, `class="post-editor-layout-meta"`) {
		t.Fatalf("encrypted edit page should keep editor body aligned with normal post layout")
	}
}

func TestDashControllerP0_NotificationsListRendersFields(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)

	nowUnix := time.Now().Unix()
	item := &db.Notification{
		Receiver:       db.NotificationReceiverDash,
		EventType:      db.NotificationEventPostLike,
		Level:          db.NotificationLevelInfo,
		Title:          "控制器测试通知标题",
		Body:           "控制器测试通知内容",
		AggregateKey:   fmt.Sprintf("controller-test:%d", nowUnix),
		AggregateCount: 7,
		CreatedAt:      nowUnix,
		UpdatedAt:      nowUnix,
	}
	id, err := db.CreateNotification(swv.Store.Model, item)
	if err != nil {
		t.Fatalf("create notification failed: %v", err)
	}

	body := assertTemplateRendered(
		t,
		requestControllerP0(t, swv, fiber.MethodGet, "/dash/notifications", nil, cookieKV, nil),
		fiber.StatusOK,
		"通知中心",
		"控制器测试通知标题",
		"控制器测试通知内容",
		"点赞",
		"未读",
		fmt.Sprintf(`data-notification-id="%d"`, id),
	)
	if !strings.Contains(body, ">7<") {
		t.Fatalf("notifications page should render aggregate count")
	}
}

func TestDashControllerP0_NotificationsUnreadCountAPIHeaders(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsDash(t, swv)

	nowUnix := time.Now().Unix()
	if _, err := db.CreateNotification(swv.Store.Model, &db.Notification{
		Receiver:     db.NotificationReceiverDash,
		EventType:    db.NotificationEventComment,
		Level:        db.NotificationLevelInfo,
		Title:        "未读通知",
		Body:         "用于测试 unread_count API",
		AggregateKey: fmt.Sprintf("notification-unread-api:%d", nowUnix),
		CreatedAt:    nowUnix,
		UpdatedAt:    nowUnix,
	}); err != nil {
		t.Fatalf("create notification failed: %v", err)
	}

	resp := requestControllerP0(t, swv, fiber.MethodGet, "/dash/api/notifications/unread_count", nil, cookieKV, map[string]string{
		"Accept": "application/json",
	})
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected unread count api status 200, got %d", resp.StatusCode)
	}

	if cacheControl := strings.TrimSpace(resp.Header.Get("Cache-Control")); !strings.Contains(cacheControl, "no-store") {
		t.Fatalf("unread count api should disable cache, got Cache-Control=%q", cacheControl)
	}

	body := readResponseBody(t, resp)
	var payload struct {
		OK   bool `json:"ok"`
		Data struct {
			UnreadCount int `json:"unread_count"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode unread count api failed: %v body=%q", err, body)
	}
	if !payload.OK {
		t.Fatalf("unread count api should return ok=true body=%q", body)
	}
	if payload.Data.UnreadCount < 1 {
		t.Fatalf("unread count api should include unread notification, got %d", payload.Data.UnreadCount)
	}
}
