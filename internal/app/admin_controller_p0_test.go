package app

import (
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
var postEditPathPattern = regexp.MustCompile(`/admin/posts/([0-9]+)/edit`)

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

func fetchCSRFToken(t *testing.T, swv SwavesApp, path string, cookieKV string) (string, string) {
	t.Helper()

	resp := requestControllerP0(t, swv, fiber.MethodGet, path, nil, cookieKV, nil)
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected csrf page status: path=%s status=%d", path, resp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, resp)

	body, _ := io.ReadAll(resp.Body)
	token := extractCSRFToken(body)
	if token == "" {
		t.Fatalf("csrf token missing: path=%s", path)
	}
	return token, cookieKV
}

func loginAsAdmin(t *testing.T, swv SwavesApp) string {
	t.Helper()

	csrfToken, cookieKV := fetchCSRFToken(t, swv, "/admin/login", "")
	form := url.Values{}
	form.Set("password", "admin")
	form.Set("_csrf_token", csrfToken)
	resp := requestControllerP0(t, swv, fiber.MethodPost, "/admin/login", form, cookieKV, nil)
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected login redirect status, got %d", resp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, resp)
	if cookieKV == "" {
		t.Fatalf("expected login session cookie")
	}
	return cookieKV
}

func TestAdminControllerP0_ProtectedRouteRequiresLogin(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	resp := requestControllerP0(t, swv, fiber.MethodGet, "/admin/posts", nil, "", nil)
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		t.Fatalf("expected redirect for unauthenticated route, got %d", resp.StatusCode)
	}
	location := strings.TrimSpace(resp.Header.Get("Location"))
	want := "/admin/login?returnUrl=%2Fadmin%2Fposts"
	if location != want {
		t.Fatalf("unexpected redirect location: got=%q want=%q", location, want)
	}
}

func TestAdminControllerP0_LoginReturnURLValidation(t *testing.T) {
	t.Run("allow internal return url", func(t *testing.T) {
		swv := newControllerP0TestApp(t)
		defer swv.Shutdown()

		csrfToken, cookieKV := fetchCSRFToken(t, swv, "/admin/login", "")
		form := url.Values{}
		form.Set("password", "admin")
		form.Set("returnUrl", "/admin/posts")
		form.Set("_csrf_token", csrfToken)

		resp := requestControllerP0(t, swv, fiber.MethodPost, "/admin/login", form, cookieKV, nil)
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			t.Fatalf("expected login redirect status, got %d", resp.StatusCode)
		}
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if location != "/admin/posts" {
			t.Fatalf("unexpected safe return redirect: got=%q want=%q", location, "/admin/posts")
		}
	})

	t.Run("reject unsafe return url", func(t *testing.T) {
		swv := newControllerP0TestApp(t)
		defer swv.Shutdown()

		csrfToken, cookieKV := fetchCSRFToken(t, swv, "/admin/login", "")
		form := url.Values{}
		form.Set("password", "admin")
		form.Set("returnUrl", "//evil.test/phish")
		form.Set("_csrf_token", csrfToken)

		resp := requestControllerP0(t, swv, fiber.MethodPost, "/admin/login", form, cookieKV, nil)
		if resp.StatusCode < 300 || resp.StatusCode >= 400 {
			t.Fatalf("expected login redirect status, got %d", resp.StatusCode)
		}
		location := strings.TrimSpace(resp.Header.Get("Location"))
		if strings.Contains(location, "evil.test") {
			t.Fatalf("unsafe return url should be rejected, got location=%q", location)
		}
		if !strings.HasPrefix(location, "/admin") {
			t.Fatalf("fallback redirect should stay in admin namespace, got=%q", location)
		}
	})
}

func TestAdminControllerP0_PostLifecycle(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	cookieKV := loginAsAdmin(t, swv)
	createPath := "/admin/posts/new"

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

	csrfToken, cookieKV := fetchCSRFToken(t, swv, createPath, cookieKV)
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

	editPath := fmt.Sprintf("/admin/posts/%d/edit", postID)
	csrfToken, cookieKV = fetchCSRFToken(t, swv, editPath, cookieKV)
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

	csrfToken, cookieKV = fetchCSRFToken(t, swv, editPath, cookieKV)
	deletePath := fmt.Sprintf("/admin/posts/%d/delete", postID)
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
