package helper

import "testing"

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
