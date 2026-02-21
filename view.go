package main

import (
	"errors"
	"fmt"
	HTML "html"
	"html/template"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"swaves/helper"
	"swaves/internal/consts"
	"swaves/internal/db"
	"swaves/internal/share"
	"swaves/internal/store"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/template/html/v3"
)

func NewViewEngine() (*html.Engine, func(app *fiber.App)) {
	urlForStore := share.NewURLForStore()
	engine := html.New("./web/templates", ".html")
	engine.Reload(true)
	RegisterViewFunc(engine, urlForStore.URLFor)
	initURLResolver := func(app *fiber.App) {
		urlForStore.SetResolver(newURLForResolver(app))
	}
	return engine, initURLResolver
}

func RegisterViewFunc(engine *html.Engine, urlFor func(name string, params map[string]string, query map[string]string) string) {
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
	engine.AddFunc("humanSize", formatHumanSize)
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
		return time.Unix(tsInt64, 0).Format("2006年1月2日 15:04")
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
	engine.AddFunc("commentAvatar", func(email, author string, size int) string {
		return helper.BuildGAvatarURL(email, author, size)
	})
	// share/context.go 中的公共函数注册到模板（接收 *db.Post 的已做 nil 安全包装）
	engine.AddFunc("GetBasePath", share.GetBasePath)
	engine.AddFunc("GetPagePath", share.GetPagePath)
	engine.AddFunc("GetSiteUrl", share.GetSiteUrl)
	engine.AddFunc("GetSiteAuthor", share.GetSiteAuthor)
	engine.AddFunc("GetSiteCopyright", share.GetSiteCopyright)
	engine.AddFunc("GetCategoryPrefix", share.GetCategoryPrefix)
	engine.AddFunc("GetTagPrefix", share.GetTagPrefix)
	engine.AddFunc("GetRSSUrl", share.GetRSSUrl)
	engine.AddFunc("GetAdminUrl", share.GetAdminUrl)
	engine.AddFunc("GetAuthorGravatarUrl", share.GetAuthorGravatarUrl)

	engine.AddFunc("url_is", func(currentRouteName string, routeNames ...string) bool {
		current := strings.TrimSpace(currentRouteName)
		if current == "" || len(routeNames) == 0 {
			return false
		}
		for _, routeName := range routeNames {
			if current == strings.TrimSpace(routeName) {
				return true
			}
		}
		return false
	})
	engine.AddFunc("url_for", func(name string, args ...interface{}) string {
		var params map[string]string
		var query map[string]string
		if len(args) > 0 {
			params = toStringMap(args[0])
		}
		if len(args) > 1 {
			query = toStringMap(args[1])
		}
		return urlFor(name, params, query)
	})
	engine.AddFunc("GetTagUrl", share.GetTagUrl)
	engine.AddFunc("GetCategoryUrl", share.GetCategoryUrl)
	engine.AddFunc("BuildPostURL", share.BuildPostURL)
	engine.AddFunc("GetPostUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetPostUrl(*p)
	})
	engine.AddFunc("GetPageUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetPageUrl(*p)
	})
	engine.AddFunc("GetArticleUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetArticleUrl(*p)
	})
	engine.AddFunc("GetPostAbsUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetPostAbsUrl(*p)
	})
	engine.AddFunc("GetAdminPostUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetAdminPostUrl(*p)
	})
	engine.AddFunc("GetAdminEditPostUrl", func(p *db.Post) string {
		if p == nil {
			return ""
		}
		return share.GetAdminEditPostUrl(*p)
	})
	// GetArticlePublishedDate 返回 map[string]string，键为 Year / Month / Day，便于模板使用
	engine.AddFunc("GetArticlePublishedDate", func(p *db.Post) map[string]string {
		if p == nil {
			return nil
		}
		y, m, d := share.GetArticlePublishedDate(*p)
		return map[string]string{"Year": y, "Month": m, "Day": d}
	})
}

func toStringMap(raw interface{}) map[string]string {
	if raw == nil {
		return nil
	}

	result := map[string]string{}
	switch values := raw.(type) {
	case map[string]string:
		for k, v := range values {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			result[key] = strings.TrimSpace(v)
		}
	case map[string]interface{}:
		for k, v := range values {
			key := strings.TrimSpace(k)
			if key == "" || v == nil {
				continue
			}
			result[key] = strings.TrimSpace(fmt.Sprint(v))
		}
	default:
		return nil
	}

	if len(result) == 0 {
		return nil
	}
	return result
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

func formatHumanSize(raw interface{}) string {
	bytes, ok := normalizeBytes(raw)
	if !ok {
		return "-"
	}
	if bytes < 1024 {
		return strconv.FormatInt(int64(bytes), 10) + " B"
	}

	units := []string{"B", "KB", "MB", "GB", "TB"}
	unitIdx := 0
	for bytes >= 1024 && unitIdx < len(units)-1 {
		bytes /= 1024
		unitIdx++
	}

	sizeText := strconv.FormatFloat(bytes, 'f', 2, 64)
	sizeText = strings.TrimRight(strings.TrimRight(sizeText, "0"), ".")
	return sizeText + " " + units[unitIdx]
}

func normalizeBytes(raw interface{}) (float64, bool) {
	switch v := raw.(type) {
	case int:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case int32:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case int64:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case uint:
		return float64(v), true
	case uint32:
		return float64(v), true
	case uint64:
		return float64(v), true
	case float32:
		if v < 0 {
			return 0, false
		}
		return float64(v), true
	case float64:
		if v < 0 {
			return 0, false
		}
		return v, true
	case *int64:
		if v == nil || *v < 0 {
			return 0, false
		}
		return float64(*v), true
	default:
		return 0, false
	}
}

func newURLForResolver(app *fiber.App) func(name string, params map[string]string, query map[string]string) (string, error) {
	return func(name string, params map[string]string, query map[string]string) (string, error) {
		route := app.GetRoute(strings.TrimSpace(name))
		if strings.TrimSpace(route.Name) == "" {
			return "", fmt.Errorf("route %q not found", name)
		}

		path := route.Path
		consumedKeys := map[string]struct{}{}
		for _, paramName := range route.Params {
			value := strings.TrimSpace(params[paramName])
			if value == "" {
				return "", fmt.Errorf("route %q missing param %q", name, paramName)
			}
			consumedKeys[paramName] = struct{}{}
			path = strings.ReplaceAll(path, ":"+paramName, url.PathEscape(value))
		}

		queryValues := url.Values{}
		for key, value := range params {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			if _, ok := consumedKeys[k]; ok {
				continue
			}
			queryValues.Set(k, value)
		}
		for key, value := range query {
			k := strings.TrimSpace(key)
			if k == "" {
				continue
			}
			queryValues.Set(k, value)
		}
		encodedQuery := queryValues.Encode()
		if encodedQuery != "" {
			path += "?" + encodedQuery
		}
		return path, nil
	}
}
