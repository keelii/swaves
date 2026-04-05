package notify

import (
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
)

func openNotifyTestDB(t *testing.T) *db.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "notify.sqlite")
	dbx := db.Open(db.Options{DSN: dbPath})
	t.Cleanup(func() {
		_ = dbx.Close()
	})

	if err := store.ReloadSettings(&store.GlobalStore{Model: dbx}); err != nil {
		t.Fatalf("reload settings failed: %v", err)
	}

	return dbx
}

func TestCreateCommentNotificationWritesCommentLinkAggregateKey(t *testing.T) {
	dbx := openNotifyTestDB(t)

	post := db.Post{
		Kind:        db.PostKindPost,
		Slug:        "comment-target",
		PublishedAt: 1700000000,
	}
	comment := db.Comment{
		ID:     42,
		Author: "Alice",
	}
	nowUnix := int64(1700001234)

	if err := CreateCommentNotification(dbx, post, comment, nowUnix); err != nil {
		t.Fatalf("CreateCommentNotification failed: %v", err)
	}

	items, err := db.ListNotificationsByEventType(dbx, db.NotificationReceiverDash, db.NotificationEventComment, 10, 0)
	if err != nil {
		t.Fatalf("ListNotificationsByEventType failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("comment notification count = %d, want 1", len(items))
	}

	item := items[0]
	if item.EventType != db.NotificationEventComment {
		t.Fatalf("event type = %q, want %q", item.EventType, db.NotificationEventComment)
	}
	if item.Receiver != db.NotificationReceiverDash {
		t.Fatalf("receiver = %q, want %q", item.Receiver, db.NotificationReceiverDash)
	}
	if !strings.HasPrefix(item.AggregateKey, commentLinkKeyPrefix+strconv.FormatInt(comment.ID, 10)+":") {
		t.Fatalf("aggregate key = %q, want prefix %q", item.AggregateKey, commentLinkKeyPrefix+strconv.FormatInt(comment.ID, 10)+":")
	}

	escapedURL := strings.TrimPrefix(item.AggregateKey, commentLinkKeyPrefix+strconv.FormatInt(comment.ID, 10)+":")
	commentURL, err := url.QueryUnescape(escapedURL)
	if err != nil {
		t.Fatalf("QueryUnescape aggregate key URL failed: %v", err)
	}
	wantSuffix := "#comment-" + strconv.FormatInt(comment.ID, 10)
	if !strings.HasSuffix(commentURL, wantSuffix) {
		t.Fatalf("comment URL = %q, want suffix %q", commentURL, wantSuffix)
	}
}

func TestCreateAppUpdateNotificationUsesAggregateKey(t *testing.T) {
	dbx := openNotifyTestDB(t)
	nowUnix := int64(1700002000)

	if err := CreateAppUpdateNotification(dbx, "v1.2.3", "v1.2.4", "https://github.com/keelii/swaves/releases/tag/v1.2.4", nowUnix); err != nil {
		t.Fatalf("CreateAppUpdateNotification failed: %v", err)
	}

	items, err := db.ListNotificationsByEventType(dbx, db.NotificationReceiverDash, db.NotificationEventAppUpdate, 10, 0)
	if err != nil {
		t.Fatalf("ListNotificationsByEventType failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("app update notification count = %d, want 1", len(items))
	}
	if items[0].AggregateKey != BuildAppUpdateAggregateKey("v1.2.4", "https://github.com/keelii/swaves/releases/tag/v1.2.4") {
		t.Fatalf("unexpected aggregate key: %q", items[0].AggregateKey)
	}
}

func TestParseAppUpdateReleaseURL(t *testing.T) {
	raw := BuildAppUpdateAggregateKey("v1.2.4", "https://github.com/keelii/swaves/releases/tag/v1.2.4")
	if got := ParseAppUpdateReleaseURL(raw); got != "https://github.com/keelii/swaves/releases/tag/v1.2.4" {
		t.Fatalf("ParseAppUpdateReleaseURL = %q", got)
	}
	if got := ParseAppUpdateReleaseURL(BuildAppUpdateAggregateKey("v1.2.4", "")); got != "" {
		t.Fatalf("ParseAppUpdateReleaseURL without release url = %q", got)
	}
}

func TestParseAppUpdateVersion(t *testing.T) {
	if got := ParseAppUpdateVersion(BuildAppUpdateAggregateKey("v1.2.4", "https://github.com/keelii/swaves/releases/tag/v1.2.4")); got != "v1.2.4" {
		t.Fatalf("ParseAppUpdateVersion = %q", got)
	}
	if got := ParseAppUpdateVersion("app_update:dev"); got != "" {
		t.Fatalf("ParseAppUpdateVersion(dev) = %q", got)
	}
}
