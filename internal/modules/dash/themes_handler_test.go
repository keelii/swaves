package dash

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/themefiles"

	"github.com/gofiber/fiber/v3"
)

func TestParseThemeFilesNormalizesLegacySitePaths(t *testing.T) {
	files, err := themefiles.ParseJSON(`{
		"site/home.html": "home",
		"site/include/nav.html": "nav",
		"site/layout/layout.html": "layout",
		"site/macro/content.html": "macro"
	}`)
	if err != nil {
		t.Fatalf("themefiles.ParseJSON failed: %v", err)
	}

	if files["home.html"] != "home" {
		t.Fatalf("home.html = %q, want %q", files["home.html"], "home")
	}
	if files["inc_nav.html"] != "nav" {
		t.Fatalf("inc_nav.html = %q, want %q", files["inc_nav.html"], "nav")
	}
	if files["layout_main.html"] != "layout" {
		t.Fatalf("layout_main.html = %q, want %q", files["layout_main.html"], "layout")
	}
	if files["macro_content.html"] != "macro" {
		t.Fatalf("macro_content.html = %q, want %q", files["macro_content.html"], "macro")
	}
}

func TestParseThemeFilesNormalizesCurrentThemePaths(t *testing.T) {
	files, err := themefiles.ParseJSON(`{
		"themes/tuft/home.html": "home",
		"themes/tuft/inc_nav.html": "nav",
		"themes/tuft/layout_main.html": "layout"
	}`)
	if err != nil {
		t.Fatalf("themefiles.ParseJSON failed: %v", err)
	}

	if files["home.html"] != "home" {
		t.Fatalf("home.html = %q, want %q", files["home.html"], "home")
	}
	if files["inc_nav.html"] != "nav" {
		t.Fatalf("inc_nav.html = %q, want %q", files["inc_nav.html"], "nav")
	}
	if files["layout_main.html"] != "layout" {
		t.Fatalf("layout_main.html = %q, want %q", files["layout_main.html"], "layout")
	}
}

func TestDuplicateThemeBuildsUniqueNameAndCode(t *testing.T) {
	source := db.Theme{
		Name:        "tuft",
		Code:        "tuft",
		Description: "default",
		Author:      "swaves",
		Files:       `{"home.html":"home"}`,
		CurrentFile: "home.html",
		Status:      "draft",
		IsCurrent:   1,
		IsBuiltin:   1,
		Version:     7,
	}
	themes := []db.Theme{
		{Name: "tuft", Code: "tuft"},
		{Name: "tuft 副本", Code: "tuft-copy"},
	}

	copied := duplicateTheme(source, themes)
	if copied.Name != "tuft 副本 2" {
		t.Fatalf("copied theme name = %q, want %q", copied.Name, "tuft 副本 2")
	}
	if copied.Code != "tuft-copy-2" {
		t.Fatalf("copied theme code = %q, want %q", copied.Code, "tuft-copy-2")
	}
	if copied.Description != source.Description || copied.Author != source.Author || copied.Files != source.Files || copied.CurrentFile != source.CurrentFile {
		t.Fatalf("copied theme should keep content fields: %+v", copied)
	}
	if copied.IsCurrent != 0 || copied.IsBuiltin != 0 || copied.Version != 1 {
		t.Fatalf("copied theme should reset runtime flags/version: %+v", copied)
	}
}

func TestDecodeThemeTransferPayloadSupportsMapAndStringFiles(t *testing.T) {
	payload, err := decodeThemeTransferPayload([]byte(`{
		"name": "demo",
		"code": "demo",
		"files": {
			"themes/demo/home.html": "home",
			"themes/demo/layout_main.html": "layout"
		},
		"current_file": "themes/demo/home.html"
	}`))
	if err != nil {
		t.Fatalf("decodeThemeTransferPayload(map) failed: %v", err)
	}
	if payload.Files["home.html"] != "home" || payload.CurrentFile != "themes/demo/home.html" {
		t.Fatalf("unexpected decoded payload: %+v", payload)
	}

	payload, err = decodeThemeTransferPayload([]byte(`{
		"name": "demo",
		"code": "demo",
		"files": "{\"site/home.html\":\"home\",\"site/layout/layout.html\":\"layout\"}",
		"current_file": "home.html"
	}`))
	if err != nil {
		t.Fatalf("decodeThemeTransferPayload(string) failed: %v", err)
	}
	if payload.Files["home.html"] != "home" || payload.Files["layout_main.html"] != "layout" {
		t.Fatalf("unexpected decoded payload from string files: %+v", payload)
	}
}

