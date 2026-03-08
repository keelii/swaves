package share

import (
	"fmt"
	"testing"
	"time"

	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
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

func TestBuildDashPath(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		path     string
		want     string
	}{
		{
			name: "default dash path root",
			settings: map[string]string{
				"dash_path": "/dash",
			},
			path: "",
			want: "/dash",
		},
		{
			name: "custom dash path with canonical input",
			settings: map[string]string{
				"dash_path": "/dash/dashboard",
			},
			path: "/dash/posts",
			want: "/dash/dashboard/posts",
		},
		{
			name: "custom dash path with direct suffix",
			settings: map[string]string{
				"dash_path": "/dash/dashboard",
			},
			path: "/posts",
			want: "/dash/dashboard/posts",
		},
		{
			name: "quoted dash path should be normalized",
			settings: map[string]string{
				"dash_path": "\"/console\"",
			},
			path: "/dash/settings/all",
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
			if got := BuildDashPath(tt.path); got != tt.want {
				t.Fatalf("BuildDashPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestCanonicalDashPath(t *testing.T) {
	tests := []struct {
		name     string
		settings map[string]string
		path     string
		want     string
	}{
		{
			name: "canonicalize custom dash root",
			settings: map[string]string{
				"dash_path": "/dash/dashboard",
			},
			path: "/dash/dashboard",
			want: "/dash",
		},
		{
			name: "canonicalize custom dash child route",
			settings: map[string]string{
				"dash_path": "/dash/dashboard",
			},
			path: "/dash/dashboard/posts",
			want: "/dash/posts",
		},
		{
			name: "already canonical route keeps original",
			settings: map[string]string{
				"dash_path": "/dash/dashboard",
			},
			path: "/dash/settings/all",
			want: "/dash/settings/all",
		},
		{
			name: "root dash path keeps original route",
			settings: map[string]string{
				"dash_path": "/",
			},
			path: "/dash/posts",
			want: "/dash/posts",
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
			if got := CanonicalDashPath(tt.path); got != tt.want {
				t.Fatalf("CanonicalDashPath(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func TestURLFor(t *testing.T) {
	SetURLForResolver(func(name string, params map[string]string, query map[string]string) (string, error) {
		if name != "dash.posts.edit" {
			return "", fmt.Errorf("unexpected route name: %s", name)
		}
		if params["id"] == "" {
			return "", fmt.Errorf("missing id param")
		}
		path := "/dash/posts/" + params["id"] + "/edit"
		if query["tab"] != "" {
			path += "?tab=" + query["tab"]
		}
		return path, nil
	})
	t.Cleanup(func() {
		SetURLForResolver(nil)
	})

	got := URLFor(
		"dash.posts.edit",
		map[string]string{"id": "123"},
		map[string]string{"tab": "comments"},
	)
	want := "/dash/posts/123/edit?tab=comments"
	if got != want {
		t.Fatalf("URLFor() = %q, want %q", got, want)
	}
}

func TestURLForWithoutResolver(t *testing.T) {
	SetURLForResolver(nil)
	got := URLFor("dash.posts.edit", map[string]string{"id": "1"}, nil)
	if got != "" {
		t.Fatalf("URLFor() = %q, want empty string when resolver not set", got)
	}
}

func TestURLForStoreIsolation(t *testing.T) {
	storeA := NewURLForStore()
	storeB := NewURLForStore()

	storeA.SetResolver(func(name string, params map[string]string, query map[string]string) (string, error) {
		return "/a/" + name, nil
	})
	storeB.SetResolver(func(name string, params map[string]string, query map[string]string) (string, error) {
		return "/b/" + name, nil
	})

	if got := storeA.URLFor("dash.home", nil, nil); got != "/a/dash.home" {
		t.Fatalf("storeA.URLFor() = %q, want %q", got, "/a/dash.home")
	}
	if got := storeB.URLFor("dash.home", nil, nil); got != "/b/dash.home" {
		t.Fatalf("storeB.URLFor() = %q, want %q", got, "/b/dash.home")
	}

	storeA.SetResolver(nil)

	if got := storeA.URLFor("dash.home", nil, nil); got != "" {
		t.Fatalf("storeA.URLFor() after nil resolver = %q, want empty string", got)
	}
	if got := storeB.URLFor("dash.home", nil, nil); got != "/b/dash.home" {
		t.Fatalf("storeB.URLFor() affected by storeA change = %q, want %q", got, "/b/dash.home")
	}
}
