package admin

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"time"

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
	importingItems, err := ListImportingPreviewItemsService(h.Model)
	if err != nil {
		logger.Error("list importing items failed: %v", err)
		return RenderAdminView(c, "dash/import.html", fiber.Map{
			"Title":          "Import Markdown",
			"ImportingItems": []PreviewPostItem{},
			"AllCategories":  []db.Category{},
			"Error":          "Load importing items failed: " + err.Error(),
		}, "")
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

	return RenderAdminView(c, "dash/import.html", fiber.Map{
		"Title":          "Import Markdown",
		"ImportingItems": importingItems,
		"AllCategories":  allCategories,
	}, "")
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
			"content":         stagedItem.Content,
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

func (h *Handler) PostImportConfirmItemHandler(c fiber.Ctx) error {
	postID, err := strconv.ParseInt(strings.TrimSpace(c.FormValue("post_id")), 10, 64)
	if err != nil || postID <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "post_id is required",
		})
	}

	kindVal := c.FormValue("kind")
	if kindVal != "0" && kindVal != "1" {
		kindVal = "0"
	}

	item := PreviewPostItem{
		PostID:     postID,
		Title:      c.FormValue("title"),
		Slug:       c.FormValue("slug"),
		Content:    c.FormValue("content"),
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
	// 获取上传的文件
	form, err := c.MultipartForm()
	if err != nil {
		statusCode := fiber.StatusBadRequest
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request entity too large") {
			statusCode = fiber.StatusRequestEntityTooLarge
		}
		c.Status(statusCode)
		logger.Warn("import multipart parse failed: status=%d method=%s path=%s ip=%s err=%v", statusCode, c.Method(), c.Path(), c.IP(), err)
		return RenderAdminView(c, "dash/import.html", fiber.Map{
			"Title": "Import Markdown",
			"Error": "Failed to parse form: " + err.Error(),
		}, "")
	}

	files := form.File["files"]
	if len(files) == 0 {
		return RenderAdminView(c, "dash/import.html", fiber.Map{
			"Title": "Import Markdown",
			"Error": "Please select at least one file to import",
		}, "")
	}

	// 获取 slug 来源选择
	slugSourceStr := c.FormValue("slug_source")
	slugSource := SlugFromTitle // 默认从 title 生成
	switch slugSourceStr {
	case "filename":
		slugSource = SlugFromFilename
	case "frontmatter":
		slugSource = SlugFromFrontmatter
	case "title":
		slugSource = SlugFromTitle
	}

	// 如果是从 frontmatter，获取字段名
	slugField := c.FormValue("slug_field")
	if slugField == "" {
		slugField = "slug"
	}

	// 获取 title 来源选择
	titleSourceStr := c.FormValue("title_source")
	titleSource := TitleFromFrontmatter // 默认从 frontmatter 获取
	switch titleSourceStr {
	case "filename":
		titleSource = TitleFromFilename
	case "frontmatter":
		titleSource = TitleFromFrontmatter
	case "markdown":
		titleSource = TitleFromMarkdown
	}

	// 如果是从 frontmatter，获取字段名
	titleField := c.FormValue("title_field")
	if titleField == "" {
		titleField = "title"
	}

	// 如果是从 markdown，获取标题级别
	titleLevel := 1 // 默认 H1
	if titleSourceStr == "markdown" {
		if levelStr := c.FormValue("title_level"); levelStr != "" {
			if level, err := strconv.Atoi(levelStr); err == nil && level >= 1 && level <= 3 {
				titleLevel = level
			}
		}
	}

	// 读取所有文件
	var importFiles []ImportFile
	for _, fileHeader := range files {
		src, err := fileHeader.Open()
		if err != nil {
			return RenderAdminView(c, "dash/import.html", fiber.Map{
				"Title": "Import Markdown",
				"Error": "Failed to open file " + fileHeader.Filename + ": " + err.Error(),
			}, "")
		}

		content, err := io.ReadAll(src)
		src.Close()
		if err != nil {
			return RenderAdminView(c, "dash/import.html", fiber.Map{
				"Title": "Import Markdown",
				"Error": "Failed to read file " + fileHeader.Filename + ": " + err.Error(),
			}, "")
		}

		// 提取文件名（不含扩展名）
		filename := fileHeader.Filename
		if idx := strings.LastIndex(filename, "."); idx > 0 {
			filename = filename[:idx]
		}

		importFiles = append(importFiles, ImportFile{
			Filename: filename,
			Content:  string(content),
		})
	}

	// 获取 created_at 来源选择
	createdSourceStr := c.FormValue("created_source")
	createdSource := CreatedFromFrontmatter // 默认从 frontmatter 获取
	switch createdSourceStr {
	case "frontmatter":
		createdSource = CreatedFromFrontmatter
	case "filetime":
		createdSource = CreatedFromFileTime
	default:
		createdSource = CreatedFromFrontmatter
	}

	// 如果是从 frontmatter，获取字段名
	createdField := c.FormValue("created_field")
	if createdField == "" {
		createdField = "date"
	}

	// 获取 status 来源选择
	statusSourceStr := c.FormValue("status_source")
	statusSource := StatusFromFrontmatter // 默认从 frontmatter 获取
	switch statusSourceStr {
	case "frontmatter":
		statusSource = StatusFromFrontmatter
	case "alldraft":
		statusSource = StatusAllDraft
	case "allpublished":
		statusSource = StatusAllPublished
	default:
		statusSource = StatusFromFrontmatter
	}

	// 如果是从 frontmatter，获取字段名
	statusField := c.FormValue("status_field")
	if statusField == "" {
		statusField = "draft"
	}

	// 获取 category 来源选择
	categorySourceStr := c.FormValue("category_source")
	categorySource := CategoryNone // 默认留空
	switch categorySourceStr {
	case "frontmatter":
		categorySource = CategoryFromFrontmatter
	case "autocreate":
		categorySource = CategoryAutoCreate
	case "none":
		categorySource = CategoryNone
	default:
		categorySource = CategoryNone
	}

	// 如果是从 frontmatter，获取字段名
	categoryField := c.FormValue("category_field")
	if categoryField == "" {
		categoryField = "category"
	}

	// 获取 tag 来源选择
	tagSourceStr := c.FormValue("tag_source")
	tagSource := TagNone // 默认留空
	switch tagSourceStr {
	case "frontmatter":
		tagSource = TagFromFrontmatter
	case "autocreate":
		tagSource = TagAutoCreate
	case "none":
		tagSource = TagNone
	default:
		tagSource = TagNone
	}

	// 如果是从 frontmatter，获取字段名
	tagField := c.FormValue("tag_field")
	if tagField == "" {
		tagField = "tags"
	}

	items, parseErr := ParseImportFiles(importFiles, slugSource, slugField, titleSource, titleField, titleLevel, createdSource, createdField, statusSource, statusField, categorySource, categoryField, tagSource, tagField)
	if parseErr != nil && len(items) == 0 {
		return RenderAdminView(c, "dash/import.html", fiber.Map{
			"Title": "Import Markdown",
			"Error": parseErr.Error(),
		}, "")
	}
	if len(items) == 0 {
		return RenderAdminView(c, "dash/import.html", fiber.Map{
			"Title": "Import Markdown",
			"Error": "No items to import",
		}, "")
	}

	if err := ImportPreviewService(h.Model, items); err != nil {
		return RenderAdminView(c, "dash/import.html", fiber.Map{
			"Title": "Import Markdown",
			"Error": err.Error(),
		}, "")
	}

	success := fmt.Sprintf("Import finished: %d items imported", len(items))
	if parseErr != nil {
		success += " (with warning: " + parseErr.Error() + ")"
	}

	return RenderAdminView(c, "dash/import.html", fiber.Map{
		"Title":   "Import Markdown",
		"Success": success,
	}, "")
}

