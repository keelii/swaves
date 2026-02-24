package admin_app

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/store"
	"swaves/internal/shared/share"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

// SettingView 用于模板展示的设置视图
type SettingView struct {
	db.Setting
	OptionsParsed  []map[string]string    // 解析后的 options（用于 radio/checkbox）
	CheckboxValues map[string]bool        // checkbox 的选中状态
	AttrsParsed    map[string]interface{} // 解析后的 attrs（用于 HTML 属性）
}

type SettingSubKindGroupView struct {
	Code     string
	Label    string
	Settings []SettingView
}

func normalizeSettingSubKind(raw string) string {
	return strings.TrimSpace(raw)
}

func resolveSettingSubKindLabel(kind string, subKind string) string {
	subKind = normalizeSettingSubKind(subKind)
	if subKind == "" {
		return ""
	}
	if labels, ok := db.SettingSubKindLabels[kind]; ok {
		if label, exists := labels[subKind]; exists && strings.TrimSpace(label) != "" {
			return label
		}
	}
	return subKind
}

func buildSettingSubKindGroups(kind string, settings []SettingView) []SettingSubKindGroupView {
	groups := make([]SettingSubKindGroupView, 0)
	groupIndex := make(map[string]int)

	for _, item := range settings {
		subKind := normalizeSettingSubKind(item.SubKind)
		item.SubKind = subKind

		idx, ok := groupIndex[subKind]
		if !ok {
			groupIndex[subKind] = len(groups)
			groups = append(groups, SettingSubKindGroupView{
				Code:     subKind,
				Label:    resolveSettingSubKindLabel(kind, subKind),
				Settings: []SettingView{},
			})
			idx = len(groups) - 1
		}

		groups[idx].Settings = append(groups[idx].Settings, item)
	}

	return groups
}

// Settings
func (h *Handler) GetSettingsHandler(c fiber.Ctx) error {
	// 获取所有 settings，以表格形式展示
	settings, err := ListAllSettings(h.Model)
	if err != nil {
		return err
	}

	return RenderAdminView(c, "settings_index", fiber.Map{
		"Title":    "Settings",
		"Settings": settings,
	}, "")
}

func (h *Handler) GetSettingsAllHandler(c fiber.Ctx) error {
	kind := strings.TrimSpace(c.Query("kind", ""))
	errMsg := strings.TrimSpace(c.Query("error", ""))

	settings, err := ListAllSettings(h.Model)
	if err != nil {
		return err
	}

	// 按 kind 分组，key=kind，value=settings，保持查询结果顺序
	settingsByKind := make(map[string][]SettingView)
	settingKinds := make([]string, 0)
	for _, s := range settings {
		view := buildSettingView(s)

		if _, ok := settingsByKind[s.Kind]; !ok {
			settingKinds = append(settingKinds, s.Kind)
		}
		settingsByKind[s.Kind] = append(settingsByKind[s.Kind], view)
	}

	activeKind := kind
	if _, ok := settingsByKind[activeKind]; !ok {
		activeKind = ""
		if len(settingKinds) > 0 {
			activeKind = settingKinds[0]
		}
	}

	settingKindLabels := make(map[string]string, len(settingKinds))
	for _, item := range settingKinds {
		label, ok := db.SettingKindLabels[item]
		if !ok || strings.TrimSpace(label) == "" {
			label = item
		}
		settingKindLabels[item] = label
	}

	activeKindGroups := make([]SettingSubKindGroupView, 0)
	if activeKind != "" {
		activeKindGroups = buildSettingSubKindGroups(activeKind, settingsByKind[activeKind])
	}

	return RenderAdminView(c, "settings_all", fiber.Map{
		"Title":              "Settings - Edit All",
		"SettingsByKind":     settingsByKind,
		"SettingKinds":       settingKinds,
		"SettingKindLabels":  settingKindLabels,
		"ActiveKindGroups":   activeKindGroups,
		"ActiveKind":         activeKind,
		"ContentRoutingKind": db.SettingKindContentRouting,
		"Error":              errMsg,
	}, "")
}

