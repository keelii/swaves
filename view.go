package main

import (
	"encoding/json"
	"errors"
	"fmt"
	HTML "html"
	"io"
	"math"
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
	"swaves/internal/logger"
	"swaves/internal/share"
	"swaves/internal/store"
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

const (
	globalUINamespace    = "ui"
	adminUIMacroTemplate = "/admin/macro/ui"
)

var (
	errFiberViewNil = errors.New("fiber view engine is nil")
)

func renderLucideIconSVG(name, size string) string {
	template, ok := lucideSVGByName[name]
	if !ok {
		return ""
	}
	return fmt.Sprintf(template, HTML.EscapeString(size))
}

var lucideSVGByName = map[string]string{
	"trash-2":         `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-trash2-icon lucide-trash-2" aria-hidden="true"><path d="M10 11v6"/><path d="M14 11v6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>`,
	"chevron-left":    `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-left-icon lucide-chevron-left"><path d="m15 18-6-6 6-6"/></svg>`,
	"chevron-right":   `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-right-icon lucide-chevron-right"><path d="m9 18 6-6-6-6"/></svg>`,
	"chevron-up":      `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-up-icon lucide-chevron-up"><path d="m18 15-6-6-6 6"/></svg>`,
	"chevron-down":    `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-chevron-down-icon lucide-chevron-down"><path d="m6 9 6 6 6-6"/></svg>`,
	"x":               `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-x-icon lucide-x"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`,
	"import":          `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-import-icon lucide-import" aria-hidden="true"><path d="M12 3v12"/><path d="m8 11 4 4 4-4"/><path d="M8 5H4a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h16a2 2 0 0 0 2-2V7a2 2 0 0 0-2-2h-4"/></svg>`,
	"list":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-list-icon lucide-list" aria-hidden="true"><path d="M3 5h.01"/> <path d="M3 12h.01"/> <path d="M3 19h.01"/> <path d="M8 5h13"/> <path d="M8 12h13"/> <path d="M8 19h13"/></svg>`,
	"list-tree":       `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-list-tree-icon lucide-list-tree" aria-hidden="true"><path d="M8 5h13"/> <path d="M13 12h8"/> <path d="M13 19h8"/> <path d="M3 10a2 2 0 0 0 2 2h3"/> <path d="M3 5v12a2 2 0 0 0 2 2h3"/></svg>`,
	"arrow-big-left":  `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-big-left-icon lucide-arrow-big-left" aria-hidden="true"><path d="M13 9a1 1 0 0 1-1-1V5.061a1 1 0 0 0-1.811-.75l-6.835 6.836a1.207 1.207 0 0 0 0 1.707l6.835 6.835a1 1 0 0 0 1.811-.75V16a1 1 0 0 1 1-1h6a1 1 0 0 0 1-1v-4a1 1 0 0 0-1-1z"/></svg>`,
	"arrow-left":      `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-arrow-left-icon lucide-arrow-left" aria-hidden="true"><path d="m12 19-7-7 7-7"/> <path d="M19 12H5"/> </svg>`,
	"square-plus":     `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-square-plus-icon lucide-square-plus"><rect width="18" height="18" x="3" y="3" rx="2"/><path d="M8 12h8"/><path d="M12 8v8"/></svg>`,
	"plus":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-plus-icon lucide-plus" aria-hidden="true"> <path d="M5 12h14"/> <path d="M12 5v14"/> </svg>`,
	"save":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-save-icon lucide-save" aria-hidden="true"> <path d="M15.2 3a2 2 0 0 1 1.4.6l3.8 3.8a2 2 0 0 1 .6 1.4V19a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2z"/> <path d="M17 21v-7a1 1 0 0 0-1-1H8a1 1 0 0 0-1 1v7"/> <path d="M7 3v4a1 1 0 0 0 1 1h7"/> </svg>`,
	"terminal":        `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-terminal-icon lucide-terminal" aria-hidden="true"> <path d="M12 19h8"/> <path d="m4 17 6-6-6-6"/> </svg>`,
	"undo":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-undo-icon lucide-undo" aria-hidden="true"> <path d="M3 7v6h6"/> <path d="M21 17a9 9 0 0 0-9-9 9 9 0 0 0-6 2.3L3 13"/> </svg>`,
	"square-pen":      `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-square-pen-icon lucide-square-pen" aria-hidden="true"> <path d="M12 3H5a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2v-7"/> <path d="M18.375 2.625a1 1 0 0 1 3 3l-9.013 9.014a2 2 0 0 1-.853.505l-2.873.84a.5.5 0 0 1-.62-.62l.84-2.873a2 2 0 0 1 .506-.852z"/> </svg>`,
	"heart":           `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-heart-icon lucide-heart"><path d="M2 9.5a5.5 5.5 0 0 1 9.591-3.676.56.56 0 0 0 .818 0A5.49 5.49 0 0 1 22 9.5c0 2.29-1.5 4-3 5.5l-5.492 5.313a2 2 0 0 1-3 .019L5 15c-1.5-1.5-3-3.2-3-5.5"/></svg>`,
	"book-open-check": `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-book-open-check-icon lucide-book-open-check"><path d="M12 21V7"/><path d="m16 12 2 2 4-4"/><path d="M22 6V4a1 1 0 0 0-1-1h-5a4 4 0 0 0-4 4 4 4 0 0 0-4-4H3a1 1 0 0 0-1 1v13a1 1 0 0 0 1 1h6a3 3 0 0 1 3 3 3 3 0 0 1 3-3h6a1 1 0 0 0 1-1v-1.3"/></svg>`,
	"link-2":          `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-link2-icon lucide-link-2"><path d="M9 17H7A5 5 0 0 1 7 7h2"/><path d="M15 7h2a5 5 0 1 1 0 10h-2"/><line x1="8" x2="16" y1="12" y2="12"/></svg>`,
	"link":            `<svg xmlns="http://www.w3.org/2000/svg" width="%[1]s" height="%[1]s" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class="lucide lucide-link-icon lucide-link"><path d="M10 13a5 5 0 0 0 7.54.54l3-3a5 5 0 0 0-7.07-7.07l-1.72 1.71"/><path d="M14 11a5 5 0 0 0-7.54-.54l-3 3a5 5 0 0 0 7.07 7.07l1.71-1.71"/></svg>`,
}

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
	macroTemplateName, err := normalizeTemplateName(adminUIMacroTemplate)
	if err != nil {
		return err
	}
	macroTemplate, err := v.env.GetTemplate(macroTemplateName)
	if err != nil {
		var templateErr *minijinja.Error
		if !errors.As(err, &templateErr) || templateErr.Kind != minijinja.ErrTemplateNotFound {
			return fmt.Errorf("load macro template %q failed: %w", macroTemplateName, err)
		}
	} else {
		state, err := macroTemplate.EvalToState(nil)
		if err != nil {
			return fmt.Errorf("evaluate macro template %q failed: %w", macroTemplateName, err)
		}
		exports := state.Exports()
		if len(exports) > 0 {
			v.env.AddGlobal(globalUINamespace, value.FromMap(exports))
		}
	}
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
	acquired := templatecore.AcquireViewContext(binding)

	if v.clearOnRender {
		v.env.ClearTemplates()
	}
	macroTemplateName, err := normalizeTemplateName(adminUIMacroTemplate)
	if err != nil {
		return err
	}
	macroTemplate, err := v.env.GetTemplate(macroTemplateName)
	if err != nil {
		var templateErr *minijinja.Error
		if !errors.As(err, &templateErr) || templateErr.Kind != minijinja.ErrTemplateNotFound {
			return fmt.Errorf("load macro template %q failed: %w", macroTemplateName, err)
		}
	} else {
		state, err := macroTemplate.EvalToState(nil)
		if err != nil {
			return fmt.Errorf("evaluate macro template %q failed: %w", macroTemplateName, err)
		}
		exports := state.Exports()
		if len(exports) > 0 {
			v.env.AddGlobal(globalUINamespace, value.FromMap(exports))
		}
	}

	templateName, err := normalizeTemplateName(name)
	if err != nil {
		return err
	}
	_ = layout

	tmpl, err := v.env.GetTemplate(templateName)
	if err != nil {
		return fmt.Errorf("load template %q failed: %w", templateName, err)
	}
	context := make(map[string]value.Value, len(acquired))
	for key, raw := range acquired {
		context[key] = value.FromAny(wrapMapLookup(raw))
	}
	return tmpl.RenderToWrite(context, out)
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
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		return name
	}

	if strings.HasPrefix(name, "/") {
		cleaned := path.Clean(name)
		return appendTemplateHTMLExtension(strings.TrimPrefix(cleaned, "/"))
	}

	if strings.HasPrefix(name, "./") || strings.HasPrefix(name, "../") || name == "." || name == ".." {
		parentDir := path.Dir(strings.TrimSpace(parent))
		cleaned := path.Clean(name)
		return appendTemplateHTMLExtension(path.Clean(path.Join(parentDir, cleaned)))
	}

	cleaned := path.Clean(name)
	if cleaned == "." {
		return ""
	}

	if strings.Contains(cleaned, "/") {
		return appendTemplateHTMLExtension(cleaned)
	}

	parentDir := path.Dir(strings.TrimSpace(parent))
	return appendTemplateHTMLExtension(path.Clean(path.Join(parentDir, cleaned)))
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
	normalized = appendTemplateHTMLExtension(normalized)
	return normalized, nil
}

