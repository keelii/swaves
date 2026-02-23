package site

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/store"
	"swaves/internal/shared/webutil"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

const (
	commentCaptchaTokenField  = "captcha_token"
	commentCaptchaAnswerField = "captcha_answer"
	commentCaptchaTTL         = 10 * time.Minute
	commentCaptchaRequiredTTL = 30 * time.Minute

	commentRateLimitMax        = 5
	commentRateLimitExpiration = time.Minute

	commentFeedbackCaptchaRequired = "captcha_required"
	commentFeedbackCaptchaFailed   = "captcha_failed"
	commentFeedbackRateLimited     = "rate_limited"
)

type commentCaptchaChallenge struct {
	Prompt string
	Token  string
}

var commentCaptchaNonceCache = struct {
	sync.Mutex
	items map[string]int64
}{
	items: map[string]int64{},
}

var commentCaptchaRequiredCache = struct {
	sync.Mutex
	items map[string]int64
}{
	items: map[string]int64{},
}

func buildCommentCaptchaChallenge(visitorID string) commentCaptchaChallenge {
	left := randomIntInRange(1, 20)
	right := randomIntInRange(1, 20)
	operator := "+"
	answer := left + right

	if randomIntInRange(0, 1) == 1 {
		if left < right {
			left, right = right, left
		}
		operator = "-"
		answer = left - right
	}

	nonce := randomToken(12)
	expiresAt := strconv.FormatInt(time.Now().Add(commentCaptchaTTL).Unix(), 10)
	signature := signCommentCaptcha(visitorID, nonce, expiresAt, strconv.Itoa(answer))
	rawToken := strings.Join([]string{nonce, expiresAt, signature}, ".")

	return commentCaptchaChallenge{
		Prompt: fmt.Sprintf("%d %s %d = ?", left, operator, right),
		Token:  base64.RawURLEncoding.EncodeToString([]byte(rawToken)),
	}
}

func verifyCommentCaptchaChallenge(visitorID, token, answer string) bool {
	token = strings.TrimSpace(token)
	answer = strings.TrimSpace(answer)
	if token == "" || answer == "" || len(answer) > 16 {
		return false
	}

	normalizedAnswer, err := strconv.Atoi(answer)
	if err != nil || normalizedAnswer < 0 {
		return false
	}
	answer = strconv.Itoa(normalizedAnswer)

	rawToken, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return false
	}

	parts := strings.Split(string(rawToken), ".")
	if len(parts) != 3 {
		return false
	}
	nonce := parts[0]
	expiresAtRaw := parts[1]
	signature := parts[2]
	if nonce == "" || expiresAtRaw == "" || signature == "" {
		return false
	}

	expiresAt, err := strconv.ParseInt(expiresAtRaw, 10, 64)
	if err != nil || time.Now().Unix() > expiresAt {
		return false
	}
	if !consumeCommentCaptchaNonce(nonce, expiresAt) {
		return false
	}

	expectedSignature := signCommentCaptcha(visitorID, nonce, expiresAtRaw, answer)
	return hmac.Equal([]byte(signature), []byte(expectedSignature))
}

func commentRateLimitMiddleware() fiber.Handler {
	return limiter.New(limiter.Config{
		Max:        commentRateLimitMax,
		Expiration: commentRateLimitExpiration,
		Next: func(c fiber.Ctx) bool {
			visitorID := middleware.GetOrCreateVisitorID(c, "")
			return isCommentCaptchaRequired(visitorID)
		},
		KeyGenerator: func(c fiber.Ctx) string {
			visitorID := middleware.GetOrCreateVisitorID(c, "")
			if visitorID == "" {
				return "ip:" + strings.TrimSpace(c.IP())
			}
			return "visitor:" + visitorID
		},
		LimitReached: func(c fiber.Ctx) error {
			visitorID := middleware.GetOrCreateVisitorID(c, "")
			markCommentCaptchaRequired(visitorID)

			redirectPath := appendQueryParam(resolveReturnPath(c), "comment_status", commentFeedbackCaptchaRequired)
			if !strings.Contains(redirectPath, "#") {
				redirectPath += "#comments"
			}
			return webutil.RedirectTo(c, redirectPath, fiber.StatusSeeOther)
		},
	})
}

func signCommentCaptcha(visitorID, nonce, expiresAt, answer string) string {
	mac := hmac.New(sha256.New, []byte(commentCaptchaSecret()))
	mac.Write([]byte("swaves-comment-captcha-v1"))
	mac.Write([]byte{0})
	mac.Write([]byte(visitorID))
	mac.Write([]byte{0})
	mac.Write([]byte(nonce))
	mac.Write([]byte{0})
	mac.Write([]byte(expiresAt))
	mac.Write([]byte{0})
	mac.Write([]byte(answer))
	return hex.EncodeToString(mac.Sum(nil))
}

func commentCaptchaSecret() string {
	settings := store.GetSettingMap()
	if secret := strings.TrimSpace(settings["comment_captcha_secret"]); secret != "" {
		return secret
	}
	if adminPasswordHash := strings.TrimSpace(settings["admin_password"]); adminPasswordHash != "" {
		return adminPasswordHash
	}
	if siteURL := strings.TrimSpace(settings["site_url"]); siteURL != "" {
		return siteURL
	}
	return "swaves-comment-captcha-default-secret"
}

func randomIntInRange(min, max int) int {
	if max <= min {
		return min
	}

	delta := max - min + 1
	n, err := rand.Int(rand.Reader, big.NewInt(int64(delta)))
	if err != nil {
		return min + int(time.Now().UnixNano()%int64(delta))
	}
	return min + int(n.Int64())
}

func randomToken(size int) string {
	if size <= 0 {
		size = 12
	}

	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}

func consumeCommentCaptchaNonce(nonce string, expiresAt int64) bool {
	now := time.Now().Unix()

	commentCaptchaNonceCache.Lock()
	defer commentCaptchaNonceCache.Unlock()

	pruneExpiredEntries(commentCaptchaNonceCache.items, now)

	if existingExpireAt, ok := commentCaptchaNonceCache.items[nonce]; ok && existingExpireAt > now {
		return false
	}

	commentCaptchaNonceCache.items[nonce] = expiresAt
	return true
}

func markCommentCaptchaRequired(visitorID string) {
	visitorID = strings.TrimSpace(visitorID)
	if visitorID == "" {
		return
	}

	expiresAt := time.Now().Add(commentCaptchaRequiredTTL).Unix()

	commentCaptchaRequiredCache.Lock()
	defer commentCaptchaRequiredCache.Unlock()

	pruneExpiredEntries(commentCaptchaRequiredCache.items, time.Now().Unix())
	commentCaptchaRequiredCache.items[visitorID] = expiresAt
}

func isCommentCaptchaRequired(visitorID string) bool {
	visitorID = strings.TrimSpace(visitorID)
	if visitorID == "" {
		return false
	}

	now := time.Now().Unix()

	commentCaptchaRequiredCache.Lock()
	defer commentCaptchaRequiredCache.Unlock()

	pruneExpiredEntries(commentCaptchaRequiredCache.items, now)

	expiresAt, ok := commentCaptchaRequiredCache.items[visitorID]
	return ok && expiresAt > now
}

func pruneExpiredEntries(items map[string]int64, now int64) {
	for key, expireAt := range items {
		if expireAt <= now {
			delete(items, key)
		}
	}
}
