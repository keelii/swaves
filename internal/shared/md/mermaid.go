package md

import (
	"bytes"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

type MermaidBlock struct {
	ast.BaseBlock
}

var KindMermaidBlock = ast.NewNodeKind("MermaidBlock")

func (n *MermaidBlock) Kind() ast.NodeKind {
	return KindMermaidBlock
}

func (n *MermaidBlock) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

func (n *MermaidBlock) IsRaw() bool {
	return true
}

type MermaidTransformer struct{}

func (t *MermaidTransformer) Transform(doc *ast.Document, reader text.Reader, ctx parser.Context) {
	source := reader.Source()
	_ = ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}

		fenced, ok := node.(*ast.FencedCodeBlock)
		if !ok || !isMermaidLanguage(fenced.Language(source)) {
			return ast.WalkContinue, nil
		}

		parent := fenced.Parent()
		if parent == nil {
			return ast.WalkContinue, nil
		}

		next := &MermaidBlock{}
		next.SetLines(fenced.Lines())
		next.SetBlankPreviousLines(fenced.HasBlankPreviousLines())
		parent.ReplaceChild(parent, fenced, next)
		return ast.WalkSkipChildren, nil
	})
}

func isMermaidLanguage(language []byte) bool {
	normalized := strings.ToLower(strings.TrimSpace(string(language)))
	return normalized == "mermaid"
}

type MermaidHTMLRenderer struct {
	html.Config
}

func NewMermaidHTMLRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &MermaidHTMLRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *MermaidHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindMermaidBlock, r.renderMermaidBlock)
}

func (r *MermaidHTMLRenderer) renderMermaidBlock(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}

	_, _ = w.WriteString(`<pre class="mermaid">`)
	r.Writer.Write(w, bytes.TrimRight(mermaidBlockSource(node, source), "\n"))
	_, _ = w.WriteString("</pre>\n")
	return ast.WalkSkipChildren, nil
}

func mermaidBlockSource(node ast.Node, source []byte) []byte {
	var buf bytes.Buffer
	lines := node.Lines()
	for i := 0; i < lines.Len(); i += 1 {
		line := lines.At(i)
		buf.Write(line.Value(source))
	}
	return buf.Bytes()
}
