package dash

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"swaves/internal/platform/db"

	"github.com/gofiber/fiber/v3"
)

func TestParseCommentURLFromAggregateKey(t *testing.T) {
	rawURL := "/posts/demo#comment-9"
	valid := dashCommentLinkKeyPrefix + "9:" + url.QueryEscape(rawURL)

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "valid comment link",
			raw:  valid,
			want: rawURL,
		},
		{
			name: "empty key",
			raw:  "",
			want: "",
		},
		{
			name: "invalid prefix",
			raw:  "other:9:" + url.QueryEscape(rawURL),
			want: "",
		},
		{
			name: "missing comment id",
			raw:  dashCommentLinkKeyPrefix + ":" + url.QueryEscape(rawURL),
			want: "",
		},
		{
			name: "missing escaped url",
			raw:  dashCommentLinkKeyPrefix + "9:",
			want: "",
		},
		{
			name: "invalid escaped url",
			raw:  dashCommentLinkKeyPrefix + "9:%zz",
			want: "",
		},
		{
			name: "reject javascript scheme",
			raw:  dashCommentLinkKeyPrefix + "9:" + url.QueryEscape("javascript:alert(1)"),
			want: "",
		},
		{
			name: "reject absolute external url",
			raw:  dashCommentLinkKeyPrefix + "9:" + url.QueryEscape("https://evil.example.com/post#comment-9"),
			want: "",
		},
		{
			name: "reject relative path without leading slash",
			raw:  dashCommentLinkKeyPrefix + "9:" + url.QueryEscape("posts/demo#comment-9"),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommentURLFromAggregateKey(tt.raw)
			if got != tt.want {
				t.Fatalf("parseCommentURLFromAggregateKey(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestBuildNotificationListItemsCommentURLFallback(t *testing.T) {
	defaultCommentURL := "/dash/comments"
	validCommentURL := "/posts/demo#comment-11"

	items := buildNotificationListItems([]db.Notification{
		{
			ID:           1,
			EventType:    dashNotificationEventComment,
			AggregateKey: dashCommentLinkKeyPrefix + "11:" + url.QueryEscape(validCommentURL),
		},
		{
			ID:           2,
			EventType:    dashNotificationEventComment,
			AggregateKey: "invalid:comment-link",
		},
		{
			ID:           4,
			EventType:    dashNotificationEventComment,
			AggregateKey: dashCommentLinkKeyPrefix + "12:" + url.QueryEscape("https://evil.example.com/post#comment-12"),
		},
		{
			ID:           3,
			EventType:    dashNotificationEventPostLike,
			AggregateKey: dashCommentLinkKeyPrefix + "99:" + url.QueryEscape("/posts/ignore#comment-99"),
		},
	}, defaultCommentURL, "")

	if len(items) != 4 {
		t.Fatalf("buildNotificationListItems len = %d, want 4", len(items))
	}
	if items[0].CommentURL != validCommentURL {
		t.Fatalf("first comment url = %q, want %q", items[0].CommentURL, validCommentURL)
	}
	if !items[0].CommentInNewTab {
		t.Fatalf("first comment should open in new tab")
	}
	if items[1].CommentURL != defaultCommentURL {
		t.Fatalf("fallback comment url = %q, want %q", items[1].CommentURL, defaultCommentURL)
	}
	if items[1].CommentInNewTab {
		t.Fatalf("fallback dash comment should stay in current tab")
	}
	if items[2].CommentURL != defaultCommentURL {
		t.Fatalf("external comment url should fallback to default, got %q want %q", items[2].CommentURL, defaultCommentURL)
	}
	if items[2].CommentInNewTab {
		t.Fatalf("external fallback should stay in current tab")
	}
	if items[3].CommentURL != "" {
		t.Fatalf("non-comment item comment url should be empty, got %q", items[3].CommentURL)
	}
	if items[3].CommentInNewTab {
		t.Fatalf("non-comment item should not set comment target")
	}
}

func TestBuildNotificationListItemsCopiesTemplateFields(t *testing.T) {
	readAt := int64(123)
	updatedAt := int64(456)

	items := buildNotificationListItems([]db.Notification{
		{
			ID:             9,
			EventType:      dashNotificationEventPostLike,
			Title:          "文章收到新点赞",
			Body:           "《demo》收到新的点赞。",
			AggregateCount: 3,
			ReadAt:         &readAt,
			UpdatedAt:      updatedAt,
		},
	}, "/dash/comments", "")

	if len(items) != 1 {
		t.Fatalf("buildNotificationListItems len = %d, want 1", len(items))
	}

	item := items[0]
	if item.ID != 9 {
		t.Fatalf("item.ID = %d, want 9", item.ID)
	}
	if item.EventType != dashNotificationEventPostLike {
		t.Fatalf("item.EventType = %q, want %q", item.EventType, dashNotificationEventPostLike)
	}
	if item.Title != "文章收到新点赞" {
		t.Fatalf("item.Title = %q", item.Title)
	}
	if item.Body != "《demo》收到新的点赞。" {
		t.Fatalf("item.Body = %q", item.Body)
	}
	if item.AggregateCount != 3 {
		t.Fatalf("item.AggregateCount = %d, want 3", item.AggregateCount)
	}
	if item.ReadAt == nil || *item.ReadAt != readAt {
		t.Fatalf("item.ReadAt = %v, want %d", item.ReadAt, readAt)
	}
	if item.UpdatedAt != updatedAt {
		t.Fatalf("item.UpdatedAt = %d, want %d", item.UpdatedAt, updatedAt)
	}
}

func TestParseNotificationIDRejectsInvalidJSONBody(t *testing.T) {
	app := fiber.New()
	app.Post("/test", func(c fiber.Ctx) error {
		_, err := parseNotificationID(c)
		if err != nil {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		return c.SendStatus(fiber.StatusOK)
	})

	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(`{"id":`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusBadRequest)
	}
}

func TestBuildNotificationListItemsAppUpdateFields(t *testing.T) {
	pageURL := "/dash/settings/version-update"
	items := buildNotificationListItems([]db.Notification{
		{
			ID:           7,
			EventType:    dashNotificationEventAppUpdate,
			AggregateKey: "app_update:v1.2.4:" + url.QueryEscape("https://github.com/keelii/swaves/releases/tag/v1.2.4"),
		},
	}, "/dash/comments", pageURL)

	if len(items) != 1 {
		t.Fatalf("buildNotificationListItems len = %d, want 1", len(items))
	}
	if items[0].AppUpdatePageURL != pageURL {
		t.Fatalf("AppUpdatePageURL = %q, want %q", items[0].AppUpdatePageURL, pageURL)
	}
}

func openDashTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbx := db.Open(db.Options{DSN: filepath.Join(t.TempDir(), "dash.sqlite")})
	if err := db.EnsureDefaultSettings(dbx); err != nil {
		t.Fatalf("EnsureDefaultSettings failed: %v", err)
	}
	t.Cleanup(func() {
		_ = dbx.Close()
	})
	return dbx
}

func mustCreateNotificationForTest(t *testing.T, dbx *db.DB, title string, readAt *int64) int64 {
	t.Helper()

	item := &db.Notification{
		Receiver:  db.NotificationReceiverDash,
		EventType: db.NotificationEventComment,
		Level:     db.NotificationLevelInfo,
		Title:     title,
		Body:      title,
		ReadAt:    readAt,
	}
	id, err := db.CreateNotification(dbx, item)
	if err != nil {
		t.Fatalf("CreateNotification failed: %v", err)
	}
	return id
}

func TestPostNotificationBatchDeleteAPIHandler(t *testing.T) {
	dbx := openDashTestDB(t)
	readAt := int64(123)
	readID := mustCreateNotificationForTest(t, dbx, "read", &readAt)
	unreadID := mustCreateNotificationForTest(t, dbx, "unread", nil)

	h := &Handler{Model: dbx}
	app := fiber.New()
	app.Post("/test", h.PostNotificationBatchDeleteAPIHandler)

	reqBody := fmt.Sprintf(`{"ids":[0,%d,%d,%d]}`, readID, unreadID, readID)
	req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(reqBody))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != fiber.StatusMultiStatus {
		t.Fatalf("status = %d, want %d", resp.StatusCode, fiber.StatusMultiStatus)
	}

	var body struct {
		OK             bool    `json:"ok"`
		RequestedCount int     `json:"requested_count"`
		DeletedCount   int     `json:"deleted_count"`
		FailedCount    int     `json:"failed_count"`
		DeletedIDs     []int64 `json:"deleted_ids"`
		Failed         []struct {
			ID     int64  `json:"id"`
			Status int    `json:"status"`
			Error  string `json:"error"`
		} `json:"failed"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if body.OK {
		t.Fatal("expected ok=false for partial delete")
	}
	if body.RequestedCount != 2 {
		t.Fatalf("requested_count = %d, want 2", body.RequestedCount)
	}
	if body.DeletedCount != 1 {
		t.Fatalf("deleted_count = %d, want 1", body.DeletedCount)
	}
	if len(body.DeletedIDs) != 1 || body.DeletedIDs[0] != readID {
		t.Fatalf("deleted_ids = %v, want [%d]", body.DeletedIDs, readID)
	}
	if body.FailedCount != 1 {
		t.Fatalf("failed_count = %d, want 1", body.FailedCount)
	}
	if len(body.Failed) != 1 {
		t.Fatalf("failed len = %d, want 1", len(body.Failed))
	}
	if body.Failed[0].ID != unreadID {
		t.Fatalf("failed id = %d, want %d", body.Failed[0].ID, unreadID)
	}
	if body.Failed[0].Status != fiber.StatusNotFound {
		t.Fatalf("failed status = %d, want %d", body.Failed[0].Status, fiber.StatusNotFound)
	}

	totalCount, err := db.CountNotifications(dbx, db.NotificationReceiverDash)
	if err != nil {
		t.Fatalf("CountNotifications failed: %v", err)
	}
	if totalCount != 1 {
		t.Fatalf("CountNotifications = %d, want 1", totalCount)
	}

	unreadCount, err := db.CountUnreadNotifications(dbx, db.NotificationReceiverDash)
	if err != nil {
		t.Fatalf("CountUnreadNotifications failed: %v", err)
	}
	if unreadCount != 1 {
		t.Fatalf("CountUnreadNotifications = %d, want 1", unreadCount)
	}
}
