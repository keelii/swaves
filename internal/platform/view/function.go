package view

import (
	"errors"
	"fmt"
	HTML "html"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/shared/helper"
	"swaves/internal/shared/share"

	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

func registerViewFunctions(env *minijinja.Environment, urlFor func(name string, params map[string]string, query map[string]string) string) {
	env.AddFunction("LucideIcon", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		name := ""
		size := "16"
		if len(args) > 0 {
			name = strings.TrimSpace(toStringValue(args[0].Raw()))
		}
		if len(args) > 1 {
			size = strings.TrimSpace(toStringValue(args[1].Raw()))
		}
		if rawName, ok := kwargs["name"]; ok && name == "" {
			name = strings.TrimSpace(toStringValue(rawName.Raw()))
		}
		if rawSize, ok := kwargs["size"]; ok && len(args) <= 1 {
			size = strings.TrimSpace(toStringValue(rawSize.Raw()))
		}
		if size == "" {
			size = "16"
		}
		return value.FromSafeString(renderLucideIconSVG(name, size)), nil
	})
	env.AddFunction("UrlFor", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		name := toStringValue(args[0].Raw())
		var params map[string]string
		var query map[string]string
		if len(args) > 1 {
			params = toStringMap(args[1].Raw())
		}
		if len(args) > 2 {
			query = toStringMap(args[2].Raw())
		}
		if len(kwargs) > 0 {
			if params == nil {
				params = map[string]string{}
			}
			for key, raw := range kwargs {
				k := strings.TrimSpace(key)
				if k == "" {
					continue
				}
				params[k] = toStringValue(raw.Raw())
			}
		}
		params = compactStringMap(params)
		query = compactStringMap(query)
		return value.FromString(urlFor(name, params, query)), nil
	})
	env.AddFunction("PagerURL", func(st *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("PagerURL does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.Undefined(), errors.New("PagerURL requires page argument")
		}
		pageRaw := strings.TrimSpace(toStringValue(args[0].Raw()))
		page, err := strconv.Atoi(pageRaw)
		if err != nil || page <= 0 {
			return value.FromString(""), nil
		}

		routeName := ""
		if len(args) > 1 {
			routeName = strings.TrimSpace(toStringValue(args[1].Raw()))
		}
		if routeName == "" {
			routeName = strings.TrimSpace(toStringValue(st.Lookup("RouteName").Raw()))
		}
		if routeName == "" {
			return value.FromString(""), nil
		}

		var query map[string]string
		if len(args) > 2 {
			query = toStringMap(args[2].Raw())
		}
		if query == nil {
			query = toStringMap(st.Lookup("Query").Raw())
		}
		query = compactStringMap(query)
		if query == nil {
			query = map[string]string{}
		}
		query["page"] = strconv.Itoa(page)
		query = compactStringMap(query)
		return value.FromString(urlFor(routeName, nil, query)), nil
	})
	env.AddFunction("Settings", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("Settings does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		key := strings.TrimSpace(toStringValue(args[0].Raw()))
		return value.FromString(store.GetSetting(key)), nil
	})
	env.AddFunction("Printf", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("Printf does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		format := toStringValue(args[0].Raw())
		values := make([]any, 0, len(args)-1)
		for _, item := range args[1:] {
			values = append(values, item.Raw())
		}
		return value.FromString(fmt.Sprintf(format, values...)), nil
	})
	env.AddFunction("LongText", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("LongText does not support keyword arguments")
		}
		text := ""
		if len(args) > 0 {
			text = toStringValue(args[0].Raw())
		}
		cols := 30
		if len(args) > 1 {
			if parsed, ok := args[1].AsInt(); ok {
				cols = int(parsed)
			} else {
				parsed, err := strconv.Atoi(strings.TrimSpace(toStringValue(args[1].Raw())))
				if err == nil {
					cols = parsed
				}
			}
		}
		if cols <= 0 {
			cols = 30
		}
		rows := 1
		if len(args) > 2 {
			if parsed, ok := args[2].AsInt(); ok {
				rows = int(parsed)
			} else {
				parsed, err := strconv.Atoi(strings.TrimSpace(toStringValue(args[2].Raw())))
				if err == nil {
					rows = parsed
				}
			}
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
	env.AddFunction("RenderAttrs", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("RenderAttrs does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromSafeString(""), nil
		}
		return value.FromSafeString(renderHTMLAttrs(args[0].Raw())), nil
	})
	env.AddFunction("HtmlAttrs", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(args) > 1 {
			return value.Undefined(), errors.New("HtmlAttrs expects 0 or 1 positional argument")
		}
		if len(args) == 1 && len(kwargs) > 0 {
			return value.Undefined(), errors.New("HtmlAttrs does not support using positional args and keyword args together")
		}
		if len(args) == 0 && len(kwargs) == 0 {
			return value.FromSafeString(""), nil
		}
		if len(kwargs) > 0 {
			attrs := make(map[string]any, len(kwargs))
			for key, raw := range kwargs {
				k := strings.TrimSpace(key)
				if k == "" {
					continue
				}
				attrs[k] = raw.Raw()
			}
			return value.FromSafeString(renderHTMLAttrs(attrs)), nil
		}
		return value.FromSafeString(renderHTMLAttrs(args[0].Raw())), nil
	})
	env.AddFunction("Highlight", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("Highlight does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromSafeString(""), nil
		}
		text := toStringValue(args[0].Raw())
		query := ""
		if len(args) > 1 {
			query = toStringValue(args[1].Raw())
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
	env.AddFunction("GetAvatarImage", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetAvatarImage does not support keyword arguments")
		}
		email := ""
		author := ""
		size := 0
		if len(args) > 0 {
			email = toStringValue(args[0].Raw())
		}
		if len(args) > 1 {
			author = toStringValue(args[1].Raw())
		}
		if len(args) > 2 {
			if parsed, ok := args[2].AsInt(); ok {
				size = int(parsed)
			} else if parsed, ok := helper.DecodeAnyToType[int](args[2].Raw()); ok {
				size = parsed
			} else {
				parsed, err := strconv.Atoi(strings.TrimSpace(toStringValue(args[2].Raw())))
				if err == nil {
					size = parsed
				}
			}
		}
		return value.FromString(helper.BuildGAvatarURL(email, author, size)), nil
	})
	env.AddFunction("GetAuthorGravatarUrl", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetAuthorGravatarUrl does not support keyword arguments")
		}
		size := 0
		if len(args) > 0 {
			if parsed, ok := args[0].AsInt(); ok {
				size = int(parsed)
			} else if parsed, ok := helper.DecodeAnyToType[int](args[0].Raw()); ok {
				size = parsed
			} else {
				parsed, err := strconv.Atoi(strings.TrimSpace(toStringValue(args[0].Raw())))
				if err == nil {
					size = parsed
				}
			}
		}
		return value.FromString(share.GetAuthorGravatarUrl(size)), nil
	})
	env.AddFunction("UrlIs", func(st *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("UrlIs does not support keyword arguments")
		}
		if len(args) != 1 {
			return value.Undefined(), errors.New("UrlIs requires exactly one route name argument")
		}
		current := strings.TrimSpace(toStringValue(st.Lookup("RouteName").Raw()))
		if current == "" {
			return value.FromBool(false), nil
		}
		return value.FromBool(current == strings.TrimSpace(toStringValue(args[0].Raw()))), nil
	})
	env.AddFunction("GetBasePath", func(_ *minijinja.State, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetBasePath does not support keyword arguments")
		}
		return value.FromString(share.GetBasePath()), nil
	})
	env.AddFunction("GetCategoryPrefix", func(_ *minijinja.State, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetCategoryPrefix does not support keyword arguments")
		}
		return value.FromString(share.GetCategoryPrefix()), nil
	})
	env.AddFunction("GetTagPrefix", func(_ *minijinja.State, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetTagPrefix does not support keyword arguments")
		}
		return value.FromString(share.GetTagPrefix()), nil
	})
	env.AddFunction("GetRSSUrl", func(_ *minijinja.State, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetRSSUrl does not support keyword arguments")
		}
		return value.FromString(share.GetRSSUrl()), nil
	})
	env.AddFunction("GetDashUrl", func(_ *minijinja.State, _ []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetDashUrl does not support keyword arguments")
		}
		return value.FromString(share.GetDashUrl()), nil
	})
	env.AddFunction("GetTagUrl", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetTagUrl does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		slug := strings.TrimSpace(toStringValue(args[0].Raw()))
		return value.FromString(share.GetTagUrl(db.Tag{Slug: slug})), nil
	})
	env.AddFunction("GetCategoryUrl", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetCategoryUrl does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		slug := strings.TrimSpace(toStringValue(args[0].Raw()))
		return value.FromString(share.GetCategoryUrl(db.Category{Slug: slug})), nil
	})
	env.AddFunction("GetPostUrl", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(kwargs) > 0 {
			return value.Undefined(), errors.New("GetPostUrl does not support keyword arguments")
		}
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		post, ok := helper.DecodeAnyToType[db.Post](args[0].Raw())
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetPostUrl(post)), nil
	})
}

func compactStringMap(items map[string]string) map[string]string {
	if len(items) == 0 {
		return nil
	}

	compact := make(map[string]string, len(items))
	for key, value := range items {
		trimmedKey := strings.TrimSpace(key)
		trimmedValue := strings.TrimSpace(value)
		if trimmedKey == "" || trimmedValue == "" {
			continue
		}
		compact[trimmedKey] = trimmedValue
	}
	if len(compact) == 0 {
		return nil
	}
	return compact
}
