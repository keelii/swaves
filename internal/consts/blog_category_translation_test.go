package consts

import "testing"

func TestLookupTechBlogCategoryTranslation(t *testing.T) {
	tests := []struct {
		name  string
		in    string
		want  string
		found bool
	}{
		{name: "中文分类", in: "后端开发", want: "backend-development", found: true},
		{name: "英文大小写", in: "GoLang", want: "go", found: true},
		{name: "全角空格", in: "　机器学习　", want: "machine-learning", found: true},
		{name: "英文多空格", in: "react   native", want: "react-native", found: true},
		{name: "不存在", in: "数码摄影", want: "", found: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := LookupTechBlogCategoryTranslation(tt.in)
			if ok != tt.found {
				t.Fatalf("found = %v, want %v", ok, tt.found)
			}
			if got != tt.want {
				t.Fatalf("translation = %q, want %q", got, tt.want)
			}
		})
	}
}
