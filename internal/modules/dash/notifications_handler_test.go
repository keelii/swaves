package dash

import (
	"net/url"
	"testing"

	"swaves/internal/platform/db"
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
			ID:           3,
			EventType:    dashNotificationEventPostLike,
			AggregateKey: dashCommentLinkKeyPrefix + "99:" + url.QueryEscape("/posts/ignore#comment-99"),
		},
	}, defaultCommentURL)

	if len(items) != 3 {
		t.Fatalf("buildNotificationListItems len = %d, want 3", len(items))
	}
	if items[0].CommentURL != validCommentURL {
		t.Fatalf("first comment url = %q, want %q", items[0].CommentURL, validCommentURL)
	}
	if items[1].CommentURL != defaultCommentURL {
		t.Fatalf("fallback comment url = %q, want %q", items[1].CommentURL, defaultCommentURL)
	}
	if items[2].CommentURL != "" {
		t.Fatalf("non-comment item comment url should be empty, got %q", items[2].CommentURL)
	}
}