func buildSettingView(s db.Setting) SettingView {
	view := SettingView{Setting: s}

	if (s.Type == "select" || s.Type == "radio" || s.Type == "checkbox") && s.Options != "" {
		var options []map[string]string
		err := json.Unmarshal([]byte(s.Options), &options)
		if err == nil {
			view.OptionsParsed = options
		} else {
			logger.Warn("Error parsing options for setting %s: %v", s.Options, err)
		}
	}

	if s.Type == "checkbox" {
		view.CheckboxValues = make(map[string]bool)
		if s.Value != "" {
			values := strings.Split(s.Value, ",")
			for _, v := range values {
				view.CheckboxValues[strings.TrimSpace(v)] = true
			}
		}
	}

	view.AttrsParsed = make(map[string]interface{})
	if s.Attrs != "" {
		var attrs map[string]interface{}
		err := json.Unmarshal([]byte(s.Attrs), &attrs)
		if err == nil {
			view.AttrsParsed = attrs
		} else {
			logger.Warn("Error parsing attrs for setting %s: %v", s.Attrs, err)
		}
	}

	return view
}

type settingsValueUpdate struct {
	Code       string
	Value      string
	SkipUpdate bool
}

func isAssetSettingCode(code string) bool {
	switch strings.TrimSpace(code) {
	case "asset_default_provider", "asset_see_api_token", "asset_imagekit_endpoint", "asset_imagekit_private_key":
		return true
	default:
		return false
	}
}

func (h *Handler) buildSettingsAllRedirectPath(c fiber.Ctx, kind string, errMsg string) string {
	params := map[string]string{}
	if strings.TrimSpace(kind) != "" {
		params["kind"] = strings.TrimSpace(kind)
	}
	if strings.TrimSpace(errMsg) != "" {
		params["error"] = strings.TrimSpace(errMsg)
	}

	redirectPath := h.adminRouteURL(c, "admin.settings.all", nil, params)
	if redirectPath == "" {
		return share.BuildAdminPath("/settings/all")
	}
	return redirectPath
}

func (h *Handler) validateAssetSettingPayload(overrides map[string]string) error {
	if len(overrides) == 0 {
		return nil
	}

	values := map[string]string{
		"asset_default_provider":     strings.TrimSpace(store.GetSetting("asset_default_provider")),
		"asset_see_api_token":        strings.TrimSpace(store.GetSetting("asset_see_api_token")),
		"asset_imagekit_endpoint":    strings.TrimSpace(store.GetSetting("asset_imagekit_endpoint")),
		"asset_imagekit_private_key": strings.TrimSpace(store.GetSetting("asset_imagekit_private_key")),
	}
	hasAssetOverride := false
	for code, value := range overrides {
		if !isAssetSettingCode(code) {
			continue
		}
		values[code] = strings.TrimSpace(value)
		hasAssetOverride = true
	}
	if !hasAssetOverride {
		return nil
	}

	rawProvider := strings.TrimSpace(strings.ToLower(values["asset_default_provider"]))
	provider := normalizeAssetProvider(rawProvider)
	if rawProvider != "" && provider == "" {
		return errors.New("保存失败：资源默认服务无效，仅支持 S.EE 或 ImageKit")
	}
	if provider == "" {
		provider = "see"
	}

	switch provider {
	case "imagekit":
		if values["asset_imagekit_private_key"] == "" {
			return errors.New("保存失败：当前资源默认服务为 ImageKit，请填写 ImageKit Private Key")
		}
		if values["asset_imagekit_endpoint"] == "" {
			return errors.New("保存失败：当前资源默认服务为 ImageKit，请填写 ImageKit-endpoint")
		}
		if err := validateImageKitEndpoint(values["asset_imagekit_endpoint"]); err != nil {
			return errors.New("保存失败：" + err.Error())
		}
	case "see":
		fallthrough
	default:
		if values["asset_see_api_token"] == "" {
			return errors.New("保存失败：当前资源默认服务为 S.EE，请填写 S.EE API Token")
		}
	}

	return nil
}

