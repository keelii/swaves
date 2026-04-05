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
		if strings.TrimSpace(result.LatestVersion) != "" {
			return fmt.Sprintf("已开始切换到 %s，服务会自动重启到新版本。", versionLabel(result.LatestVersion))
		}
		return "已开始切换到新版本，服务会自动重启。"
	}
	if strings.TrimSpace(result.Reason) != "" {
		return result.Reason
	}
	return "当前已是最新版本。"
}

func versionUpdateSupportState() (bool, string, updater.RuntimeInfo) {
	if runtime.GOOS == "windows" {
		return false, "Windows 暂不支持 daemon-mode 自动更新", updater.RuntimeInfo{}
	}

	runtimeInfo, err := updater.ReadActiveRuntimeInfo()
	if err != nil {
		return false, "当前 daemon-mode master 不可用，无法执行自动更新或本地安装包切换：" + err.Error(), updater.RuntimeInfo{}
	}
	return true, "", runtimeInfo
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
	updateSupported, updateSupportMessage, runtimeInfo := versionUpdateSupportState()

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

	return h.RenderDashView(c, "dash/settings_version_update.html", fiber.Map{
		"Title":                "版本更新",
		"FrontendArea":         frontendArea,
		"BackendArea":          backendArea,
		"FrontendFirstSection": firstSettingSectionCode(frontendArea),
		"CurrentVersion":       versionLabel(buildinfo.Version),
		"LatestVersion":        latestInfo.Version,
		"LatestReleaseURL":     latestInfo.ReleaseURL,
		"AutoUpdateEnabled":    updateSupported && latestInfo.AutoUpdateEnabled,
		"UpdateSupported":      updateSupported,
		"UpdateSupportMessage": updateSupportMessage,
		"VersionUpdateNotice":  strings.TrimSpace(c.Query("notice")),
		"VersionUpdateError":   strings.TrimSpace(c.Query("error")),
		"VersionRuntimePID":    runtimeInfo.PID,
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
	})
}
