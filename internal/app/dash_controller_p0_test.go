package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
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
	return NewApp(types.AppConfig{
		SqliteFile: dbPath,
		ListenAddr: ":0",
		AppName:    "swaves-test",
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
	req := httptest.NewRequest(method, path, body)
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
