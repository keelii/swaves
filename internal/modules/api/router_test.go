package api

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
)

func TestMarkdownTOCReturnsOnlyTOCHTML(t *testing.T) {
	app := fiber.New()
	RegisterRouter(app)

	body := `{"content":"# 标题\n\n## 小节\n\n正文"}`
	req := httptest.NewRequest(fiber.MethodPost, "/api/markdown/toc", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	raw, _ := io.ReadAll(resp.Body)
	var payload struct {
		Data string `json:"data"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if !strings.Contains(payload.Data, `class="toc"`) {
		t.Fatalf("expected toc html, got: %s", payload.Data)
	}
	if strings.Contains(payload.Data, "<h1") {
		t.Fatalf("expected toc-only html, got: %s", payload.Data)
	}
}

func TestMarkdownTOCRejectsInvalidJSON(t *testing.T) {
	app := fiber.New()
	RegisterRouter(app)

	req := httptest.NewRequest(fiber.MethodPost, "/api/markdown/toc", strings.NewReader("{bad"))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("expected 400, got %d", resp.StatusCode)
	}
}
