package site

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseCommentFormDefaults(t *testing.T) {
	raw := "author=%E5%BC%A0%E4%B8%89&author_email=test%40example.com&author_url=https%3A%2F%2Fexample.com&content=hello+world"
	got := parseCommentFormDefaults(raw)

	if got.Author != "张三" {
		t.Fatalf("unexpected author: %q", got.Author)
	}
	if got.AuthorEmail != "test@example.com" {
		t.Fatalf("unexpected author email: %q", got.AuthorEmail)
	}
	if got.AuthorURL != "https://example.com" {
		t.Fatalf("unexpected author url: %q", got.AuthorURL)
	}
	if got.Content != "hello world" {
		t.Fatalf("unexpected content: %q", got.Content)
	}
	if !got.RememberMe {
		t.Fatal("remember me should be true when any remembered field exists")
	}
}

func TestParseCommentFormDefaultsInvalidOrOversized(t *testing.T) {
	if got := parseCommentFormDefaults("%%%"); got.RememberMe {
		t.Fatal("remember me should be false for invalid cookie content")
	}

	values := url.Values{}
	values.Set("author", "x")
	values.Set("author_email", "ok@example.com")
	values.Set("author_url", "https://example.com")
	got := parseCommentFormDefaults(values.Encode())
	if got.Author != "x" || got.AuthorEmail == "" || got.AuthorURL == "" {
		t.Fatalf("unexpected normal cookie parse result: %#v", got)
	}

	values.Set("author", strings.Repeat("a", 81))
	got = parseCommentFormDefaults(values.Encode())
	if got.Author != "" {
		t.Fatalf("oversized author should be ignored, got %q", got.Author)
	}
	if !got.RememberMe {
		t.Fatal("remember me should stay true when email/url exist")
	}

	values = url.Values{}
	values.Set("content", strings.Repeat("x", 5001))
	got = parseCommentFormDefaults(values.Encode())
	if got.Content != "" {
		t.Fatalf("oversized content should be ignored, got len=%d", len(got.Content))
	}
}

func TestIsCommentRememberMeEnabled(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{raw: "1", want: true},
		{raw: "true", want: true},
		{raw: "on", want: true},
		{raw: "yes", want: true},
		{raw: "", want: false},
		{raw: "0", want: false},
		{raw: "off", want: false},
	}

	for _, tc := range cases {
		if got := isCommentRememberMeEnabled(tc.raw); got != tc.want {
			t.Fatalf("isCommentRememberMeEnabled(%q) = %v, want %v", tc.raw, got, tc.want)
		}
	}
}
