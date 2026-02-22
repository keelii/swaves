package md

import (
	"strings"
	"testing"
)

func TestParseMarkdown_GuessesGoFenceLanguage(t *testing.T) {
	input := "```\npackage main\n\nimport \"fmt\"\n\nfunc main() {\n  fmt.Println(1)\n}\n```"
	result := ParseMarkdown(input, false)
	if !strings.Contains(result.HTML, "font-weight:bold") {
		t.Fatalf("expected highlighted output, got: %s", result.HTML)
	}
}
