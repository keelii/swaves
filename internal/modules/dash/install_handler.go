package dash

import (
	"net/url"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/shared/pathutil"

	"github.com/gofiber/fiber/v3"
)

type installSettingPresentationOverride struct {
	Code         string
	Name         string
	Description  string
	DefaultValue any
}

const installSettingSeparatorCode = "sep"

const (
	installPostURLPreviewDatePath       = "2024/01/02"
	installPostURLPreviewSlug           = "hello-world"
	installPostURLPreviewTitle          = "my-first-post"
	installPostURLPreviewID       int64 = 123
)

var installSettingPresentationOverrides = []installSettingPresentationOverride{
	{
		Code:         "site_name",
		DefaultValue: "",
		Description:  "公开展示的站点名称",
	},
	{
		Code:         "site_desc",
		DefaultValue: "",
		Description:  "公开展示的站点描述",
	},
	{
		Code:        "site_url",
		Description: "站点访问地址，不包含路径前缀",
	},
	{
		Code:        installSettingSeparatorCode,
		Description: "",
	},
	{
		Code:         "author",
		DefaultValue: "",
		Description:  "公开内容显示的作者名",
	},
	{
		Code:        installSettingSeparatorCode,
		Name:        "后台",
		Description: "",
	},
	{
		Code:        "dash_path",
		Description: "管理后台访问路径，修改后需重启",
	},
	{
		Code:        "dash_password",
		Description: "后台登录密码",
	},
	{
		Code:        installSettingSeparatorCode,
		Name:        "前台（可选）",
		Description: "",
	},
	{
		Code:        "base_path",
		Description: "前台统一路径前缀，留空表示根路径",
	},
	{
		Code:        "post_url_prefix",
		Description: "前台文章路径前缀",
	},
	{
		Code:        "post_url_name",
		Description: "文章链接地址中使用的名称格式",
	},
	{
		Code:        "post_url_ext",
		Description: "文章链接地址扩展名",
	},
}

func routeURL(c fiber.Ctx, name string) string {
	path, err := c.GetRouteURL(name, fiber.Map{})
	if err != nil {
		return ""
	}
	return path
}

func resolveInstallSiteURL(c fiber.Ctx) string {
	baseURL := strings.TrimSpace(c.BaseURL())
	if baseURL == "" {
		return ""
	}

	currentURL := strings.TrimSpace(c.OriginalURL())
	if currentURL == "" {
		return baseURL
	}

	parsed, err := url.Parse(currentURL)
	if err != nil {
		return baseURL
	}

	path := strings.TrimSpace(parsed.Path)
	if path == "" || path == "/" || path == "/install" {
		return strings.TrimRight(baseURL, "/")
	}

	if strings.HasSuffix(path, "/install") {
		path = strings.TrimSuffix(path, "/install")
	}
	path = strings.TrimRight(path, "/")
	if path == "" {
		return strings.TrimRight(baseURL, "/")
	}

	return strings.TrimRight(baseURL, "/") + path
}

func buildInstallSettingViews(settings []db.Setting) []SettingView {
	settingsByCode := make(map[string]db.Setting, len(settings))
	for _, setting := range settings {
		code := strings.TrimSpace(setting.Code)
		if code == "" {
			continue
		}
		settingsByCode[code] = setting
	}

	views := make([]SettingView, 0, len(installSettingPresentationOverrides))
	for _, override := range installSettingPresentationOverrides {
		code := strings.TrimSpace(override.Code)
		if code == "" {
			continue
		}
		if code == installSettingSeparatorCode {
			views = append(views, SettingView{
				Setting: db.Setting{
					Code:        code,
					Name:        override.Name,
					Description: override.Description,
				},
			})
			continue
		}

		setting, ok := settingsByCode[code]
		if !ok {
			continue
		}

		view := buildSettingView(setting)
		if strings.TrimSpace(override.Name) != "" {
			view.Name = override.Name
		}
		if strings.TrimSpace(override.Description) != "" {
			view.Description = override.Description
		}

		views = append(views, view)
	}

	return views
}

func findInstallSettingPresentationOverride(code string) (installSettingPresentationOverride, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return installSettingPresentationOverride{}, false
	}

	for _, override := range installSettingPresentationOverrides {
		if strings.TrimSpace(override.Code) != code {
			continue
		}
		return override, true
	}

	return installSettingPresentationOverride{}, false
}

func installSettingDefaultOverrideValue(override installSettingPresentationOverride) (string, bool) {
	if override.DefaultValue == nil {
		return "", false
	}

	value, ok := override.DefaultValue.(string)
	if !ok {
		panic("install setting default value must be string")
	}

	return value, true
}

func cloneInstallDefaultSettingsWithSiteURL(siteURL string) []db.Setting {
	settings := make([]db.Setting, 0, len(db.DefaultSettings))
	siteURL = strings.TrimSpace(siteURL)

	for _, item := range db.DefaultSettings {
		setting := item
		if override, ok := findInstallSettingPresentationOverride(setting.Code); ok {
			if defaultValue, ok := installSettingDefaultOverrideValue(override); ok {
				setting.Value = defaultValue
			}
		}
		switch setting.Code {
		case "site_url":
			if siteURL != "" {
				setting.Value = siteURL
			}
		case "dash_password":
			setting.Value = ""
		}
		settings = append(settings, setting)
	}

	return settings
}

