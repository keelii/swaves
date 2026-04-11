package dash

import (
	"io"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strings"
	"swaves/internal/platform/view"
	"swaves/internal/shared/types"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

func dashTemplateRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "web", "templates")
}

func TestRedirectFlashMessagesAreStoredInSession(t *testing.T) {
	viewEngine, initURLResolver := view.NewViewEngine(dashTemplateRoot(t), false)
	fv, ok := viewEngine.(*view.FiberView)
	if !ok {
		t.Fatal("view engine should be *view.FiberView")
	}
	if err := fv.Load(); err != nil {
		t.Fatalf("load templates failed: %v", err)
	}

	app := fiber.New(fiber.Config{Views: viewEngine})
	sessionStore := &types.SessionStore{Store: session.NewStore()}
	handler := &Handler{Session: sessionStore}

	app.Use(func(c fiber.Ctx) error {
		fiber.Locals(c, "DashSessionStore", sessionStore)
		fiber.Locals(c, "IsLogin", false)
		fiber.Locals(c, "CsrfToken", "test-csrf-token")
		return c.Next()
	})

	app.Get("/source", func(c fiber.Ctx) error {
		return handler.redirectToDashRoute(c, "dash.login.show", nil, map[string]string{
			"error": "flash error",
			"page":  "2",
		})
	})
	app.Get("/dash/login", func(c fiber.Ctx) error {
		return RenderDashView(c, "dash/dash_login.html", fiber.Map{
			"Title": "登录",
		}, "")
	}).Name("dash.login.show")
	app.Post("/dash/login", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusNoContent)
	}).Name("dash.login.submit")
	initURLResolver(app)

	firstReq := httptest.NewRequest(fiber.MethodGet, "/source", nil)
	firstResp, err := app.Test(firstReq)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if firstResp.StatusCode != fiber.StatusSeeOther {
		t.Fatalf("status=%d, want %d", firstResp.StatusCode, fiber.StatusSeeOther)
	}
	location := firstResp.Header.Get("Location")
	if strings.Contains(location, "error=") {
		t.Fatalf("redirect location should not contain error query: %s", location)
	}
	if !strings.Contains(location, "page=2") {
		t.Fatalf("redirect location should keep normal query: %s", location)
	}
	cookie := firstResp.Header.Get("Set-Cookie")
	if cookie == "" {
		t.Fatal("expected session cookie after redirect flash")
	}

	secondReq := httptest.NewRequest(fiber.MethodGet, location, nil)
	secondReq.Header.Set("Cookie", cookie)
	secondResp, err := app.Test(secondReq)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	secondBody, _ := io.ReadAll(secondResp.Body)
	if !strings.Contains(string(secondBody), "flash error") {
		t.Fatalf("expected rendered page to contain flash error, got %s", string(secondBody))
	}

	thirdReq := httptest.NewRequest(fiber.MethodGet, "/dash/login", nil)
	thirdReq.Header.Set("Cookie", cookie)
	thirdResp, err := app.Test(thirdReq)
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	thirdBody, _ := io.ReadAll(thirdResp.Body)
	if strings.Contains(string(thirdBody), "flash error") {
		t.Fatalf("expected flash error to be consumed after one render, got %s", string(thirdBody))
	}
}
