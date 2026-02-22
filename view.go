package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	HTML "html"
	"io"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"swaves/helper"
	"swaves/internal/consts"
	"swaves/internal/db"
	"swaves/internal/share"
	"swaves/internal/store"
	itypes "swaves/internal/types"
	"time"

	"github.com/gofiber/fiber/v3"
	templatecore "github.com/gofiber/template/v2"
	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

type FiberView struct {
	env           *minijinja.Environment
	templateRoot  string
	clearOnRender bool
}

type templateReqMeta struct {
	RouteName string
	Path      string
	Query     map[string]string
	ReqID     string
}

type templateAuthMeta struct {
	IsLogin bool
}

type templateSiteMeta struct {
	Settings map[string]string
}

const internalRootContextKey = "__root"

var (
	errFiberViewNil         = errors.New("fiber view engine is nil")
	reservedBindingKeyNames = []string{"Req", "Auth", "Site", internalRootContextKey}
)

func NewViewEngine(dir string, reload bool) (
	fiber.Views,
	func(app *fiber.App),
	func(name string, params map[string]string, query map[string]string) string,
) {
	urlForStore := share.NewURLForStore()
	view := newMiniJinjaView(dir, reload)
	registerViewFunc(view.env, urlForStore.URLFor)
	initURLResolver := func(app *fiber.App) {
		urlForStore.SetResolver(newURLForResolver(app))
	}
	return view, initURLResolver, urlForStore.URLFor
}

func newMiniJinjaView(templateRoot string, clearOnRender bool) *FiberView {
	env := minijinja.NewEnvironment()
	env.SetUndefinedBehavior(minijinja.UndefinedLenient)
	env.SetDebug(clearOnRender)
	env.SetLoader(newMiniJinjaTemplateLoader(templateRoot))
	env.SetPathJoinCallback(resolveTemplateImportPath)
	return &FiberView{
		env:           env,
		templateRoot:  templateRoot,
		clearOnRender: clearOnRender,
	}
}

func (v *FiberView) Load() error {
	if v == nil || v.env == nil {
		return errFiberViewNil
	}

	templateNames, err := collectTemplateNames(v.templateRoot)
	if err != nil {
		return err
	}

	v.env.ClearTemplates()
	for _, name := range templateNames {
		if _, err := v.env.GetTemplate(name); err != nil {
			return fmt.Errorf("load template %q failed: %w", name, err)
		}
	}
	return nil
}

func (v *FiberView) Render(out io.Writer, name string, binding any, layout ...string) error {
	if v == nil || v.env == nil {
		return errFiberViewNil
	}
	prepared, err := prepareTemplateBinding(binding)
	if err != nil {
		return err
	}
	miniBinding := buildMiniBinding(prepared)

	if v.clearOnRender {
		v.env.ClearTemplates()
	}

	templateName, err := normalizeTemplateName(name)
	if err != nil {
		return err
	}

	if len(layout) > 0 {
		layoutName := strings.TrimSpace(layout[0])
		if layoutName != "" {
			return v.renderWithLayout(out, templateName, layoutName, miniBinding)
		}
	}

	tmpl, err := v.env.GetTemplate(templateName)
	if err != nil {
		return fmt.Errorf("load template %q failed: %w", templateName, err)
	}
	return tmpl.RenderToWrite(miniBinding, out)
}

func (v *FiberView) renderWithLayout(out io.Writer, pageName string, layoutName string, binding map[string]value.Value) error {
	pageTemplate, err := v.env.GetTemplate(pageName)
	if err != nil {
		return fmt.Errorf("load template %q failed: %w", pageName, err)
	}

	var pageBody bytes.Buffer
	if err := pageTemplate.RenderToWrite(binding, &pageBody); err != nil {
		return fmt.Errorf("render template %q failed: %w", pageName, err)
	}

	layoutTemplateName, err := normalizeTemplateName(layoutName)
	if err != nil {
		return err
	}

	layoutBinding := cloneValueMap(binding)
	layoutBinding["embed"] = value.FromSafeString(pageBody.String())

	layoutTemplate, err := v.env.GetTemplate(layoutTemplateName)
	if err != nil {
		return fmt.Errorf("load layout %q failed: %w", layoutTemplateName, err)
	}
	return layoutTemplate.RenderToWrite(layoutBinding, out)
}

