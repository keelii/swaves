package job

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// S3PutInput holds the parameters for a single S3 PutObject request.
type S3PutInput struct {
	ObjectKey   string
	ContentType string
	Body        io.Reader
	Size        int64
	// Metadata are extra request headers (e.g. x-amz-meta-* keys).
	Metadata map[string]string
}

// PutS3Object executes an authenticated S3 PutObject via stdlib net/http with
// AWS4-HMAC-SHA256 signing. It returns the ETag value, the HTTP status code,
// and any transport or non-2xx error.
func PutS3Object(ctx context.Context, cfg pushJobConfig, input S3PutInput) (etag string, statusCode int, err error) {
	objectURL := buildS3ObjectURL(cfg, input.ObjectKey)
	parsedURL, err := url.Parse(objectURL)
	if err != nil {
		return "", 0, fmt.Errorf("parse object url failed: %w", err)
	}

	now := time.Now().UTC()
	dateISO := now.Format("20060102T150405Z")
	dateShort := now.Format("20060102")

	authHeader := s3Sign(cfg, parsedURL.Host, input.ObjectKey, input.ContentType, dateISO, dateShort, input.Metadata)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, objectURL, input.Body)
	if err != nil {
		return "", 0, fmt.Errorf("build request failed: %w", err)
	}
	req.ContentLength = input.Size
	req.Header.Set("Content-Type", input.ContentType)
	req.Header.Set("Authorization", authHeader)
	req.Header.Set("X-Amz-Date", dateISO)
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	for k, v := range input.Metadata {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("s3 put object failed: %w", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body) // drain to allow connection reuse; drain errors are non-fatal

	statusCode = resp.StatusCode
	if statusCode < 200 || statusCode >= 300 {
		return "", statusCode, fmt.Errorf("s3 put object failed: status=%d", statusCode)
	}

	etag = strings.Trim(resp.Header.Get("ETag"), `"`)
	return etag, statusCode, nil
}

// buildS3ObjectURL constructs the full PUT URL for the given object key.
// Virtual-hosted style is used by default; path style when S3ForcePath is set.
// When no custom endpoint is configured, standard AWS S3 is assumed.
func buildS3ObjectURL(cfg pushJobConfig, objectKey string) string {
	endpoint := cfg.S3Endpoint
	if endpoint == "" {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com", cfg.S3Region)
	}
	encodedKey := url.PathEscape(objectKey)
	if cfg.S3ForcePath {
		return strings.TrimRight(endpoint, "/") + "/" + cfg.S3Bucket + "/" + encodedKey
	}
	// endpoint is validated in normalizeS3Endpoint / splitS3EndpointBucket upstream,
	// so url.Parse will succeed; fall back to path style on unexpected parse failure.
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return strings.TrimRight(endpoint, "/") + "/" + cfg.S3Bucket + "/" + encodedKey
	}
	return fmt.Sprintf("%s://%s.%s/%s", u.Scheme, cfg.S3Bucket, u.Host, encodedKey)
}

// s3Sign returns an AWS4-HMAC-SHA256 Authorization header value for an S3 PUT.
// It signs with UNSIGNED-PAYLOAD so the file body does not need to be buffered.
func s3Sign(cfg pushJobConfig, host, objectKey, contentType, dateISO, dateShort string, metaHeaders map[string]string) string {
	const payloadHash = "UNSIGNED-PAYLOAD"

	type header struct{ k, v string }
	hdrs := []header{
		{"content-type", contentType},
		{"host", host},
		{"x-amz-content-sha256", payloadHash},
		{"x-amz-date", dateISO},
	}
	for k, v := range metaHeaders {
		hdrs = append(hdrs, header{strings.ToLower(k), v})
	}
	sort.Slice(hdrs, func(i, j int) bool { return hdrs[i].k < hdrs[j].k })

	canonicalHeaders := ""
	signedHeaderParts := make([]string, len(hdrs))
	for i, h := range hdrs {
		canonicalHeaders += h.k + ":" + h.v + "\n"
		signedHeaderParts[i] = h.k
	}
	signedHeaders := strings.Join(signedHeaderParts, ";")

	encodedKey := "/" + url.PathEscape(objectKey)
	if cfg.S3ForcePath {
		encodedKey = "/" + cfg.S3Bucket + encodedKey
	}

	canonicalRequest := strings.Join([]string{
		"PUT",
		encodedKey,
		"",
		canonicalHeaders,
		signedHeaders,
		payloadHash,
	}, "\n")

	reqHash := sha256.Sum256([]byte(canonicalRequest))
	scope := dateShort + "/" + cfg.S3Region + "/s3/aws4_request"
	stringToSign := "AWS4-HMAC-SHA256\n" + dateISO + "\n" + scope + "\n" + hex.EncodeToString(reqHash[:])

	kDate := s3HMAC([]byte("AWS4"+cfg.S3SecretKey), []byte(dateShort))
	kRegion := s3HMAC(kDate, []byte(cfg.S3Region))
	kService := s3HMAC(kRegion, []byte("s3"))
	kSigning := s3HMAC(kService, []byte("aws4_request"))
	sig := hex.EncodeToString(s3HMAC(kSigning, []byte(stringToSign)))

	return fmt.Sprintf("AWS4-HMAC-SHA256 Credential=%s/%s,SignedHeaders=%s,Signature=%s",
		cfg.S3AccessKey, scope, signedHeaders, sig)
}

func s3HMAC(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}