func TestBuildImportedThemeKeepsOrRenamesConflicts(t *testing.T) {
	themes := []db.Theme{
		{Name: "demo", Code: "demo"},
	}
	payload := &themeTransferPayload{
		Name:        "demo",
		Code:        "demo",
		Files:       map[string]string{"home.html": "home", "layout_main.html": "layout"},
		CurrentFile: "home.html",
		Status:      "published",
	}

	theme, err := buildImportedTheme(payload, themes)
	if err != nil {
		t.Fatalf("buildImportedTheme failed: %v", err)
	}
	if theme.Name != "demo 副本" {
		t.Fatalf("imported theme name = %q, want %q", theme.Name, "demo 副本")
	}
	if theme.Code != "demo-copy" {
		t.Fatalf("imported theme code = %q, want %q", theme.Code, "demo-copy")
	}
	if theme.IsCurrent != 0 || theme.IsBuiltin != 0 || theme.Version != 1 {
		t.Fatalf("imported theme should reset runtime flags/version: %+v", theme)
	}
}

func createThemeRecord(t *testing.T, dbx *db.DB, code string, isCurrent int) *db.Theme {
	t.Helper()

	theme := &db.Theme{
		Name:        code,
		Code:        code,
		Description: code,
		Author:      "tester",
		Files:       `{"home.html":"<h1>` + code + `</h1>"}`,
		CurrentFile: "home.html",
		Status:      "draft",
		Version:     1,
	}
	if _, err := db.CreateTheme(dbx, theme); err != nil {
		t.Fatalf("CreateTheme(%s) failed: %v", code, err)
	}
	if isCurrent == 1 {
		if err := db.SetThemeCurrent(dbx, theme.ID); err != nil {
			t.Fatalf("SetThemeCurrent(%s) failed: %v", code, err)
		}
		theme.IsCurrent = 1
	}
	return theme
}

func withTemplateReload(t *testing.T, enabled bool) {
	t.Helper()

	original := config.TemplateReload
	config.TemplateReload = enabled
	t.Cleanup(func() {
		config.TemplateReload = original
	})
}