func cloneInstallDefaultSettings(c fiber.Ctx) []db.Setting {
	return cloneInstallDefaultSettingsWithSiteURL(resolveInstallSiteURL(c))
}

func applyInstallFormValues(c fiber.Ctx, settings []db.Setting) []db.Setting {
	args := c.Request().PostArgs()

	for i := range settings {
		if isHiddenSettingCode(settings[i].Code) {
			continue
		}

		fieldName := "setting_" + settings[i].Code
		if settings[i].Type == "checkbox" {
			valuesBytes := args.PeekMulti(fieldName)
			values := make([]string, 0, len(valuesBytes))
			for _, value := range valuesBytes {
				values = append(values, string(value))
			}
			settings[i].Value = strings.Join(values, ",")
			continue
		}

		if !args.Has(fieldName) {
			continue
		}
		settings[i].Value = c.FormValue(fieldName)
	}

	return settings
}

func buildInstallOverrides(settings []db.Setting) map[string]string {
	overrides := make(map[string]string, len(settings))
	for _, setting := range settings {
		overrides[setting.Code] = setting.Value
	}
	return overrides
}

func installSettingValue(settings []db.Setting, code string) string {
	code = strings.TrimSpace(code)
	if code == "" {
		return ""
	}

	for _, setting := range settings {
		if strings.TrimSpace(setting.Code) != code {
			continue
		}
		return strings.TrimSpace(setting.Value)
	}

	return ""
}

func buildInstallPostURLPreview(settings []db.Setting) string {
	siteURL := strings.TrimRight(installSettingValue(settings, "site_url"), "/")
	basePath := pathutil.JoinRelative(installSettingValue(settings, "base_path"))
	postPrefix := pathutil.JoinRelative(installSettingValue(settings, "post_url_prefix"))
	postPrefix = strings.ReplaceAll(postPrefix, "{datetime}", installPostURLPreviewDatePath)

	postName := installSettingValue(settings, "post_url_name")
	if postName == "" {
		postName = "{slug}"
	}
	postName = strings.ReplaceAll(postName, "{slug}", installPostURLPreviewSlug)
	postName = strings.ReplaceAll(postName, "{id}", strconv.FormatInt(installPostURLPreviewID, 10))
	postName = strings.ReplaceAll(postName, "{title}", installPostURLPreviewTitle)
	if strings.TrimSpace(postName) == "" {
		postName = installPostURLPreviewSlug
	}

	postExt := installSettingValue(settings, "post_url_ext")
	postPath := pathutil.JoinAbsolute(basePath, postPrefix, postName+postExt)
	if siteURL == "" {
		return postPath
	}

	return siteURL + postPath
}

func changedReloadSettingNames(settings []db.Setting) []string {
	defaultByCode := make(map[string]db.Setting, len(db.DefaultSettings))
	for _, setting := range db.DefaultSettings {
		defaultByCode[setting.Code] = setting
	}

	names := make([]string, 0)
	for _, setting := range settings {
		if setting.Reload != 1 {
			continue
		}
		defaultSetting, ok := defaultByCode[setting.Code]
		if !ok {
			continue
		}
		if setting.Value == defaultSetting.Value {
			continue
		}
		names = append(names, setting.Name)
	}
	return names
}

func (h *Handler) renderInstallPage(c fiber.Ctx, settings []db.Setting, errMsg string) error {
	return RenderDashView(c, "dash/install.html", fiber.Map{
		"Title":                 "Install Swaves",
		"InstallSettings":       buildInstallSettingViews(settings),
		"InstallPostURLPreview": buildInstallPostURLPreview(settings),
		"Error":                 strings.TrimSpace(errMsg),
	}, "")
}

func (h *Handler) GetInstallHandler(c fiber.Ctx) error {
	installed, err := db.HasInstalledSettings(h.Model)
	if err != nil {
		return err
	}
	if installed {
		return fiber.ErrNotFound
	}

	return h.renderInstallPage(c, cloneInstallDefaultSettings(c), "")
}

func (h *Handler) PostInstallHandler(c fiber.Ctx) error {
	installed, err := db.HasInstalledSettings(h.Model)
	if err != nil {
		return err
	}
	if installed {
		return fiber.ErrNotFound
	}

	settings := applyInstallFormValues(c, cloneInstallDefaultSettings(c))
	dashPassword := ""
	for _, setting := range settings {
		if setting.Code == "dash_password" {
			dashPassword = strings.TrimSpace(setting.Value)
			break
		}
	}
	if dashPassword == "" {
		return h.renderInstallPage(c, settings, "安装失败：请先设置后台登录密码")
	}

	if err = db.BootstrapDefaultSettings(h.Model, buildInstallOverrides(settings)); err != nil {
		return h.renderInstallPage(c, settings, "安装失败："+err.Error())
	}

	reloadSettingNames := changedReloadSettingNames(settings)
	return RenderDashView(c, "dash/install_done.html", fiber.Map{
		"Title":               "Install Complete",
		"ReloadSettingNames":  reloadSettingNames,
		"RestartRequired":     len(reloadSettingNames) > 0,
		"CurrentDashLoginURL": routeURL(c, "dash.login.show"),
		"CurrentSiteHomeURL":  routeURL(c, "site.home"),
	}, "")
}
