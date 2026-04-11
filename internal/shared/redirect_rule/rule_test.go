package redirect_rule

import "testing"

func TestRule(t *testing.T) {
	validCases := map[string]struct {
		from      string
		to        string
		path      string
		target    string
		isPattern bool
		missPath  string
	}{
		"exact": {
			from:      "/legacy-post",
			to:        "/new-post",
			path:      "/legacy-post",
			target:    "/new-post",
			isPattern: false,
			missPath:  "/another-post",
		},
		"exact trailing slash": {
			from:      "/legacy-post",
			to:        "/new-post",
			path:      "/legacy-post/",
			target:    "/new-post",
			isPattern: false,
		},
		"param": {
			from:      "/{year}/{month}/{day}/{slug}",
			to:        "/{slug}",
			path:      "/2019/01/01/political-correctness",
			target:    "/political-correctness",
			isPattern: true,
			missPath:  "/2019/01/political-correctness",
		},
		"wildcard": {
			from:      "/*/*/*/{slug}",
			to:        "/{slug}",
			path:      "/2019/01/01/legacy-post",
			target:    "/legacy-post",
			isPattern: true,
			missPath:  "/2019/01/legacy-post",
		},
	}

	invalidCases := map[string]struct {
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

	for name, tt := range validCases {
		rule, err := Compile(tt.from, tt.to)
		if err != nil {
			t.Fatalf("%s: compile failed: %v", name, err)
		}
		if rule.IsPattern() != tt.isPattern {
			t.Fatalf("%s: unexpected pattern flag: got=%v want=%v", name, rule.IsPattern(), tt.isPattern)
		}
		target, ok := rule.Match(tt.path)
		if !ok || target != tt.target {
			t.Fatalf("%s: expected match target=%q, got ok=%v target=%q", name, tt.target, ok, target)
		}
		if tt.missPath != "" {
			if _, ok := rule.Match(tt.missPath); ok {
				t.Fatalf("%s: expected mismatch for path=%q", name, tt.missPath)
			}
		}
	}

	for name, tt := range invalidCases {
		if _, err := Compile(tt.from, tt.to); err == nil {
			t.Fatalf("%s: expected compile to fail: from=%q to=%q", name, tt.from, tt.to)
		}
	}

	rules := []Rule{
		mustCompileRule(t, "/*/*/*/{slug}", "/wild/{slug}"),
		mustCompileRule(t, "/{year}/{month}/{day}/{slug}", "/param/{slug}"),
		mustCompileRule(t, "/blog/{year}/{month}/{slug}", "/blog/{slug}"),
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

func mustCompileRule(t *testing.T, fromPath string, toPath string) Rule {
	t.Helper()
	rule, err := Compile(fromPath, toPath)
	if err != nil {
		t.Fatalf("compile redirect rule failed: %v", err)
	}
	return rule
}