func newMiniJinjaTemplateLoader(templateRoot string) minijinja.LoaderFunc {
	return func(name string) (string, error) {
		normalizedName, err := normalizeTemplateName(name)
		if err != nil {
			return "", err
		}

		rootPath, err := filepath.Abs(templateRoot)
		if err != nil {
			return "", err
		}
		templatePath := filepath.Join(rootPath, filepath.FromSlash(normalizedName))
		cleanedPath := filepath.Clean(templatePath)
		if !strings.HasPrefix(cleanedPath, rootPath+string(filepath.Separator)) && cleanedPath != rootPath {
			return "", fmt.Errorf("template path escapes root: %s", name)
		}

		content, err := os.ReadFile(cleanedPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", minijinja.NewError(minijinja.ErrTemplateNotFound, normalizedName)
			}
			return "", err
		}
		return string(content), nil
	}
}

func resolveTemplateImportPath(name string, parent string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return name
	}
	if strings.HasPrefix(name, "/") {
		return strings.TrimPrefix(path.Clean(name), "/")
	}
	parentDir := path.Dir(strings.TrimSpace(parent))
	return path.Clean(path.Join(parentDir, name))
}

func collectTemplateNames(templateRoot string) ([]string, error) {
	rootPath, err := filepath.Abs(templateRoot)
	if err != nil {
		return nil, err
	}

	var names []string
	err = filepath.WalkDir(rootPath, func(filePath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !strings.HasSuffix(entry.Name(), ".html") {
			return nil
		}
		relativePath, err := filepath.Rel(rootPath, filePath)
		if err != nil {
			return err
		}
		names = append(names, filepath.ToSlash(relativePath))
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

func normalizeTemplateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("template name is empty")
	}
	normalized := strings.TrimPrefix(path.Clean(strings.ReplaceAll(name, "\\", "/")), "./")
	if normalized == "." || normalized == "" {
		return "", fmt.Errorf("invalid template name %q", name)
	}
	if strings.HasPrefix(normalized, "../") || normalized == ".." {
		return "", fmt.Errorf("template name %q points outside root", name)
	}
	if path.Ext(normalized) == "" {
		normalized += ".html"
	}
	return normalized, nil
}

func cloneAnyMap(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func cloneValueMap(source map[string]value.Value) map[string]value.Value {
	cloned := make(map[string]value.Value, len(source))
	for key, item := range source {
		cloned[key] = item
	}
	return cloned
}

type templateMapLookup struct {
	data any
}

func (m *templateMapLookup) GetAttr(name string) value.Value {
	item, ok := safeLookup(m.data, name)
	if !ok {
		return value.Undefined()
	}
	return anyToMiniValue(item)
}

func (m *templateMapLookup) GetItem(key value.Value) value.Value {
	item, ok := safeLookup(m.data, miniValueToAny(key))
	if !ok {
		return value.Undefined()
	}
	return anyToMiniValue(item)
}

func wrapMapLookup(raw any) any {
	if raw == nil {
		return nil
	}

	target := reflect.ValueOf(raw)
	for target.Kind() == reflect.Interface || target.Kind() == reflect.Ptr {
		if target.IsNil() {
			return raw
		}
		target = target.Elem()
	}

	if target.Kind() != reflect.Map || !target.IsValid() || target.IsNil() {
		return raw
	}
	if target.Type().Key().Kind() == reflect.String {
		return raw
	}
	return &templateMapLookup{data: raw}
}

func buildMiniBinding(input map[string]any) map[string]value.Value {
	converted := make(map[string]value.Value, len(input))
	for key, item := range input {
		converted[key] = anyToMiniValue(item)
	}
	return converted
}

func registerViewFunc(env *minijinja.Environment, urlFor func(name string, params map[string]string, query map[string]string) string) {
	registerTemplateFunction(env, consts.GlobalSettingKey, func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		key := strings.TrimSpace(miniValueAsString(args[0]))
		return value.FromString(store.GetSetting(key)), nil
	})

	registerTemplateFunction(env, "dict", func(args []value.Value) (value.Value, error) {
		if len(args)%2 != 0 {
			return value.Undefined(), errors.New("invalid dict call: must have even number of arguments")
		}
		values := make(map[string]any, len(args)/2)
		for idx := 0; idx < len(args); idx += 2 {
			key, ok := args[idx].AsString()
			if !ok {
				return value.Undefined(), errors.New("dict keys must be strings")
			}
			values[key] = miniValueToAny(args[idx+1])
		}
		return anyToMiniValue(values), nil
	})

	registerTemplateFunction(env, "slice", func(args []value.Value) (value.Value, error) {
		values := make([]any, 0, len(args))
		for _, item := range args {
			values = append(values, miniValueToAny(item))
		}
		return anyToMiniValue(values), nil
	})

	registerTemplateFunction(env, "index", func(args []value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.Undefined(), nil
		}
		return args[0].GetItem(args[1]), nil
	})

	registerTemplateFunction(env, "printf", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		format := miniValueAsString(args[0])
		values := make([]any, 0, len(args)-1)
		for _, item := range args[1:] {
			values = append(values, miniValueToAny(item))
		}
		return value.FromString(fmt.Sprintf(format, values...)), nil
	})

	registerTemplateFunction(env, "long_text", func(args []value.Value) (value.Value, error) {
		text := ""
		if len(args) > 0 {
			text = miniValueAsString(args[0])
		}
		cols := 30
		if len(args) > 1 {
			cols = miniValueAsInt(args[1])
		}
		if cols <= 0 {
			cols = 30
		}
		rows := 1
		if len(args) > 2 {
			rows = miniValueAsInt(args[2])
		}
		if rows <= 0 {
			rows = 1
		}
		rendered := fmt.Sprintf(
			`<textarea class="long-text" cols="%d" rows="%d" readonly>%s</textarea>`,
			cols,
			rows,
			HTML.EscapeString(text),
		)
		return value.FromSafeString(rendered), nil
	})

	registerTemplateFunction(env, "page_items", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return anyToMiniValue([]itypes.PageItem{}), nil
		}
		pager := toPagination(miniValueToAny(args[0]))
		return anyToMiniValue(pager.GetPageItems()), nil
	})
	registerTemplateFunction(env, "page_has_prev", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromBool(false), nil
		}
		return value.FromBool(toPagination(miniValueToAny(args[0])).HasPrev()), nil
	})
	registerTemplateFunction(env, "page_has_next", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromBool(false), nil
		}
		return value.FromBool(toPagination(miniValueToAny(args[0])).HasNext()), nil
	})
	registerTemplateFunction(env, "page_prev", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromInt(1), nil
		}
		return value.FromInt(int64(toPagination(miniValueToAny(args[0])).PrevPage())), nil
	})
	registerTemplateFunction(env, "page_next", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromInt(1), nil
		}
		return value.FromInt(int64(toPagination(miniValueToAny(args[0])).NextPage())), nil
	})

	registerTimestampFormatter(env, "formatTime", "-", func(ts int64) string {
		return time.Unix(ts, 0).Format(consts.TimeFormat)
	})
	registerValueFormatter(env, "humanSize", formatHumanSize)
	registerTimestampFormatter(env, "relativeTime", "-", relativeTimeString)
	registerTimestampFormatter(env, "articleTime", "-", func(ts int64) string {
		return time.Unix(ts, 0).Format("2006年1月2日 15:04")
	})
	registerTimestampFormatter(env, "formatDateTimeLocal", "", func(ts int64) string {
		return time.Unix(ts, 0).Format("2006-01-02T15:04")
	})

	registerTemplateFunction(env, "renderAttrs", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSafeString(""), nil
		}
		return value.FromSafeString(renderHTMLAttrs(miniValueToAny(args[0]))), nil
	})

	registerTemplateFunction(env, "highlight", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSafeString(""), nil
		}
		text := miniValueAsString(args[0])
		query := ""
		if len(args) > 1 {
			query = miniValueAsString(args[1])
		}
		if query == "" {
			return value.FromSafeString(HTML.EscapeString(text)), nil
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
		return value.FromSafeString(buf.String()), nil
	})

	registerTemplateFunction(env, "replace_datetime", func(args []value.Value) (value.Value, error) {
		text := ""
		if len(args) > 0 {
			text = miniValueAsString(args[0])
		}
		return value.FromString(strings.ReplaceAll(text, "{{year}}", time.Now().Format("2006"))), nil
	})

	registerTemplateFunction(env, "commentAvatar", func(args []value.Value) (value.Value, error) {
		email := ""
		author := ""
		size := 0
		if len(args) > 0 {
			email = miniValueAsString(args[0])
		}
		if len(args) > 1 {
			author = miniValueAsString(args[1])
		}
		if len(args) > 2 {
			size = miniValueAsInt(args[2])
		}
		return value.FromString(helper.BuildGAvatarURL(email, author, size)), nil
	})

	registerNoArgStringFunction(env, "GetBasePath", share.GetBasePath)
	registerNoArgStringFunction(env, "GetPagePath", share.GetPagePath)
	registerNoArgStringFunction(env, "GetSiteUrl", share.GetSiteUrl)
	registerNoArgStringFunction(env, "GetSiteAuthor", share.GetSiteAuthor)
	registerNoArgStringFunction(env, "GetSiteCopyright", share.GetSiteCopyright)
	registerNoArgStringFunction(env, "GetCategoryPrefix", share.GetCategoryPrefix)
	registerNoArgStringFunction(env, "GetTagPrefix", share.GetTagPrefix)
	registerNoArgStringFunction(env, "GetRSSUrl", share.GetRSSUrl)
	registerNoArgStringFunction(env, "GetAdminUrl", share.GetAdminUrl)
	registerTemplateFunction(env, "GetAuthorGravatarUrl", func(args []value.Value) (value.Value, error) {
		size := 0
		if len(args) > 0 {
			size = miniValueAsInt(args[0])
		}
		return value.FromString(share.GetAuthorGravatarUrl(size)), nil
	})

	registerTemplateFunction(env, "url_is", func(args []value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.FromBool(false), nil
		}
		current := strings.TrimSpace(miniValueAsString(args[0]))
		if current == "" {
			return value.FromBool(false), nil
		}
		for _, candidate := range args[1:] {
			if current == strings.TrimSpace(miniValueAsString(candidate)) {
				return value.FromBool(true), nil
			}
		}
		return value.FromBool(false), nil
	})

	registerTemplateFunction(env, "url_for", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		name := miniValueAsString(args[0])
		var params map[string]string
		var query map[string]string
		if len(args) > 1 {
			params = toStringMap(miniValueToAny(args[1]))
		}
		if len(args) > 2 {
			query = toStringMap(miniValueToAny(args[2]))
		}
		return value.FromString(urlFor(name, params, query)), nil
	})

	registerTemplateFunction(env, "GetTagUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		tag, ok := decodeTemplateArg[db.Tag](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetTagUrl(tag)), nil
	})
	registerTemplateFunction(env, "GetCategoryUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		category, ok := decodeTemplateArg[db.Category](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetCategoryUrl(category)), nil
	})
	registerTemplateFunction(env, "BuildPostURL", func(args []value.Value) (value.Value, error) {
		if len(args) < 4 {
			return value.FromString(""), nil
		}
		id := miniValueAsInt64(args[0])
		kind := db.PostKind(miniValueAsInt(args[1]))
		slug := miniValueAsString(args[2])
		publishedAt := miniValueAsInt64(args[3])
		return value.FromString(share.BuildPostURL(id, kind, slug, publishedAt)), nil
	})
	registerTemplateFunction(env, "GetPostUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetPostUrl(post)), nil
	})
	registerTemplateFunction(env, "GetPageUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetPageUrl(post)), nil
	})
	registerTemplateFunction(env, "GetArticleUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetArticleUrl(post)), nil
	})
	registerTemplateFunction(env, "GetPostAbsUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetPostAbsUrl(post)), nil
	})
	registerTemplateFunction(env, "GetAdminPostUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetAdminPostUrl(post)), nil
	})
	registerTemplateFunction(env, "GetAdminEditPostUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetAdminEditPostUrl(post)), nil
	})
	registerTemplateFunction(env, "GetArticlePublishedDate", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return anyToMiniValue(map[string]string{}), nil
		}
		post, ok := decodeTemplateArg[db.Post](args[0])
		if !ok {
			return anyToMiniValue(map[string]string{}), nil
		}
		year, month, day := share.GetArticlePublishedDate(post)
		return anyToMiniValue(map[string]string{"Year": year, "Month": month, "Day": day}), nil
	})
}

