package main

import (
	"errors"
	"fmt"
	HTML "html"
	"html/template"
	"regexp"
	"strings"
	"swaves/internal/consts"
	"swaves/internal/db"
	"swaves/internal/share"
	"swaves/internal/store"
	"time"

	"github.com/gofiber/template/html/v3"
)

func NewViewEngine() *html.Engine {
	engine := html.New("./web/templates", ".html")
	engine.AddFunc(consts.GlobalSettingKey, func(key string) string {
		return store.GetSetting(key)
	})
	engine.AddFunc("dict", func(values ...interface{}) (map[string]interface{}, error) {
		if len(values)%2 != 0 {
			return nil, errors.New("invalid dict call: must have even number of arguments")
		}
		dict := make(map[string]interface{}, len(values)/2)
		for i := 0; i < len(values); i += 2 {
			key, ok := values[i].(string)
			if !ok {
				return nil, errors.New("dict keys must be strings")
			}
			dict[key] = values[i+1]
		}
		return dict, nil
	})
	// slice 将可变参数收集为 []interface{}，用于在模板中构造选项列表，如 slice (dict "Value" "0" "Label" "选项") ...
	engine.AddFunc("slice", func(values ...interface{}) []interface{} {
		return values
	})
	engine.AddFunc("add", func(a, b int) int {
		return a + b
	})
	engine.AddFunc("concat", func(a, b string) string {
		return a + b
	})
	engine.AddFunc("safeHTML", func(s interface{}) template.HTML {
		if s == nil {
			return ""
		}
		return template.HTML(fmt.Sprint(s))
	})
	engine.AddFunc("ellipsis", func(a string) template.HTML {
		return template.HTML("<span title='" + a + "' class='ellipsis'>" + a + "</span>")
	})
	engine.AddFunc("long_text", func(a string, cols int, rows int) template.HTML {
		return template.HTML("<textarea class=\"long-text\" cols=\"" + fmt.Sprint(cols) + "\" rows=\"" + fmt.Sprint(rows) + "\" readonly>" + a + "</textarea>")
	})
	engine.AddFunc("until", func(count int) []int {
		var step []int
		for i := 0; i < count; i++ {
			step = append(step, i)
		}
		return step
	})
	engine.AddFunc("formatTime", func(ts interface{}) string {
		var tsInt64 int64
		switch v := ts.(type) {
		case int64:
			tsInt64 = v
		case *int64:
			if v == nil {
				return "-"
			}
			tsInt64 = *v
		default:
			return "-"
		}
		if tsInt64 == 0 {
			return "-"
		}
		return time.Unix(tsInt64, 0).Format(consts.TimeFormat)
	})
	// relativeTime 将 Unix timestamp 转为相对时间：刚刚、1分钟前、1小时前、1天前、1月前、1年前
	engine.AddFunc("relativeTime", func(ts interface{}) string {
		var tsInt64 int64
		switch v := ts.(type) {
		case int64:
			tsInt64 = v
		case *int64:
			if v == nil {
				return "-"
			}
			tsInt64 = *v
		default:
			return "-"
		}
		if tsInt64 == 0 {
			return "-"
		}
		return relativeTimeString(tsInt64)
	})
	engine.AddFunc("articleTime", func(ts interface{}) string {
		var tsInt64 int64
		switch v := ts.(type) {
		case int64:
			tsInt64 = v
		case *int64:
			if v == nil {
				return "-"
			}
		}
		if tsInt64 == 0 {
			return "-"
		}
		return time.Unix(tsInt64, 0).Format("2006-01-02")
	})
	// formatDateTimeLocal 将 Unix timestamp 转换为 datetime-local 输入格式 (YYYY-MM-DDTHH:mm)
	engine.AddFunc("formatDateTimeLocal", func(ts interface{}) string {
		var tsInt64 int64
		switch v := ts.(type) {
		case int64:
			tsInt64 = v
		case *int64:
			if v == nil {
				return ""
			}
			tsInt64 = *v
		default:
			return ""
		}
		if tsInt64 == 0 {
			return ""
		}
		return time.Unix(tsInt64, 0).Format("2006-01-02T15:04")
	})
	// 辅助函数：将 map[string]interface{} 转换为 HTML 属性字符串
	engine.AddFunc("renderAttrs", func(attrs map[string]interface{}) template.HTMLAttr {
		if attrs == nil || len(attrs) == 0 {
			return ""
		}
		var parts []string
		for k, v := range attrs {
			// 将值转换为字符串，处理布尔值（如果为 false 则跳过）
			var val string
			switch tv := v.(type) {
			case bool:
				if tv {
					parts = append(parts, k)
				}
				continue
			case string:
				val = tv
			case float64:
				// JSON 数字会被解析为 float64
				val = fmt.Sprintf("%v", tv)
			default:
				val = fmt.Sprintf("%v", tv)
			}
			// HTML 转义属性值
			val = strings.ReplaceAll(val, `"`, `&quot;`)
			parts = append(parts, fmt.Sprintf(`%s="%s"`, k, val))
		}
		if len(parts) == 0 {
			return ""
		}
		return template.HTMLAttr(" " + strings.Join(parts, " "))
	})
	// 辅助函数：搜索关键词高亮，将 text 中与 query 匹配处用 <mark> 包裹（不区分大小写、HTML 转义安全）
	engine.AddFunc("highlight", func(text, query string) template.HTML {
		if query == "" {
			return template.HTML(HTML.EscapeString(text))
		}
		lowerText := strings.ToLower(text)
		lowerQuery := strings.ToLower(query)
		var buf strings.Builder
		start := 0
		for {
			idx := strings.Index(lowerText[start:], lowerQuery)
			if idx == -1 {
				break
			}
			pos := start + idx
			buf.WriteString(HTML.EscapeString(text[start:pos]))
			buf.WriteString("<mark>")
			buf.WriteString(HTML.EscapeString(text[pos : pos+len(query)]))
			buf.WriteString("</mark>")
			start = pos + len(query)
		}
		buf.WriteString(HTML.EscapeString(text[start:]))
		return template.HTML(buf.String())
	})
	// 辅助函数：检查字符串是否以指定前缀开头
	engine.AddFunc("hasPrefix", func(s, prefix string) bool {
		return strings.HasPrefix(s, prefix)
	})
	// 辅助函数：检查字符串是否以指定后缀结尾
	engine.AddFunc("hasSuffix", func(s, suffix string) bool {
		return strings.HasSuffix(s, suffix)
	})
	// 辅助函数：检查字符串是否匹配正则表达式
	engine.AddFunc("match", func(s, pattern string) bool {
		matched, err := regexp.MatchString(pattern, s)
		if err != nil {
			return false
		}
		return matched
	})
	// 辅助函数：分割字符串
	engine.AddFunc("split", func(s, sep string) []string {
		return strings.Split(s, sep)
	})
	engine.AddFunc("replace_datetime", func(s string) string {
		s = strings.ReplaceAll(s, "{{year}}", time.Now().Format("2006"))
		return s
	})
	// 前台 URL（用于后台列表中「URL 标识」列加链接）
	engine.AddFunc("GetPostUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetPostUrl(*p)
	})
	engine.AddFunc("GetTagUrl", share.GetTagUrl)
	engine.AddFunc("GetCategoryUrl", share.GetCategoryUrl)
	engine.Reload(true)

	return engine
}

// relativeTimeString 将 Unix 时间戳转为相对时间中文描述
func relativeTimeString(ts int64) string {
	now := time.Now().Unix()
	diff := now - ts
	if diff < 0 {
		diff = -diff
		if diff < 60 {
			return "刚刚"
		}
		return time.Unix(ts, 0).Format(consts.TimeFormat)
	}
	switch {
	case diff < 60:
		return "刚刚"
	case diff < 3600:
		return fmt.Sprintf("%d分钟前", diff/60)
	case diff < 86400:
		return fmt.Sprintf("%d小时前", diff/3600)
	case diff < 30*86400:
		return fmt.Sprintf("%d天前", diff/86400)
	case diff < 365*86400:
		return fmt.Sprintf("%d月前", diff/(30*86400))
	default:
		return fmt.Sprintf("%d年前", diff/(365*86400))
	}
}
