package md

import (
	"strings"
	"testing"
)

func TestExtractTOCHTML_OnlyReturnsTOCNode(t *testing.T) {
	input := `<html><body><div class="toc"><h2 class="toc-title">目录</h2><ol class="toc-list"><li><a href="#a">A</a></li></ol></div><h1 id="a">A</h1></body></html>`
	got := ExtractTOCHTML(input)

	if !strings.Contains(got, `class="toc"`) {
		t.Fatalf("expected toc wrapper, got: %s", got)
	}
	if strings.Contains(got, `<h1 id="a">`) {
		t.Fatalf("expected only toc html, got: %s", got)
	}
}

func TestParseMarkdownTOC_ReturnsTOCForHeadings(t *testing.T) {
	markdown := "# 一级\n\n## 二级"
	got := ParseMarkdownTOC(markdown)

	if !strings.Contains(got, `class="toc"`) {
		t.Fatalf("expected toc html, got: %s", got)
	}
	if !strings.Contains(got, `href="#`) {
		t.Fatalf("expected heading links in toc, got: %s", got)
	}
}

func TestParseMarkdownTOC_ReturnsEmptyWhenNoHeading(t *testing.T) {
	got := ParseMarkdownTOC("只有正文，没有标题。")
	if got != "" {
		t.Fatalf("expected empty toc html, got: %s", got)
	}
}