func registerNoArgStringFunction(env *minijinja.Environment, name string, fn func() string) {
	registerTemplateFunction(env, name, func(_ []value.Value) (value.Value, error) {
		return value.FromString(fn()), nil
	})
}

func registerTemplateFunction(
	env *minijinja.Environment,
	name string,
	fn func(args []value.Value) (value.Value, error),
) {
	env.AddFunction(name, func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), fmt.Errorf("%s does not support keyword arguments", name)
		}
		return fn(args)
	})
}

func registerTemplateFilter(
	env *minijinja.Environment,
	name string,
	fn func(val value.Value, args []value.Value) (value.Value, error),
) {
	env.AddFilter(name, func(_ minijinja.FilterState, val value.Value, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), fmt.Errorf("%s filter does not support keyword arguments", name)
		}
		return fn(val, args)
	})
}

func registerValueFormatter(env *minijinja.Environment, name string, formatter func(raw interface{}) string) {
	registerTemplateFilter(env, name, func(val value.Value, _ []value.Value) (value.Value, error) {
		return value.FromString(formatter(miniValueToAny(val))), nil
	})
}

func registerTimestampFormatter(
	env *minijinja.Environment,
	name string,
	empty string,
	formatter func(ts int64) string,
) {
	registerTemplateFilter(env, name, func(val value.Value, _ []value.Value) (value.Value, error) {
		ts, ok := parseTimestamp(miniValueToAny(val))
		if !ok || ts == 0 {
			return value.FromString(empty), nil
		}
		return value.FromString(formatter(ts)), nil
	})
}

