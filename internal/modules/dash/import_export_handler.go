package dash

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/shared/types"

	"github.com/gofiber/fiber/v3"
)

// Import/Export
type importParseOptions struct {
	slugSource     SlugSource
	slugField      string
	titleSource    TitleSource
	titleField     string
	titleLevel     int
	createdSource  CreatedSource
	createdField   string
	statusSource   StatusSource
	statusField    string
	categorySource CategorySource
	categoryField  string
	tagSource      TagSource
	tagField       string
}

func defaultImportPager() types.Pagination {
	return types.Pagination{
		Page:     config.DefaultPage,
		PageSize: config.DefaultPageSize,
		Num:      0,
		Total:    0,
	}
}

func renderImportView(c fiber.Ctx, data fiber.Map) error {
	viewData := fiber.Map{
		"Title":          "Import Markdown",
		"ImportingItems": []PreviewPostItem{},
		"ImportingTotal": 0,
		"Pager":          defaultImportPager(),
		"AllCategories":  []db.Category{},
	}
	for k, v := range data {
		viewData[k] = v
	}
	return RenderDashView(c, "dash/import.html", viewData, "")
}

func readImportParseOptions(c fiber.Ctx) importParseOptions {
	// slug
	slugSourceStr := c.FormValue("slug_source")
	slugSource := SlugFromTitle
	switch slugSourceStr {
	case "filename":
		slugSource = SlugFromFilename
	case "frontmatter":
		slugSource = SlugFromFrontmatter
	case "title":
		slugSource = SlugFromTitle
	}
	slugField := c.FormValue("slug_field")
	if slugField == "" {
		slugField = "slug"
	}

	// title
	titleSourceStr := c.FormValue("title_source")
	titleSource := TitleFromFrontmatter
	switch titleSourceStr {
	case "filename":
		titleSource = TitleFromFilename
	case "frontmatter":
		titleSource = TitleFromFrontmatter
	case "markdown":
		titleSource = TitleFromMarkdown
	}
	titleField := c.FormValue("title_field")
	if titleField == "" {
		titleField = "title"
	}
	titleLevel := 1
	if titleSourceStr == "markdown" {
		if levelStr := c.FormValue("title_level"); levelStr != "" {
			if level, err := strconv.Atoi(levelStr); err == nil && level >= 1 && level <= 3 {
				titleLevel = level
			}
		}
	}

	// created_at
	createdSourceStr := c.FormValue("created_source")
	createdSource := CreatedFromFrontmatter
	switch createdSourceStr {
	case "frontmatter":
		createdSource = CreatedFromFrontmatter
	case "filetime":
		createdSource = CreatedFromFileTime
	}
	createdField := c.FormValue("created_field")
	if createdField == "" {
		createdField = "date"
	}

	// status
	statusSourceStr := c.FormValue("status_source")
	statusSource := StatusFromFrontmatter
	switch statusSourceStr {
	case "frontmatter":
		statusSource = StatusFromFrontmatter
	case "alldraft":
		statusSource = StatusAllDraft
	case "allpublished":
		statusSource = StatusAllPublished
	}
	statusField := c.FormValue("status_field")
	if statusField == "" {
		statusField = "draft"
	}

	// category
	categorySourceStr := c.FormValue("category_source")
	categorySource := CategoryNone
	switch categorySourceStr {
	case "frontmatter":
		categorySource = CategoryFromFrontmatter
	case "autocreate":
		categorySource = CategoryAutoCreate
	case "none":
		categorySource = CategoryNone
	}
	categoryField := c.FormValue("category_field")
	if categoryField == "" {
		categoryField = "category"
	}

	// tag
	tagSourceStr := c.FormValue("tag_source")
	tagSource := TagNone
	switch tagSourceStr {
	case "frontmatter":
		tagSource = TagFromFrontmatter
	case "autocreate":
		tagSource = TagAutoCreate
	case "none":
		tagSource = TagNone
	}
	tagField := c.FormValue("tag_field")
	if tagField == "" {
		tagField = "tags"
	}

	return importParseOptions{
		slugSource:     slugSource,
		slugField:      slugField,
		titleSource:    titleSource,
		titleField:     titleField,
		titleLevel:     titleLevel,
		createdSource:  createdSource,
		createdField:   createdField,
		statusSource:   statusSource,
		statusField:    statusField,
		categorySource: categorySource,
		categoryField:  categoryField,
		tagSource:      tagSource,
		tagField:       tagField,
	}
}

