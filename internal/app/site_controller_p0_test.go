package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/shared/pathutil"
	"swaves/internal/shared/share"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

type likeActionResponse struct {
	Liked     bool `json:"liked"`
	LikeCount int  `json:"likeCount"`
}

func createPublishedSitePost(t *testing.T, swv SwavesApp, title string) db.Post {
	t.Helper()

	post := &db.Post{
		Title:   title,
		Slug:    fmt.Sprintf("site-p0-%d", time.Now().UnixNano()),
		Content: "site controller p0 content",
		Status:  "published",
		Kind:    db.PostKindPost,
	}
	if _, err := db.CreatePost(swv.Store.Model, post); err != nil {
		t.Fatalf("create published post failed: %v", err)
	}
	return *post
}

func createApprovedSiteComment(
	t *testing.T,
	swv SwavesApp,
	postID int64,
	parentID int64,
	author string,
	content string,
	createdAt int64,
) int64 {
	t.Helper()

	item := &db.Comment{
		PostID:      postID,
		ParentID:    parentID,
		Author:      author,
		AuthorEmail: fmt.Sprintf("%s@example.com", author),
		AuthorIP:    "127.0.0.1",
		VisitorID:   fmt.Sprintf("site-p0-%s-%d", author, createdAt),
		Content:     content,
		Status:      db.CommentStatusApproved,
		Type:        "comment",
		CreatedAt:   createdAt,
		UpdatedAt:   createdAt,
	}
	id, err := db.CreateComment(swv.Store.Model, item)
	if err != nil {
		t.Fatalf("create approved comment failed: %v", err)
	}
	return id
}

func decodeLikeActionResponse(t *testing.T, body []byte) likeActionResponse {
	t.Helper()

	var payload likeActionResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode like action response failed: %v body=%s", err, strings.TrimSpace(string(body)))
	}
	return payload
}

func TestSiteControllerP0_HomePostAndNotFound(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	post := createPublishedSitePost(t, swv, "Site P0 Home Post")
	homePath := share.GetBasePath()

	homeResp := requestControllerP0(t, swv, fiber.MethodGet, homePath, nil, "", nil)
	postPath := share.GetPostUrl(post)
	assertTemplateRendered(
		t,
		homeResp,
		fiber.StatusOK,
		post.Title,
		`<ul class="articles">`,
		`<main>`,
	)

	postResp := requestControllerP0(t, swv, fiber.MethodGet, postPath, nil, "", nil)
	assertTemplateRendered(
		t,
		postResp,
		fiber.StatusOK,
		post.Title,
		`id="comments"`,
		`id="comment-form"`,
		`name="author"`,
	)

	missingPath := postPath + "-missing"
	missingResp := requestControllerP0(t, swv, fiber.MethodGet, missingPath, nil, "", nil)
	assertTemplateRendered(
		t,
		missingResp,
		fiber.StatusNotFound,
		"页面找不到",
		"返回首页",
	)
}

func TestSiteControllerP0_LikeActionJSONToggle(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	post := createPublishedSitePost(t, swv, "Site P0 Like Post")
	actionPath := pathutil.JoinAbsolute(share.GetBasePath(), "_action", "like", strconv.FormatInt(post.ID, 10))
	headers := map[string]string{"Accept": fiber.MIMEApplicationJSON}

	firstResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, nil, "", headers)
	if firstResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected first like status: %d", firstResp.StatusCode)
	}
	firstBody, _ := io.ReadAll(firstResp.Body)
	firstPayload := decodeLikeActionResponse(t, firstBody)
	if !firstPayload.Liked || firstPayload.LikeCount != 1 {
		t.Fatalf("unexpected first like payload: %+v", firstPayload)
	}
	cookieKV := responseCookieKV(firstResp)
	if cookieKV == "" {
		t.Fatalf("expected visitor cookie in first like response")
	}

	secondResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, nil, cookieKV, headers)
	if secondResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected second like status: %d", secondResp.StatusCode)
	}
	secondBody, _ := io.ReadAll(secondResp.Body)
	secondPayload := decodeLikeActionResponse(t, secondBody)
	if secondPayload.Liked || secondPayload.LikeCount != 0 {
		t.Fatalf("unexpected second like payload: %+v", secondPayload)
	}

	likeCount, err := db.CountEntityLikes(swv.Store.Model, post.ID)
	if err != nil {
		t.Fatalf("count entity likes failed: %v", err)
	}
	if likeCount != 0 {
		t.Fatalf("unexpected persisted like count: got=%d want=0", likeCount)
	}
}

