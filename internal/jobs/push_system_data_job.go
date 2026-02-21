package job

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"swaves/internal/db"
	"swaves/internal/store"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

const (
	settingSyncPushEnabled    = "sync_push_enabled"
	settingSyncPushEndpoint   = "sync_push_endpoint"
	settingSyncPushTimeoutSec = "sync_push_timeout_sec"
)

type pushJobConfig struct {
	Enabled     bool
	S3Bucket    string
	S3Region    string
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3ForcePath bool
	Timeout     time.Duration
}

const remoteBackupMediaProvider = "s3"

func PushSystemDataJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}

	cfg := loadPushJobConfig()
	if !cfg.Enabled {
		return jobMessage("remote data backup disabled"), nil
	}
	if cfg.S3Bucket == "" {
		return nil, errors.New("s3 bucket is empty")
	}
	if cfg.S3Region == "" {
		return nil, errors.New("s3 region is empty")
	}
	if cfg.S3AccessKey == "" {
		log.Printf("[task] remote_backup_data missing env: SWAVES_S3_ACCESS_KEY_ID")
	}
	if cfg.S3SecretKey == "" {
		log.Printf("[task] remote_backup_data missing env: SWAVES_S3_SECRET_ACCESS_KEY")
	}

	tmpDir, err := os.MkdirTemp("", "swaves-sync-push-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir failed: %w", err)
	}
	defer func() {
		if removeErr := os.RemoveAll(tmpDir); removeErr != nil {
			log.Printf("[task] remote_backup_data cleanup tmp dir failed: %v", removeErr)
		}
	}()

	snapshot, err := db.ExportSQLiteWithHash(reg.DB, tmpDir)
	if err != nil {
		return nil, fmt.Errorf("export sqlite snapshot failed: %w", err)
	}

	objectKey := buildS3ObjectKey(snapshot.File)
	response, statusCode, err := uploadSnapshotToS3(cfg, reg.Config.AppName, snapshot, objectKey)
	if err != nil {
		return nil, fmt.Errorf("push failed status=%d response=%s: %w", statusCode, response, err)
	}

	mediaID, err := createRemoteBackupMediaRecord(reg.DB, cfg, snapshot, objectKey)
	if err != nil {
		return nil, fmt.Errorf("remote backup saved but media record failed: %w", err)
	}

	return jobMessage(fmt.Sprintf(
		"status=%d hash=%s size=%dB media_id=%d response=%s",
		statusCode,
		shortHash(snapshot.Hash),
		snapshot.Size,
		mediaID,
		response,
	)), nil
}

func uploadSnapshotToS3(cfg pushJobConfig, appName string, snapshot *db.ExportResult, objectKey string) (response string, statusCode int, err error) {
	file, err := os.Open(snapshot.File)
	if err != nil {
		return "", 0, fmt.Errorf("open snapshot failed: %w", err)
	}
	defer file.Close()

	awsCfg, err := buildS3AWSConfig(cfg)
	if err != nil {
		return "", 0, fmt.Errorf("build s3 config failed: %w", err)
	}

	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = cfg.S3ForcePath
	})

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	putOut, err := client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(cfg.S3Bucket),
		Key:         aws.String(objectKey),
		Body:        file,
		ContentType: aws.String("application/x-sqlite3"),
		Metadata: map[string]string{
			"swaves-app":      appName,
			"snapshot-hash":   snapshot.Hash,
			"snapshot-date":   snapshot.Date,
			"snapshot-source": "remote_backup_data",
		},
	})
	if err != nil {
		statusCode = extractS3StatusCode(err)
		return "", statusCode, fmt.Errorf("s3 put object failed: %w", err)
	}

	statusCode = http.StatusOK
	etag := strings.TrimSpace(aws.ToString(putOut.ETag))
	response = fmt.Sprintf("bucket=%s key=%s etag=%s", cfg.S3Bucket, objectKey, etag)
	return response, statusCode, nil
}

func createRemoteBackupMediaRecord(dbx *db.DB, cfg pushJobConfig, snapshot *db.ExportResult, objectKey string) (int64, error) {
	assetID := buildRemoteBackupAssetID(cfg.S3Bucket, objectKey)
	deleteKey := assetID
	fileURL := buildRemoteBackupFileURL(cfg.S3Bucket, objectKey)
	originalName := filepath.Base(snapshot.File)
	if originalName == "." || originalName == "" || originalName == "/" {
		originalName = buildS3ObjectKey(snapshot.File)
	}

	item := &db.Media{
		Kind:              db.MediaKindBackup,
		Provider:          remoteBackupMediaProvider,
		ProviderAssetID:   assetID,
		ProviderDeleteKey: deleteKey,
		FileURL:           fileURL,
		OriginalName:      originalName,
		SizeBytes:         snapshot.Size,
		CreatedAt:         time.Now().Unix(),
	}

	id, err := db.CreateMedia(dbx, item)
	if err == nil {
		return id, nil
	}

	if strings.Contains(strings.ToLower(err.Error()), "unique constraint failed") {
		existing, getErr := db.GetMediaByProviderAssetID(dbx, item.Provider, item.ProviderAssetID)
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

func buildS3AWSConfig(cfg pushJobConfig) (aws.Config, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.S3Region),
	}

	if cfg.S3AccessKey != "" && cfg.S3SecretKey != "" {
		provider := credentials.NewStaticCredentialsProvider(cfg.S3AccessKey, cfg.S3SecretKey, "")
		opts = append(opts, awsconfig.WithCredentialsProvider(provider))
	}

	if cfg.S3Endpoint != "" {
		endpointURL := cfg.S3Endpoint
		opts = append(opts, awsconfig.WithEndpointResolverWithOptions(aws.EndpointResolverWithOptionsFunc(
			func(service, region string, _ ...interface{}) (aws.Endpoint, error) {
				if service == s3.ServiceID {
					return aws.Endpoint{
						URL:               endpointURL,
						HostnameImmutable: true,
					}, nil
				}
				return aws.Endpoint{}, &aws.EndpointNotFoundError{}
			},
		)))
	}

	return awsconfig.LoadDefaultConfig(context.Background(), opts...)
}

func buildS3ObjectKey(snapshotFile string) string {
	key := filepath.Base(snapshotFile)
	if key == "" || key == "." || key == "/" {
		key = "snapshot.sqlite"
	}
	return key
}

func extractS3StatusCode(err error) int {
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		return respErr.HTTPStatusCode()
	}
	return 0
}

func loadPushJobConfig() pushJobConfig {
	timeoutSec := store.GetSettingInt(settingSyncPushTimeoutSec, 60)
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	s3Endpoint := strings.TrimSpace(store.GetSetting(settingSyncPushEndpoint))
	s3Endpoint, endpointBucket, endpointForcePath, parseErr := splitS3EndpointBucket(s3Endpoint)
	if parseErr != nil {
		log.Printf("[task] push_system_data invalid endpoint: %v", parseErr)
	}

	s3Bucket := endpointBucket

	s3Region := "us-east-1"
	if s3Endpoint != "" {
		s3Region = "auto"
	}

	s3ForcePath := endpointForcePath

	return pushJobConfig{
		Enabled:     store.GetSettingBool(settingSyncPushEnabled, false),
		S3Bucket:    s3Bucket,
		S3Region:    s3Region,
		S3Endpoint:  s3Endpoint,
		S3AccessKey: strings.TrimSpace(store.GetSetting("s3_access_key_id")),
		S3SecretKey: strings.TrimSpace(store.GetSetting("s3_secret_access_key")),
		S3ForcePath: s3ForcePath,
		Timeout:     time.Duration(timeoutSec) * time.Second,
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
