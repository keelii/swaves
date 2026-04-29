package dash

import (
	"swaves/internal/platform/db"
	"swaves/internal/platform/store"
	"swaves/internal/platform/updater"
	"swaves/internal/shared/types"
)

type systemUpdateDeps struct {
	checkLatestRelease func(currentVersion string, goos string, goarch string) (updater.CheckResult, error)
	readActiveRuntime  func() (updater.RuntimeInfo, error)
	restartRuntime     func() (int, error)
	installLatest      func(currentVersion string, goos string, goarch string) (updater.InstallResult, error)
	installLocal       func(archiveName string, archivePath string, currentVersion string, goos string, goarch string) (updater.InstallResult, error)
}

func defaultSystemUpdateDeps() systemUpdateDeps {
	return systemUpdateDeps{
		checkLatestRelease: updater.CheckLatestRelease,
		readActiveRuntime:  updater.ReadActiveRuntimeInfo,
		restartRuntime:     updater.RestartActiveRuntime,
		installLatest:      updater.InstallLatestRelease,
		installLocal:       updater.InstallLocalReleaseArchive,
	}
}

type Handler struct {
	Model        *db.DB
	Session      *types.SessionStore
	Service      *Service
	Monitor      *MonitorStore
	systemUpdate systemUpdateDeps
}

func (h *Handler) resolvedSystemUpdateDeps() systemUpdateDeps {
	deps := defaultSystemUpdateDeps()
	if h == nil {
		return deps
	}
	if h.systemUpdate.checkLatestRelease != nil {
		deps.checkLatestRelease = h.systemUpdate.checkLatestRelease
	}
	if h.systemUpdate.readActiveRuntime != nil {
		deps.readActiveRuntime = h.systemUpdate.readActiveRuntime
	}
	if h.systemUpdate.restartRuntime != nil {
		deps.restartRuntime = h.systemUpdate.restartRuntime
	}
	if h.systemUpdate.installLatest != nil {
		deps.installLatest = h.systemUpdate.installLatest
	}
	if h.systemUpdate.installLocal != nil {
		deps.installLocal = h.systemUpdate.installLocal
	}
	return deps
}

func NewHandler(
	gStore *store.GlobalStore,
	dashService *Service,
	monitorStore *MonitorStore,
) *Handler {
	return &Handler{
		Model:        gStore.Model,
		Session:      gStore.Session,
		Service:      dashService,
		Monitor:      monitorStore,
		systemUpdate: defaultSystemUpdateDeps(),
	}
}