func TestSiteControllerP0_CommentActionCreatesPending(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	post := createPublishedSitePost(t, swv, "Site P0 Comment Post")
	postPath := share.GetPostUrl(post)
	postResp := requestControllerP0(t, swv, fiber.MethodGet, postPath, nil, "", nil)
	if postResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected post detail status: path=%s status=%d", postPath, postResp.StatusCode)
	}
	cookieKV := responseCookieKV(postResp)

	actionPath := pathutil.JoinAbsolute(share.GetBasePath(), "_action", "comment", strconv.FormatInt(post.ID, 10))
	form := url.Values{}
	form.Set("author", "site-p0-user")
	form.Set("author_email", "site-p0@example.com")
	form.Set("author_url", "https://example.com")
	form.Set("content", "site controller comment content")
	form.Set("remember_me", "1")
	form.Set("return_url", postPath)

	commentResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, form, cookieKV, nil)
	if commentResp.StatusCode < 300 || commentResp.StatusCode >= 400 {
		t.Fatalf("expected comment redirect, got %d", commentResp.StatusCode)
	}
	location := strings.TrimSpace(commentResp.Header.Get("Location"))
	if !strings.Contains(location, "comment_status=pending") {
		t.Fatalf("comment redirect should include pending status, got location=%q", location)
	}
	if !strings.Contains(location, "#comments") {
		t.Fatalf("comment redirect should include comments anchor, got location=%q", location)
	}
	rememberCookie := strings.TrimSpace(commentResp.Header.Get("Set-Cookie"))
	if !strings.Contains(rememberCookie, "swv_commenter=") {
		t.Fatalf("comment response should set remember cookie, got=%q", rememberCookie)
	}

	feedbackPath := location
	if idx := strings.Index(feedbackPath, "#"); idx >= 0 {
		feedbackPath = feedbackPath[:idx]
	}
	feedbackResp := requestControllerP0(t, swv, fiber.MethodGet, feedbackPath, nil, responseCookieKV(commentResp), nil)
	assertTemplateRendered(
		t,
		feedbackResp,
		fiber.StatusOK,
		"评论已提交，等待审核。",
		`id="comment-form"`,
		`value="site-p0-user"`,
		`value="site-p0@example.com"`,
	)

	pendingComments, err := db.ListPostComments(swv.Store.Model, post.ID, db.CommentStatusPending)
	if err != nil {
		t.Fatalf("list pending comments failed: %v", err)
	}
	if len(pendingComments) != 1 {
		t.Fatalf("unexpected pending comment count: got=%d want=1", len(pendingComments))
	}
	comment := pendingComments[0]
	if comment.Author != "site-p0-user" {
		t.Fatalf("unexpected comment author: %q", comment.Author)
	}
	if comment.Content != "site controller comment content" {
		t.Fatalf("unexpected comment content: %q", comment.Content)
	}
	if comment.Status != db.CommentStatusPending {
		t.Fatalf("unexpected comment status: %q", comment.Status)
	}
}

