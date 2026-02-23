package middleware

import (
	"io"
	"net/http/httptest"
	"net/url"
	"strings"
	"swaves/internal/shared/types"
	"testing"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/session"
)

func TestAdminCSRF_AllowsSafeMethodAndProvidesToken(t *testing.T) {
	app := fiber.New()
	store := &types.SessionStore{Store: session.NewStore()}
	app.Use(AdminCSRF(store))
	app.Get("/admin/login", func(c fiber.Ctx) error {
		return c.SendString(fiber.Locals[string](c, "CsrfToken"))
	})

	req := httptest.NewRequest(fiber.MethodGet, "/admin/login", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	token := strings.TrimSpace(string(body))
	if token == "" {
		t.Fatalf("csrf token should not be empty")
	}
}

func TestAdminCSRF_RejectsMissingToken(t *testing.T) {
	app := fiber.New()
	store := &types.SessionStore{Store: session.NewStore()}
	app.Use(AdminCSRF(store))
	app.Get("/admin/login", func(c fiber.Ctx) error {
		return c.SendString(fiber.Locals[string](c, "CsrfToken"))
	})
	app.Post("/admin/login", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	getReq := httptest.NewRequest(fiber.MethodGet, "/admin/login", nil)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	cookie := getResp.Header.Get("Set-Cookie")
	if cookie == "" {
		t.Fatalf("expected session cookie")
	}

	postReq := httptest.NewRequest(fiber.MethodPost, "/admin/login", strings.NewReader(""))
	postReq.Header.Set("Cookie", cookie)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postResp, err := app.Test(postReq)
	if err != nil {
		t.Fatalf("post request failed: %v", err)
	}
	if postResp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("expected 403, got %d", postResp.StatusCode)
	}
}

func TestAdminCSRF_AllowsValidFormToken(t *testing.T) {
	app := fiber.New()
	store := &types.SessionStore{Store: session.NewStore()}
	app.Use(AdminCSRF(store))
	app.Get("/admin/login", func(c fiber.Ctx) error {
		return c.SendString(fiber.Locals[string](c, "CsrfToken"))
	})
	app.Post("/admin/login", func(c fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	getReq := httptest.NewRequest(fiber.MethodGet, "/admin/login", nil)
	getResp, err := app.Test(getReq)
	if err != nil {
		t.Fatalf("get request failed: %v", err)
	}
	cookie := getResp.Header.Get("Set-Cookie")
	if cookie == "" {
		t.Fatalf("expected session cookie")
	}

	bodyBytes, _ := io.ReadAll(getResp.Body)
	token := strings.TrimSpace(string(bodyBytes))
	if token == "" {
		t.Fatalf("expected csrf token")
	}

	form := url.Values{}
	form.Set(AdminCSRFFormField, token)
	postReq := httptest.NewRequest(fiber.MethodPost, "/admin/login", strings.NewReader(form.Encode()))
	postReq.Header.Set("Cookie", cookie)
	postReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	postResp, err := app.Test(postReq)
	if err != nil {
		t.Fatalf("post request failed: %v", err)
	}
	if postResp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", postResp.StatusCode)
	}
}