func (h *Handler) GetImportHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	importingItems, err := ListImportingPreviewItemsService(h.Model, &pager)
	if err != nil {
		logger.Error("list importing items failed: %v", err)
		return renderImportView(c, fiber.Map{
			"Title":          "Import Markdown",
			"ImportingItems": []PreviewPostItem{},
			"ImportingTotal": 0,
			"Pager":          pager,
			"AllCategories":  []db.Category{},
			"Error":          "Load importing items failed: " + err.Error(),
		})
	}

	allCategories, err := GetAllCategoriesFlat(h.Model)
	if err != nil {
		logger.Warn("list categories for import failed: %v", err)
		allCategories = []db.Category{}
	} else {
		for i := range allCategories {
			allCategories[i].Name = strings.Trim(strings.TrimSpace(allCategories[i].Name), "\"'")
		}
	}

	return renderImportView(c, fiber.Map{
		"Title":          "Import Markdown",
		"ImportingItems": importingItems,
		"ImportingTotal": pager.Total,
		"Pager":          pager,
		"AllCategories":  allCategories,
	})
}

func (h *Handler) PostImportParseItemHandler(c fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		statusCode := fiber.StatusBadRequest
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request entity too large") {
			statusCode = fiber.StatusRequestEntityTooLarge
		}
		return c.Status(statusCode).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	src, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "open file failed: " + err.Error(),
		})
	}
	defer src.Close()

	content, err := io.ReadAll(src)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "read file failed: " + err.Error(),
		})
	}

	filename := fileHeader.Filename
	if idx := strings.LastIndex(filename, "."); idx > 0 {
		filename = filename[:idx]
	}

	options := readImportParseOptions(c)
	items, parseErr := ParseImportFiles(
		[]ImportFile{{Filename: filename, Content: string(content)}},
		options.slugSource,
		options.slugField,
		options.titleSource,
		options.titleField,
		options.titleLevel,
		options.createdSource,
		options.createdField,
		options.statusSource,
		options.statusField,
		options.categorySource,
		options.categoryField,
		options.tagSource,
		options.tagField,
	)
	if len(items) == 0 {
		errMsg := "parse file failed"
		if parseErr != nil {
			errMsg = parseErr.Error()
		}
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":       false,
			"filename": fileHeader.Filename,
			"error":    errMsg,
		})
	}

	item := items[0]
	item.Filename = fileHeader.Filename
	item.ContentPreview = buildImportContentPreview(item.Content)
	stagedItem, err := ImportPreviewItemAsImportingService(h.Model, item)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":       false,
			"filename": fileHeader.Filename,
			"error":    err.Error(),
		})
	}

	resp := fiber.Map{
		"ok":       true,
		"title":    stagedItem.Title,
		"slug":     stagedItem.Slug,
		"filename": fileHeader.Filename,
		"item": fiber.Map{
			"post_id":         stagedItem.PostID,
			"title":           stagedItem.Title,
			"slug":            stagedItem.Slug,
			"content_preview": stagedItem.ContentPreview,
			"status":          stagedItem.Status,
			"kind":            stagedItem.Kind,
			"created_at":      stagedItem.CreatedAt,
			"created_at_unix": stagedItem.CreatedAtUnix,
			"tags":            stagedItem.Tags,
			"category":        stagedItem.Category,
			"categories":      stagedItem.Categories,
			"filename":        stagedItem.Filename,
		},
	}
	if parseErr != nil {
		resp["warning"] = parseErr.Error()
	}

	return c.JSON(resp)
}

func readImportPreviewItemFromForm(c fiber.Ctx) (PreviewPostItem, error) {
	postID, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("post_id")), 10, 64)
	if err != nil || postID <= 0 {
		return PreviewPostItem{}, errors.New("post_id is required")
	}

	kindVal := c.FormValue("kind")
	if kindVal != "0" && kindVal != "1" {
		kindVal = "0"
	}

	item := PreviewPostItem{
		PostID:     postID,
		Title:      c.FormValue("title"),
		Slug:       c.FormValue("slug"),
		Status:     c.FormValue("status"),
		Kind:       kindVal,
		CreatedAt:  c.FormValue("created_at"),
		Tags:       c.FormValue("tags"),
		Category:   c.FormValue("category"),
		Categories: c.FormValue("categories"),
		Filename:   c.FormValue("filename"),
	}
	if createdAtUnix := strings.TrimSpace(c.FormValue("created_at_unix")); createdAtUnix != "" {
		if ts, parseErr := strconv.ParseInt(createdAtUnix, 10, 64); parseErr == nil {
			item.CreatedAtUnix = ts
		}
	}

	return item, nil
}