func TestSiteControllerP0_CommentActionDuplicateShowsFeedback(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	post := createPublishedSitePost(t, swv, "Site P0 Duplicate Comment Post")
	postPath := share.GetPostUrl(post)
	postResp := requestControllerP0(t, swv, fiber.MethodGet, postPath, nil, "", nil)
	if postResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected post detail status: path=%s status=%d", postPath, postResp.StatusCode)
	}
	cookieKV := responseCookieKV(postResp)

	actionPath := pathutil.JoinAbsolute(share.GetBasePath(), "_action", "comment", strconv.FormatInt(post.ID, 10))
	form := url.Values{}
	form.Set("author", "site-p0-duplicate-user")
	form.Set("author_email", "site-p0-duplicate@example.com")
	form.Set("author_url", "https://example.com")
	form.Set("content", "site controller duplicate content")
	form.Set("remember_me", "1")
	form.Set("return_url", postPath)

	firstResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, form, cookieKV, nil)
	if firstResp.StatusCode < 300 || firstResp.StatusCode >= 400 {
		t.Fatalf("expected first comment redirect, got %d", firstResp.StatusCode)
	}
	cookieKV = mergeCookieKV(cookieKV, firstResp)

	secondResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, form, cookieKV, nil)
	if secondResp.StatusCode < 300 || secondResp.StatusCode >= 400 {
		t.Fatalf("expected duplicate comment redirect, got %d", secondResp.StatusCode)
	}

	location := strings.TrimSpace(secondResp.Header.Get("Location"))
	if !strings.Contains(location, "comment_status=duplicate") {
		t.Fatalf("duplicate redirect should include duplicate status, got location=%q", location)
	}
	if !strings.Contains(location, "#comments") {
		t.Fatalf("duplicate redirect should include comments anchor, got location=%q", location)
	}

	feedbackPath := location
	if idx := strings.Index(feedbackPath, "#"); idx >= 0 {
		feedbackPath = feedbackPath[:idx]
	}
	feedbackResp := requestControllerP0(t, swv, fiber.MethodGet, feedbackPath, nil, mergeCookieKV(cookieKV, secondResp), nil)
	assertTemplateRendered(
		t,
		feedbackResp,
		fiber.StatusOK,
		"请勿重复提交相同评论内容。",
		`id="comment-form"`,
	)
}

func TestSiteControllerP0_CommentRateLimitRequiresCaptchaThenShowsCaptchaFailed(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	post := createPublishedSitePost(t, swv, "Site P0 Comment Captcha Guard Post")
	postPath := share.GetPostUrl(post)
	postResp := requestControllerP0(t, swv, fiber.MethodGet, postPath, nil, "", nil)
	if postResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected post detail status: path=%s status=%d", postPath, postResp.StatusCode)
	}
	visitorCookie := responseCookieKV(postResp)
	if visitorCookie == "" {
		t.Fatalf("expected visitor cookie from post detail response")
	}

	actionPath := pathutil.JoinAbsolute(share.GetBasePath(), "_action", "comment", strconv.FormatInt(post.ID, 10))
	form := url.Values{}
	form.Set("author", "site-p0-captcha-user")
	form.Set("author_email", "site-p0-captcha@example.com")
	form.Set("author_url", "https://example.com")
	form.Set("content", "site controller captcha flow content")
	form.Set("remember_me", "1")
	form.Set("return_url", postPath)

	firstResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, form, visitorCookie, nil)
	if firstResp.StatusCode < 300 || firstResp.StatusCode >= 400 {
		t.Fatalf("expected first comment redirect, got %d", firstResp.StatusCode)
	}
	firstLocation := strings.TrimSpace(firstResp.Header.Get("Location"))
	if !strings.Contains(firstLocation, "comment_status=pending") {
		t.Fatalf("first comment redirect should include pending status, got location=%q", firstLocation)
	}

	secondResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, form, visitorCookie, nil)
	if secondResp.StatusCode < 300 || secondResp.StatusCode >= 400 {
		t.Fatalf("expected second comment redirect, got %d", secondResp.StatusCode)
	}
	secondLocation := strings.TrimSpace(secondResp.Header.Get("Location"))
	if !strings.Contains(secondLocation, "comment_status=captcha_required") {
		t.Fatalf("second comment redirect should include captcha_required status, got location=%q", secondLocation)
	}
	if !strings.Contains(secondLocation, "#comments") {
		t.Fatalf("second comment redirect should include comments anchor, got location=%q", secondLocation)
	}

	captchaRequiredPath := secondLocation
	if idx := strings.Index(captchaRequiredPath, "#"); idx >= 0 {
		captchaRequiredPath = captchaRequiredPath[:idx]
	}
	captchaRequiredResp := requestControllerP0(t, swv, fiber.MethodGet, captchaRequiredPath, nil, visitorCookie, nil)
	assertTemplateRendered(
		t,
		captchaRequiredResp,
		fiber.StatusOK,
		"提交较频繁，请先完成验证码再继续评论。",
		`id="comment-form"`,
	)

	form.Set("captcha_token", "invalid-token")
	form.Set("captcha_answer", "9")
	thirdResp := requestControllerP0(t, swv, fiber.MethodPost, actionPath, form, visitorCookie, nil)
	if thirdResp.StatusCode < 300 || thirdResp.StatusCode >= 400 {
		t.Fatalf("expected third comment redirect, got %d", thirdResp.StatusCode)
	}
	thirdLocation := strings.TrimSpace(thirdResp.Header.Get("Location"))
	if !strings.Contains(thirdLocation, "comment_status=captcha_failed") {
		t.Fatalf("third comment redirect should include captcha_failed status, got location=%q", thirdLocation)
	}
	if !strings.Contains(thirdLocation, "#comments") {
		t.Fatalf("third comment redirect should include comments anchor, got location=%q", thirdLocation)
	}

	captchaFailedPath := thirdLocation
	if idx := strings.Index(captchaFailedPath, "#"); idx >= 0 {
		captchaFailedPath = captchaFailedPath[:idx]
	}
	captchaFailedResp := requestControllerP0(t, swv, fiber.MethodGet, captchaFailedPath, nil, visitorCookie, nil)
	assertTemplateRendered(
		t,
		captchaFailedResp,
		fiber.StatusOK,
		"验证码错误或已过期，请刷新页面后重试。",
		`id="comment-form"`,
	)

	pendingComments, err := db.ListPostComments(swv.Store.Model, post.ID, db.CommentStatusPending)
	if err != nil {
		t.Fatalf("list pending comments failed: %v", err)
	}
	if len(pendingComments) != 1 {
		t.Fatalf("unexpected pending comment count after captcha flow: got=%d want=1", len(pendingComments))
	}
}

