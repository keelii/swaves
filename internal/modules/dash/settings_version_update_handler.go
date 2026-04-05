package dash

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/notify"
	"swaves/internal/platform/updater"

	"github.com/gofiber/fiber/v3"
)

var checkLatestRelease = updater.CheckLatestRelease

type latestVersionInfo struct {
	Version           string
	ReleaseURL        string
	AutoUpdateEnabled bool
}

type versionUpdateViewState struct {
	AutoUpdateSupported       bool
	AutoUpdateSupportMessage  string
	ManualUpdateEnabled       bool
	GlobalUpdateMessage       string
	AutoUpdateUnavailableHint string
	RuntimeInfo               updater.RuntimeInfo
}

func versionLabel(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "dev"
	}
	return version
}

func findSettingAreaByCode(areas []SettingAreaView, code string) SettingAreaView {
	index := findSettingAreaIndex(areas, code)
	if index < 0 {
		return SettingAreaView{}
	}
	return areas[index]
}

func firstSettingSectionCode(area SettingAreaView) string {
	if len(area.Sections) == 0 {
		return ""
	}
	return area.Sections[0].Code
}

func buildVersionUpdateNotice(result updater.InstallResult) string {
	if result.Installed {
		if result.RestartedPID > 0 && strings.TrimSpace(result.LatestVersion) != "" {
			return fmt.Sprintf("已开始切换到 %s，服务会自动重启到新版本。", versionLabel(result.LatestVersion))
		}
		if result.RestartedPID > 0 {
			return "已开始切换到新版本，服务会自动重启。"
		}
		if strings.TrimSpace(result.LatestVersion) != "" {
			return fmt.Sprintf("已安装 %s，请手动重启服务后生效。", versionLabel(result.LatestVersion))
		}
		return "安装包已写入当前可执行文件，请手动重启服务后生效。"
	}
	if strings.TrimSpace(result.Reason) != "" {
		return result.Reason
	}
	return "当前已是最新版本。"
}

func versionUpdateSupportState(autoUpdateEnabled bool) versionUpdateViewState {
	state := versionUpdateViewState{
		ManualUpdateEnabled: runtime.GOOS != "windows",
	}

	if runtime.GOOS == "windows" {
		state.AutoUpdateSupported = false
		state.AutoUpdateSupportMessage = "Windows 暂不支持自动更新。"
		state.GlobalUpdateMessage = "当前平台暂不支持自动更新；如需升级，请在服务器侧手动替换可执行文件。"
		state.AutoUpdateUnavailableHint = "当前平台暂不支持自动更新。"
		return state
	}

	runtimeInfo, err := updater.ReadActiveRuntimeInfo()
	if err != nil {
		state.AutoUpdateSupported = false
		state.AutoUpdateSupportMessage = "daemon-mode 未启用，自动更新不可用。"
		state.GlobalUpdateMessage = "daemon-mode 未启用时，自动更新不可用；你仍可安装本地发布包，安装后手动重启服务。"
		if autoUpdateEnabled {
			state.AutoUpdateUnavailableHint = "启用 daemon-mode 后可自动下载并切换到新版本。"
		} else {
			state.AutoUpdateUnavailableHint = "当前已是最新版本；如需重装，可先启用 daemon-mode 或使用手动更新。"
		}
		return state
	}

	state.AutoUpdateSupported = true
	state.RuntimeInfo = runtimeInfo
	if autoUpdateEnabled {
		state.AutoUpdateUnavailableHint = "检测到新版本后，可直接自动下载并切换。"
	} else {
		state.AutoUpdateUnavailableHint = "当前已是最新版本。"
	}
	return state
}

func loadLatestVersionInfo(currentVersion string, goos string, goarch string, fallbackVersion string, fallbackReleaseURL string) latestVersionInfo {
	currentVersion = strings.TrimSpace(currentVersion)
	fallbackVersion = strings.TrimSpace(fallbackVersion)
	fallbackReleaseURL = strings.TrimSpace(fallbackReleaseURL)

	result, err := checkLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		return latestVersionInfo{
			Version:           fallbackVersion,
			ReleaseURL:        fallbackReleaseURL,
			AutoUpdateEnabled: currentVersion == "" || fallbackVersion == "" || fallbackVersion != currentVersion,
		}
	}

	latestVersion := strings.TrimSpace(result.LatestVersion)
	latestReleaseURL := strings.TrimSpace(result.LatestReleaseURL)
	if latestVersion == "" {
		latestVersion = fallbackVersion
	}
	if latestReleaseURL == "" {
		latestReleaseURL = fallbackReleaseURL
	}
	return latestVersionInfo{
		Version:           latestVersion,
		ReleaseURL:        latestReleaseURL,
		AutoUpdateEnabled: currentVersion == "" || latestVersion == "" || latestVersion != currentVersion,
	}
}