func appendTemplateHTMLExtension(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if path.Ext(name) != "" {
		return name
	}
	return name + ".html"
}

type templateMapLookup struct {
	data any
}

func (m *templateMapLookup) GetAttr(name string) value.Value {
	item, ok := safeLookup(m.data, name)
	if !ok {
		return value.Undefined()
	}
	return value.FromAny(wrapMapLookup(item))
}

func (m *templateMapLookup) GetItem(key value.Value) value.Value {
	item, ok := safeLookup(m.data, key.Raw())
	if !ok {
		return value.Undefined()
	}
	return value.FromAny(wrapMapLookup(item))
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

func registerViewFunc(env *minijinja.Environment, urlFor func(name string, params map[string]string, query map[string]string) string) {
	env.AddFunction("lucide_icon", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
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

	registerTemplateFunction(env, consts.GlobalSettingKey, func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		key := strings.TrimSpace(toStringValue(args[0].Raw()))
		return value.FromString(store.GetSetting(key)), nil
	})

	registerTemplateFunction(env, "printf", func(args []value.Value) (value.Value, error) {
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

	registerTemplateFunction(env, "long_text", func(args []value.Value) (value.Value, error) {
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

	registerValueFormatter(env, "humanSize", formatHumanSize)
	registerTemplateFilter(env, "formatTime", func(val value.Value, _ []value.Value) (value.Value, error) {
		ts := int64(0)
		if parsed, ok := val.AsInt(); ok {
			ts = parsed
		} else {
			raw := val.Raw()
			if parsed, ok := decodeAnyToType[int64](raw); ok {
				ts = parsed
			} else {
				parsed, err := strconv.ParseInt(strings.TrimSpace(toStringValue(raw)), 10, 64)
				if err == nil {
					ts = parsed
				}
			}
		}
		if ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(time.Unix(ts, 0).Format(consts.TimeFormat)), nil
	})
	registerTemplateFilter(env, "relativeTime", func(val value.Value, _ []value.Value) (value.Value, error) {
		ts := int64(0)
		if parsed, ok := val.AsInt(); ok {
			ts = parsed
		} else {
			raw := val.Raw()
			if parsed, ok := decodeAnyToType[int64](raw); ok {
				ts = parsed
			} else {
				parsed, err := strconv.ParseInt(strings.TrimSpace(toStringValue(raw)), 10, 64)
				if err == nil {
					ts = parsed
				}
			}
		}
		if ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(relativeTimeString(ts)), nil
	})
	registerTemplateFilter(env, "articleTime", func(val value.Value, _ []value.Value) (value.Value, error) {
		ts := int64(0)
		if parsed, ok := val.AsInt(); ok {
			ts = parsed
		} else {
			raw := val.Raw()
			if parsed, ok := decodeAnyToType[int64](raw); ok {
				ts = parsed
			} else {
				parsed, err := strconv.ParseInt(strings.TrimSpace(toStringValue(raw)), 10, 64)
				if err == nil {
					ts = parsed
				}
			}
		}
		if ts == 0 {
			return value.FromString("-"), nil
		}
		return value.FromString(time.Unix(ts, 0).Format("2006年1月2日 15:04")), nil
	})
	registerTemplateFilter(env, "formatDateTimeLocal", func(val value.Value, _ []value.Value) (value.Value, error) {
		ts := int64(0)
		if parsed, ok := val.AsInt(); ok {
			ts = parsed
		} else {
			raw := val.Raw()
			if parsed, ok := decodeAnyToType[int64](raw); ok {
				ts = parsed
			} else {
				parsed, err := strconv.ParseInt(strings.TrimSpace(toStringValue(raw)), 10, 64)
				if err == nil {
					ts = parsed
				}
			}
		}
		if ts == 0 {
			return value.FromString(""), nil
		}
		return value.FromString(time.Unix(ts, 0).Format("2006-01-02T15:04")), nil
	})

	registerTemplateFunction(env, "renderAttrs", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSafeString(""), nil
		}
		return value.FromSafeString(renderHTMLAttrs(args[0].Raw())), nil
	})

	registerTemplateFunction(env, "highlight", func(args []value.Value) (value.Value, error) {
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

	registerTemplateFilter(env, "replace_datetime", func(val value.Value, _ []value.Value) (value.Value, error) {
		text := toStringValue(val.Raw())
		return value.FromString(strings.ReplaceAll(text, "{{year}}", time.Now().Format("2006"))), nil
	})

	registerTemplateFunction(env, "commentAvatar", func(args []value.Value) (value.Value, error) {
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
			} else if parsed, ok := decodeAnyToType[int](args[2].Raw()); ok {
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

	registerNoArgStringFunction(env, "GetBasePath", share.GetBasePath)
	registerNoArgStringFunction(env, "GetCategoryPrefix", share.GetCategoryPrefix)
	registerNoArgStringFunction(env, "GetTagPrefix", share.GetTagPrefix)
	registerNoArgStringFunction(env, "GetRSSUrl", share.GetRSSUrl)
	registerNoArgStringFunction(env, "GetAdminUrl", share.GetAdminUrl)
	registerTemplateFunction(env, "GetAuthorGravatarUrl", func(args []value.Value) (value.Value, error) {
		size := 0
		if len(args) > 0 {
			if parsed, ok := args[0].AsInt(); ok {
				size = int(parsed)
			} else if parsed, ok := decodeAnyToType[int](args[0].Raw()); ok {
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

	registerTemplateFunction(env, "url_is", func(args []value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.FromBool(false), nil
		}
		current := strings.TrimSpace(toStringValue(args[0].Raw()))
		if current == "" {
			return value.FromBool(false), nil
		}
		for _, candidate := range args[1:] {
			if current == strings.TrimSpace(toStringValue(candidate.Raw())) {
				return value.FromBool(true), nil
			}
		}
		return value.FromBool(false), nil
	})

	env.AddFunction("url_for", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
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
		return value.FromString(urlFor(name, params, query)), nil
	})

	registerTemplateFunction(env, "GetTagUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		slug := strings.TrimSpace(toStringValue(args[0].Raw()))
		return value.FromString(share.GetTagUrl(db.Tag{Slug: slug})), nil
	})
	registerTemplateFunction(env, "GetCategoryUrl", func(args []value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		slug := strings.TrimSpace(toStringValue(args[0].Raw()))
		return value.FromString(share.GetCategoryUrl(db.Category{Slug: slug})), nil
	})
	registerTemplateFunction(env, "GetPostUrl", func(args []value.Value) (value.Value, error) {
		post, ok := buildPostFromArgs(args)
		if !ok {
			return value.FromString(""), nil
		}
		return value.FromString(share.GetPostUrl(post)), nil
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
		return value.FromString(formatter(val.Raw())), nil
	})
}

func decodeAnyToType[T any](raw any) (T, bool) {
	var zero T
	raw = unwrapTemplateValue(raw)
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
		logger.Error("failed to marshal template argument: %v", err)
		return zero, false
	}
	var decoded T
	if err := json.Unmarshal(payload, &decoded); err != nil {
		logger.Error("failed to unmarshal template argument: %v", err)
		return zero, false
	}
	return decoded, true
}

func buildPostFromArgs(args []value.Value) (db.Post, bool) {
	if len(args) == 0 {
		return db.Post{}, false
	}
	if len(args) == 1 {
		return buildPostFromRaw(args[0].Raw()), true
	}
	if len(args) < 4 {
		return db.Post{}, false
	}

	id, ok := readInt64TemplateArg(args[0])
	if !ok {
		return db.Post{}, false
	}
	kind, ok := readInt64TemplateArg(args[1])
	if !ok {
		return db.Post{}, false
	}
	slug := strings.TrimSpace(toStringValue(args[2].Raw()))
	if slug == "" {
		return db.Post{}, false
	}
	publishedAt, ok := readInt64TemplateArg(args[3])
	if !ok {
		return db.Post{}, false
	}

	return db.Post{
		ID:          id,
		Kind:        db.PostKind(kind),
		Slug:        slug,
		PublishedAt: publishedAt,
	}, true
}

func buildPostFromRaw(raw any) db.Post {
	switch typed := raw.(type) {
	case db.Post:
		return typed
	case *db.Post:
		if typed != nil {
			return *typed
		}
		return db.Post{}
	}

	raw = unwrapTemplateValue(raw)
	post := db.Post{}
	if id, ok := readInt64Field(raw, "ID", "Id", "id"); ok {
		post.ID = id
	}
	if kind, ok := readInt64Field(raw, "Kind", "kind"); ok {
		post.Kind = db.PostKind(kind)
	}
	if slug, ok := readStringField(raw, "Slug", "slug"); ok {
		post.Slug = strings.TrimSpace(slug)
	}
	if title, ok := readStringField(raw, "Title", "title"); ok {
		post.Title = strings.TrimSpace(title)
	}
	if publishedAt, ok := readInt64Field(raw, "PublishedAt", "publishedAt", "published_at"); ok {
		post.PublishedAt = publishedAt
	}
	return post
}

func readInt64TemplateArg(v value.Value) (int64, bool) {
	if parsed, ok := v.AsInt(); ok {
		return parsed, true
	}
	return toInt64Value(v.Raw())
}

func readTemplateField(raw any, keys ...string) (any, bool) {
	for _, key := range keys {
		trimmed := strings.TrimSpace(key)
		if trimmed == "" {
			continue
		}
		if item, ok := safeLookup(raw, trimmed); ok {
			return item, true
		}
	}
	return nil, false
}

func readInt64Field(raw any, keys ...string) (int64, bool) {
	item, ok := readTemplateField(raw, keys...)
	if !ok {
		return 0, false
	}
	return toInt64Value(item)
}

func readStringField(raw any, keys ...string) (string, bool) {
	item, ok := readTemplateField(raw, keys...)
	if !ok {
		return "", false
	}
	item = unwrapTemplateValue(item)
	if item == nil {
		return "", false
	}
	return toStringValue(item), true
}

func toInt64Value(raw any) (int64, bool) {
	raw = unwrapTemplateValue(raw)
	switch typed := raw.(type) {
	case int:
		return int64(typed), true
	case int8:
		return int64(typed), true
	case int16:
		return int64(typed), true
	case int32:
		return int64(typed), true
	case int64:
		return typed, true
	case uint:
		if uint64(typed) > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case uint8:
		return int64(typed), true
	case uint16:
		return int64(typed), true
	case uint32:
		return int64(typed), true
	case uint64:
		if typed > uint64(math.MaxInt64) {
			return 0, false
		}
		return int64(typed), true
	case float32:
		value64 := float64(typed)
		valueInt := int64(value64)
		if value64 != float64(valueInt) {
			return 0, false
		}
		return valueInt, true
	case float64:
		valueInt := int64(typed)
		if typed != float64(valueInt) {
			return 0, false
		}
		return valueInt, true
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed, true
		}
		parsedFloat, err := typed.Float64()
		if err != nil {
			return 0, false
		}
		parsedInt := int64(parsedFloat)
		if parsedFloat != float64(parsedInt) {
			return 0, false
		}
		return parsedInt, true
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	default:
		return 0, false
	}
}

func unwrapTemplateValue(raw any) any {
	switch typed := raw.(type) {
	case value.Value:
		if typed.IsNone() || typed.IsUndefined() || typed.IsSilentUndefined() {
			return nil
		}
		return unwrapTemplateValue(typed.Raw())
	case map[string]value.Value:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = unwrapTemplateValue(item)
		}
		return converted
	case map[string]any:
		converted := make(map[string]any, len(typed))
		for key, item := range typed {
			converted[key] = unwrapTemplateValue(item)
		}
		return converted
	case []value.Value:
		converted := make([]any, len(typed))
		for idx, item := range typed {
			converted[idx] = unwrapTemplateValue(item)
		}
		return converted
	case []any:
		converted := make([]any, len(typed))
		for idx, item := range typed {
			converted[idx] = unwrapTemplateValue(item)
		}
		return converted
	default:
		return raw
	}
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

func safeLookup(container any, key any) (any, bool) {
	if container == nil {
		return nil, false
	}
	parseIndex := func(raw any) (int, bool) {
		switch typed := raw.(type) {
		case int:
			return typed, true
		case int8:
			return int(typed), true
		case int16:
			return int(typed), true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case uint:
			return int(typed), true
		case uint8:
			return int(typed), true
		case uint16:
			return int(typed), true
		case uint32:
			return int(typed), true
		case uint64:
			return int(typed), true
		case string:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed))
			if err != nil {
				return 0, false
			}
			return parsed, true
		case json.Number:
			parsed, err := strconv.Atoi(strings.TrimSpace(typed.String()))
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
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
		idx, ok := parseIndex(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return values[idx], true
	case []string:
		idx, ok := parseIndex(key)
		if !ok || idx < 0 || idx >= len(values) {
			return nil, false
		}
		return values[idx], true
	case string:
		idx, ok := parseIndex(key)
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
		idx, ok := parseIndex(key)
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
	case map[string]value.Value:
		for k, v := range values {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			rawValue := v.Raw()
			if rawValue == nil {
				continue
			}
			result[key] = strings.TrimSpace(fmt.Sprint(rawValue))
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
			result[trimmed] = item.Raw()
		}
	default:
		return nil
	}

	if len(result) == 0 {
		return nil
	}
	return result
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
