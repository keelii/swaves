package admin

import (
	"errors"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"swaves/internal/asset"
	"swaves/internal/db"
	"swaves/internal/logger"
	"swaves/internal/middleware"
	"swaves/internal/store"

	"github.com/gofiber/fiber/v3"
)

type assetProviderOption struct {
	Value string
	Label string
}

var assetProviderOptions = []assetProviderOption{
	{Value: "see", Label: "S.EE"},
	{Value: "imagekit", Label: "ImageKit"},
}

var assetKindOptions = []string{db.AssetKindImage, db.AssetKindBackup, db.AssetKindFile}

var imageAssetExtensions = map[string]struct{}{
	"apng": {},
	"avif": {},
	"bmp":  {},
	"gif":  {},
	"heic": {},
	"heif": {},
	"ico":  {},
	"jpeg": {},
	"jpg":  {},
	"png":  {},
	"svg":  {},
	"tif":  {},
	"tiff": {},
	"webp": {},
}

func (h *Handler) GetAssetListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	kind := normalizeAssetKind(c.Query("kind"))
	if kind == "" {
		kind = db.AssetKindImage
	}
	provider := ""
	defaultProvider := h.defaultAssetProvider()
	defaultProviderErr := h.validateAssetProviderConfig(defaultProvider)
	kindCounts := map[string]int{}

	total, err := db.CountAssets(h.Model, kind, provider)
	if err != nil {
		logger.Error("[asset] count asset list failed: err=%v", err)
		return err
	}
	kindCounts[""] = total
	for _, item := range assetKindOptions {
		count, countErr := db.CountAssets(h.Model, item, provider)
		if countErr != nil {
			logger.Error("[asset] count asset kind failed: kind=%s err=%v", item, countErr)
			return countErr
		}
		kindCounts[item] = count
	}
	pager.Total = total
	if pager.PageSize > 0 {
		pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	}
	if pager.Page < 1 {
		pager.Page = 1
	}
	if pager.Num > 0 && pager.Page > pager.Num {
		pager.Page = pager.Num
	}
	offset := (pager.Page - 1) * pager.PageSize

	items, err := db.ListAssets(h.Model, db.AssetQueryOptions{
		Kind:     kind,
		Provider: provider,
		Limit:    pager.PageSize,
		Offset:   offset,
	})
	if err != nil {
		logger.Error("[asset] list assets failed: page=%d size=%d err=%v", pager.Page, pager.PageSize, err)
		return err
	}

	return RenderAdminView(c, "assets_index", fiber.Map{
		"Title":                 "资源库",
		"Items":                 items,
		"Pager":                 pager,
		"CurrentKind":           kind,
		"KindCounts":            kindCounts,
		"AssetKindLabelMap":     assetKindLabelMap(),
		"DefaultProvider":       defaultProvider,
		"DefaultProviderReady":  defaultProviderErr == nil,
		"DefaultProviderError":  errorString(defaultProviderErr),
		"AssetProviderLabelMap": assetProviderLabelMap(),
	}, "")
}

