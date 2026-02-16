package md

import (
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.abhg.dev/goldmark/toc"
)

// 自定义容器节点
type TOCContainer struct {
	ast.BaseBlock
}

func (n *TOCContainer) Dump(source []byte, level int) {
	ast.DumpHelper(n, source, level, nil, nil)
}

var KindTOCContainer = ast.NewNodeKind("TOCContainer")

func (n *TOCContainer) Kind() ast.NodeKind {
	return KindTOCContainer
}

func NewTOCContainer() *TOCContainer {
	return &TOCContainer{}
}

// Renderer
type TOCContainerHTMLRenderer struct {
	html.Config
}

func NewTOCContainerHTMLRenderer(opts ...html.Option) renderer.NodeRenderer {
	r := &TOCContainerHTMLRenderer{
		Config: html.NewConfig(),
	}
	for _, opt := range opts {
		opt.SetHTMLOption(&r.Config)
	}
	return r
}

func (r *TOCContainerHTMLRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(KindTOCContainer, r.renderTOCContainer)
}

func (r *TOCContainerHTMLRenderer) renderTOCContainer(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		_, _ = w.WriteString(`<div class="toc"><span class="toc-toggle" onclick="this.parentNode.classList.toggle('open')">§</span>`)
		_, _ = w.WriteString("\n")
	} else {
		_, _ = w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

// 自定义 Transformer
type MyTransformer struct {
	toc.Transformer
}

func (t *MyTransformer) Transform(doc *ast.Document, reader text.Reader, ctx parser.Context) {
	tocTree, err := toc.Inspect(doc, reader.Source())
	if err != nil {
		return
	}

	if len(tocTree.Items) == 0 {
		return
	}

	listNode := toc.RenderOrderedList(tocTree)
	listNode.SetAttributeString("class", "toc-list")

	// 创建标题
	heading := ast.NewHeading(2)
	heading.SetAttributeString("class", "toc-title")
	heading.AppendChild(heading, ast.NewString([]byte("目录")))

	// 使用容器节点
	container := NewTOCContainer()
	container.AppendChild(container, heading)
	container.AppendChild(container, listNode)

	doc.InsertBefore(doc, doc.FirstChild(), container)
}
