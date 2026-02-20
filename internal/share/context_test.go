package share

import (
	"fmt"
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
				"base_path":       "",
				"post_url_prefix": "{datetime}",
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
				"base_path":       "/blog",
				"page_url_prefix": "",
				"post_url_name":   "{slug}",
				"post_url_ext":    "",
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
				"base_path":       "/blog",
				"page_url_prefix": "/docs",
				"post_url_name":   "{slug}",
				"post_url_ext":    "",
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

func TestBuildAdminPath(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		path     string
		want     string
	}{
		{
			name: "default admin path root",
			settings: map[string]string{
				"admin_path": "/admin",
			},
			path: "",
			want: "/admin",
		},
		{
			name: "custom admin path with canonical input",
			settings: map[string]string{
				"admin_path": "/admin/dashboard",
			},
			path: "/admin/posts",
			want: "/admin/dashboard/posts",
		},
		{
			name: "custom admin path with direct suffix",
			settings: map[string]string{
				"admin_path": "/admin/dashboard",
			},
			path: "/posts",
			want: "/admin/dashboard/posts",
		},
		{
			name: "quoted admin path should be normalized",
			settings: map[string]string{
				"admin_path": "\"/console\"",
			},
			path: "/admin/settings/all",
			want: "/console/settings/all",
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
			if got := BuildAdminPath(tt.path); got != tt.want {
				t.Fatalf("BuildAdminPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestCanonicalAdminPath(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		path     string
		want     string
	}{
		{
			name: "canonicalize custom admin root",
			settings: map[string]string{
				"admin_path": "/admin/dashboard",
			},
			path: "/admin/dashboard",
			want: "/admin",
		},
		{
			name: "canonicalize custom admin child route",
			settings: map[string]string{
				"admin_path": "/admin/dashboard",
			},
			path: "/admin/dashboard/posts",
			want: "/admin/posts",
		},
		{
			name: "already canonical route keeps original",
			settings: map[string]string{
				"admin_path": "/admin/dashboard",
			},
			path: "/admin/settings/all",
			want: "/admin/settings/all",
		},
		{
			name: "root admin path keeps original route",
			settings: map[string]string{
				"admin_path": "/",
			},
			path: "/admin/posts",
			want: "/admin/posts",
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
			if got := CanonicalAdminPath(tt.path); got != tt.want {
				t.Fatalf("CanonicalAdminPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestURLFor(t *testing.T) {
	SetURLForResolver(func(name string, params map[string]string, query map[string]string) (string, error) {
		if name != "admin.posts.edit" {
			return "", fmt.Errorf("unexpected route name: %s", name)
		}
		if params["id"] == "" {
			return "", fmt.Errorf("missing id param")
		}
		path := "/admin/posts/" + params["id"] + "/edit"
		if query["tab"] != "" {
			path += "?tab=" + query["tab"]
		}
		return path, nil
	})
	t.Cleanup(func() {
		SetURLForResolver(nil)
	})

	got := URLFor(
		"admin.posts.edit",
		map[string]string{"id": "123"},
		map[string]string{"tab": "comments"},
	)
	want := "/admin/posts/123/edit?tab=comments"
	if got != want {
		t.Fatalf("URLFor() = %q, want %q", got, want)
	}
}

func TestURLForWithoutResolver(t *testing.T) {
	SetURLForResolver(nil)
	got := URLFor("admin.posts.edit", map[string]string{"id": "1"}, nil)
	if got != "" {
		t.Fatalf("URLFor() = %q, want empty string when resolver not set", got)
	}
}
