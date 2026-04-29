package dash

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"swaves/internal/platform/config"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/updater"
	"swaves/internal/shared/pathutil"
	"swaves/internal/shared/webutil"

	"github.com/gofiber/fiber/v3"
)

type InstallSettingsOption struct {
	Code         string
	Name         string
	Description  string
	Required     bool
	DefaultValue any
	Placeholder  string
}

const installSettingSeparatorCode = "sep"

const (
	installPostURLPreviewDatePath       = "2024/01/02"
	installPostURLPreviewSlug           = "hello-world"
	installPostURLPreviewTitle          = "my-first-post"
	installPostURLPreviewID       int64 = 123
)

var installSettings = []InstallSettingsOption{
	{
		Code:         "site_title",
		DefaultValue: "",
		Placeholder:  "",
		Description:  "公开展示的站点标题",
	},
	{
		Code:         "site_desc",
		DefaultValue: "",
		Description:  "公开展示的站点描述",
	},
	//{
	//	Code:        "site_url",
	//	Description: "站点访问地址，不包含路径前缀",
	//},
	{
		Code:         "author",
		DefaultValue: "",
		Description:  "公开内容显示的作者名",
	},
	{
		Code:        db.SettingCodeBlockSearchEngineCrawlers,
		Description: "开启后 robots.txt 将禁止所有搜索引擎抓取站点内容",
	},
	{
		Code:        installSettingSeparatorCode,
		Name:        "后台",
		Description: "",
	},
	{
		Code:        "dash_path",
		Description: "管理后台访问路径",
	},
	{
		Code:        "dash_password",
		Required:    true,
		Description: "管理后台登录密码",
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

	views := make([]SettingView, 0, len(installSettings))
	for _, override := range installSettings {
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
				Required:    override.Required,
				Placeholder: override.Placeholder,
			})
			continue
		}

		setting, ok := settingsByCode[code]
		if !ok {
			continue
		}

		view := buildSettingView(setting)
		if override.Name != "" {
			view.Name = override.Name
		}
		if override.Description != "" {
			view.Description = override.Description
		}
		if override.Placeholder != "" {
			view.Placeholder = override.Placeholder
		}
		view.Required = override.Required

		views = append(views, view)
	}

	return views
}

func findInstallSettingPresentationOverride(code string) (InstallSettingsOption, bool) {
	code = strings.TrimSpace(code)
	if code == "" {
		return InstallSettingsOption{}, false
	}

	for _, override := range installSettings {
		if strings.TrimSpace(override.Code) != code {
			continue
		}
		return override, true
	}

	return InstallSettingsOption{}, false
}

func installSettingDefaultOverrideValue(override InstallSettingsOption) (string, bool) {
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

func installDashPath(settings []db.Setting) string {
	dashPath := strings.TrimSpace(installSettingValue(settings, "dash_path"))
	if dashPath == "" {
		dashPath = "/dash"
	}
	return pathutil.JoinAbsolute(dashPath)
}

func currentExecutablePath() string {
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(path)
}

func (h *Handler) saveInstallSession(c fiber.Ctx, notice string) bool {
	if h == nil || h.Session == nil {
		return false
	}

	sess, err := h.Session.AcquireSession(c)
	if err != nil {
		logger.Error("get session failed: %v", err)
		return false
	}
	defer sess.Release()

	if err := sess.Regenerate(); err != nil {
		logger.Error("session regenerate failed: %v", err)
		return false
	}

	sess.Set(config.LoginDashName, true)
	sess.SetIdleTimeout(config.LoginSessionExpire)

	notice = strings.TrimSpace(notice)
	if notice != "" {
		sess.Set(dashFlashNoticeKey, notice)
	}

	if err := sess.Save(); err != nil {
		logger.Error("session save failed: %v", err)
		return false
	}
	logger.Info("session saved")
	return true
}

func (h *Handler) renderInstallPage(c fiber.Ctx, settings []db.Setting, errMsg string) error {
	return RenderDashView(c, "dash/install.html", fiber.Map{
		"Title":                  "Install Swaves",
		"InstallSettings":        buildInstallSettingViews(settings),
		"InstallPostURLPreview":  buildInstallPostURLPreview(settings),
		"InstallPreviewSiteURL":  installSettingValue(settings, "site_url"),
		"InstallPreviewBasePath": installSettingValue(settings, "base_path"),
		"InstallPreviewPostPath": installSettingValue(settings, "post_url_prefix"),
		"InstallPreviewPostName": installSettingValue(settings, "post_url_name"),
		"InstallPreviewPostExt":  installSettingValue(settings, "post_url_ext"),
		"Error":                  strings.TrimSpace(errMsg),
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

	currentDashHomePath := h.dashRoutePath(c, "dash.home", nil)
	if currentDashHomePath == "" {
		currentDashHomePath = "/dash"
	}
	targetDashHomePath := installDashPath(settings)
	targetPath := targetDashHomePath

	runtimeInfo, runtimeErr := updater.ReadActiveRuntimeInfo()
	currentExecutable := currentExecutablePath()
	if runtimeErr != nil || currentExecutable == "" || filepath.Clean(strings.TrimSpace(runtimeInfo.Executable)) != currentExecutable {
		notice := "安装已完成，请重启服务后让新路径配置生效。"
		if runtimeErr != nil {
			logger.Warn("[install] restart required after install but unavailable: ip=%s target=%s err=%v", c.IP(), targetPath, runtimeErr)
		} else {
			logger.Warn("[install] skip install restart due to runtime mismatch: ip=%s target=%s current=%s master=%s", c.IP(), targetPath, currentExecutable, strings.TrimSpace(runtimeInfo.Executable))
		}
		if normalizePathForCompare(targetDashHomePath) != normalizePathForCompare(currentDashHomePath) {
			notice = fmt.Sprintf("安装已完成，请重启服务后从 %s 继续访问后台。", targetDashHomePath)
		}
		sessionSaved := h.saveInstallSession(c, notice)
		if sessionSaved {
			return h.redirectToDashRoute(c, "dash.home", nil, nil)
		}
		return h.redirectToDashRoute(c, "dash.login.show", nil, nil)
	}

	pid, restartErr := updater.RestartActiveRuntime()
	if restartErr != nil {
		logger.Warn("[install] restart request failed after install: ip=%s target=%s err=%v", c.IP(), targetPath, restartErr)
		notice := "安装已完成，请重启服务后让新路径配置生效。"
		if normalizePathForCompare(targetDashHomePath) != normalizePathForCompare(currentDashHomePath) {
			notice = fmt.Sprintf("安装已完成，请重启服务后从 %s 继续访问后台。", targetDashHomePath)
		}
		sessionSaved := h.saveInstallSession(c, notice)
		if sessionSaved {
			return h.redirectToDashRoute(c, "dash.home", nil, nil)
		}
		return h.redirectToDashRoute(c, "dash.login.show", nil, nil)
	}

	notice := fmt.Sprintf("安装已完成，已向服务发送重启信号（pid=%d）。", pid)
	sessionSaved := h.saveInstallSession(c, notice)
	if !sessionSaved {
		targetPath = pathutil.JoinAbsolute(targetDashHomePath, "login")
	}
	logger.Info("[install] install completed and restart requested: ip=%s master_pid=%d target=%s", c.IP(), pid, targetPath)
	return webutil.RedirectTo(c, targetPath)
}
