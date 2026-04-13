package dash

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

const defaultThemeCurrentFile = "site/home.html"

func parseThemeFiles(raw string) (map[string]string, error) {
	if strings.TrimSpace(raw) == "" {
		return map[string]string{}, nil
	}
	files := make(map[string]string)
	if err := json.Unmarshal([]byte(raw), &files); err != nil {
		return nil, err
	}
	for path, content := range files {
		if !isValidThemeFilePath(path) {
			return nil, fmt.Errorf("invalid theme file path: %s", path)
		}
		files[path] = content
	}
	return files, nil
}

func marshalThemeFiles(files map[string]string) (string, error) {
	data, err := json.Marshal(files)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func sortedThemeFilePaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths
}

func isValidThemeFilePath(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if !strings.HasPrefix(path, "site/") || !strings.HasSuffix(path, ".html") {
		return false
	}
	return !strings.Contains(path, "..")
}

func resolveThemeCurrentFile(theme db.Theme, files map[string]string, candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate != "" {
		if _, ok := files[candidate]; ok {
			return candidate
		}
	}
	if stored := strings.TrimSpace(theme.CurrentFile); stored != "" {
		if _, ok := files[stored]; ok {
			return stored
		}
	}
	if _, ok := files[defaultThemeCurrentFile]; ok {
		return defaultThemeCurrentFile
	}
	for _, path := range sortedThemeFilePaths(files) {
		return path
	}
	return defaultThemeCurrentFile
}

func parseThemeVersion(raw string) (int64, error) {
	version, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || version <= 0 {
		return 0, fiber.ErrBadRequest
	}
	return version, nil
}

func (h *Handler) GetThemeListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	offset := 0
	if pager.Page > 1 {
		offset = (pager.Page - 1) * pager.PageSize
	}
	themes, err := db.ListThemesPaged(h.Model, pager.PageSize, offset)
	if err != nil {
		return err
	}
	total, err := db.CountThemes(h.Model)
	if err != nil {
		return err
	}
	pager.Total = total
	if pager.Total > 0 {
		pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	}
	recordTabCounts, err := getRecordTabCounts(h.Model)
	if err != nil {
		return err
	}

	return RenderDashView(c, "dash/themes_index.html", fiber.Map{
		"Title":            "主题",
		"Themes":           themes,
		"Pager":            pager,
		"RecordTabCounts":  recordTabCounts,
		"OperationTableID": "themes-table",
	}, "")
}

func (h *Handler) GetThemeEntryHandler(c fiber.Ctx) error {
	theme, err := db.GetThemeByCode(h.Model, db.DefaultThemeTemplateCode)
	if err != nil {
		return err
	}

	return h.redirectToDashRoute(c, "dash.themes.edit", map[string]string{
		"id": strconv.FormatInt(theme.ID, 10),
	}, map[string]string{
		"file": theme.CurrentFile,
	})
}

func (h *Handler) GetThemeNewHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/themes_new.html", fiber.Map{
		"Title": "新建主题",
		"Theme": db.Theme{Author: "", CurrentFile: defaultThemeCurrentFile},
	}, "")
}