func (h *Handler) GetSettingsVersionUpdateHandler(c fiber.Ctx) error {
	settings, err := ListAllSettings(h.Model)
	if err != nil {
		return err
	}

	settingAreas := buildSettingAreas(settings)
	frontendArea := findSettingAreaByCode(settingAreas, settingAreaFrontend)
	backendArea := findSettingAreaByCode(settingAreas, settingAreaBackend)
	latestVersion := ""
	latestReleaseURL := ""
	latestNotification, err := GetLatestNotificationByEventTypeService(h.Model, dashNotificationReceiver, dashNotificationEventAppUpdate)
	if err != nil {
		return err
	}
	if latestNotification != nil {
		latestVersion = notify.ParseAppUpdateVersion(latestNotification.AggregateKey)
		latestReleaseURL = notify.ParseAppUpdateReleaseURL(latestNotification.AggregateKey)
	}
	latestInfo := loadLatestVersionInfo(buildinfo.Version, runtime.GOOS, runtime.GOARCH, latestVersion, latestReleaseURL)
	viewState := versionUpdateSupportState(latestInfo.AutoUpdateEnabled)

	return h.RenderDashView(c, "dash/settings_version_update.html", fiber.Map{
		"Title":                     "版本更新",
		"FrontendArea":              frontendArea,
		"BackendArea":               backendArea,
		"FrontendFirstSection":      firstSettingSectionCode(frontendArea),
		"CurrentVersion":            versionLabel(buildinfo.Version),
		"LatestVersion":             latestInfo.Version,
		"LatestReleaseURL":          latestInfo.ReleaseURL,
		"AutoUpdateEnabled":         viewState.AutoUpdateSupported && latestInfo.AutoUpdateEnabled,
		"AutoUpdateSupported":       viewState.AutoUpdateSupported,
		"AutoUpdateUnavailableHint": viewState.AutoUpdateUnavailableHint,
		"ManualUpdateEnabled":       viewState.ManualUpdateEnabled,
		"GlobalUpdateMessage":       viewState.GlobalUpdateMessage,
		"VersionUpdateNotice":       strings.TrimSpace(c.Query("notice")),
		"VersionUpdateError":        strings.TrimSpace(c.Query("error")),
		"VersionUpdateAutoRefresh":  strings.TrimSpace(c.Query("refresh")) == "1",
		"VersionRuntimePID":         viewState.RuntimeInfo.PID,
	}, "")
}

func (h *Handler) PostSettingsVersionAutoUpdateHandler(c fiber.Ctx) error {
	if runtime.GOOS == "windows" {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "Windows 暂不支持 daemon-mode 自动更新",
		})
	}

	result, err := updater.InstallLatestRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": err.Error(),
		})
	}

	return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
		"notice": buildVersionUpdateNotice(result),
		"refresh": func() string {
			if result.RestartedPID > 0 {
				return "1"
			}
			return ""
		}(),
	})
}

func (h *Handler) PostSettingsVersionManualUpdateHandler(c fiber.Ctx) error {
	if runtime.GOOS == "windows" {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "Windows 暂不支持 daemon-mode 自动更新",
		})
	}

	fileHeader, err := c.FormFile("archive")
	if err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "读取安装包失败：" + err.Error(),
		})
	}

	src, err := fileHeader.Open()
	if err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "打开安装包失败：" + err.Error(),
		})
	}
	defer func() { _ = src.Close() }()

	tmpDir, err := os.MkdirTemp("", ".swaves-manual-upgrade-")
	if err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "创建临时目录失败：" + err.Error(),
		})
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archiveName := filepath.Base(fileHeader.Filename)
	archivePath := filepath.Join(tmpDir, archiveName)
	dst, err := os.Create(archivePath)
	if err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "创建临时安装包失败：" + err.Error(),
		})
	}
	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "保存安装包失败：" + err.Error(),
		})
	}
	if err = dst.Close(); err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": "关闭临时安装包失败：" + err.Error(),
		})
	}

	result, err := updater.InstallLocalReleaseArchive(archiveName, archivePath, buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
			"error": err.Error(),
		})
	}

	return h.redirectToDashRoute(c, "dash.settings.version_update", nil, map[string]string{
		"notice": buildVersionUpdateNotice(result),
		"refresh": func() string {
			if result.RestartedPID > 0 {
				return "1"
			}
			return ""
		}(),
	})
}
