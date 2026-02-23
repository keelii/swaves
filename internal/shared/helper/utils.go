package helper

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/logger"

	"github.com/gosimple/slug"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
	"golang.org/x/net/html"
)

var (
	slugASCIIToNonASCIIBoundary = regexp.MustCompile(`([A-Za-z0-9])([^\x00-\x7F])`)
	slugNonASCIIToASCIIBoundary = regexp.MustCompile(`([^\x00-\x7F])([A-Za-z0-9])`)
	slugPreReplacePairs         = []string{
		"C++", "CPP",
		"c++", "cpp",
		"C#", "CSharp",
		"c#", "csharp",
		"F#", "FSharp",
		"f#", "fsharp",
		".NET", "DotNet",
		".net", "dotnet",
	}
	slugPreReplacer = strings.NewReplacer(slugPreReplacePairs...)
)

// FlattenTOC 扁平化目录结构
// listType: "ul" 或 "ol"，指定要查找的列表类型
func FlattenTOC(input string, listType string, attrKey string) string {
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		logger.Warn("解析 HTML 失败: %v", err)
		return input
	}

	// 查找并修复指定类型的 toc-list
	var fixTocList func(*html.Node)
	fixTocList = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == listType {
			for _, attr := range n.Attr {
				if attr.Key == attrKey && attr.Val == "toc-list" {
					flattenTocList(n, listType)
					return
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			fixTocList(c)
		}
	}

	fixTocList(doc)

	var buf strings.Builder
	if err = html.Render(&buf, doc); err != nil {
		logger.Warn("渲染 HTML 失败: %v", err)
		return input
	}
	return buf.String()
}

// flattenTocList 扁平化指定类型的列表
func flattenTocList(list *html.Node, listType string) {
	var items []*html.Node

	for child := list.FirstChild; child != nil; {
		next := child.NextSibling

		if child.Type == html.ElementNode && child.Data == "li" {
			// 查找 li 内部指定类型的列表
			for grandchild := child.FirstChild; grandchild != nil; {
				gcNext := grandchild.NextSibling

				// 只检查指定的列表类型
				if grandchild.Type == html.ElementNode && grandchild.Data == listType {
					// 提取内部列表的所有 li 项
					for item := grandchild.FirstChild; item != nil; {
						itemNext := item.NextSibling
						if item.Type == html.ElementNode && item.Data == "li" {
							grandchild.RemoveChild(item)
							items = append(items, item)
						}
						item = itemNext
					}
				}
				grandchild = gcNext
			}
			list.RemoveChild(child)
		}

		child = next
	}

	for _, item := range items {
		list.AppendChild(item)
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

func MakeSlug(str string) string {
	normalized := normalizeSlugInput(str)
	if normalized == "" {
		return ""
	}
	return strings.Trim(slug.Make(normalized), "-")
}

func normalizeSlugInput(str string) string {
	str = strings.TrimSpace(str)
	if str == "" {
		return ""
	}
	str = slugPreReplacer.Replace(str)
	str = slugASCIIToNonASCIIBoundary.ReplaceAllString(str, `$1 $2`)
	str = slugNonASCIIToASCIIBoundary.ReplaceAllString(str, `$1 $2`)
	return str
}

func JSONEncode(str string) string {
	b, _ := json.Marshal(str)
	return strings.Trim(string(b), `"`)
}
func JSONStringify(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func DecodeAnyToType[T any](raw any) (T, bool) {
	var zero T
	switch typed := raw.(type) {
	case T:
		return typed, true
	case *T:
		if typed == nil {
			return zero, false
		}
		return *typed, true
	}
	raw = normalizeDecodeInput(raw)
	switch typed := raw.(type) {
	case T:
		return typed, true
	case *T:
		if typed == nil {
			return zero, false
		}
		return *typed, true
	}
	payload, err := json.Marshal(raw)
	if err != nil {
		logger.Error("failed to marshal value for type decode: %v", err)
		return zero, false
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		logger.Error("failed to unmarshal value for type decode: %v", err)
		return zero, false
	}
	return decoded, true
}

func normalizeDecodeInput(raw any) any {
	switch typed := raw.(type) {
	case value.Value:
		if typed.IsNone() || typed.IsUndefined() || typed.IsSilentUndefined() {
			return nil
		}
		return normalizeDecodeInput(typed.Raw())
	case map[string]value.Value:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = normalizeDecodeInput(item)
		}
		return converted
	case map[string]any:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = normalizeDecodeInput(item)
		}
		return converted
	case []value.Value:
		converted := make([]any, len(typed))
		for idx, item := range typed {
			converted[idx] = normalizeDecodeInput(item)
		}
		return converted
	case []any:
		converted := make([]any, len(typed))
		for idx, item := range typed {
			converted[idx] = normalizeDecodeInput(item)
		}
		return converted
	default:
		return raw
	}
}

func BuildGAvatarURL(email, author string, size int) string {
	if size <= 0 {
		size = 40
	}
	if size > 512 {
		size = 512
	}

	seed := strings.ToLower(strings.TrimSpace(email))
	if seed == "" {
		authorSeed := strings.ToLower(strings.TrimSpace(author))
		if authorSeed == "" {
			authorSeed = "anonymous"
		}
		seed = "swaves:" + authorSeed
	}

	sum := md5.Sum([]byte(seed))
	hash := hex.EncodeToString(sum[:])

	query := url.Values{}
	query.Set("s", fmt.Sprintf("%d", size))
	query.Set("d", "identicon")
	query.Set("r", "g")

	//return "https://cravatar.cn/avatar/" + hash + "?" + query.Encode()
	return fmt.Sprintf("%s/avatar/%s?%s", consts.GravatarDomain, hash, query.Encode())
}
