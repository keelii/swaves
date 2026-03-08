package md

import (
	"strings"

	"golang.org/x/net/html"
)

func ParseMarkdownTOC(text string) string {
	result := ParseMarkdown(text, true)
	if result == nil {
		return ""
	}
	return ExtractTOCHTML(result.HTML)
}

func ExtractTOCHTML(input string) string {
	source := strings.TrimSpace(input)
	if source == "" {
		return ""
	}

	doc, err := html.Parse(strings.NewReader(source))
	if err != nil {
		return ""
	}

	tocNode := findNodeByClass(doc, "toc")
	if tocNode == nil {
		return ""
	}

	var builder strings.Builder
	if err = html.Render(&builder, tocNode); err != nil {
		return ""
	}
	return builder.String()
}

func findNodeByClass(node *html.Node, className string) *html.Node {
	if node == nil {
		return nil
	}
	if node.Type == html.ElementNode && hasClassName(node, className) {
		return node
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if matched := findNodeByClass(child, className); matched != nil {
			return matched
		}
	}
	return nil
}

func hasClassName(node *html.Node, className string) bool {
	for _, attr := range node.Attr {
		if attr.Key != "class" {
			continue
		}
		for _, token := range strings.Fields(attr.Val) {
			if token == className {
				return true
			}
		}
	}
	return false
}
