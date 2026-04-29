package dash

import (
	"encoding/json"
	"fmt"
	"io"
	"runtime"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/themefiles"
	"swaves/internal/platform/updater"
	"time"

	"swaves/internal/platform/db"
	"swaves/internal/platform/middleware"

	"github.com/gofiber/fiber/v3"
)

var protectedThemeFilePaths = map[string]struct{}{
	"404.html":    {},
	"detail.html": {},
	"error.html":  {},
	"home.html":   {},
	"list.html":   {},
	"post.html":   {},
}

func isProtectedThemeFilePath(path string) bool {
	path, ok := themefiles.NormalizePath(path)
	if !ok {
		return false
	}
	_, exists := protectedThemeFilePaths[path]
	return exists
}

func themeProtectedFileFlags(files map[string]string) map[string]bool {
	flags := make(map[string]bool, len(files))
	for path := range files {
		if isProtectedThemeFilePath(path) {
			flags[path] = true
		}
	}
	return flags
}

func wantsThemeJSONResponse(c fiber.Ctx) bool {
	accept := strings.ToLower(strings.TrimSpace(c.Get(fiber.HeaderAccept)))
	if strings.Contains(accept, fiber.MIMEApplicationJSON) {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(c.Get("X-Requested-With")), "XMLHttpRequest")
}

func parseThemeVersion(raw string) (int64, error) {
	version, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || version <= 0 {
		return 0, fiber.ErrBadRequest
	}
	return version, nil
}

type themeSwitchResult struct {
	RestartedPID    int
	AlreadyCurrent  bool
	RestartRequired bool
}

type themeTransferPayload struct {
	Name        string            `json:"name"`
	Code        string            `json:"code"`
	Description string            `json:"description"`
	Author      string            `json:"author"`
	Files       map[string]string `json:"files"`
	CurrentFile string            `json:"current_file"`
	Status      string            `json:"status"`
	ExportedAt  int64             `json:"exported_at"`
}

func buildCopiedThemeName(base string, taken map[string]struct{}) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "未命名主题"
	}
	for i := 1; ; i++ {
		candidate := base + " 副本"
		if i > 1 {
			candidate = fmt.Sprintf("%s 副本 %d", base, i)
		}
		if _, ok := taken[candidate]; !ok {
			return candidate
		}
	}
}

func buildCopiedThemeCode(base string, taken map[string]struct{}) string {
	base = strings.TrimSpace(base)
	if base == "" {
		base = "theme"
	}
	for i := 1; ; i++ {
		candidate := base + "-copy"
		if i > 1 {
			candidate = fmt.Sprintf("%s-copy-%d", base, i)
		}
		if _, ok := taken[candidate]; !ok {
			return candidate
		}
	}
}

func duplicateTheme(source db.Theme, themes []db.Theme) *db.Theme {
	nameSet, codeSet := themeNameSets(themes)

	nowUnix := time.Now().Unix()
	return &db.Theme{
		Name:        buildCopiedThemeName(source.Name, nameSet),
		Code:        buildCopiedThemeCode(source.Code, codeSet),
		Description: source.Description,
		Author:      source.Author,
		Files:       source.Files,
		CurrentFile: source.CurrentFile,
		Status:      source.Status,
		IsCurrent:   0,
		IsBuiltin:   0,
		Version:     1,
		CreatedAt:   nowUnix,
		UpdatedAt:   nowUnix,
	}
}

func themeNameSets(themes []db.Theme) (map[string]struct{}, map[string]struct{}) {
	nameSet := make(map[string]struct{}, len(themes))
	codeSet := make(map[string]struct{}, len(themes))
	for _, item := range themes {
		nameSet[item.Name] = struct{}{}
		codeSet[item.Code] = struct{}{}
	}
	return nameSet, codeSet
}

func decodeThemeTransferFiles(raw json.RawMessage) (map[string]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("theme files is required")
	}

	if raw[0] == '"' {
		var rawFiles string
		if err := json.Unmarshal(raw, &rawFiles); err != nil {
			return nil, err
		}
		return themefiles.ParseJSON(rawFiles)
	}

	rawFiles := map[string]string{}
	if err := json.Unmarshal(raw, &rawFiles); err != nil {
		return nil, err
	}
	rawFilesJSON, err := json.Marshal(rawFiles)
	if err != nil {
		return nil, err
	}
	return themefiles.ParseJSON(string(rawFilesJSON))
}

