package dash

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/notify"
	"swaves/internal/platform/updater"

	"github.com/gofiber/fiber/v3"
)

const defaultRefreshDelaySeconds = 3

type latestVersionInfo struct {
	Version           string
	ReleaseURL        string
	HasSystemUpdate   bool
	AutoUpdateEnabled bool
}

type systemUpdateViewState struct {
	AutoUpdateEnabled    bool
	ManualUpdateEnabled  bool
	RestartEnabled       bool
	GlobalUpdateMessage  string
	RuntimeStatusMessage string
	SystemRuntimePID     int
	SystemRuntimePath    string
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

func buildSystemUpdateNotice(result updater.InstallResult) string {
	if result.Installed {
		if result.RestartedPID > 0 && strings.TrimSpace(result.LatestVersion) != "" {
			return fmt.Sprintf("已开始切换到 %s，服务会自动重启到新版本。", versionLabel(result.LatestVersion))
		}
		if result.RestartedPID > 0 {
			return "已开始切换到新版本，服务会自动重启。"
		}
		if strings.TrimSpace(result.LatestVersion) != "" {
			return fmt.Sprintf("已安装 %s，请手动重启服务后生效[kill -HUP PID]。", versionLabel(result.LatestVersion))
		}
		return "安装包已写入当前可执行文件，请手动重启服务后生效[kill -HUP PID]。"
	}
	if strings.TrimSpace(result.Reason) != "" {
		return result.Reason
	}
	return "当前已是最新版本。"
}

func parseRefreshDelaySeconds(raw string) int {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0
	}

	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds <= 0 {
		return 0
	}
	return seconds
}

func systemUpdateRefreshQuery(result updater.InstallResult) map[string]string {
	if result.RestartedPID <= 0 {
		return nil
	}
	return map[string]string{"refresh": strconv.Itoa(defaultRefreshDelaySeconds)}
}

func systemUpdateSupportState(readActiveRuntimeInfo func() (updater.RuntimeInfo, error), autoUpdateEnabled bool) systemUpdateViewState {
	state := systemUpdateViewState{}

	if runtime.GOOS == "windows" {
		state.GlobalUpdateMessage = "当前平台暂不支持系统更新。"
		return state
	}

	state.AutoUpdateEnabled = autoUpdateEnabled
	state.ManualUpdateEnabled = true

	runtimeInfo, err := readActiveRuntimeInfo()
	if err != nil {
		state.RuntimeStatusMessage = "未检测到可重启的 master 进程：" + err.Error() + "。更新仍可写入当前可执行文件，但需要手动重启后生效。"
		return state
	}

	state.RestartEnabled = true
	state.SystemRuntimePID = runtimeInfo.PID
	state.SystemRuntimePath = strings.TrimSpace(runtimeInfo.Executable)
	return state
}

func loadLatestVersionInfo(checkLatestRelease func(currentVersion string, goos string, goarch string) (updater.CheckResult, error), currentVersion string, goos string, goarch string, fallbackVersion string, fallbackReleaseURL string) latestVersionInfo {
	currentVersion = strings.TrimSpace(currentVersion)
	fallbackVersion = strings.TrimSpace(fallbackVersion)
	fallbackReleaseURL = strings.TrimSpace(fallbackReleaseURL)

	result, err := checkLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		return latestVersionInfo{
			Version:           fallbackVersion,
			ReleaseURL:        fallbackReleaseURL,
			HasSystemUpdate:   false,
			AutoUpdateEnabled: false,
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
	hasSystemUpdate := currentVersion == "" || latestVersion == "" || latestVersion != currentVersion
	return latestVersionInfo{
		Version:           latestVersion,
		ReleaseURL:        latestReleaseURL,
		HasSystemUpdate:   hasSystemUpdate,
		AutoUpdateEnabled: hasSystemUpdate,
	}
}

func (h *Handler) GetSettingsSystemUpdateHandler(c fiber.Ctx) error {
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
	latestInfo := loadLatestVersionInfo(updater.CheckLatestRelease, buildinfo.Version, runtime.GOOS, runtime.GOARCH, latestVersion, latestReleaseURL)
	viewState := systemUpdateSupportState(updater.ReadActiveRuntimeInfo, latestInfo.AutoUpdateEnabled)
	if viewState.RuntimeStatusMessage != "" {
		logger.Warn("[dash] system update runtime unavailable: ip=%s current=%s message=%s",
			c.IP(), versionLabel(buildinfo.Version), viewState.RuntimeStatusMessage)
	}

	return RenderDashView(c, "dash/settings_system_update.html", fiber.Map{
		"Title":                    "系统更新",
		"FrontendArea":             frontendArea,
		"BackendArea":              backendArea,
		"FrontendFirstSection":     firstSettingSectionCode(frontendArea),
		"CurrentVersion":           versionLabel(buildinfo.Version),
		"LatestVersion":            latestInfo.Version,
		"LatestReleaseURL":         latestInfo.ReleaseURL,
		"HasSystemUpdate":          latestInfo.HasSystemUpdate,
		"AutoUpdateEnabled":        viewState.AutoUpdateEnabled,
		"ManualUpdateEnabled":      viewState.ManualUpdateEnabled,
		"RestartEnabled":           viewState.RestartEnabled,
		"GlobalUpdateMessage":      viewState.GlobalUpdateMessage,
		"RuntimeStatusMessage":     viewState.RuntimeStatusMessage,
		"SystemUpdateRefreshDelay": parseRefreshDelaySeconds(c.Query("refresh")),
		"SystemRuntimePID":         viewState.SystemRuntimePID,
		"SystemRuntimePath":        viewState.SystemRuntimePath,
	}, "")
}

func (h *Handler) PostSettingsSystemRestartHandler(c fiber.Ctx) error {
	if runtime.GOOS == "windows" {
		logger.Warn("[dash] system restart blocked: ip=%s platform=%s/%s", c.IP(), runtime.GOOS, runtime.GOARCH)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "Windows 暂不支持系统重启")
	}

	pid, err := updater.RestartActiveRuntime()
	if err != nil {
		logger.Error("[dash] system restart failed: ip=%s err=%v", c.IP(), err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "系统重启失败："+err.Error())
	}

	logger.Info("[dash] system restart requested: ip=%s master_pid=%d", c.IP(), pid)
	return h.redirectToDashRouteWithNotice(c, "dash.settings.system_update", nil, map[string]string{
		"refresh": strconv.Itoa(defaultRefreshDelaySeconds),
	}, fmt.Sprintf("已向服务发送重启信号（pid=%d）。", pid))
}

