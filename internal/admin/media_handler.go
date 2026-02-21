package admin

import (
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/db"
	"swaves/internal/media"
	"swaves/internal/middleware"
	"swaves/internal/store"

	"github.com/gofiber/fiber/v3"
)

type mediaProviderOption struct {
	Value string
	Label string
}

var mediaProviderOptions = []mediaProviderOption{
	{Value: "see", Label: "S.EE"},
	{Value: "imagekit", Label: "ImageKit"},
}

var mediaKindOptions = []string{db.MediaKindImage, db.MediaKindBackup, db.MediaKindFile}

func (h *Handler) GetMediaListHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	kind := ""
	provider := ""
	defaultProvider := h.defaultMediaProvider()
	defaultProviderErr := h.validateMediaProviderConfig(defaultProvider)

	total, err := db.CountMedia(h.Model, kind, provider)
	if err != nil {
		log.Printf("[media] count media list failed: err=%v", err)
		return err
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

	items, err := db.ListMedia(h.Model, db.MediaQueryOptions{
		Kind:     kind,
		Provider: provider,
		Limit:    pager.PageSize,
		Offset:   offset,
	})
	if err != nil {
		log.Printf("[media] list media failed: page=%d size=%d err=%v", pager.Page, pager.PageSize, err)
		return err
	}

	return RenderAdminView(c, "media_index", fiber.Map{
		"Title":                 "Media",
		"Items":                 items,
		"Pager":                 pager,
		"DefaultProvider":       defaultProvider,
		"DefaultProviderReady":  defaultProviderErr == nil,
		"DefaultProviderError":  errorString(defaultProviderErr),
		"MediaProviderLabelMap": mediaProviderLabelMap(),
	}, "")
}