func (h *Handler) GetDevUIComponentsHandler(c fiber.Ctx) error {
	return RenderAdminView(c, "dash/dev_ui_components.html", fiber.Map{
		"Title": "UI组件",
	}, "")
}

// Export
func (h *Handler) GetExportHandler(c fiber.Ctx) error {
	return RenderAdminView(c, "dash/export.html", fiber.Map{
		"Title": "导出数据库",
	}, "")
}

func (h *Handler) GetExportDownloadHandler(c fiber.Ctx) error {
	// 生成导出文件名（包含时间戳）
	name := strings.ToLower(c.App().Config().AppName) + "_export"

	logger.Info("export to: tmp_dir=%s name=%s", os.TempDir(), name)

	// 创建临时目录
	tmpDir, err := os.MkdirTemp(os.TempDir(), name+"-")
	if err != nil {
		return RenderAdminView(c, "dash/export.html", fiber.Map{
			"Title": "导出数据库",
			"Error": "Failed to create export directory: " + err.Error(),
		}, "")
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
		return RenderAdminView(c, "dash/export.html", fiber.Map{
			"Title": "导出数据库",
			"Error": "Failed to export database: " + err.Error(),
		}, "")
	}

	// 返回文件下载（从完整路径中提取文件名）
	filename := filepath.Base(result.File)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Set("Content-Type", "application/x-sqlite3")

	// 发送文件
	if err := c.SendFile(result.File); err != nil {
		cleanupTmpDir()
		return RenderAdminView(c, "dash/export.html", fiber.Map{
			"Title": "导出数据库",
			"Error": "Failed to send file: " + err.Error(),
		}, "")
	}

	// 下载完成后删除临时文件
	go func(dir string) {
		time.Sleep(5 * time.Second) // 等待下载完成
		if removeErr := os.RemoveAll(dir); removeErr != nil {
			logger.Warn("export cleanup temp dir failed: dir=%s err=%v", dir, removeErr)
		}
	}(tmpDir)

	return nil
}
