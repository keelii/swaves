package helper

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/gosimple/slug"
	"golang.org/x/net/html"
)

func FlattenTOC(input string) string {
	var err error
	var doc *html.Node
	doc, err = html.Parse(strings.NewReader(input))
	if err != nil {
		log.Println("解析 HTML 失败:", err)
		return input
	}
	fixTocList(doc)

	var buf strings.Builder
	if err = html.Render(&buf, doc); err != nil {
		log.Println("渲染 HTML 失败:", err)
		return input
	}
	return buf.String()
}

func fixTocList(n *html.Node) {
	if n.Type == html.ElementNode && n.Data == "ol" {
		for _, attr := range n.Attr {
			if attr.Key == "id" && attr.Val == "toc-list" {
				flattenTocList(n)
				return
			}
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		fixTocList(c)
	}
}

// flattenTocList 将 <ul id="toc-list"><li><ul>...items...</ul></li></ul>
// 转换为 <ul id="toc-list">...items...</ul>
func flattenTocList(ul *html.Node) {
	var items []*html.Node // 存储真正的列表项

	// 遍历直接子节点
	for child := ul.FirstChild; child != nil; {
		next := child.NextSibling

		if child.Type == html.ElementNode && child.Data == "li" {
			// 查找这个 li 内部的 ul
			for grandchild := child.FirstChild; grandchild != nil; {
				gcNext := grandchild.NextSibling

				if grandchild.Type == html.ElementNode && grandchild.Data == "ul" {
					// 提取这个内部 ul 的所有 li 子项
					for item := grandchild.FirstChild; item != nil; {
						itemNext := item.NextSibling
						if item.Type == html.ElementNode && item.Data == "li" {
							// 从原位置移除
							grandchild.RemoveChild(item)
							items = append(items, item)
						}
						item = itemNext
					}
				}
				grandchild = gcNext
			}
			// 移除这个包装用的 li
			ul.RemoveChild(child)
		}

		child = next
	}

	// 将提取的 items 添加到 ul 中
	for _, item := range items {
		ul.AppendChild(item)
	}
}

func EnsureDir(dirPath string, perm os.FileMode) error {
	// 检查路径是否存在
	info, err := os.Stat(dirPath)
	if err == nil {
		// 路径存在，检查是否是目录
		if !info.IsDir() {
			return fmt.Errorf("路径存在但不是目录: %s", dirPath)
		}
		return nil // 目录已存在
	}

	// 如果错误是"不存在"，则创建目录
	if os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, perm)
		if err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
		return nil
	}

	// 其他错误（权限问题等）
	return fmt.Errorf("检查目录失败: %w", err)
}

func IsSlug(str string) bool {
	return slug.IsSlug(str)
}

func JSONEncode(str string) string {
	b, _ := json.Marshal(str)
	return strings.Trim(string(b), `"`)
}
func JSONStringify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