func (h *Handler) GetMediaAssetsAPIHandler(c fiber.Ctx) error {
	pager := middleware.GetPagination(c)
	kind := normalizeMediaKind(c.Query("kind"))
	provider := normalizeMediaProvider(c.Query("provider"))

	total, err := db.CountMedia(h.Model, kind, provider)
	if err != nil {
		log.Printf("[media] count assets failed: kind=%s provider=%s err=%v", kind, provider, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	offset := (pager.Page - 1) * pager.PageSize
	items, err := db.ListMedia(h.Model, db.MediaQueryOptions{
		Kind:     kind,
		Provider: provider,
		Limit:    pager.PageSize,
		Offset:   offset,
	})
	if err != nil {
		log.Printf("[media] list assets failed: kind=%s provider=%s page=%d size=%d err=%v", kind, provider, pager.Page, pager.PageSize, err)
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

func (h *Handler) PostMediaUploadAPIHandler(c fiber.Ctx) error {
	providerName := h.defaultMediaProvider()

	provider, err := h.resolveMediaProvider(providerName)
	if err != nil {
		log.Printf("[media] resolve provider failed: provider=%s err=%v", providerName, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}
	if err = h.validateMediaProviderConfig(provider.Name()); err != nil {
		log.Printf("[media] upload blocked: provider=%s reason=%v", provider.Name(), err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	kind := db.MediaKindImage

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

	uploaded, err := provider.Upload(c.RequestCtx(), media.UploadInput{
		FileName:    fileHeader.Filename,
		ContentType: contentType,
		Bytes:       content,
	})
	if err != nil {
		log.Printf("[media] upload failed: provider=%s file=%s err=%v", provider.Name(), fileHeader.Filename, err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	item := &db.Media{
		Kind:              kind,
		Provider:          provider.Name(),
		ProviderAssetID:   strings.TrimSpace(uploaded.ProviderAssetID),
		ProviderDeleteKey: strings.TrimSpace(uploaded.ProviderDeleteKey),
		FileURL:           strings.TrimSpace(uploaded.FileURL),
		OriginalName:      strings.TrimSpace(uploaded.OriginalName),
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

	_, err = db.CreateMedia(h.Model, item)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
			existing, getErr := db.GetMediaByProviderAssetID(h.Model, item.Provider, item.ProviderAssetID)
			if getErr == nil {
				return c.JSON(fiber.Map{
					"ok":        true,
					"duplicate": true,
					"data":      existing,
				})
			}
		}
		log.Printf("[media] create media record failed: provider=%s asset_id=%s name=%s err=%v", item.Provider, item.ProviderAssetID, item.OriginalName, err)
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

func (h *Handler) DeleteMediaAssetAPIHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "invalid id",
		})
	}

	item, err := db.GetMediaByID(h.Model, id)
	if err != nil {
		if db.IsErrNotFound(err) {
			return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
				"ok":    false,
				"error": "media not found",
			})
		}
		log.Printf("[media] get media by id failed: id=%d err=%v", id, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	provider, err := h.resolveMediaProvider(item.Provider)
	if err != nil {
		log.Printf("[media] resolve provider for delete failed: id=%d provider=%s err=%v", id, item.Provider, err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}
	if err = h.validateMediaProviderConfig(provider.Name()); err != nil {
		log.Printf("[media] delete blocked by provider config: id=%d provider=%s err=%v", id, provider.Name(), err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	deleteKey := strings.TrimSpace(item.ProviderDeleteKey)
	if deleteKey == "" {
		deleteKey = strings.TrimSpace(item.ProviderAssetID)
	}
	if deleteKey == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"ok":    false,
			"error": "missing delete key",
		})
	}

	if err = provider.Delete(c.RequestCtx(), deleteKey); err != nil {
		log.Printf("[media] provider delete failed: id=%d provider=%s delete_key=%s err=%v", id, provider.Name(), deleteKey, err)
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	if err = db.DeleteMedia(h.Model, item.ID); err != nil {
		log.Printf("[media] delete media record failed: id=%d err=%v", item.ID, err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"ok": true,
	})
}

func (h *Handler) resolveMediaProvider(providerName string) (media.Provider, error) {
	factory := media.NewFactory(media.FactoryConfig{
		DefaultProvider: h.defaultMediaProvider(),
		SEEBaseURL:      store.GetSetting("media_see_api_base"),
		SEEToken:        store.GetSetting("media_see_api_token"),

		ImageKitEndpoint:   store.GetSetting("media_imagekit_endpoint"),
		ImageKitPrivateKey: store.GetSetting("media_imagekit_private_key"),
	})

	rawProvider := strings.TrimSpace(providerName)
	resolved := normalizeMediaProvider(providerName)
	if rawProvider != "" && resolved == "" {
		return nil, errors.New("unsupported provider: " + providerName)
	}

	provider := factory.Resolve(resolved)
	if provider == nil {
		return nil, errors.New("unable to resolve media provider")
	}
	return provider, nil
}

func (h *Handler) defaultMediaProvider() string {
	provider := strings.TrimSpace(strings.ToLower(store.GetSetting("media_default_provider")))
	if provider == "imagekit" {
		return provider
	}
	return "see"
}

func mediaProviderLabelMap() map[string]string {
	result := map[string]string{}
	for _, item := range mediaProviderOptions {
		result[item.Value] = item.Label
	}
	return result
}

func normalizeMediaProvider(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	switch raw {
	case "s.ee", "smms", "sm.ms":
		raw = "see"
	}

	for _, item := range mediaProviderOptions {
		if raw == item.Value {
			return raw
		}
	}
	return ""
}

func normalizeMediaKind(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	for _, item := range mediaKindOptions {
		if raw == item {
			return raw
		}
	}
	return ""
}

func (h *Handler) validateMediaProviderConfig(providerName string) error {
	switch strings.TrimSpace(strings.ToLower(providerName)) {
	case "imagekit":
		if strings.TrimSpace(store.GetSetting("media_imagekit_private_key")) == "" {
			return errors.New("ImageKit Private Key 未配置，请到设置 > 第三方服务 填写")
		}
		imageKitEndpoint := strings.TrimSpace(store.GetSetting("media_imagekit_endpoint"))
		if imageKitEndpoint == "" {
			return errors.New("ImageKit-endpoint 未配置，请到设置 > 第三方服务 填写")
		}
		if err := validateImageKitEndpoint(imageKitEndpoint); err != nil {
			return err
		}
	case "see":
		fallthrough
	default:
		if strings.TrimSpace(store.GetSetting("media_see_api_token")) == "" {
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
