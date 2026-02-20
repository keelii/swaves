package share

import (
	"testing"
	"time"

	"swaves/internal/db"
	"swaves/internal/store"
)

func TestBuildPostURL(t *testing.T) {
	publishedAt := time.Date(2026, time.February, 20, 9, 30, 0, 0, time.UTC).Unix()

	tests := []struct {
		name      string
		kind      db.PostKind
		slug      string
		published int64
		settings  map[string]string
		want      string
	}{
		{
			name:      "post with root base and datetime prefix",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "/",
				"post_url_prefix": "/{datetime}",
				"post_url_name":   "{slug}",
				"post_url_ext":    "",
			},
			want: "/2026/02/20/hello-world",
		},
		{
			name:      "post with custom base and datetime prefix",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "/blog",
				"post_url_prefix": "/{datetime}",
				"post_url_name":   "{slug}",
				"post_url_ext":    "",
			},
			want: "/blog/2026/02/20/hello-world",
		},
		{
			name:      "page with root page path under custom base",
			kind:      db.PostKindPage,
			slug:      "about",
			published: publishedAt,
			settings: map[string]string{
				"base_path":     "/blog",
				"page_path":     "/",
				"post_url_name": "{slug}",
				"post_url_ext":  "",
			},
			want: "/blog/about",
		},
		{
			name:      "page with nested page path",
			kind:      db.PostKindPage,
			slug:      "about",
			published: publishedAt,
			settings: map[string]string{
				"base_path":     "/blog",
				"page_path":     "/docs",
				"post_url_name": "{slug}",
				"post_url_ext":  "",
			},
			want: "/blog/docs/about",
		},
	}

	previous, hasPrevious := store.Settings.Load().(map[string]string)
	t.Cleanup(func() {
		if !hasPrevious {
			store.Settings.Store(map[string]string{})
			return
		}
		store.Settings.Store(previous)
	})

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store.Settings.Store(tt.settings)

			got := BuildPostURL(tt.kind, tt.slug, tt.published)
			if got != tt.want {
				t.Fatalf("BuildPostURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