func (h *Handler) PostSettingsSystemAutoUpdateHandler(c fiber.Ctx) error {
	if runtime.GOOS == "windows" {
		logger.Warn("[dash] auto update blocked: ip=%s platform=%s/%s", c.IP(), runtime.GOOS, runtime.GOARCH)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "Windows 暂不支持 daemon-mode 自动更新")
	}

	logger.Info("[dash] auto update requested: ip=%s current=%s target=%s/%s", c.IP(), versionLabel(buildinfo.Version), runtime.GOOS, runtime.GOARCH)
	result, err := updater.InstallLatestRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		logger.Error("[dash] auto update failed: ip=%s current=%s err=%v", c.IP(), versionLabel(buildinfo.Version), err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, err.Error())
	}
	logSystemUpdateResult("auto", c.IP(), result)

	return h.redirectToDashRouteWithNotice(c, "dash.settings.system_update", nil, systemUpdateRefreshQuery(result), buildSystemUpdateNotice(result))
}

func (h *Handler) PostSettingsSystemManualUpdateHandler(c fiber.Ctx) error {
	if runtime.GOOS == "windows" {
		logger.Warn("[dash] manual update blocked: ip=%s platform=%s/%s", c.IP(), runtime.GOOS, runtime.GOARCH)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "Windows 暂不支持 daemon-mode 自动更新")
	}

	fileHeader, err := c.FormFile("archive")
	if err != nil {
		logger.Error("[dash] manual update read upload failed: ip=%s err=%v", c.IP(), err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "读取安装包失败："+err.Error())
	}
	logger.Info("[dash] manual update requested: ip=%s archive=%s size=%d current=%s", c.IP(), filepath.Base(fileHeader.Filename), fileHeader.Size, versionLabel(buildinfo.Version))

	src, err := fileHeader.Open()
	if err != nil {
		logger.Error("[dash] manual update open upload failed: ip=%s archive=%s err=%v", c.IP(), filepath.Base(fileHeader.Filename), err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "打开安装包失败："+err.Error())
	}
	defer func() { _ = src.Close() }()

	tmpDir, err := updater.CreateUpgradeTempDir(".swaves-manual-upgrade-")
	if err != nil {
		logger.Error("[dash] manual update create temp dir failed: ip=%s archive=%s err=%v", c.IP(), filepath.Base(fileHeader.Filename), err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "创建临时目录失败："+err.Error())
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archiveName := filepath.Base(fileHeader.Filename)
	archivePath := filepath.Join(tmpDir, archiveName)
	dst, err := os.Create(archivePath)
	if err != nil {
		logger.Error("[dash] manual update create temp archive failed: ip=%s archive=%s err=%v", c.IP(), archiveName, err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "创建临时安装包失败："+err.Error())
	}
	if _, err = io.Copy(dst, src); err != nil {
		_ = dst.Close()
		logger.Error("[dash] manual update save upload failed: ip=%s archive=%s err=%v", c.IP(), archiveName, err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "保存安装包失败："+err.Error())
	}
	if err = dst.Close(); err != nil {
		logger.Error("[dash] manual update close temp archive failed: ip=%s archive=%s err=%v", c.IP(), archiveName, err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, "关闭临时安装包失败："+err.Error())
	}

	result, err := updater.InstallLocalReleaseArchive(archiveName, archivePath, buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		logger.Error("[dash] manual update failed: ip=%s archive=%s err=%v", c.IP(), archiveName, err)
		return h.redirectToDashRouteWithError(c, "dash.settings.system_update", nil, nil, err.Error())
	}
	logSystemUpdateResult("manual", c.IP(), result)

	return h.redirectToDashRouteWithNotice(c, "dash.settings.system_update", nil, systemUpdateRefreshQuery(result), buildSystemUpdateNotice(result))
}

func logSystemUpdateResult(kind string, ip string, result updater.InstallResult) {
	if result.Installed && result.RestartedPID <= 0 {
		logger.Warn("[dash] %s update installed without runtime restart: ip=%s current=%s latest=%s reason=%s",
			kind, ip, versionLabel(result.CurrentVersion), versionLabel(result.LatestVersion), strings.TrimSpace(result.Reason))
		return
	}
	logger.Info("[dash] %s update completed: ip=%s latest=%s installed=%t master_pid=%d",
		kind, ip, versionLabel(result.LatestVersion), result.Installed, result.RestartedPID)
}