func (h *Handler) PostImportSaveItemHandler(c fiber.Ctx) error {
	item, err := readImportPreviewItemFromForm(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	savedItem, err := SaveImportPreviewItemService(h.Model, item)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"item": fiber.Map{
			"post_id":         savedItem.PostID,
			"title":           savedItem.Title,
			"slug":            savedItem.Slug,
			"content_preview": savedItem.ContentPreview,
			"status":          savedItem.Status,
			"kind":            savedItem.Kind,
			"created_at":      savedItem.CreatedAt,
			"created_at_unix": savedItem.CreatedAtUnix,
			"tags":            savedItem.Tags,
			"category":        savedItem.Category,
			"categories":      savedItem.Categories,
			"filename":        savedItem.Filename,
		},
	})
}

func (h *Handler) PostImportConfirmItemHandler(c fiber.Ctx) error {
	item, err := readImportPreviewItemFromForm(c)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	if err = ConfirmImportPreviewItemService(h.Model, item); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
	})
}

func (h *Handler) PostImportConfirmAllHandler(c fiber.Ctx) error {
	result, err := ConfirmAllImportingPreviewItemsService(h.Model)
	if err != nil {
		logger.Error("confirm all importing items failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "确认导入失败: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"ok":      true,
		"total":   result.Total,
		"success": result.Success,
		"fail":    result.Fail,
		"errors":  result.Errors,
	})
}

func (h *Handler) PostImportCancelHandler(c fiber.Ctx) error {
	deletedCount, err := CancelImportingPreviewItemsService(h.Model)
	if err != nil {
		logger.Error("cancel importing items failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": "取消导入失败: " + err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"ok":            true,
		"deleted_count": deletedCount,
	})
}

func (h *Handler) PostImportHandler(c fiber.Ctx) error {
	c.Status(fiber.StatusMethodNotAllowed)
	return renderImportView(c, fiber.Map{
		"Title": "Import Markdown",
		"Error": "当前导入仅支持页面内异步解析与确认流程，请保持 JavaScript 启用后逐个解析并确认导入。",
	})
}

func (h *Handler) GetDevUIComponentsHandler(c fiber.Ctx) error {
	return RenderDashView(c, "dash/dev_ui_components.html", fiber.Map{
		"Title": "UI组件",
	}, "")
}

// Export
func (h *Handler) GetExportDownloadHandler(c fiber.Ctx) error {
	// 生成导出文件名（包含时间戳）
	name := strings.ToLower(c.App().Config().AppName) + "_export"

	logger.Info("export to: tmp_dir=%s name=%s", os.TempDir(), name)

	// 创建临时目录
	tmpDir, err := os.MkdirTemp(os.TempDir(), name+"-")
	if err != nil {
		return h.renderExportView(c, fiber.Map{
			"Error": "Failed to create export directory: " + err.Error(),
		})
	}
	cleanupTmpDir := func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			logger.Warn("export cleanup temp dir failed: dir=%s err=%v", tmpDir, removeErr)
		}
	}

	// 调用 ExportSQLiteWithHash 函数（传目录，函数内自动生成文件名）
	result, err := db.ExportSQLiteWithHash(h.Model, tmpDir)
	if err != nil {
		cleanupTmpDir()
		return h.renderExportView(c, fiber.Map{
			"Error": "Failed to export database: " + err.Error(),
		})
	}

	// 返回文件下载（从完整路径中提取文件名）
	filename := filepath.Base(result.File)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Set("Content-Type", "application/x-sqlite3")

	stream, err := openCleanupFileStream(result.File, cleanupTmpDir)
	if err != nil {
		cleanupTmpDir()
		return h.renderExportView(c, fiber.Map{
			"Error": "Failed to open export file: " + err.Error(),
		})
	}
	return c.SendStream(stream)
}

type cleanupFileStream struct {
	file    *os.File
	cleanup func()
}

func openCleanupFileStream(path string, cleanup func()) (*cleanupFileStream, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	return &cleanupFileStream{
		file:    file,
		cleanup: cleanup,
	}, nil
}

func (s *cleanupFileStream) Read(p []byte) (int, error) {
	return s.file.Read(p)
}

func (s *cleanupFileStream) Close() error {
	closeErr := s.file.Close()
	if s.cleanup != nil {
		s.cleanup()
	}
	return closeErr
}