func parseTimestamp(raw any) (int64, bool) {
	switch typed := raw.(type) {
	case *int64:
		if typed == nil {
			return 0, false
		}
		return *typed, true
	case *int:
		if typed == nil {
			return 0, false
		}
		return int64(*typed), true
	case *uint64:
		if typed == nil {
			return 0, false
		}
		return int64(*typed), true
	}
	parsed, ok := toIndexValue(raw)
	if !ok {
		return 0, false
	}
	return int64(parsed), true
}

func miniValueAsString(input value.Value) string {
	if text, ok := input.AsString(); ok {
		return text
	}
	return toStringValue(miniValueToAny(input))
}

func miniValueAsInt64(input value.Value) int64 {
	if integer, ok := input.AsInt(); ok {
		return integer
	}
	if number, ok := toIndexValue(miniValueToAny(input)); ok {
		return int64(number)
	}
	return 0
}

func miniValueAsInt(input value.Value) int {
	return int(miniValueAsInt64(input))
}

func decodeTemplateArg[T any](input value.Value) (T, bool) {
	return decodeAnyToType[T](miniValueToAny(input))
}

func decodeAnyToType[T any](raw any) (T, bool) {
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
	payload, err := json.Marshal(raw)
	if err != nil {
		return zero, false
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return zero, false
	}
	return decoded, true
}

