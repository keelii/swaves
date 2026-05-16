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

func TestParseMarkdown_RendersMermaidFenceAsMermaidBlock(t *testing.T) {
	input := "```mermaid\ngraph TD\n  A --> B\n```"
	result := ParseMarkdown(input, false)

	if !strings.Contains(result.HTML, `<pre class="mermaid">`) {
		t.Fatalf("expected mermaid pre block, got: %s", result.HTML)
	}
	if !strings.Contains(result.HTML, "graph TD") {
		t.Fatalf("expected mermaid source to be preserved, got: %s", result.HTML)
	}
	if !strings.Contains(result.HTML, "A --&gt; B") {
		t.Fatalf("expected mermaid source to be escaped, got: %s", result.HTML)
	}
	if strings.Contains(result.HTML, `style=";-webkit-text-size-adjust:none;"`) || strings.Contains(result.HTML, "display:flex") {
		t.Fatalf("mermaid block should not be syntax highlighted, got: %s", result.HTML)
	}
}

func TestParseMarkdown_RendersMermaidFenceCaseInsensitively(t *testing.T) {
	input := "```Mermaid\nsequenceDiagram\n  Alice->>Bob: Hi\n```"
	result := ParseMarkdown(input, false)

	if !strings.Contains(result.HTML, `<pre class="mermaid">`) {
		t.Fatalf("expected mermaid pre block, got: %s", result.HTML)
	}
	if !strings.Contains(result.HTML, "Alice-&gt;&gt;Bob: Hi") {
		t.Fatalf("expected mermaid source to be escaped, got: %s", result.HTML)
	}
}

func TestParseMarkdown_RendersMermaidFenceWithTOCEnabled(t *testing.T) {
	input := "# Title\n\n```mermaid\ngraph TD\n  A --> B\n```"
	result := ParseMarkdown(input, true)

	if !strings.Contains(result.HTML, `<pre class="mermaid">`) {
		t.Fatalf("expected mermaid pre block with TOC enabled, got: %s", result.HTML)
	}
	if !strings.Contains(result.TOCHTML, `class="toc"`) {
		t.Fatalf("expected toc html, got: %s", result.TOCHTML)
	}
	if strings.Contains(result.HTML, "display:flex") {
		t.Fatalf("mermaid block should not be syntax highlighted with TOC enabled, got: %s", result.HTML)
	}
}