func (h *Handler) PostUpdateSettingsAllHandler(c fiber.Ctx) error {
	activeKind := strings.TrimSpace(c.Query("kind", ""))
	if activeKind == "" {
		return h.redirectToAdminRoute(c, "admin.settings.all", nil, nil)
	}

	// 只更新当前 tab(kind) 下的配置，避免把其它 tab 未提交字段写空
	settings, err := ListSettingsByKind(h.Model, activeKind)
	if err != nil {
		return err
	}

	updates := make([]settingsValueUpdate, 0, len(settings))
	assetOverrides := make(map[string]string)

	for _, setting := range settings {
		fieldName := "setting_" + setting.Code

		// checkbox 类型需要特殊处理，可能有多个值
		if setting.Type == "checkbox" {
			valuesBytes := c.Request().PostArgs().PeekMulti(fieldName)

			var values []string
			for _, v := range valuesBytes {
				values = append(values, string(v))
			}

			value := strings.Join(values, ",")
			updates = append(updates, settingsValueUpdate{
				Code:  setting.Code,
				Value: value,
			})
			if isAssetSettingCode(setting.Code) && value != setting.Value {
				assetOverrides[setting.Code] = value
			}
		} else {
			// 其他类型直接获取单个值
			value := c.FormValue(fieldName)
			// 对于 password 类型，如果为空则不更新（保持原值）
			if setting.Type == "password" && value == "" {
				updates = append(updates, settingsValueUpdate{
					Code:       setting.Code,
					SkipUpdate: true,
				})
				continue
			}
			updates = append(updates, settingsValueUpdate{
				Code:  setting.Code,
				Value: value,
			})
			if isAssetSettingCode(setting.Code) && value != setting.Value {
				assetOverrides[setting.Code] = value
			}
		}
	}

	if err = h.validateAssetSettingPayload(assetOverrides); err != nil {
		return webutil.RedirectTo(c, h.buildSettingsAllRedirectPath(c, activeKind, err.Error()))
	}

	for _, item := range updates {
		if item.SkipUpdate {
			continue
		}
		if err = UpdateSettingValueService(h.Model, item.Code, item.Value); err != nil {
			return err
		}
	}

	return webutil.RedirectTo(c, h.buildSettingsAllRedirectPath(c, activeKind, ""))
}

func (h *Handler) GetSettingEditHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	setting, err := GetSettingByID(h.Model, id)
	if err != nil {
		return err
	}

	// 转换为视图结构，解析 options 和 attrs
	view := SettingView{Setting: *setting}

	// 解析 options（如果是 radio、checkbox 或 select）
	if (setting.Type == "select" || setting.Type == "radio" || setting.Type == "checkbox") && setting.Options != "" {
		var options []map[string]string
		err := json.Unmarshal([]byte(setting.Options), &options)
		if err == nil {
			view.OptionsParsed = options
		} else {
			logger.Warn("Error parsing options for setting %s: %v", setting.Options, err)
		}
	}

	// 解析 attrs（HTML 属性）
	view.AttrsParsed = make(map[string]interface{}) // 初始化为空 map，避免 nil
	if setting.Attrs != "" {
		var attrs map[string]interface{}
		err := json.Unmarshal([]byte(setting.Attrs), &attrs)
		if err == nil {
			view.AttrsParsed = attrs
		} else {
			logger.Warn("Error parsing attrs for setting %s: %v", setting.Attrs, err)
		}
	}

	return RenderAdminView(c, "settings_edit", fiber.Map{
		"Title":   "Edit Setting",
		"Setting": view,
	}, "")
}