func decodeThemeTransferPayload(raw []byte) (*themeTransferPayload, error) {
	var envelope struct {
		Name        string          `json:"name"`
		Code        string          `json:"code"`
		Description string          `json:"description"`
		Author      string          `json:"author"`
		Files       json.RawMessage `json:"files"`
		CurrentFile string          `json:"current_file"`
		Status      string          `json:"status"`
		ExportedAt  int64           `json:"exported_at"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, err
	}
	if len(envelope.Files) == 0 {
		return nil, fmt.Errorf("theme files is required")
	}

	files, err := decodeThemeTransferFiles(envelope.Files)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("theme files is empty")
	}

	return &themeTransferPayload{
		Name:        strings.TrimSpace(envelope.Name),
		Code:        strings.TrimSpace(envelope.Code),
		Description: envelope.Description,
		Author:      envelope.Author,
		Files:       files,
		CurrentFile: strings.TrimSpace(envelope.CurrentFile),
		Status:      strings.TrimSpace(envelope.Status),
		ExportedAt:  envelope.ExportedAt,
	}, nil
}

func buildImportedTheme(payload *themeTransferPayload, themes []db.Theme) (*db.Theme, error) {
	if payload == nil {
		return nil, fmt.Errorf("theme payload is required")
	}
	if len(payload.Files) == 0 {
		return nil, fmt.Errorf("theme files is empty")
	}

	nameSet, codeSet := themeNameSets(themes)
	name := strings.TrimSpace(payload.Name)
	code := strings.TrimSpace(payload.Code)
	if name == "" {
		name = code
	}
	if name == "" {
		name = "未命名主题"
	}
	if code == "" {
		code = "theme"
	}
	if _, ok := nameSet[name]; ok {
		name = buildCopiedThemeName(name, nameSet)
	}
	if _, ok := codeSet[code]; ok {
		code = buildCopiedThemeCode(code, codeSet)
	}

	filesJSON, err := themefiles.MarshalJSON(payload.Files)
	if err != nil {
		return nil, err
	}

	nowUnix := time.Now().Unix()
	theme := db.Theme{
		Name:        name,
		Code:        code,
		Description: payload.Description,
		Author:      payload.Author,
		Files:       filesJSON,
		CurrentFile: themefiles.ResolveCurrentFile(payload.Files, payload.CurrentFile),
		Status:      payload.Status,
		IsCurrent:   0,
		IsBuiltin:   0,
		Version:     1,
		CreatedAt:   nowUnix,
		UpdatedAt:   nowUnix,
	}
	if theme.Status == "" {
		theme.Status = "draft"
	}
	return &theme, nil
}

func exportThemePayload(theme db.Theme) (*themeTransferPayload, error) {
	files, err := themefiles.ParseJSON(theme.Files)
	if err != nil {
		return nil, err
	}
	return &themeTransferPayload{
		Name:        theme.Name,
		Code:        theme.Code,
		Description: theme.Description,
		Author:      theme.Author,
		Files:       files,
		CurrentFile: theme.CurrentFile,
		Status:      theme.Status,
		ExportedAt:  time.Now().Unix(),
	}, nil
}

func themeNewViewData(theme db.Theme, errorMessage string) fiber.Map {
	return fiber.Map{
		"Title": "新建主题",
		"Error": errorMessage,
		"Theme": theme,
	}
}

func themeEditViewData(theme db.Theme, files map[string]string, currentFile string, errorMessage string) fiber.Map {
	return fiber.Map{
		"Title":               "编辑主题",
		"Error":               errorMessage,
		"Theme":               theme,
		"ThemeFiles":          files,
		"ThemeFilePaths":      themefiles.SortedPaths(files),
		"ThemeProtectedFiles": themeProtectedFileFlags(files),
		"CurrentFile":         currentFile,
	}
}

func (h *Handler) respondThemeEditUpdate(c fiber.Ctx, returnJSON bool, themeID, version int64, currentFile string, message string, extraData fiber.Map) error {
	if returnJSON {
		data := fiber.Map{
			"version":      version,
			"current_file": currentFile,
		}
		for key, value := range extraData {
			data[key] = value
		}
		return c.JSON(fiber.Map{
			"ok":      true,
			"message": message,
			"data":    data,
		})
	}

	return h.redirectToDashRouteWithNotice(
		c,
		"dash.themes.edit",
		map[string]string{"id": strconv.FormatInt(themeID, 10)},
		map[string]string{"file": currentFile},
		message,
	)
}

func setCurrentThemeAndRestart(model *db.DB, id int64) (themeSwitchResult, error) {
	result := themeSwitchResult{}

	theme, err := db.GetThemeByID(model, id)
	if err != nil {
		return result, err
	}
	if theme.IsCurrent == 1 {
		result.AlreadyCurrent = true
		return result, nil
	}

	if err := db.SetThemeCurrent(model, id); err != nil {
		return result, err
	}

	if config.TemplateReload {
		return result, nil
	}
	if runtime.GOOS == "windows" {
		result.RestartRequired = true
		return result, nil
	}
	if _, err := updater.ReadActiveRuntimeInfo(); err != nil {
		logger.Warn("[theme] current theme updated without hot reload: id=%d err=%v", id, err)
		result.RestartRequired = true
		return result, nil
	}

	pid, err := updater.RestartActiveRuntime()
	if err != nil {
		logger.Error("[theme] current theme updated but restart signal failed: id=%d err=%v", id, err)
		result.RestartRequired = true
		return result, nil
	}

	result.RestartedPID = pid
	return result, nil
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
		"Title":           "主题",
		"Themes":          themes,
		"Pager":           pager,
		"RecordTabCounts": recordTabCounts,
	}, "")
}

func (h *Handler) GetDefaultThemeEntryHandler(c fiber.Ctx) error {
	theme, err := db.GetThemeByCode(h.Model, db.DefaultThemeCode)
	if err != nil {
		return err
	}
	files, err := themefiles.ParseJSON(theme.Files)
	if err != nil {
		return err
	}
	currentFile := themefiles.ResolveCurrentFile(files, theme.CurrentFile)
	return h.redirectToDashRoute(c, "dash.themes.edit", map[string]string{
		"id": strconv.FormatInt(theme.ID, 10),
	}, map[string]string{
		"file": currentFile,
	})
}

func (h *Handler) GetThemeNewHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/themes_new.html", themeNewViewData(db.Theme{}, ""), "")
}

func (h *Handler) PostCreateThemeHandler(c fiber.Ctx) error {
	name := strings.TrimSpace(c.FormValue("name"))
	code := strings.TrimSpace(c.FormValue("code"))
	author := strings.TrimSpace(c.FormValue("author"))
	description := strings.TrimSpace(c.FormValue("description"))
	formTheme := db.Theme{
		Name:        name,
		Code:        code,
		Author:      author,
		Description: description,
	}
	if name == "" || code == "" {
		return RenderDashView(c, "dash/themes_new.html", themeNewViewData(formTheme, "主题名称和编码不能为空。"), "")
	}
	defaultTheme, err := db.GetThemeByCode(h.Model, db.DefaultThemeCode)
	if err != nil {
		return RenderDashView(c, "dash/themes_new.html", themeNewViewData(formTheme, "读取默认主题失败："+err.Error()), "")
	}
	files, err := themefiles.ParseJSON(defaultTheme.Files)
	if err != nil {
		return RenderDashView(c, "dash/themes_new.html", themeNewViewData(formTheme, "读取默认主题失败："+err.Error()), "")
	}
	filesJSON, err := themefiles.MarshalJSON(files)
	if err != nil {
		return RenderDashView(c, "dash/themes_new.html", themeNewViewData(formTheme, "序列化默认主题失败："+err.Error()), "")
	}
	theme := &db.Theme{
		Name:        name,
		Code:        code,
		Description: description,
		Author:      author,
		Files:       filesJSON,
		CurrentFile: themefiles.ResolveCurrentFile(files, defaultTheme.CurrentFile),
		Status:      "draft",
		IsCurrent:   0,
		IsBuiltin:   0,
		Version:     1,
		CreatedAt:   time.Now().Unix(),
		UpdatedAt:   time.Now().Unix(),
	}
	if _, err := db.CreateTheme(h.Model, theme); err != nil {
		return RenderDashView(c, "dash/themes_new.html", themeNewViewData(*theme, err.Error()), "")
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
	files, err := themefiles.ParseJSON(theme.Files)
	if err != nil {
		return err
	}
	currentFile := themefiles.ResolveCurrentFile(files, c.Query("file"), theme.CurrentFile)
	theme.CurrentFile = currentFile
	return RenderDashView(c, "dash/themes_edit.html", themeEditViewData(*theme, files, currentFile, ""), "")
}

func (h *Handler) PostUpdateThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}
	returnJSON := wantsThemeJSONResponse(c)
	theme, err := db.GetThemeByID(h.Model, id)
	if err != nil {
		return err
	}
	files, err := themefiles.ParseJSON(theme.Files)
	if err != nil {
		return err
	}
	currentFile, ok := themefiles.NormalizePath(c.FormValue("current_file"))
	if !ok {
		return fiber.ErrBadRequest
	}
	if _, ok := files[currentFile]; !ok {
		return fiber.ErrBadRequest
	}
	renderEdit := func(errorMessage string) error {
		if returnJSON {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"ok":    false,
				"error": errorMessage,
			})
		}
		return RenderDashView(c, "dash/themes_edit.html", themeEditViewData(*theme, files, currentFile, errorMessage), "")
	}
	expectedVersion, err := parseThemeVersion(c.FormValue("version"))
	if err != nil {
		return err
	}
	files[currentFile] = c.FormValue("current_content")
	newFilePath := strings.TrimSpace(c.FormValue("new_file_path"))
	deleteFilePath := strings.TrimSpace(c.FormValue("delete_file_path"))
	if newFilePath != "" && deleteFilePath != "" {
		return renderEdit("不能同时创建和删除文件。")
	}
	if newFilePath != "" {
		if _, ok := themefiles.NormalizePath(newFilePath); !ok {
			return renderEdit("新文件路径无效，必须是扁平的 *.html 文件名。")
		}
		newFilePath, _ = themefiles.NormalizePath(newFilePath)
		if _, exists := files[newFilePath]; exists {
			return renderEdit("文件已存在。")
		}
		files[newFilePath] = ""
		currentFile = newFilePath
	}
	if deleteFilePath != "" {
		if _, ok := themefiles.NormalizePath(deleteFilePath); !ok {
			return renderEdit("待删除文件路径无效。")
		}
		deleteFilePath, _ = themefiles.NormalizePath(deleteFilePath)
		if isProtectedThemeFilePath(deleteFilePath) {
			return renderEdit("该文件为内置入口模板，不能删除。")
		}
		if _, exists := files[deleteFilePath]; !exists {
			return renderEdit("文件不存在。")
		}
		if len(files) <= 1 {
			return renderEdit("主题至少保留一个文件。")
		}
		delete(files, deleteFilePath)
		if currentFile == deleteFilePath {
			currentFile = themefiles.ResolveCurrentFile(files, theme.CurrentFile)
		}
	}
	filesJSON, err := themefiles.MarshalJSON(files)
	if err != nil {
		return err
	}
	theme.Name = strings.TrimSpace(c.FormValue("name"))
	theme.Author = strings.TrimSpace(c.FormValue("author"))
	theme.Description = strings.TrimSpace(c.FormValue("description"))
	theme.CurrentFile = currentFile
	theme.Files = filesJSON
	if theme.Name == "" {
		return renderEdit("主题名称不能为空。")
	}
	if err := db.UpdateTheme(h.Model, theme, expectedVersion); err != nil {
		if db.IsErrNotFound(err) || db.IsErrConflict(err) {
			return renderEdit("主题已被其他修改覆盖，请刷新后重试。")
		}
		return renderEdit(err.Error())
	}
	message := "主题已保存。"
	if newFilePath != "" {
		return h.respondThemeEditUpdate(c, returnJSON, theme.ID, theme.Version, currentFile, "文件已创建。", fiber.Map{
			"new_file": newFilePath,
		})
	}
	if deleteFilePath != "" {
		return h.respondThemeEditUpdate(c, returnJSON, theme.ID, theme.Version, currentFile, "文件已删除。", fiber.Map{
			"deleted_file": deleteFilePath,
		})
	}
	return h.respondThemeEditUpdate(c, returnJSON, theme.ID, theme.Version, currentFile, message, nil)
}

func (h *Handler) PostSetCurrentThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	result, err := setCurrentThemeAndRestart(h.Model, id)
	if err != nil {
		logger.Error("[theme] switch current theme failed: id=%d ip=%s err=%v", id, c.IP(), err)
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, err.Error())
	}
	if result.AlreadyCurrent {
		return h.redirectToDashRouteWithNotice(c, "dash.themes.list", nil, nil, "已是当前主题。")
	}
	if result.RestartRequired {
		logger.Info("[theme] current theme switched: id=%d ip=%s restart_pending=1", id, c.IP())
		return h.redirectToDashRouteWithNotice(c, "dash.themes.list", nil, nil, "当前主题已更新；当前运行实例未自动重载，请手动重启服务后生效。")
	}

	logger.Info("[theme] current theme switched: id=%d ip=%s master_pid=%d", id, c.IP(), result.RestartedPID)
	if result.RestartedPID > 0 {
		return h.redirectToDashRouteWithNotice(c, "dash.themes.list", nil, nil, fmt.Sprintf("当前主题已更新，已向服务发送重启信号（pid=%d）。", result.RestartedPID))
	}
	return h.redirectToDashRouteWithNotice(c, "dash.themes.list", nil, nil, "当前主题已更新。")
}

func (h *Handler) PostCopyThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	source, err := db.GetThemeByID(h.Model, id)
	if err != nil {
		return err
	}
	total, err := db.CountThemes(h.Model)
	if err != nil {
		return err
	}
	themes, err := db.ListThemesPaged(h.Model, total+1, 0)
	if err != nil {
		return err
	}

	copiedTheme := duplicateTheme(*source, themes)
	if _, err := db.CreateTheme(h.Model, copiedTheme); err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "复制主题失败："+err.Error())
	}
	logger.Info("[theme] theme duplicated: source_id=%d new_id=%d ip=%s", source.ID, copiedTheme.ID, c.IP())
	return h.redirectToDashRouteWithNotice(c, "dash.themes.edit", map[string]string{
		"id": strconv.FormatInt(copiedTheme.ID, 10),
	}, map[string]string{
		"file": copiedTheme.CurrentFile,
	}, "主题已复制。")
}

func (h *Handler) PostDeleteThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := db.DeleteTheme(h.Model, id); err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, err.Error())
	}
	logger.Info("[theme] theme deleted: id=%d ip=%s", id, c.IP())
	return h.redirectToDashRouteKeepQuery(c, "dash.themes.list", nil, nil)
}

func (h *Handler) GetExportThemeHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	theme, err := db.GetThemeByID(h.Model, id)
	if err != nil {
		return err
	}
	payload, err := exportThemePayload(*theme)
	if err != nil {
		return err
	}
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	filename := fmt.Sprintf("%s.json", theme.Code)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Set("Content-Type", "application/json; charset=utf-8")
	return c.SendStream(strings.NewReader(string(body)))
}

func (h *Handler) PostImportThemeHandler(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "请选择要导入的主题 JSON 文件。")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "打开主题文件失败："+err.Error())
	}
	defer src.Close()

	raw, err := io.ReadAll(src)
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "读取主题文件失败："+err.Error())
	}
	payload, err := decodeThemeTransferPayload(raw)
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "解析主题文件失败："+err.Error())
	}

	total, err := db.CountThemes(h.Model)
	if err != nil {
		return err
	}
	themes, err := db.ListThemesPaged(h.Model, total+1, 0)
	if err != nil {
		return err
	}

	theme, err := buildImportedTheme(payload, themes)
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "导入主题失败："+err.Error())
	}
	if _, err := db.CreateTheme(h.Model, theme); err != nil {
		return h.redirectToDashRouteWithError(c, "dash.themes.list", nil, nil, "导入主题失败："+err.Error())
	}
	logger.Info("[theme] theme imported: id=%d code=%s ip=%s", theme.ID, theme.Code, c.IP())
	return h.redirectToDashRouteWithNotice(c, "dash.themes.edit", map[string]string{
		"id": strconv.FormatInt(theme.ID, 10),
	}, map[string]string{
		"file": theme.CurrentFile,
	}, "主题已导入。")
}