func toPagination(raw any) itypes.Pagination {
	switch typed := raw.(type) {
	case itypes.Pagination:
		return normalizePagination(typed)
	case *itypes.Pagination:
		if typed == nil {
			return normalizePagination(itypes.Pagination{})
		}
		return normalizePagination(*typed)
	case map[string]any:
		return normalizePagination(itypes.Pagination{
			Page:     readIntFromMap(typed, "Page", "page"),
			PageSize: readIntFromMap(typed, "PageSize", "pageSize"),
			Num:      readIntFromMap(typed, "Num", "num"),
			Total:    readIntFromMap(typed, "Total", "total"),
		})
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = item
		}
		return normalizePagination(itypes.Pagination{
			Page:     readIntFromMap(converted, "Page", "page"),
			PageSize: readIntFromMap(converted, "PageSize", "pageSize"),
			Num:      readIntFromMap(converted, "Num", "num"),
			Total:    readIntFromMap(converted, "Total", "total"),
		})
	default:
		decoded, ok := decodeAnyToType[itypes.Pagination](raw)
		if ok {
			return normalizePagination(decoded)
		}
	}
	return normalizePagination(itypes.Pagination{})
}

func readIntFromMap(values map[string]any, keys ...string) int {
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		if parsed, ok := toIndexValue(raw); ok {
			return parsed
		}
	}
	return 0
}

