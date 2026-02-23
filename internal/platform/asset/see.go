package asset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

type SEEConfig struct {
	BaseURL string
	Token   string
	Client  *http.Client
}

type SEEProvider struct {
	apiBaseURL string
	token      string
	client     *http.Client
}

func NewSEEProvider(cfg SEEConfig) *SEEProvider {
	client := cfg.Client
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}

	return &SEEProvider{
		apiBaseURL: normalizeSEEAPIBaseURL(cfg.BaseURL),
		token:      strings.TrimSpace(cfg.Token),
		client:     client,
	}
}

func (p *SEEProvider) Name() string {
	return "see"
}

func (p *SEEProvider) Upload(ctx context.Context, in UploadInput) (*UploadResult, error) {
	if strings.TrimSpace(p.token) == "" {
		return nil, errors.New("S.EE token is empty")
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
	if err = writer.Close(); err != nil {
		return nil, err
	}

	uploadURL := p.apiBaseURL + "/file/upload"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadURL, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)
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
		return nil, fmt.Errorf("S.EE upload failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	payload, err := parseSEEUploadPayload(raw)
	if err != nil {
		return nil, err
	}

	deleteKey := payload.Data.Hash.String()
	if deleteKey == "" {
		deleteKey = parseSEEDeleteKey(payload.Data.Delete.String())
	}

	out := &UploadResult{
		ProviderAssetID:   payload.Data.FileID.String(),
		ProviderDeleteKey: deleteKey,
		FileURL:           payload.Data.URL.String(),
		OriginalName:      payload.Data.Filename.String(),
		SizeBytes:         int64(payload.Data.Size),
	}

	if out.ProviderAssetID == "" {
		out.ProviderAssetID = deleteKey
	}
	if out.ProviderAssetID == "" {
		return nil, errors.New("S.EE upload response missing file identifier")
	}
	if out.OriginalName == "" {
		out.OriginalName = strings.TrimSpace(in.FileName)
	}
	if out.SizeBytes <= 0 {
		out.SizeBytes = int64(len(in.Bytes))
	}

	return out, nil
}

func (p *SEEProvider) List(ctx context.Context, in ListInput) (*ListResult, error) {
	if strings.TrimSpace(p.token) == "" {
		return nil, errors.New("S.EE token is empty")
	}

	page := in.Page
	if page <= 0 {
		page = 1
	}
	pageSize := in.PageSize
	if pageSize <= 0 {
		pageSize = 20
	}

	listURL := p.apiBaseURL + "/files?page=" + strconv.Itoa(page) + "&per_page=" + strconv.Itoa(pageSize)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

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
		return nil, fmt.Errorf("S.EE list failed: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	items, err := parseSEEListPayload(raw)
	if err != nil {
		return nil, err
	}

	return &ListResult{Items: items}, nil
}

func (p *SEEProvider) Delete(ctx context.Context, deleteKey string) error {
	if strings.TrimSpace(p.token) == "" {
		return errors.New("S.EE token is empty")
	}
	deleteKey = strings.TrimSpace(deleteKey)
	if deleteKey == "" {
		return errors.New("delete key is empty")
	}

	deleteURL := p.apiBaseURL + "/file/delete/" + url.PathEscape(deleteKey)
	err := p.callDelete(ctx, http.MethodGet, deleteURL)
	if err == nil {
		return nil
	}

	if strings.Contains(err.Error(), "status=404") || strings.Contains(err.Error(), "status=405") {
		return p.callDelete(ctx, http.MethodDelete, deleteURL)
	}

	return err
}

func (p *SEEProvider) callDelete(ctx context.Context, method string, deleteURL string) error {
	req, err := http.NewRequestWithContext(ctx, method, deleteURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

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
		return fmt.Errorf("S.EE delete failed: method=%s status=%d body=%s", method, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	var generic struct {
		Success bool          `json:"success"`
		Code    seeFlexString `json:"code"`
		Message seeFlexString `json:"message"`
	}
	if err = json.Unmarshal(raw, &generic); err == nil {
		code := normalizeSEECode(generic.Code.String())
		message := strings.TrimSpace(generic.Message.String())
		if generic.Success || isSEESuccessCode(code) {
			return nil
		}
		if code == "" && message == "" {
			return nil
		}
		if message == "" {
			message = "unknown error"
		}
		if code != "" {
			return fmt.Errorf("S.EE delete failed: code=%s %s", code, message)
		}
		return fmt.Errorf("S.EE delete failed: %s", message)
	}

	return nil
}

type seeUploadPayload struct {
	Success bool          `json:"success"`
	Code    seeFlexString `json:"code"`
	Message seeFlexString `json:"message"`
	Data    struct {
		FileID   seeFlexString `json:"file_id"`
		Filename seeFlexString `json:"filename"`
		URL      seeFlexString `json:"url"`
		Delete   seeFlexString `json:"delete"`
		Hash     seeFlexString `json:"hash"`
		Size     seeFlexInt64  `json:"size"`
	} `json:"data"`
}

func parseSEEUploadPayload(raw []byte) (*seeUploadPayload, error) {
	var payload seeUploadPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("parse S.EE upload response failed: %w", err)
	}

	code := normalizeSEECode(payload.Code.String())
	msg := strings.TrimSpace(payload.Message.String())
	if payload.Success || isSEESuccessCode(code) {
		return &payload, nil
	}
	if code == "" && msg == "" {
		return &payload, nil
	}
	if msg == "" {
		msg = "unknown error"
	}
	if code != "" {
		return nil, errors.New("S.EE upload failed: code=" + code + " " + msg)
	}
	return nil, errors.New("S.EE upload failed: " + msg)

}

func parseSEEListPayload(raw []byte) ([]ListItem, error) {
	var envelope struct {
		Success bool            `json:"success"`
		Code    seeFlexString   `json:"code"`
		Message seeFlexString   `json:"message"`
		Data    json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("parse S.EE list response failed: %w", err)
	}
	code := normalizeSEECode(envelope.Code.String())
	msg := strings.TrimSpace(envelope.Message.String())
	if !(envelope.Success || isSEESuccessCode(code) || (code == "" && msg == "")) {
		if msg == "" {
			msg = "unknown error"
		}
		if code != "" {
			return nil, errors.New("S.EE list failed: code=" + code + " " + msg)
		}
		return nil, errors.New("S.EE list failed: " + msg)
	}

	items := make([]seeListDataItem, 0)
	if len(envelope.Data) > 0 {
		if err := json.Unmarshal(envelope.Data, &items); err != nil {
			var nested struct {
				Items []seeListDataItem `json:"items"`
				Data  []seeListDataItem `json:"data"`
			}
			if nErr := json.Unmarshal(envelope.Data, &nested); nErr != nil {
				return nil, fmt.Errorf("parse S.EE list data failed: %w", err)
			}
			if len(nested.Items) > 0 {
				items = nested.Items
			} else {
				items = nested.Data
			}
		}
	}

	result := make([]ListItem, 0, len(items))
	for _, item := range items {
		deleteKey := item.Hash.String()
		if deleteKey == "" {
			deleteKey = parseSEEDeleteKey(item.Delete.String())
		}

		createdAt := int64(0)
		if item.CreatedAt.String() != "" {
			if parsed, err := strconv.ParseInt(item.CreatedAt.String(), 10, 64); err == nil {
				createdAt = parsed
			}
		}

		providerAssetID := item.FileID.String()
		if providerAssetID == "" {
			providerAssetID = deleteKey
		}

		result = append(result, ListItem{
			ProviderAssetID:   providerAssetID,
			ProviderDeleteKey: deleteKey,
			FileURL:           item.URL.String(),
			OriginalName:      item.Filename.String(),
			SizeBytes:         int64(item.Size),
			CreatedAt:         createdAt,
		})
	}

	return result, nil
}

type seeListDataItem struct {
	FileID    seeFlexString `json:"file_id"`
	Filename  seeFlexString `json:"filename"`
	URL       seeFlexString `json:"url"`
	Delete    seeFlexString `json:"delete"`
	Hash      seeFlexString `json:"hash"`
	Size      seeFlexInt64  `json:"size"`
	CreatedAt seeFlexString `json:"created_at"`
}

func parseSEEDeleteKey(deleteURL string) string {
	deleteURL = strings.TrimSpace(deleteURL)
	if deleteURL == "" {
		return ""
	}

	u, err := url.Parse(deleteURL)
	if err != nil {
		return ""
	}

	value := path.Base(strings.TrimSpace(u.Path))
	if value == "." || value == "/" {
		return ""
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	decoded, decodeErr := url.PathUnescape(value)
	if decodeErr == nil {
		return strings.TrimSpace(decoded)
	}
	return value
}

func normalizeSEEAPIBaseURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "https://s.ee/api/v1"
	}

	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return "https://s.ee/api/v1"
	}

	pathValue := strings.TrimSuffix(strings.TrimSpace(u.Path), "/")
	if pathValue == "" {
		pathValue = "/api/v1"
	} else if strings.HasSuffix(pathValue, "/api/v1/file/upload") {
		pathValue = strings.TrimSuffix(pathValue, "/file/upload")
	} else if strings.Contains(pathValue, "/api/v1/") {
		idx := strings.Index(pathValue, "/api/v1/")
		pathValue = pathValue[:idx+len("/api/v1")]
	} else if !strings.HasSuffix(pathValue, "/api/v1") {
		pathValue += "/api/v1"
	}

	u.Path = pathValue
	u.RawPath = ""
	u.RawQuery = ""
	u.Fragment = ""

	return strings.TrimRight(u.String(), "/")
}

func normalizeSEECode(raw string) string {
	return strings.TrimSpace(strings.ToLower(raw))
}

func isSEESuccessCode(code string) bool {
	switch normalizeSEECode(code) {
	case "success", "ok", "200", "0", "image_repeated", "image_delete_success":
		return true
	default:
		return false
	}
}

type seeFlexString string

func (s *seeFlexString) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*s = ""
		return nil
	}

	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*s = seeFlexString(text)
		return nil
	}

	var number json.Number
	if err := json.Unmarshal(data, &number); err == nil {
		*s = seeFlexString(number.String())
		return nil
	}

	var any interface{}
	if err := json.Unmarshal(data, &any); err == nil {
		*s = seeFlexString(fmt.Sprint(any))
		return nil
	}

	return nil
}

func (s seeFlexString) String() string {
	return strings.TrimSpace(string(s))
}

type seeFlexInt64 int64

func (n *seeFlexInt64) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*n = 0
		return nil
	}

	var iv int64
	if err := json.Unmarshal(data, &iv); err == nil {
		*n = seeFlexInt64(iv)
		return nil
	}

	var fv float64
	if err := json.Unmarshal(data, &fv); err == nil {
		*n = seeFlexInt64(int64(fv))
		return nil
	}

	var sv string
	if err := json.Unmarshal(data, &sv); err == nil {
		sv = strings.TrimSpace(sv)
		if sv == "" {
			*n = 0
			return nil
		}
		if parsed, parseErr := strconv.ParseInt(sv, 10, 64); parseErr == nil {
			*n = seeFlexInt64(parsed)
			return nil
		}
		if parsedF, parseErr := strconv.ParseFloat(sv, 64); parseErr == nil {
			*n = seeFlexInt64(int64(parsedF))
			return nil
		}
	}

	*n = 0
	return nil
}
