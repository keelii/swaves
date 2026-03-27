package dash

import (
	"strings"
	"swaves/internal/platform/db"

	"github.com/gofiber/fiber/v3"
)

var installSettingCodes = []string{
	"site_name",
	"site_url",
	"author",
	"base_path",
	"post_url_name",
	"dash_path",
	"dash_password",
}

var installSettingPresentationOverrides = map[string]struct {
	Name        string
	Description string
}{
	"site_name": {
		Description: "公开展示的站点名称。",
	},
	"author": {
		Description: "公开内容显示的作者名。",
	},
	"site_url": {
		Description: "站点访问地址，不包含路径前缀。",
	},
	"base_path": {
		Description: "站点前台统一路径前缀，留空表示根路径。",
	},
	"post_url_name": {
		Description: "文章链接中使用的名称格式。",
	},
	"dash_path": {
		Description: "管理后台访问路径，修改后需重启。",
	},
	"dash_password": {
		Description: "后台登录密码。",
	},
}

func routeURL(c fiber.Ctx, name string) string {
	path, err := c.GetRouteURL(name, fiber.Map{})
	if err != nil {
		return ""
	}
	return path
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

	views := make([]SettingView, 0, len(installSettingCodes))
	for _, code := range installSettingCodes {
		setting, ok := settingsByCode[code]
		if !ok {
			continue
		}
		view := buildSettingView(setting)
		if override, ok := installSettingPresentationOverrides[code]; ok {
			if strings.TrimSpace(override.Name) != "" {
				view.Name = override.Name
			}
			if strings.TrimSpace(override.Description) != "" {
				view.Description = override.Description
			}
		}
		views = append(views, view)
	}

	return views
}

func cloneInstallDefaultSettings(c fiber.Ctx) []db.Setting {
	settings := make([]db.Setting, 0, len(db.DefaultSettings))
	siteURL := strings.TrimSpace(c.BaseURL())

	for _, item := range db.DefaultSettings {
		setting := item
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
		"Title":           "Install Swaves",
		"InstallSettings": buildInstallSettingViews(settings),
		"Error":           strings.TrimSpace(errMsg),
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
