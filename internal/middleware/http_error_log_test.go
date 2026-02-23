package middleware

import (
	"strings"
	"testing"
)

func TestSanitizeQueryParams_RedactsSensitiveValues(t *testing.T) {
	got := sanitizeQueryParams("page=1&password=abc123&access_token=token123")
	if !strings.Contains(got, "page=1") {
		t.Fatalf("expected page query retained, got %q", got)
	}
	if !strings.Contains(got, "password=%5BREDACTED%5D") {
		t.Fatalf("expected password redacted, got %q", got)
	}
	if !strings.Contains(got, "access_token=%5BREDACTED%5D") {
		t.Fatalf("expected access_token redacted, got %q", got)
	}
}

func TestSanitizeBodyParams_FormURLEncoded(t *testing.T) {
	got := sanitizeBodyParams("title=ok&secret=my-secret", "application/x-www-form-urlencoded")
	if !strings.Contains(got, "title=ok") {
		t.Fatalf("expected non-sensitive form value retained, got %q", got)
	}
	if !strings.Contains(got, "secret=%5BREDACTED%5D") {
		t.Fatalf("expected secret redacted, got %q", got)
	}
}

func TestSanitizeBodyParams_JSON(t *testing.T) {
	got := sanitizeBodyParams(`{"name":"ok","password":"abc","nested":{"token":"123"}}`, "application/json")
	if strings.Contains(got, `"abc"`) {
		t.Fatalf("expected password not present, got %q", got)
	}
	if strings.Contains(got, `"123"`) {
		t.Fatalf("expected nested token not present, got %q", got)
	}
	if !strings.Contains(got, `"[REDACTED]"`) {
		t.Fatalf("expected redacted marker, got %q", got)
	}
}

func TestSanitizeBodyParams_MultipartOmitted(t *testing.T) {
	got := sanitizeBodyParams("ignored", "multipart/form-data; boundary=abc")
	if got != "[multipart body omitted]" {
		t.Fatalf("unexpected multipart sanitize result: %q", got)
	}
}

func TestSanitizeBodyParams_UnknownContentTypeOmitted(t *testing.T) {
	got := sanitizeBodyParams("raw-secret", "text/plain")
	if got != "[body omitted content-type=text/plain]" {
		t.Fatalf("unexpected unknown type sanitize result: %q", got)
	}
}

func TestIsSensitiveFieldName(t *testing.T) {
	cases := []struct {
		name      string
		sensitive bool
	}{
		{name: "password", sensitive: true},
		{name: "access_token", sensitive: true},
		{name: "private_key", sensitive: true},
		{name: "title", sensitive: false},
	}
	for _, tc := range cases {
		if got := isSensitiveFieldName(tc.name); got != tc.sensitive {
			t.Fatalf("isSensitiveFieldName(%q) = %v, want %v", tc.name, got, tc.sensitive)
		}
	}
}
