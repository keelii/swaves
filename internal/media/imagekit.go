package media

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
	APIBaseURL    string
	UploadBaseURL string
	PrivateKey    string
	Client        *http.Client
}

type ImageKitProvider struct {
	apiBaseURL    string
	uploadBaseURL string
	privateKey    string
	client        *http.Client
}

func NewImageKitProvider(cfg ImageKitConfig) *ImageKitProvider {
	apiBaseURL := strings.TrimSpace(cfg.APIBaseURL)
	if apiBaseURL == "" {
		apiBaseURL = "https://api.imagekit.io/v1"
	}

	uploadBaseURL := strings.TrimSpace(cfg.UploadBaseURL)
	if uploadBaseURL == "" {
		uploadBaseURL = "https://upload.imagekit.io/api/v1"
	}

	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &ImageKitProvider{
		apiBaseURL:    strings.TrimRight(apiBaseURL, "/"),
		uploadBaseURL: strings.TrimRight(uploadBaseURL, "/"),
		privateKey:    strings.TrimSpace(cfg.PrivateKey),
		client:        client,
	}
}

func (p *ImageKitProvider) Name() string {
	return "imagekit"
}

func (p *ImageKitProvider) Upload(ctx context.Context, in UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(p.privateKey) == "" {
		return nil, errors.New("ImageKit private key is empty")
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

	uploadURL := p.uploadBaseURL + "/files/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
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
		return nil, fmt.Errorf("ImageKit upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
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
