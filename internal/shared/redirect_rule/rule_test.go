package redirect_rule

import "testing"

func TestRule(t *testing.T) {
	validCases := map[string]struct {
		from      string
		to        string
		path      string
		want      string
		isPattern bool
	}{
		"exact": {
			from:      "/legacy-post",
			to:        "/new-post",
			path:      "/legacy-post",
			isPattern: false,
		},
		"exact trailing slash": {
			from:      "/legacy-post",
			to:        "/new-post",
			path:      "/legacy-post/",
			isPattern: false,
		},
		"param": {
			from:      "/{year}/{month}/{day}/{slug}",
			to:        "/{slug}",
			path:      "/2019/01/01/political-correctness",
			want:      "/political-correctness",
			isPattern: true,
		},
		"wildcard": {
			from:      "/*/*/*/{slug}",
			to:        "/{slug}",
			path:      "/2019/01/01/legacy-post",
			want:      "/legacy-post",
			isPattern: true,
		},
	}

	invalidCompileCases := map[string]struct {
		from string
		to   string
	}{
		"empty from":           {from: "", to: "/target"},
		"empty to":             {from: "/source", to: ""},
		"duplicate param":      {from: "/{slug}/{slug}", to: "/{slug}"},
		"undefined target var": {from: "/{slug}", to: "/{id}"},
		"wildcard target":      {from: "/{slug}", to: "/*"},
		"invalid param name":   {from: "/{1slug}", to: "/target"},
		"broken param segment": {from: "/prefix-{slug}", to: "/target"},
	}

	invalidMatchCases := map[string]struct {
		from string
		to   string
		path string
	}{
		"exact miss":    {from: "/legacy-post", to: "/new-post", path: "/another-post"},
		"param miss":    {from: "/{year}/{month}/{day}/{slug}", to: "/{slug}", path: "/2019/01/political-correctness"},
		"wildcard miss": {from: "/*/*/*/{slug}", to: "/{slug}", path: "/2019/01/legacy-post"},
	}

	for name, tt := range validCases {
		rule, err := Compile(tt.from, tt.to)
		if err != nil {
			t.Fatalf("%s: compile failed: %v", name, err)
		}
		if rule.IsPattern() != tt.isPattern {
			t.Fatalf("%s: unexpected pattern flag: got=%v want=%v", name, rule.IsPattern(), tt.isPattern)
		}
		target, ok := rule.Match(tt.path)
		if tt.want == "" {
			tt.want = tt.to
		}
		if !ok || target != tt.want {
			t.Fatalf("%s: expected match target=%q, got ok=%v target=%q", name, tt.want, ok, target)
		}
	}

	for name, tt := range invalidCompileCases {
		if _, err := Compile(tt.from, tt.to); err == nil {
			t.Fatalf("%s: expected compile to fail: from=%q to=%q", name, tt.from, tt.to)
		}
	}

	for name, tt := range invalidMatchCases {
		rule, err := Compile(tt.from, tt.to)
		if err != nil {
			t.Fatalf("%s: compile failed: %v", name, err)
		}
		if _, ok := rule.Match(tt.path); ok {
			t.Fatalf("%s: expected mismatch for path=%q", name, tt.path)
		}
	}

	rules := []Rule{
		{From: "/*/*/*/{slug}", paramCnt: 1, wildcardCnt: 3, fromParts: make([]segment, 4)},
		{From: "/{year}/{month}/{day}/{slug}", paramCnt: 4, fromParts: make([]segment, 4)},
		{From: "/blog/{year}/{month}/{slug}", literalCnt: 1, paramCnt: 3, fromParts: make([]segment, 4)},
	}
	SortRules(rules)
	if rules[0].From != "/blog/{year}/{month}/{slug}" {
		t.Fatalf("expected most literal rule first, got=%q", rules[0].From)
	}
	if rules[1].From != "/{year}/{month}/{day}/{slug}" {
		t.Fatalf("expected param rule second, got=%q", rules[1].From)
	}
	if rules[2].From != "/*/*/*/{slug}" {
		t.Fatalf("expected wildcard rule last, got=%q", rules[2].From)
	}
}