func TestSiteControllerP0_PostCommentsPaginationByRootThread(t *testing.T) {
	swv := newControllerP0TestApp(t)
	defer swv.Shutdown()

	post := createPublishedSitePost(t, swv, "Site P0 Comment Pagination Post")
	rootAID := createApprovedSiteComment(t, swv, post.ID, 0, "root-a", "root comment a", 1)
	childAID := createApprovedSiteComment(t, swv, post.ID, rootAID, "child-a", "child comment a", 2)
	rootBID := createApprovedSiteComment(t, swv, post.ID, 0, "root-b", "root comment b", 3)
	rootCID := createApprovedSiteComment(t, swv, post.ID, 0, "root-c", "root comment c", 4)

	postPath := share.GetPostUrl(post)
	page1Path := fmt.Sprintf("%s?page=1&pageSize=2", postPath)
	page1Resp := requestControllerP0(t, swv, fiber.MethodGet, page1Path, nil, "", nil)
	page1Body := assertTemplateRendered(
		t,
		page1Resp,
		fiber.StatusOK,
		fmt.Sprintf("comment-%d", rootCID),
		fmt.Sprintf("comment-%d", rootBID),
		`?page=2&pageSize=2#comments`,
	)
	if strings.Contains(page1Body, fmt.Sprintf("comment-%d", rootAID)) {
		t.Fatalf("page 1 should not include root comment a: id=%d", rootAID)
	}
	if strings.Contains(page1Body, fmt.Sprintf("comment-%d", childAID)) {
		t.Fatalf("page 1 should not include child of root comment a: id=%d", childAID)
	}
	if strings.Index(page1Body, fmt.Sprintf("comment-%d", rootCID)) > strings.Index(page1Body, fmt.Sprintf("comment-%d", rootBID)) {
		t.Fatalf("page 1 should order roots by newest first: root-c=%d root-b=%d", rootCID, rootBID)
	}

	page2Path := fmt.Sprintf("%s?page=2&pageSize=2", postPath)
	page2Resp := requestControllerP0(t, swv, fiber.MethodGet, page2Path, nil, "", nil)
	page2Body := assertTemplateRendered(
		t,
		page2Resp,
		fiber.StatusOK,
		fmt.Sprintf("comment-%d", rootAID),
		fmt.Sprintf("comment-%d", childAID),
		`?page=1&pageSize=2#comments`,
	)
	if strings.Contains(page2Body, fmt.Sprintf("comment-%d", rootBID)) {
		t.Fatalf("page 2 should not include root comment b: id=%d", rootBID)
	}
	if strings.Contains(page2Body, fmt.Sprintf("comment-%d", rootCID)) {
		t.Fatalf("page 2 should not include root comment c: id=%d", rootCID)
	}
	if strings.Index(page2Body, fmt.Sprintf("comment-%d", rootAID)) > strings.Index(page2Body, fmt.Sprintf("comment-%d", childAID)) {
		t.Fatalf("page 2 should render parent comment before child: root-a=%d child-a=%d", rootAID, childAID)
	}
}