func TestPostUpdateThemeHandlerReturnsJSONForAjaxSave(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "themes-update-json.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	theme := createThemeRecord(t, dbx, "theme-a", 0)

	app := fiber.New()
	handler := &Handler{Model: dbx}
	app.Post("/themes/:id", handler.PostUpdateThemeHandler)

	form := strings.NewReader("version=1&current_file=home.html&name=theme-a+updated&author=tester&description=updated&current_content=%3Ch1%3Eupdated%3C%2Fh1%3E")
	req := httptest.NewRequest(fiber.MethodPost, "/themes/"+strconv.FormatInt(theme.ID, 10), form)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set(fiber.HeaderAccept, fiber.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var body struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Data    struct {
			Version     int64  `json:"version"`
			CurrentFile string `json:"current_file"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !body.OK {
		t.Fatal("expected ok response")
	}
	if body.Message != "主题已保存。" {
		t.Fatalf("message = %q, want %q", body.Message, "主题已保存。")
	}
	if body.Data.Version != 2 {
		t.Fatalf("version = %d, want %d", body.Data.Version, 2)
	}
	if body.Data.CurrentFile != "home.html" {
		t.Fatalf("current_file = %q, want %q", body.Data.CurrentFile, "home.html")
	}

	updated, err := db.GetThemeByID(dbx, theme.ID)
	if err != nil {
		t.Fatalf("GetThemeByID failed: %v", err)
	}
	if updated.Name != "theme-a updated" {
		t.Fatalf("updated name = %q, want %q", updated.Name, "theme-a updated")
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want %d", updated.Version, 2)
	}
	if updated.Status != "draft" {
		t.Fatalf("updated status = %q, want draft", updated.Status)
	}
	files, err := themefiles.ParseJSON(updated.Files)
	if err != nil {
		t.Fatalf("themefiles.ParseJSON failed: %v", err)
	}
	if files["home.html"] != "<h1>updated</h1>" {
		t.Fatalf("home.html content = %q, want %q", files["home.html"], "<h1>updated</h1>")
	}
}

func TestPostUpdateThemeHandlerUpdatesStatus(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "themes-update-status.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	theme := createThemeRecord(t, dbx, "theme-status", 0)

	app := fiber.New()
	handler := &Handler{Model: dbx}
	app.Post("/themes/:id", handler.PostUpdateThemeHandler)

	form := strings.NewReader("version=1&current_file=home.html&name=theme-status&status=published&author=tester&description=updated&current_content=%3Ch1%3Epublished%3C%2Fh1%3E")
	req := httptest.NewRequest(fiber.MethodPost, "/themes/"+strconv.FormatInt(theme.ID, 10), form)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set(fiber.HeaderAccept, fiber.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	updated, err := db.GetThemeByID(dbx, theme.ID)
	if err != nil {
		t.Fatalf("GetThemeByID failed: %v", err)
	}
	if updated.Status != "published" {
		t.Fatalf("updated status = %q, want published", updated.Status)
	}
}

func TestSetCurrentThemeRejectsDraft(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "themes-set-current-draft.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	draft := createThemeRecord(t, dbx, "theme-draft-current", 0)
	currentBefore, err := db.GetCurrentTheme(dbx)
	if err != nil {
		t.Fatalf("GetCurrentTheme before failed: %v", err)
	}

	if _, err := setCurrentThemeAndRestart(dbx, draft.ID); err == nil || err.Error() != "草稿主题不能应用为当前主题。" {
		t.Fatalf("setCurrentThemeAndRestart(draft) error = %v, want 草稿主题不能应用为当前主题。", err)
	}

	currentAfter, err := db.GetCurrentTheme(dbx)
	if err != nil {
		t.Fatalf("GetCurrentTheme after failed: %v", err)
	}
	if currentAfter.ID != currentBefore.ID {
		t.Fatalf("current theme changed after draft rejection: before=%+v after=%+v", currentBefore, currentAfter)
	}
}

func TestPostUpdateThemeHandlerReturnsJSONValidationError(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "themes-update-json-error.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	theme := createThemeRecord(t, dbx, "theme-a", 0)

	app := fiber.New()
	handler := &Handler{Model: dbx}
	app.Post("/themes/:id", handler.PostUpdateThemeHandler)

	form := strings.NewReader("version=1&current_file=home.html&name=&author=tester&description=updated&current_content=%3Ch1%3Eupdated%3C%2Fh1%3E")
	req := httptest.NewRequest(fiber.MethodPost, "/themes/"+strconv.FormatInt(theme.ID, 10), form)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set(fiber.HeaderAccept, fiber.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}

	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if body.OK {
		t.Fatal("expected failed response")
	}
	if body.Error != "主题名称不能为空。" {
		t.Fatalf("error = %q, want %q", body.Error, "主题名称不能为空。")
	}
}

func TestPostUpdateThemeHandlerDeletesThemeFile(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "themes-delete-json.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	theme := &db.Theme{
		Name:        "theme-delete",
		Code:        "theme-delete",
		Description: "theme-delete",
		Author:      "tester",
		Files:       `{"home.html":"<h1>home</h1>","inc_footer.html":"<footer>footer</footer>"}`,
		CurrentFile: "inc_footer.html",
		Status:      "draft",
		Version:     1,
	}
	if _, err := db.CreateTheme(dbx, theme); err != nil {
		t.Fatalf("CreateTheme failed: %v", err)
	}

	app := fiber.New()
	handler := &Handler{Model: dbx}
	app.Post("/themes/:id", handler.PostUpdateThemeHandler)

	form := strings.NewReader("version=1&current_file=inc_footer.html&name=theme-delete&author=tester&description=theme-delete&current_content=%3Cfooter%3Efooter%3C%2Ffooter%3E&delete_file_path=inc_footer.html")
	req := httptest.NewRequest(fiber.MethodPost, "/themes/"+strconv.FormatInt(theme.ID, 10), form)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set(fiber.HeaderAccept, fiber.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusOK)
	}

	var body struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
		Data    struct {
			Version     int64  `json:"version"`
			CurrentFile string `json:"current_file"`
			DeletedFile string `json:"deleted_file"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !body.OK {
		t.Fatal("expected ok response")
	}
	if body.Message != "文件已删除。" {
		t.Fatalf("message = %q, want %q", body.Message, "文件已删除。")
	}
	if body.Data.Version != 2 {
		t.Fatalf("version = %d, want %d", body.Data.Version, 2)
	}
	if body.Data.CurrentFile != "home.html" {
		t.Fatalf("current_file = %q, want %q", body.Data.CurrentFile, "home.html")
	}
	if body.Data.DeletedFile != "inc_footer.html" {
		t.Fatalf("deleted_file = %q, want %q", body.Data.DeletedFile, "inc_footer.html")
	}

	updated, err := db.GetThemeByID(dbx, theme.ID)
	if err != nil {
		t.Fatalf("GetThemeByID failed: %v", err)
	}
	if updated.Version != 2 {
		t.Fatalf("updated version = %d, want %d", updated.Version, 2)
	}
	if updated.CurrentFile != "home.html" {
		t.Fatalf("updated current_file = %q, want %q", updated.CurrentFile, "home.html")
	}
	files, err := themefiles.ParseJSON(updated.Files)
	if err != nil {
		t.Fatalf("themefiles.ParseJSON failed: %v", err)
	}
	if _, ok := files["inc_footer.html"]; ok {
		t.Fatal("expected inc_footer.html to be deleted")
	}
}

func TestPostUpdateThemeHandlerRejectsProtectedThemeFileDelete(t *testing.T) {
	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "themes-delete-protected.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() { _ = dbx.Close() })

	theme := &db.Theme{
		Name:        "theme-protected",
		Code:        "theme-protected",
		Description: "theme-protected",
		Author:      "tester",
		Files:       `{"home.html":"<h1>home</h1>","inc_footer.html":"<footer>footer</footer>"}`,
		CurrentFile: "home.html",
		Status:      "draft",
		Version:     1,
	}
	if _, err := db.CreateTheme(dbx, theme); err != nil {
		t.Fatalf("CreateTheme failed: %v", err)
	}

	app := fiber.New()
	handler := &Handler{Model: dbx}
	app.Post("/themes/:id", handler.PostUpdateThemeHandler)

	form := strings.NewReader("version=1&current_file=home.html&name=theme-protected&author=tester&description=theme-protected&current_content=%3Ch1%3Ehome%3C%2Fh1%3E&delete_file_path=home.html")
	req := httptest.NewRequest(fiber.MethodPost, "/themes/"+strconv.FormatInt(theme.ID, 10), form)
	req.Header.Set(fiber.HeaderContentType, fiber.MIMEApplicationForm)
	req.Header.Set(fiber.HeaderAccept, fiber.MIMEApplicationJSON)
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}

	var body struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if body.OK {
		t.Fatal("expected failed response")
	}
	if body.Error != "该文件为内置入口模板，不能删除。" {
		t.Fatalf("error = %q, want %q", body.Error, "该文件为内置入口模板，不能删除。")
	}
}
