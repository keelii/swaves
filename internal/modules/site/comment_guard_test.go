package site

import (
	"encoding/base64"
	"strconv"
	"strings"
	"testing"
	"time"
)

func resetCommentCaptchaTestState() {
	commentCaptchaNonceCache.Lock()
	commentCaptchaNonceCache.items = map[string]int64{}
	commentCaptchaNonceCache.Unlock()

	commentCaptchaRequiredCache.Lock()
	commentCaptchaRequiredCache.items = map[string]int64{}
	commentCaptchaRequiredCache.Unlock()
}

func buildCaptchaTokenForTest(visitorID, nonce string, expiresAt int64, answer string) string {
	expiresAtRaw := strconv.FormatInt(expiresAt, 10)
	signature := signCommentCaptcha(visitorID, nonce, expiresAtRaw, answer)
	rawToken := strings.Join([]string{nonce, expiresAtRaw, signature}, ".")
	return base64.RawURLEncoding.EncodeToString([]byte(rawToken))
}

func TestVerifyCommentCaptchaChallengeAcceptsValidTokenOnce(t *testing.T) {
	resetCommentCaptchaTestState()

	visitorID := "visitor-valid"
	token := buildCaptchaTokenForTest(visitorID, "nonce-valid", time.Now().Add(time.Minute).Unix(), "7")

	if !verifyCommentCaptchaChallenge(visitorID, token, "7") {
		t.Fatal("valid captcha token should pass verification")
	}
	if verifyCommentCaptchaChallenge(visitorID, token, "7") {
		t.Fatal("captcha token should be single-use and reject replay")
	}
}

func TestVerifyCommentCaptchaChallengeRejectsExpiredToken(t *testing.T) {
	resetCommentCaptchaTestState()

	visitorID := "visitor-expired"
	token := buildCaptchaTokenForTest(visitorID, "nonce-expired", time.Now().Add(-time.Minute).Unix(), "9")

	if verifyCommentCaptchaChallenge(visitorID, token, "9") {
		t.Fatal("expired captcha token should fail verification")
	}
}

func TestVerifyCommentCaptchaChallengeRejectsWrongVisitor(t *testing.T) {
	resetCommentCaptchaTestState()

	token := buildCaptchaTokenForTest("visitor-owner", "nonce-owner", time.Now().Add(time.Minute).Unix(), "5")

	if verifyCommentCaptchaChallenge("visitor-other", token, "5") {
		t.Fatal("captcha token should be bound to the original visitor")
	}
}

func TestIsCommentCaptchaRequiredPrunesExpiredEntry(t *testing.T) {
	resetCommentCaptchaTestState()

	commentCaptchaRequiredCache.Lock()
	commentCaptchaRequiredCache.items["visitor-expired"] = time.Now().Add(-time.Minute).Unix()
	commentCaptchaRequiredCache.Unlock()

	if isCommentCaptchaRequired("visitor-expired") {
		t.Fatal("expired captcha required flag should not be considered active")
	}

	commentCaptchaRequiredCache.Lock()
	_, exists := commentCaptchaRequiredCache.items["visitor-expired"]
	commentCaptchaRequiredCache.Unlock()
	if exists {
		t.Fatal("expired captcha required flag should be pruned from cache")
	}
}
