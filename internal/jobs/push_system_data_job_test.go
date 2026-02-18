package job

import (
	"testing"
)

func TestBuildS3ObjectKey(t *testing.T) {
	testCases := []struct {
		name string
		path string
		want string
	}{
		{name: "normal path", path: "/tmp/a.sqlite", want: "a.sqlite"},
		{name: "invalid path fallback", path: ".", want: "snapshot.sqlite"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildS3ObjectKey(tc.path)
			if got != tc.want {
				t.Fatalf("buildS3ObjectKey(%q)=%q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestSplitS3EndpointBucket(t *testing.T) {
	testCases := []struct {
		name          string
		input         string
		wantEndpoint  string
		wantBucket    string
		wantForcePath bool
		wantErr       bool
	}{
		{
			name:          "empty endpoint",
			input:         "",
			wantEndpoint:  "",
			wantBucket:    "",
			wantForcePath: false,
			wantErr:       false,
		},
		{
			name:          "endpoint without bucket",
			input:         "https://s3.amazonaws.com",
			wantEndpoint:  "https://s3.amazonaws.com",
			wantBucket:    "",
			wantForcePath: false,
			wantErr:       false,
		},
		{
			name:          "endpoint with bucket path",
			input:         "https://s3.example.com/my-bucket",
			wantEndpoint:  "https://s3.example.com",
			wantBucket:    "my-bucket",
			wantForcePath: true,
			wantErr:       false,
		},
		{
			name:          "endpoint with bucket and base path",
			input:         "https://proxy.example.com/root/my-bucket",
			wantEndpoint:  "https://proxy.example.com/my-bucket",
			wantBucket:    "root",
			wantForcePath: true,
			wantErr:       false,
		},
		{
			name:          "invalid endpoint",
			input:         "not-a-url",
			wantEndpoint:  "not-a-url",
			wantBucket:    "",
			wantForcePath: false,
			wantErr:       true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			gotEndpoint, gotBucket, gotForcePath, err := splitS3EndpointBucket(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("splitS3EndpointBucket(%q) expected error", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("splitS3EndpointBucket(%q) unexpected error: %v", tc.input, err)
			}
			if gotEndpoint != tc.wantEndpoint {
				t.Fatalf("endpoint=%q, want %q", gotEndpoint, tc.wantEndpoint)
			}
			if gotBucket != tc.wantBucket {
				t.Fatalf("bucket=%q, want %q", gotBucket, tc.wantBucket)
			}
			if gotForcePath != tc.wantForcePath {
				t.Fatalf("forcePath=%v, want %v", gotForcePath, tc.wantForcePath)
			}
		})
	}
}

func TestShortHash(t *testing.T) {
	if got := shortHash("1234567890"); got != "12345678" {
		t.Fatalf("shortHash should keep first 8 chars, got %q", got)
	}
	if got := shortHash("1234"); got != "1234" {
		t.Fatalf("shortHash should keep original for short strings, got %q", got)
	}
}