func (h *Handler) GetAssetListAPIHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	kind := normalizeAssetKind(c.Query("kind"))
	provider := normalizeAssetProvider(c.Query("provider"))

	total, err := db.CountAssets(h.Model, kind, provider)
	if err != nil {
		logger.Error("[asset] count assets failed: kind=%s provider=%s err=%v", kind, provider, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	offset := (pager.Page - 1) * pager.PageSize
	items, err := db.ListAssets(h.Model, db.AssetQueryOptions{
		Kind:     kind,
		Provider: provider,
		Limit:    pager.PageSize,
		Offset:   offset,
	})
	if err != nil {
		logger.Error("[asset] list assets failed: kind=%s provider=%s page=%d size=%d err=%v", kind, provider, pager.Page, pager.PageSize, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	pager.Total = total
	if pager.PageSize > 0 {
		pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	}

	return c.JSON(fiber.Map{
		"ok": true,
		"data": fiber.Map{
			"items": items,
			"pager": pager,
		},
	})
}

func (h *Handler) PostAssetUploadAPIHandler(c fiber.Ctx) error {
	providerName := h.defaultAssetProvider()
	remark := strings.TrimSpace(c.FormValue("remark"))

	provider, err := h.resolveAssetProvider(providerName)
	if err != nil {
		logger.Warn("[asset] resolve provider failed: provider=%s err=%v", providerName, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}
	if err = h.validateAssetProviderConfig(provider.Name()); err != nil {
		logger.Warn("[asset] upload blocked: provider=%s reason=%v", provider.Name(), err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

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
	if len(content) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "file is empty",
		})
	}

	contentType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}

	uploaded, err := provider.Upload(c.RequestCtx(), asset.UploadInput{
		FileName:    fileHeader.Filename,
		ContentType: contentType,
		Bytes:       content,
	})
	if err != nil {
		logger.Error("[asset] upload failed: provider=%s file=%s err=%v", provider.Name(), fileHeader.Filename, err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	originalName := strings.TrimSpace(uploaded.OriginalName)
	if originalName == "" {
		originalName = strings.TrimSpace(fileHeader.Filename)
	}

	item := &db.Asset{
		Kind:              detectAssetKindByUploadedSuffix(uploaded, originalName),
		Provider:          provider.Name(),
		ProviderAssetID:   strings.TrimSpace(uploaded.ProviderAssetID),
		ProviderDeleteKey: strings.TrimSpace(uploaded.ProviderDeleteKey),
		FileURL:           strings.TrimSpace(uploaded.FileURL),
		OriginalName:      originalName,
		Remark:            remark,
		SizeBytes:         uploaded.SizeBytes,
	}
	if item.ProviderAssetID == "" {
		item.ProviderAssetID = item.ProviderDeleteKey
	}
	if item.ProviderDeleteKey == "" {
		item.ProviderDeleteKey = item.ProviderAssetID
	}
	if item.OriginalName == "" {
		item.OriginalName = fileHeader.Filename
	}
	if item.SizeBytes <= 0 {
		item.SizeBytes = int64(len(content))
	}

	_, err = db.CreateAsset(h.Model, item)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
			existing, getErr := db.GetAssetByProviderAssetID(h.Model, item.Provider, item.ProviderAssetID)
			if getErr == nil {
				return c.JSON(fiber.Map{
					"ok":        true,
					"duplicate": true,
					"data":      existing,
				})
			}
		}
		logger.Error("[asset] create asset record failed: provider=%s asset_id=%s name=%s err=%v", item.Provider, item.ProviderAssetID, item.OriginalName, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"ok":   true,
		"data": item,
	})
}

func detectAssetKindByUploadedSuffix(uploaded *asset.UploadResult, fallbackName string) string {
	if uploaded == nil {
		if ext := normalizedAssetSuffix(fallbackName); ext != "" && isImageAssetSuffix(ext) {
			return db.AssetKindImage
		}
		return db.AssetKindFile
	}

	if ext := normalizedAssetSuffix(uploaded.OriginalName); ext != "" {
		if isImageAssetSuffix(ext) {
			return db.AssetKindImage
		}
		return db.AssetKindFile
	}

	if ext := normalizedAssetSuffix(uploaded.FileURL); ext != "" {
		if isImageAssetSuffix(ext) {
			return db.AssetKindImage
		}
		return db.AssetKindFile
	}

	if ext := normalizedAssetSuffix(fallbackName); ext != "" && isImageAssetSuffix(ext) {
		return db.AssetKindImage
	}
	return db.AssetKindFile
}

func normalizedAssetSuffix(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	parsed, err := url.Parse(raw)
	if err == nil {
		if parsed.Scheme != "" || parsed.Host != "" {
			raw = parsed.Path
		} else if parsed.Path != "" {
			raw = parsed.Path
		}
	}

	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if idx := strings.IndexAny(raw, "?#"); idx >= 0 {
		raw = raw[:idx]
	}

	return strings.TrimPrefix(strings.ToLower(path.Ext(raw)), ".")
}

func isImageAssetSuffix(suffix string) bool {
	_, ok := imageAssetExtensions[strings.TrimSpace(strings.ToLower(suffix))]
	return ok
}

func (h *Handler) DeleteAssetAPIHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "invalid id",
		})
	}

	warning, statusCode, err := h.deleteAssetByID(c, id)
	if err != nil {
		return c.Status(statusCode).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	response := fiber.Map{"ok": true}
	if warning != "" {
		response["warning"] = warning
	}
	return c.JSON(response)
}

