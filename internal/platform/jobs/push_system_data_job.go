package job

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/platform/asset"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/store"
	"swaves/internal/platform/updater"
	"time"
)

const (
	pushProviderS3       = "s3"
	pushProviderImageKit = "imagekit"
)

type pushJobConfig struct {
	Provider           string
	Enabled            bool
	S3Bucket           string
	S3Region           string
	S3Endpoint         string
	S3AccessKey        string
	S3SecretKey        string
	S3ForcePath        bool
	ImageKitEndpoint   string
	ImageKitPrivateKey string
	Timeout            time.Duration
}

func PushSystemDataJob(reg *Registry) (*string, error) {
	return runRemoteBackupJob(reg, true)
}

func runRemoteBackupJob(reg *Registry, allowNoOp bool) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}

	cfg := loadPushJobConfig()
	if !cfg.Enabled {
		if allowNoOp {
			return nil, nil
		}
		return jobMessage("远程数据备份未启用，跳过"), nil
	}

	tmpDir, err := updater.CreateUpgradeTempDir("swaves-sync-push-")
	if err != nil {
		return nil, fmt.Errorf("create temp dir failed: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			logger.Warn("[task] remote_backup_data cleanup tmp dir failed: %v", removeErr)
		}
	}()

	snapshot, err := db.ExportSQLiteWithHash(reg.DB, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("export sqlite snapshot failed: %w", err)
	}

	objectKey := buildS3ObjectKey(snapshot.File)
	provider := normalizePushProvider(cfg.Provider)

	statusCode := 0
	response := ""
	assetID := int64(0)

	switch provider {
	case pushProviderImageKit:
		uploadRes, uploadResponse, uploadStatus, uploadErr := uploadSnapshotToImageKit(cfg, snapshot, objectKey)
		if uploadErr != nil {
			return nil, fmt.Errorf("push failed provider=%s status=%d response=%s: %w", provider, uploadStatus, uploadResponse, uploadErr)
		}

		statusCode = uploadStatus
		response = uploadResponse
		assetID, err = createRemoteBackupAssetRecordByUpload(reg.DB, provider, snapshot, uploadRes)
		if err != nil {
			return nil, fmt.Errorf("remote backup saved but asset record failed: %w", err)
		}
	default:
		if cfg.S3Bucket == "" {
			return nil, errors.New("s3 bucket is empty (setting: s3_bucket)")
		}
		if cfg.S3Region == "" {
			return nil, errors.New("s3 region is empty")
		}
		if cfg.S3AccessKey == "" {
			logger.Warn("[task] remote_backup_data missing setting: s3_access_key_id")
		}
		if cfg.S3SecretKey == "" {
			logger.Warn("[task] remote_backup_data missing setting: s3_secret_access_key")
		}

		uploadResponse, uploadStatus, uploadErr := uploadSnapshotToS3(cfg, reg.Config.AppName, snapshot, objectKey)
		if uploadErr != nil {
			return nil, fmt.Errorf("push failed provider=%s status=%d response=%s: %w", provider, uploadStatus, uploadResponse, uploadErr)
		}

		statusCode = uploadStatus
		response = uploadResponse
		assetID, err = createRemoteBackupAssetRecordForS3(reg.DB, cfg, snapshot, objectKey)
		if err != nil {
			return nil, fmt.Errorf("remote backup saved but asset record failed: %w", err)
		}
	}

	return jobMessage(fmt.Sprintf(
		"provider=%s status=%d hash=%s size=%dB asset_id=%d response=%s",
		provider,
		statusCode,
		shortHash(snapshot.Hash),
		snapshot.Size,
		assetID,
		response,
	)), nil
}

func uploadSnapshotToImageKit(cfg pushJobConfig, snapshot *db.ExportResult, objectKey string) (*asset.UploadResult, string, int, error) {
	if strings.TrimSpace(cfg.ImageKitEndpoint) == "" {
		return nil, "", 0, errors.New("imagekit endpoint is empty")
	}
	if strings.TrimSpace(cfg.ImageKitPrivateKey) == "" {
		return nil, "", 0, errors.New("imagekit private key is empty")
	}

	fileBytes, err := os.ReadFile(snapshot.File)
	if err != nil {
		return nil, "", 0, fmt.Errorf("open snapshot failed: %w", err)
	}

	provider := asset.NewImageKitProvider(asset.ImageKitConfig{
		Endpoint:   cfg.ImageKitEndpoint,
		PrivateKey: cfg.ImageKitPrivateKey,
	})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	result, err := provider.Upload(ctx, asset.UploadInput{
		FileName:    objectKey,
		ContentType: "application/x-sqlite3",
		Bytes:       fileBytes,
	})
	if err != nil {
		return nil, "", 0, fmt.Errorf("imagekit upload failed: %w", err)
	}

	statusCode := http.StatusOK
	response := fmt.Sprintf("provider=imagekit file_id=%s url=%s", strings.TrimSpace(result.ProviderAssetID), strings.TrimSpace(result.FileURL))
	return result, response, statusCode, nil
}