func (h *Handler) PostCreateThemeHandler(c fiber.Ctx) error {
	name := strings.TrimSpace(c.FormValue("name"))
	code := strings.TrimSpace(c.FormValue("code"))
	author := strings.TrimSpace(c.FormValue("author"))
	description := strings.TrimSpace(c.FormValue("description"))
	if name == "" || code == "" {
		return RenderDashView(c, "dash/themes_new.html", fiber.Map{
			"Title": "新建主题",
			"Error": "主题名称和编码不能为空。",
			"Theme": db.Theme{Name: name, Code: code, Author: author, Description: description, CurrentFile: defaultThemeCurrentFile},
		}, "")
	}
	templateTheme, err := db.GetThemeByCode(h.Model, db.DefaultThemeTemplateCode)
	if err != nil {
		return RenderDashView(c, "dash/themes_new.html", fiber.Map{
			"Title": "新建主题",
			"Error": "读取主题模板失败：" + err.Error(),
			"Theme": db.Theme{Name: name, Code: code, Author: author, Description: description, CurrentFile: defaultThemeCurrentFile},
		}, "")
	}
	files, err := parseThemeFiles(templateTheme.Files)
	if err != nil {
		return RenderDashView(c, "dash/themes_new.html", fiber.Map{
			"Title": "新建主题",
			"Error": "读取主题模板失败：" + err.Error(),
			"Theme": db.Theme{Name: name, Code: code, Author: author, Description: description, CurrentFile: defaultThemeCurrentFile},
		}, "")
	}
	filesJSON, err := marshalThemeFiles(files)
	if err != nil {
		return RenderDashView(c, "dash/themes_new.html", fiber.Map{
			"Title": "新建主题",
			"Error": "序列化主题模板失败：" + err.Error(),
			"Theme": db.Theme{Name: name, Code: code, Author: author, Description: description, CurrentFile: defaultThemeCurrentFile},
		}, "")
	}
	theme := &db.Theme{
		Name:        name,
		Code:        code,
		Description: description,
		Author:      author,
		Files:       filesJSON,
		CurrentFile: resolveThemeCurrentFile(*templateTheme, files, templateTheme.CurrentFile),
		Status:      "draft",
		IsCurrent:   0,
		IsBuiltin:   0,
		Version:     1,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	if _, err := db.CreateTheme(h.Model, theme); err != nil {
		return RenderDashView(c, "dash/themes_new.html", fiber.Map{
			"Title": "新建主题",
			"Error": err.Error(),
			"Theme": *theme,
		}, "")
	}
	return h.redirectToDashRouteWithNotice(c, "dash.themes.edit", map[string]string{"id": strconv.FormatInt(theme.ID, 10)}, map[string]string{"file": theme.CurrentFile}, "主题已创建。")
}

func (h *Handler) GetThemeEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	theme, err := db.GetThemeByID(h.Model, id)
	if err != nil {
		return err
	}
	files, err := parseThemeFiles(theme.Files)
	if err != nil {
		return err
	}
	currentFile := resolveThemeCurrentFile(*theme, files, c.Query("file"))
	theme.CurrentFile = currentFile
	return RenderDashView(c, "dash/themes_edit.html", fiber.Map{
		"Title":          "编辑主题",
		"Theme":          *theme,
		"ThemeFiles":     files,
		"ThemeFilePaths": sortedThemeFilePaths(files),
		"CurrentFile":    currentFile,
	}, "")
}

func (h *Handler) PostUpdateThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	theme, err := db.GetThemeByID(h.Model, id)
	if err != nil {
		return err
	}
	files, err := parseThemeFiles(theme.Files)
	if err != nil {
		return err
	}
	currentFile := strings.TrimSpace(c.FormValue("current_file"))
	if !isValidThemeFilePath(currentFile) {
		return fiber.ErrBadRequest
	}
	if _, ok := files[currentFile]; !ok {
		return fiber.ErrBadRequest
	}
	expectedVersion, err := parseThemeVersion(c.FormValue("version"))
	if err != nil {
		return err
	}
	files[currentFile] = c.FormValue("current_content")
	filesJSON, err := marshalThemeFiles(files)
	if err != nil {
		return err
	}
	theme.Name = strings.TrimSpace(c.FormValue("name"))
	theme.Author = strings.TrimSpace(c.FormValue("author"))
	theme.Description = strings.TrimSpace(c.FormValue("description"))
	theme.CurrentFile = currentFile
	theme.Files = filesJSON
	if theme.Name == "" {
		return RenderDashView(c, "dash/themes_edit.html", fiber.Map{
			"Title":          "编辑主题",
			"Error":          "主题名称不能为空。",
			"Theme":          *theme,
			"ThemeFiles":     files,
			"ThemeFilePaths": sortedThemeFilePaths(files),
			"CurrentFile":    currentFile,
		}, "")
	}
	if err := db.UpdateTheme(h.Model, theme, expectedVersion); err != nil {
		return RenderDashView(c, "dash/themes_edit.html", fiber.Map{
			"Title":          "编辑主题",
			"Error":          err.Error(),
			"Theme":          *theme,
			"ThemeFiles":     files,
			"ThemeFilePaths": sortedThemeFilePaths(files),
			"CurrentFile":    currentFile,
		}, "")
	}
	return h.redirectToDashRouteWithNotice(c, "dash.themes.edit", map[string]string{"id": strconv.FormatInt(theme.ID, 10)}, map[string]string{"file": currentFile}, "主题已保存。")
}

func (h *Handler) PostSetCurrentThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	if err := db.SetThemeCurrent(h.Model, id); err != nil {
		return err
	}
	return h.redirectToDashRouteWithNotice(c, "dash.themes.list", nil, nil, "当前主题已更新。")
}