func (h *Handler) PostUpdateSettingHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	// 获取现有设置
	setting, err := GetSettingByID(h.Model, id)
	if err != nil {
		return err
	}
	originalCode := strings.TrimSpace(setting.Code)
	originalValue := setting.Value

	renderEditWithError := func(message string) error {
		view := buildSettingView(*setting)
		return RenderAdminView(c, "settings_edit", fiber.Map{
			"Title":   "Edit Setting",
			"Error":   message,
			"Setting": view,
		}, "")
	}

	// 更新字段
	setting.Kind = c.FormValue("kind")
	setting.SubKind = c.FormValue("sub_kind")
	setting.Name = c.FormValue("name")
	setting.Code = c.FormValue("code")
	setting.Type = c.FormValue("type")
	setting.Options = c.FormValue("options")
	setting.Attrs = c.FormValue("attrs")
	setting.DefaultOptionValue = c.FormValue("default_option_value")
	setting.PrefixValue = c.FormValue("prefix_value")
	setting.Description = c.FormValue("description")

	if sortStr := c.FormValue("sort"); sortStr != "" {
		if sort, err := strconv.ParseInt(sortStr, 10, 64); err == nil {
			setting.Sort = sort
		}
	}
	if c.FormValue("reload") != "" {
		setting.Reload = 1
	} else {
		setting.Reload = 0
	}

	// 处理 value 字段
	value := c.FormValue("value")
	// 对于 password 类型，如果为空则不更新（保持原值）
	if setting.Type == "password" && value == "" {
		// 保持原值，不做更新
	} else {
		setting.Value = value
	}

	assetOverrides := make(map[string]string)
	if isAssetSettingCode(originalCode) && setting.Value != originalValue {
		assetOverrides[originalCode] = setting.Value
	}
	if err = h.validateAssetSettingPayload(assetOverrides); err != nil {
		return renderEditWithError(err.Error())
	}

	if err := UpdateSettingService(h.Model, setting); err != nil {
		return renderEditWithError(err.Error())
	}

	return h.redirectToAdminRoute(c, "admin.settings.list", nil, nil)
}

func (h *Handler) PostDeleteSettingHandler(c fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil {
		return fiber.ErrBadRequest
	}

	if err := DeleteSettingService(h.Model, id); err != nil {
		return err
	}

	return h.redirectToAdminRoute(c, "admin.settings.list", nil, nil)
}

func (h *Handler) GetSettingNewHandler(c fiber.Ctx) error {
	// 如果需要处理预填充的 options（例如从错误返回），可以在这里解析
	var optionsParsed []map[string]string
	var defaultOptionValue string

	// 从查询参数获取 options（用于错误回显）
	if optionsJSON := c.Query("options"); optionsJSON != "" {
		if err := json.Unmarshal([]byte(optionsJSON), &optionsParsed); err == nil {
			defaultOptionValue = c.Query("default_option_value", "")
		}
	}

	return RenderAdminView(c, "settings_new", fiber.Map{
		"Title":              "New Setting",
		"OptionsParsed":      optionsParsed,
		"DefaultOptionValue": defaultOptionValue,
	}, "")
}

func (h *Handler) PostCreateSettingHandler(c fiber.Ctx) error {
	s := &db.Setting{
		Kind:               c.FormValue("kind"),
		SubKind:            c.FormValue("sub_kind"),
		Name:               c.FormValue("name"),
		Code:               c.FormValue("code"),
		Type:               c.FormValue("type"),
		Options:            c.FormValue("options"),
		Attrs:              c.FormValue("attrs"),
		Value:              c.FormValue("value"),
		DefaultOptionValue: c.FormValue("default_option_value"),
		PrefixValue:        c.FormValue("prefix_value"),
		Description:        c.FormValue("description"),
	}

	if sortStr := c.FormValue("sort"); sortStr != "" {
		if sort, err := strconv.ParseInt(sortStr, 10, 64); err == nil {
			s.Sort = sort
		}
	}
	if c.FormValue("reload") != "" {
		s.Reload = 1
	} else {
		s.Reload = 0
	}

	if s.Kind == "" {
		s.Kind = "default"
	}

	if err := CreateSettingService(h.Model, s); err != nil {
		// 解析 options 用于错误回显
		var optionsParsed []map[string]string
		if s.Options != "" {
			json.Unmarshal([]byte(s.Options), &optionsParsed)
		}

		return RenderAdminView(c, "settings_new", fiber.Map{
			"Title":              "New Setting",
			"Error":              err.Error(),
			"Setting":            s,
			"OptionsParsed":      optionsParsed,
			"DefaultOptionValue": s.DefaultOptionValue,
		}, "")
	}

	return h.redirectToAdminRoute(c, "admin.settings.list", nil, nil)
}
