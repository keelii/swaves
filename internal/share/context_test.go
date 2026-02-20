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
		id        int64
		name      string
		kind      db.PostKind
		slug      string
		published int64
		settings  map[string]string
		want      string
	}{
		{
			id:        1,
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
			id:        1,
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
			id:        1,
			name:      "post with custom base and datetime prefix",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "/blog",
				"post_url_prefix": "/{datetime}/archives",
				"post_url_name":   "{slug}",
				"post_url_ext":    "",
			},
			want: "/blog/2026/02/20/archives/hello-world",
		},
		{
			id:        1,
			name:      "post with custom base and slug ext",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "/blog",
				"post_url_prefix": "/{datetime}/archives",
				"post_url_name":   "{slug}",
				"post_url_ext":    ".html",
			},
			want: "/blog/2026/02/20/archives/hello-world.html",
		},
		{
			id:        1,
			name:      "post with custom base and id ext",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "/blog",
				"post_url_prefix": "/{datetime}/archives",
				"post_url_name":   "{id}",
				"post_url_ext":    ".html",
			},
			want: "/blog/2026/02/20/archives/1.html",
		},
		{
			id:        1,
			name:      "post with custom base and id ext",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "",
				"post_url_prefix": "",
				"post_url_name":   "{id}",
				"post_url_ext":    ".html",
			},
			want: "/1.html",
		},
		{
			id:        1,
			name:      "post with custom base and id ext",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "",
				"post_url_prefix": "",
				"post_url_name":   "{id}",
				"post_url_ext":    "",
			},
			want: "/1",
		},
		{
			id:        1,
			name:      "post with custom base and id ext",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "",
				"post_url_prefix": "",
				"post_url_name":   "{slug}",
				"post_url_ext":    "",
			},
			want: "/hello-world",
		},
		{
			id:        1,
			name:      "post with custom base and id ext",
			kind:      db.PostKindPost,
			slug:      "hello-world",
			published: publishedAt,
			settings: map[string]string{
				"base_path":       "/blog",
				"post_url_prefix": "/archives",
				"post_url_name":   "{id}",
				"post_url_ext":    ".html",
			},
			want: "/blog/archives/1.html",
		},
		{
			id:        1,
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
			id:        1,
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

			got := BuildPostURL(tt.id, tt.kind, tt.slug, tt.published)
			if got != tt.want {
				t.Fatalf("BuildPostURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
