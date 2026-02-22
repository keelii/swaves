package helper

import (
	"testing"

	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

func TestMakeSlug(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "mixed Chinese and English inserts separator",
			input: "Web版的VNC",
			want:  "web-ban-de-vnc",
		},
		{
			name:  "distinguishes c and c plus plus",
			input: "如何学好C语言",
			want:  "ru-he-xue-hao-c-yu-yan",
		},
		{
			name:  "maps c plus plus before slugging",
			input: "如何学好C++语言",
			want:  "ru-he-xue-hao-cpp-yu-yan",
		},
		{
			name:  "maps c sharp before slugging",
			input: "C# 入门",
			want:  "csharp-ru-men",
		},
		{
			name:  "keeps regular English behavior",
			input: "Hello, World",
			want:  "hello-world",
		},
		{
			name:  "empty input",
			input: "   ",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := MakeSlug(tc.input)
			if got != tc.want {
				t.Fatalf("MakeSlug(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestDecodeAnyToType(t *testing.T) {
	type sample struct {
		ID        int64
		Name      string
		DeletedAt *int64
	}

	decoded, ok := DecodeAnyToType[sample](map[string]any{
		"ID":   int64(7),
		"Name": "demo",
	})
	if !ok {
		t.Fatalf("DecodeAnyToType should decode map payload")
	}
	if decoded.ID != 7 {
		t.Fatalf("decoded id = %d, want 7", decoded.ID)
	}
	if decoded.Name != "demo" {
		t.Fatalf("decoded name = %q, want %q", decoded.Name, "demo")
	}
	if decoded.DeletedAt != nil {
		t.Fatalf("decoded deleted_at = %v, want nil", *decoded.DeletedAt)
	}
}

func TestDecodeAnyToTypeReturnsFalseOnInvalidPayload(t *testing.T) {
	type sample struct {
		ID int64
	}

	_, ok := DecodeAnyToType[sample](map[string]any{
		"ID": map[string]any{"bad": "shape"},
	})
	if ok {
		t.Fatalf("DecodeAnyToType should return false for invalid payload")
	}
}

func TestDecodeAnyToTypeHandlesMiniJinjaValues(t *testing.T) {
	type sample struct {
		ID        int64
		Name      string
		DeletedAt *int64
	}

	decoded, ok := DecodeAnyToType[sample](map[string]value.Value{
		"ID":        value.FromInt(21),
		"Name":      value.FromString("alice"),
		"DeletedAt": value.None(),
	})
	if !ok {
		t.Fatalf("DecodeAnyToType should decode mini jinja value map")
	}
	if decoded.ID != 21 {
		t.Fatalf("decoded id = %d, want 21", decoded.ID)
	}
	if decoded.Name != "alice" {
		t.Fatalf("decoded name = %q, want %q", decoded.Name, "alice")
	}
	if decoded.DeletedAt != nil {
		t.Fatalf("decoded deleted_at = %v, want nil", *decoded.DeletedAt)
	}
}

func TestDecodeAnyToTypeHandlesTypedInput(t *testing.T) {
	type sample struct {
		ID int64
	}

	want := sample{ID: 9}
	got, ok := DecodeAnyToType[sample](want)
	if !ok {
		t.Fatalf("DecodeAnyToType should pass through typed input")
	}
	if got != want {
		t.Fatalf("decoded typed input = %+v, want %+v", got, want)
	}

	got, ok = DecodeAnyToType[sample](&want)
	if !ok {
		t.Fatalf("DecodeAnyToType should dereference typed pointer input")
	}
	if got != want {
		t.Fatalf("decoded typed pointer input = %+v, want %+v", got, want)
	}
}