type assetBatchDeletePayload struct {
	IDs []int64 `json:"ids"`
}

func (h *Handler) PostAssetBatchDeleteAPIHandler(c fiber.Ctx) error {
	var payload assetBatchDeletePayload
	if err := c.Bind().Body(&payload); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "invalid json",
		})
	}

	ids := normalizeAssetIDs(payload.IDs)
	if len(ids) == 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "ids is required",
		})
	}

	deletedIDs := make([]int64, 0, len(ids))
	failed := make([]fiber.Map, 0)
	warnings := make([]fiber.Map, 0)
	for _, id := range ids {
		warning, statusCode, err := h.deleteAssetByID(c, id)
		if err != nil {
			failed = append(failed, fiber.Map{
				"id":     id,
				"status": statusCode,
				"error":  err.Error(),
			})
			continue
		}

		deletedIDs = append(deletedIDs, id)
		if warning != "" {
			warnings = append(warnings, fiber.Map{
				"id":      id,
				"warning": warning,
			})
		}
	}

	ok := len(failed) == 0
	response := fiber.Map{
		"ok":              ok,
		"requested_count": len(ids),
		"deleted_count":   len(deletedIDs),
		"failed_count":    len(failed),
		"deleted_ids":     deletedIDs,
		"failed":          failed,
		"warnings":        warnings,
	}
	if !ok {
		return c.Status(fiber.StatusMultiStatus).JSON(response)
	}
	return c.JSON(response)
}

func (h *Handler) deleteAssetByID(c fiber.Ctx, id int64) (string, int, error) {
	item, err := db.GetAssetByID(h.Model, id)
	if err != nil {
		if db.IsErrNotFound(err) {
			return "", fiber.StatusNotFound, errors.New("asset not found")
		}
		logger.Error("[asset] get asset by id failed: id=%d err=%v", id, err)
		return "", fiber.StatusInternalServerError, err
	}

	provider, err := h.resolveAssetProvider(item.Provider)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unsupported provider") {
			logger.Warn("[asset] skip provider delete for unsupported provider: id=%d provider=%s", id, item.Provider)
			if err = db.DeleteAsset(h.Model, item.ID); err != nil {
				logger.Error("[asset] delete asset record failed after unsupported provider skip: id=%d err=%v", item.ID, err)
				return "", fiber.StatusInternalServerError, err
			}
			return "provider unsupported, deleted metadata only", 0, nil
		}
		logger.Warn("[asset] resolve provider for delete failed: id=%d provider=%s err=%v", id, item.Provider, err)
		return "", fiber.StatusBadRequest, err
	}
	if err = h.validateAssetProviderConfig(provider.Name()); err != nil {
		logger.Warn("[asset] delete blocked by provider config: id=%d provider=%s err=%v", id, provider.Name(), err)
		return "", fiber.StatusBadRequest, err
	}

	deleteKey := strings.TrimSpace(item.ProviderDeleteKey)
	if deleteKey == "" {
		deleteKey = strings.TrimSpace(item.ProviderAssetID)
	}
	if deleteKey == "" {
		return "", fiber.StatusBadRequest, errors.New("missing delete key")
	}

	if err = provider.Delete(c.RequestCtx(), deleteKey); err != nil {
		logger.Error("[asset] provider delete failed: id=%d provider=%s delete_key=%s err=%v", id, provider.Name(), deleteKey, err)
		return "", fiber.StatusBadGateway, err
	}

	if err = db.DeleteAsset(h.Model, item.ID); err != nil {
		logger.Error("[asset] delete asset record failed: id=%d err=%v", item.ID, err)
		return "", fiber.StatusInternalServerError, err
	}

	return "", 0, nil
}

func normalizeAssetIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}

	seen := map[int64]struct{}{}
	result := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (h *Handler) resolveAssetProvider(providerName string) (asset.Provider, error) {
	factory := asset.NewFactory(asset.FactoryConfig{
		DefaultProvider: h.defaultAssetProvider(),
		SEEBaseURL:      store.GetSetting("asset_see_api_base"),
		SEEToken:        store.GetSetting("asset_see_api_token"),

		ImageKitEndpoint:   store.GetSetting("asset_imagekit_endpoint"),
		ImageKitPrivateKey: store.GetSetting("asset_imagekit_private_key"),
	})

	rawProvider := strings.TrimSpace(providerName)
	resolved := normalizeAssetProvider(providerName)
	if rawProvider != "" && resolved == "" {
		return nil, errors.New("unsupported provider: " + providerName)
	}

	provider := factory.Resolve(resolved)
	if provider == nil {
		return nil, errors.New("unable to resolve asset provider")
	}
	return provider, nil
}

func (h *Handler) defaultAssetProvider() string {
	provider := strings.TrimSpace(strings.ToLower(store.GetSetting("asset_default_provider")))
	if provider == "imagekit" {
		return provider
	}
	return "see"
}

func assetProviderLabelMap() map[string]string {
	result := map[string]string{}
	for _, item := range assetProviderOptions {
		result[item.Value] = item.Label
	}
	return result
}

func assetKindLabelMap() map[string]string {
	return map[string]string{
		"":                 "全部",
		db.AssetKindImage:  "图片",
		db.AssetKindFile:   "文件",
		db.AssetKindBackup: "备份",
	}
}

func normalizeAssetProvider(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "s.ee", "smms", "sm.ms":
		raw = "see"
	}

	for _, item := range assetProviderOptions {
		if raw == item.Value {
			return raw
		}
	}
	return ""
}

func normalizeAssetKind(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	for _, item := range assetKindOptions {
		if raw == item {
			return raw
		}
	}
	return ""
}

func (h *Handler) validateAssetProviderConfig(providerName string) error {
	switch strings.TrimSpace(strings.ToLower(providerName)) {
	case "imagekit":
		if strings.TrimSpace(store.GetSetting("asset_imagekit_private_key")) == "" {
			return errors.New("ImageKit Private Key 未配置，请到设置 > 第三方服务 填写")
		}
		imageKitEndpoint := strings.TrimSpace(store.GetSetting("asset_imagekit_endpoint"))
		if imageKitEndpoint == "" {
			return errors.New("ImageKit-endpoint 未配置，请到设置 > 第三方服务 填写")
		}
		if err := validateImageKitEndpoint(imageKitEndpoint); err != nil {
			return err
		}
	case "see":
		fallthrough
	default:
		if strings.TrimSpace(store.GetSetting("asset_see_api_token")) == "" {
			return errors.New("S.EE API Token 未配置，请到设置 > 第三方服务 填写")
		}
	}
	return nil
}

func validateImageKitEndpoint(endpoint string) error {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("ImageKit-endpoint 格式错误，请填写类似 https://upload.imagekit.io/api/v1")
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "ik.imagekit.io" || strings.HasSuffix(host, ".ik.imagekit.io") {
		return errors.New("ImageKit-endpoint 看起来是文件访问域名，请改为上传 API 地址（例如 https://upload.imagekit.io/api/v1）")
	}

	path := strings.ToLower(strings.TrimSpace(parsed.Path))
	if !strings.Contains(path, "/api/v1") && !strings.Contains(path, "/v1") {
		return errors.New("ImageKit-endpoint 路径应包含 /api/v1（例如 https://upload.imagekit.io/api/v1）")
	}

	return nil
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
