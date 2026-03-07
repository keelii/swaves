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
	if homeResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected home status: %d", homeResp.StatusCode)
	}
	homeBody, _ := io.ReadAll(homeResp.Body)
	if !strings.Contains(string(homeBody), post.Title) {
		t.Fatalf("home page should contain post title: %q", post.Title)
	}

	postPath := share.GetPostUrl(post)
	postResp := requestControllerP0(t, swv, fiber.MethodGet, postPath, nil, "", nil)
	if postResp.StatusCode != fiber.StatusOK {
		t.Fatalf("unexpected post detail status: path=%s status=%d", postPath, postResp.StatusCode)
	}
	postBody, _ := io.ReadAll(postResp.Body)
	if !strings.Contains(string(postBody), post.Title) {
		t.Fatalf("post detail page should contain post title: %q", post.Title)
	}

	missingPath := postPath + "-missing"
	missingResp := requestControllerP0(t, swv, fiber.MethodGet, missingPath, nil, "", nil)
	if missingResp.StatusCode != fiber.StatusNotFound {
		t.Fatalf("unexpected missing post status: path=%s status=%d", missingPath, missingResp.StatusCode)
	}
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
