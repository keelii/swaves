package view

import (
	"strings"
	"testing"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
)

func TestHtmlAttrs_PositionalMap(t *testing.T) {
	env := minijinja.NewEnvironment()
	registerViewFunctions(env, func(string, map[string]string, map[string]string) string { return "" })
	tmpl, err := env.TemplateFromString(`X{{ HtmlAttrs({"data-x":"1","disabled":true}) }}Y`)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	out, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, ` data-x="1"`) {
		t.Fatalf("expected data-x attr, got: %q", out)
	}
	if !strings.Contains(out, ` disabled`) {
		t.Fatalf("expected disabled boolean attr, got: %q", out)
	}
}

func TestHtmlAttrs_KeywordArgs(t *testing.T) {
	env := minijinja.NewEnvironment()
	registerViewFunctions(env, func(string, map[string]string, map[string]string) string { return "" })
	tmpl, err := env.TemplateFromString(`X{{ HtmlAttrs(class="a", tabindex="0") }}Y`)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	out, err := tmpl.Render(nil)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !strings.Contains(out, ` class="a"`) || !strings.Contains(out, ` tabindex="0"`) {
		t.Fatalf("expected attrs, got: %q", out)
	}
}
