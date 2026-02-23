package asset

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type ImageKitConfig struct {
	Endpoint   string
	PrivateKey string
	Client     *http.Client
}

type ImageKitProvider struct {
	apiBaseURL string
	uploadURL  string
	privateKey string
	client     *http.Client
}

func NewImageKitProvider(cfg ImageKitConfig) *ImageKitProvider {
	uploadURL := resolveImageKitUploadURL(cfg.Endpoint)
	apiBaseURL := deriveImageKitAPIBaseURL(uploadURL)

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &ImageKitProvider{
		apiBaseURL: strings.TrimRight(apiBaseURL, "/"),
		uploadURL:  uploadURL,
		privateKey: strings.TrimSpace(cfg.PrivateKey),
		client:     client,
	}
}

func (p *ImageKitProvider) Name() string {
	return "imagekit"
}

func (p *ImageKitProvider) Upload(ctx context.Context, in UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(p.privateKey) == "" {
		return nil, errors.New("ImageKit private key is empty")
	}
	if err := validateImageKitUploadURL(p.uploadURL); err != nil {
		return nil, err
	}
	if len(in.Bytes) == 0 {
		return nil, errors.New("upload file is empty")
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("file", in.FileName)
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(in.Bytes); err != nil {
		return nil, err
	}
	if err = writer.WriteField("fileName", in.FileName); err != nil {
		return nil, err
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.uploadURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", p.basicAuthHeader())
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyText := strings.TrimSpace(string(raw))
		if resp.StatusCode == http.StatusForbidden && strings.Contains(strings.ToLower(bodyText), "cloudfront") {
			return nil, fmt.Errorf(
				"ImageKit upload failed: status=%d endpoint=%s，看起来 endpoint 不是上传 API 地址，请改为 https://upload.imagekit.io/api/v1",
				resp.StatusCode,
				p.uploadURL,
			)
		}
		return nil, fmt.Errorf("ImageKit upload failed: status=%d body=%s", resp.StatusCode, compactImageKitErrorBody(bodyText))
	}

	var payload struct {
		FileID string `json:"fileId"`
		Name   string `json:"name"`
		URL    string `json:"url"`
		Size   int64  `json:"size"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse ImageKit upload response failed: %w", err)
	}

	if strings.TrimSpace(payload.FileID) == "" {
		return nil, errors.New("ImageKit upload response missing fileId")
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		name = strings.TrimSpace(in.FileName)
	}

	sizeBytes := payload.Size
	if sizeBytes <= 0 {
		sizeBytes = int64(len(in.Bytes))
	}

	return &UploadResult{
		ProviderAssetID:   strings.TrimSpace(payload.FileID),
		ProviderDeleteKey: strings.TrimSpace(payload.FileID),
		FileURL:           strings.TrimSpace(payload.URL),
		OriginalName:      name,
		SizeBytes:         sizeBytes,
	}, nil
}

func (p *ImageKitProvider) List(ctx context.Context, in ListInput) (*ListResult, error) {
	if strings.TrimSpace(p.privateKey) == "" {
		return nil, errors.New("ImageKit private key is empty")
	}

	page := in.Page
	if page <= 0 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	skip := (page - 1) * pageSize
	query := url.Values{}
	query.Set("limit", strconv.Itoa(pageSize))
	query.Set("skip", strconv.Itoa(skip))

	listURL := p.apiBaseURL + "/files?" + query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", p.basicAuthHeader())

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ImageKit list failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var payload []struct {
		FileID    string `json:"fileId"`
		Name      string `json:"name"`
		URL       string `json:"url"`
		Size      int64  `json:"size"`
		CreatedAt string `json:"createdAt"`
	}
	if err = json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse ImageKit list response failed: %w", err)
	}

	items := make([]ListItem, 0, len(payload))
	for _, item := range payload {
		createdAt := int64(0)
		if strings.TrimSpace(item.CreatedAt) != "" {
			if ts, err := time.Parse(time.RFC3339, strings.TrimSpace(item.CreatedAt)); err == nil {
				createdAt = ts.Unix()
			}
		}

		items = append(items, ListItem{
			ProviderAssetID:   strings.TrimSpace(item.FileID),
			ProviderDeleteKey: strings.TrimSpace(item.FileID),
			FileURL:           strings.TrimSpace(item.URL),
			OriginalName:      strings.TrimSpace(item.Name),
			SizeBytes:         item.Size,
			CreatedAt:         createdAt,
		})
	}

	return &ListResult{Items: items}, nil
}

func (p *ImageKitProvider) Delete(ctx context.Context, deleteKey string) error {
	if strings.TrimSpace(p.privateKey) == "" {
		return errors.New("ImageKit private key is empty")
	}
	deleteKey = strings.TrimSpace(deleteKey)
	if deleteKey == "" {
		return errors.New("delete key is empty")
	}

	deleteURL := p.apiBaseURL + "/files/" + url.PathEscape(deleteKey)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, deleteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", p.basicAuthHeader())

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ImageKit delete failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	return nil
}

func (p *ImageKitProvider) basicAuthHeader() string {
	encoded := base64.StdEncoding.EncodeToString([]byte(p.privateKey + ":"))
	return "Basic " + encoded
}

func resolveImageKitUploadURL(endpoint string) string {
	const defaultUploadURL = "https://upload.imagekit.io/api/v1/files/upload"

	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return defaultUploadURL
	}

	endpoint = strings.TrimRight(endpoint, "/")
	lower := strings.ToLower(endpoint)
	if strings.HasSuffix(lower, "/files/upload") {
		return endpoint
	}

	if strings.HasSuffix(lower, "/api/v1") || strings.HasSuffix(lower, "/v1") {
		return endpoint + "/files/upload"
	}

	return endpoint + "/api/v1/files/upload"
}

func deriveImageKitAPIBaseURL(uploadURL string) string {
	const defaultAPIBaseURL = "https://api.imagekit.io/v1"

	parsed, err := url.Parse(uploadURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return defaultAPIBaseURL
	}

	host := parsed.Host
	lowerHost := strings.ToLower(host)
	if strings.HasPrefix(lowerHost, "upload.") && len(host) > len("upload.") {
		host = "api." + host[len("upload."):]
	}

	return parsed.Scheme + "://" + host + "/v1"
}

func validateImageKitUploadURL(uploadURL string) error {
	parsed, err := url.Parse(uploadURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return errors.New("ImageKit endpoint invalid: expected URL like https://upload.imagekit.io/api/v1")
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "ik.imagekit.io" || strings.HasSuffix(host, ".ik.imagekit.io") {
		return errors.New("ImageKit endpoint 配置错误：当前看起来是文件访问域名，请改为上传 API 地址（例如 https://upload.imagekit.io/api/v1）")
	}

	path := strings.ToLower(strings.TrimSpace(parsed.Path))
	if !strings.Contains(path, "/api/v1/") && !strings.HasSuffix(path, "/api/v1") &&
		!strings.Contains(path, "/v1/") && !strings.HasSuffix(path, "/v1") {
		return errors.New("ImageKit endpoint 配置错误：路径应包含 /api/v1（例如 https://upload.imagekit.io/api/v1）")
	}

	return nil
}

func compactImageKitErrorBody(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	if len(raw) > 400 {
		return raw[:400] + "...(truncated)"
	}
	return raw
}