func uploadSnapshotToS3(cfg pushJobConfig, appName string, snapshot *db.ExportResult, objectKey string) (response string, statusCode int, err error) {
	file, err := os.Open(snapshot.File)
	if err != nil {
		return "", 0, fmt.Errorf("open snapshot failed: %w", err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return "", 0, fmt.Errorf("stat snapshot failed: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	etag, statusCode, err := PutS3Object(ctx, cfg, S3PutInput{
		ObjectKey:   objectKey,
		ContentType: "application/x-sqlite3",
		Body:        file,
		Size:        info.Size(),
		Metadata: map[string]string{
			"x-amz-meta-swaves-app":      appName,
			"x-amz-meta-snapshot-hash":   snapshot.Hash,
			"x-amz-meta-snapshot-date":   snapshot.Date,
			"x-amz-meta-snapshot-source": "remote_backup_data",
		},
	})
	if err != nil {
		return "", statusCode, err
	}

	response = fmt.Sprintf("bucket=%s key=%s etag=%s", cfg.S3Bucket, objectKey, etag)
	return response, statusCode, nil
}

func createRemoteBackupAssetRecordForS3(dbx *db.DB, cfg pushJobConfig, snapshot *db.ExportResult, objectKey string) (int64, error) {
	assetID := buildRemoteBackupAssetID(cfg.S3Bucket, objectKey)
	deleteKey := assetID
	fileURL := buildRemoteBackupFileURL(cfg.S3Bucket, objectKey)
	originalName := filepath.Base(snapshot.File)
	if originalName == "." || originalName == "" || originalName == "/" {
		originalName = buildS3ObjectKey(snapshot.File)
	}

	return createRemoteBackupAssetRecordByUpload(dbx, pushProviderS3, snapshot, &asset.UploadResult{
		ProviderAssetID:   assetID,
		ProviderDeleteKey: deleteKey,
		FileURL:           fileURL,
		OriginalName:      originalName,
		SizeBytes:         snapshot.Size,
	})
}

func createRemoteBackupAssetRecordByUpload(dbx *db.DB, providerName string, snapshot *db.ExportResult, upload *asset.UploadResult) (int64, error) {
	if upload == nil {
		return 0, errors.New("upload result is nil")
	}

	originalName := strings.TrimSpace(upload.OriginalName)
	if originalName == "." || originalName == "" || originalName == "/" {
		originalName = buildS3ObjectKey(snapshot.File)
	}

	sizeBytes := upload.SizeBytes
	if sizeBytes <= 0 {
		sizeBytes = snapshot.Size
	}

	item := &db.Asset{
		Kind:              db.AssetKindBackup,
		Provider:          strings.TrimSpace(providerName),
		ProviderAssetID:   strings.TrimSpace(upload.ProviderAssetID),
		ProviderDeleteKey: strings.TrimSpace(upload.ProviderDeleteKey),
		FileURL:           strings.TrimSpace(upload.FileURL),
		OriginalName:      originalName,
		SizeBytes:         sizeBytes,
		CreatedAt:         time.Now().Unix(),
	}

	id, err := db.CreateAsset(dbx, item)
	if err == nil {
		return id, nil
	}

	if db.IsErrUniqueConstraint(err) {
		existing, getErr := db.GetAssetByProviderAssetID(dbx, item.Provider, item.ProviderAssetID)
		if getErr == nil {
			return existing.ID, nil
		}
	}

	return 0, err
}

func buildRemoteBackupAssetID(bucket string, objectKey string) string {
	bucket = strings.TrimSpace(bucket)
	objectKey = strings.TrimSpace(strings.TrimPrefix(objectKey, "/"))
	if bucket == "" {
		return objectKey
	}
	if objectKey == "" {
		return bucket
	}
	return bucket + "/" + objectKey
}

func buildRemoteBackupFileURL(bucket string, objectKey string) string {
	assetID := buildRemoteBackupAssetID(bucket, objectKey)
	if assetID == "" {
		return ""
	}
	return "s3://" + assetID
}

func buildS3ObjectKey(snapshotFile string) string {
	key := filepath.Base(snapshotFile)
	if key == "" || key == "." || key == "/" {
		key = "snapshot.sqlite"
	}
	return key
}

func loadPushJobConfig() pushJobConfig {
	timeoutSec := store.GetSettingInt("sync_push_timeout_sec", 60)
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	rawS3Endpoint := strings.TrimSpace(store.GetSetting("s3_api_endpoint"))
	if rawS3Endpoint == "" {
		rawS3Endpoint = strings.TrimSpace(config.S3Endpoint)
	}
	s3Bucket := strings.TrimSpace(store.GetSetting("s3_bucket"))

	s3Endpoint := ""
	endpointBucket := ""
	endpointForcePath := false
	parseErr := error(nil)
	if s3Bucket != "" {
		s3Endpoint, endpointForcePath, parseErr = normalizeS3Endpoint(rawS3Endpoint)
	} else {
		s3Endpoint, endpointBucket, endpointForcePath, parseErr = splitS3EndpointBucket(rawS3Endpoint)
	}
	if parseErr != nil {
		logger.Warn("[task] push_system_data invalid endpoint: %v", parseErr)
	}

	if s3Bucket == "" {
		s3Bucket = strings.TrimSpace(endpointBucket)
	}

	s3Region := "us-east-1"
	if s3Endpoint != "" {
		s3Region = "auto"
	}

	s3ForcePath := endpointForcePath

	return pushJobConfig{
		Provider:           normalizePushProvider(store.GetSetting("sync_push_provider")),
		Enabled:            store.GetSettingBool("sync_push_enabled", false),
		S3Bucket:           s3Bucket,
		S3Region:           s3Region,
		S3Endpoint:         s3Endpoint,
		S3AccessKey:        strings.TrimSpace(firstNonEmpty(store.GetSetting("s3_access_key_id"), config.S3AccessKeyID)),
		S3SecretKey:        strings.TrimSpace(firstNonEmpty(store.GetSetting("s3_secret_access_key"), config.S3SecretAccessKey)),
		S3ForcePath:        s3ForcePath,
		ImageKitEndpoint:   strings.TrimSpace(store.GetSetting("asset_imagekit_endpoint")),
		ImageKitPrivateKey: strings.TrimSpace(store.GetSetting("asset_imagekit_private_key")),
		Timeout:            time.Duration(timeoutSec) * time.Second,
	}
}

func normalizeS3Endpoint(raw string) (endpoint string, forcePath bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", false, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw, false, err
	}
	if u.Scheme == "" || u.Host == "" {
		return raw, false, fmt.Errorf("endpoint must include scheme and host: %s", raw)
	}

	path := strings.Trim(u.Path, "/")
	forcePath = path != ""
	endpoint = strings.TrimRight(u.String(), "/")
	return endpoint, forcePath, nil
}

func normalizePushProvider(raw string) string {
	value := strings.TrimSpace(strings.ToLower(raw))
	switch value {
	case pushProviderImageKit:
		return pushProviderImageKit
	default:
		return pushProviderS3
	}
}

func splitS3EndpointBucket(raw string) (endpoint string, bucket string, forcePath bool, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", false, nil
	}

	u, err := url.Parse(raw)
	if err != nil {
		return raw, "", false, err
	}
	if u.Scheme == "" || u.Host == "" {
		return raw, "", false, fmt.Errorf("endpoint must include scheme and host: %s", raw)
	}

	path := strings.Trim(u.Path, "/")
	if path != "" {
		parts := strings.SplitN(path, "/", 2)
		bucket = strings.TrimSpace(parts[0])
		forcePath = bucket != ""

		if len(parts) == 2 {
			u.Path = "/" + parts[1]
		} else {
			u.Path = ""
		}
	}

	endpoint = strings.TrimRight(u.String(), "/")
	return endpoint, bucket, forcePath, nil
}

func shortHash(hash string) string {
	if len(hash) <= 8 {
		return hash
	}
	return hash[:8]
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