func normalizePagination(pager itypes.Pagination) itypes.Pagination {
	if pager.Page <= 0 {
		pager.Page = 1
	}
	if pager.PageSize <= 0 {
		pager.PageSize = 10
	}
	if pager.Num < 0 {
		pager.Num = 0
	}
	if pager.Total < 0 {
		pager.Total = 0
	}
	if pager.Num > 0 && pager.Page > pager.Num {
		pager.Page = pager.Num
	}
	return pager
}

func renderHTMLAttrs(raw any) string {
	attrs := toStringAnyMap(raw)
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(attrs))
	for _, key := range keys {
		item := attrs[key]
		switch typed := item.(type) {
		case bool:
			if typed {
				parts = append(parts, key)
			}
			continue
		}
		text := HTML.EscapeString(toStringValue(item))
		parts = append(parts, fmt.Sprintf(`%s="%s"`, key, text))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func anyToMiniValue(raw any) value.Value {
	if raw == nil {
		return value.Undefined()
	}
	switch typed := raw.(type) {
	case value.Value:
		return typed
	case map[string]any:
		converted := make(map[string]value.Value, len(typed))
		for key, item := range typed {
			converted[key] = anyToMiniValue(item)
		}
		return value.FromMap(converted)
	case map[string]string:
		converted := make(map[string]value.Value, len(typed))
		for key, item := range typed {
			converted[key] = value.FromString(item)
		}
		return value.FromMap(converted)
	case []any:
		items := make([]value.Value, 0, len(typed))
		for _, item := range typed {
			items = append(items, anyToMiniValue(item))
		}
		return value.FromSlice(items)
	}
	return value.FromAny(wrapMapLookup(raw))
}

func miniValueToAny(input value.Value) any {
	switch input.Kind() {
	case value.KindUndefined, value.KindNone:
		return nil
	case value.KindBool:
		boolean, _ := input.AsBool()
		return boolean
	case value.KindNumber:
		if integer, ok := input.AsInt(); ok {
			return integer
		}
		floatValue, _ := input.AsFloat()
		return floatValue
	case value.KindString:
		text, _ := input.AsString()
		return text
	case value.KindMap:
		items, ok := input.AsMap()
		if !ok {
			return map[string]any{}
		}
		result := make(map[string]any, len(items))
		for key, item := range items {
			result[key] = miniValueToAny(item)
		}
		return result
	case value.KindSeq, value.KindIterable:
		items := input.Iter()
		result := make([]any, 0, len(items))
		for _, item := range items {
			result = append(result, miniValueToAny(item))
		}
		return result
	default:
		if raw := input.Raw(); raw != nil {
			return raw
		}
		if text, ok := input.AsString(); ok {
			return text
		}
		return input.String()
	}
}

func prepareTemplateBinding(binding any) (map[string]any, error) {
	acquired := templatecore.AcquireViewContext(binding)
	prepared := make(map[string]any, len(acquired)+4)
	for key, value := range acquired {
		prepared[key] = value
	}

	if reservedKey := findReservedBindingKey(prepared); reservedKey != "" {
		return nil, fmt.Errorf("template binding key %q is reserved", reservedKey)
	}

	prepared["Req"] = buildTemplateReqMeta(prepared)
	prepared["Auth"] = templateAuthMeta{
		IsLogin: toBoolValue(prepared["IsLogin"]),
	}
	prepared["Site"] = templateSiteMeta{
		Settings: cloneStringMap(store.GetSettingMap()),
	}
	for key, raw := range prepared {
		prepared[key] = wrapMapLookup(raw)
	}
	prepared[internalRootContextKey] = cloneAnyMap(prepared)
	return prepared, nil
}

func findReservedBindingKey(binding map[string]any) string {
	for _, key := range reservedBindingKeyNames {
		if _, exists := binding[key]; exists {
			return key
		}
	}
	return ""
}

func buildTemplateReqMeta(data map[string]any) templateReqMeta {
	path := strings.TrimSpace(toStringValue(data["UrlPath"]))
	if path == "" {
		path = strings.TrimSpace(toStringValue(data["Path"]))
	}

	query := toStringMap(data["Query"])
	if query == nil {
		query = map[string]string{}
	}

	return templateReqMeta{
		RouteName: strings.TrimSpace(toStringValue(data["RouteName"])),
		Path:      path,
		Query:     query,
		ReqID:     strings.TrimSpace(toStringValue(data["ReqID"])),
	}
}

func safeLookup(container any, key any) (any, bool) {
	if container == nil {
		return nil, false
	}

	switch values := container.(type) {
	case map[string]any:
		value, ok := values[fmt.Sprint(key)]
		return value, ok
	case map[string]string:
		value, ok := values[fmt.Sprint(key)]
		if !ok {
			return nil, false
		}
		return value, true
	case []any:
		idx, ok := toIndexValue(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return values[idx], true
	case []string:
		idx, ok := toIndexValue(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return values[idx], true
	case string:
		idx, ok := toIndexValue(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return string(values[idx]), true
	}

	value := reflect.ValueOf(container)
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Ptr {
		if value.IsNil() {
			return nil, false
		}
		value = value.Elem()
	}

	switch value.Kind() {
	case reflect.Map:
		mapKey := reflect.ValueOf(key)
		keyType := value.Type().Key()
		if !mapKey.IsValid() {
			return nil, false
		}
		if mapKey.Type().AssignableTo(keyType) {
			lookup := value.MapIndex(mapKey)
			if !lookup.IsValid() {
				return nil, false
			}
			return lookup.Interface(), true
		}
		if mapKey.Type().ConvertibleTo(keyType) {
			lookup := value.MapIndex(mapKey.Convert(keyType))
			if !lookup.IsValid() {
				return nil, false
			}
			return lookup.Interface(), true
		}
		if keyType.Kind() == reflect.String {
			lookup := value.MapIndex(reflect.ValueOf(fmt.Sprint(key)))
			if !lookup.IsValid() {
				return nil, false
			}
			return lookup.Interface(), true
		}
		return nil, false
	case reflect.Slice, reflect.Array:
		idx, ok := toIndexValue(key)
		if !ok || idx < 0 || idx >= value.Len() {
			return nil, false
		}
		return value.Index(idx).Interface(), true
	case reflect.Struct:
		fieldName := strings.TrimSpace(fmt.Sprint(key))
		if fieldName == "" {
			return nil, false
		}
		field := value.FieldByName(fieldName)
		if !field.IsValid() || !field.CanInterface() {
			return nil, false
		}
		return field.Interface(), true
	default:
		return nil, false
	}
}

func toIndexValue(raw any) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int8:
		return int(value), true
	case int16:
		return int(value), true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case uint:
		return int(value), true
	case uint8:
		return int(value), true
	case uint16:
		return int(value), true
	case uint32:
		return int(value), true
	case uint64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
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

func toStringAnyMap(raw interface{}) map[string]any {
	if raw == nil {
		return nil
	}

	result := map[string]any{}
	switch values := raw.(type) {
	case map[string]any:
		for key, item := range values {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			result[trimmed] = item
		}
	case map[string]string:
		for key, item := range values {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			result[trimmed] = item
		}
	case map[string]value.Value:
		for key, item := range values {
			trimmed := strings.TrimSpace(key)
			if trimmed == "" {
				continue
			}
			result[trimmed] = miniValueToAny(item)
		}
	default:
		return nil
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func toStringValue(raw interface{}) string {
	if raw == nil {
		return ""
	}
	if value, ok := raw.(string); ok {
		return value
	}
	if value, ok := raw.(fmt.Stringer); ok {
		return value.String()
	}
	return fmt.Sprint(raw)
}

func toBoolValue(raw interface{}) bool {
	switch value := raw.(type) {
	case bool:
		return value
	case string:
		switch strings.TrimSpace(strings.ToLower(value)) {
		case "1", "true", "yes", "on":
			return true
		default:
			return false
		}
	case int:
		return value != 0
	case int32:
		return value != 0
	case int64:
		return value != 0
	case uint:
		return value != 0
	case uint32:
		return value != 0
	case uint64:
		return value != 0
	case float32:
		return value != 0
	case float64:
		return value != 0
	default:
		return false
	}
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
