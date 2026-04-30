package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"swaves/internal/platform/logger"
	"swaves/internal/shared/semverutil"
	"sync"
	"syscall"
)

// ArchiveSource identifies where the binary archive comes from.
type ArchiveSource int

const (
	// ArchiveSourceRemote downloads the archive from a GitHub release.
	ArchiveSourceRemote ArchiveSource = iota
	// ArchiveSourceLocal uses an archive already present on disk.
	ArchiveSourceLocal
)

// InstallSource describes a versioned binary archive to be installed.
// For ArchiveSourceRemote, set Kind only; the release target is resolved
// from GitHub during Install.
// For ArchiveSourceLocal, set Kind, ArchiveName, ArchivePath, and Version.
type InstallSource struct {
	Kind        ArchiveSource
	ArchiveName string         // archive filename (e.g. swaves_v1.2.3_linux_amd64.tar.gz)
	ArchivePath string         // on-disk path; required for ArchiveSourceLocal
	Target      *ReleaseTarget // populated internally for ArchiveSourceRemote after release check
	Version     string         // target version label
}

// RestartPolicy controls how a running daemon master is handled after install.
type RestartPolicy int

const (
	// RestartRequireMaster requires an active daemon master; fails if none is
	// running. Installs to the master's executable and always signals a restart.
	RestartRequireMaster RestartPolicy = iota
	// RestartIfMatchingMaster installs to the current executable. Signals a
	// restart only when an active master's executable matches the installed path.
	RestartIfMatchingMaster
	// RestartWithMasterFallback installs to the active master's executable and
	// signals a restart when a master is running; otherwise installs to the
	// current executable without restarting.
	RestartWithMasterFallback
)

type InstallResult struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	ArchiveName    string
	Installed      bool
	RestartedPID   int
	Reason         string
}

var (
	installMu sync.Mutex
)

type executableRollback func() error

// Install is the single core installation function. It resolves the archive
// (downloading from GitHub or reading a local file), extracts the binary,
// replaces the target executable, and optionally restarts the daemon master
// according to policy. All three public entry-points are thin wrappers around
// this function.
func (c Client) Install(source InstallSource, currentVersion string, goos string, goarch string, policy RestartPolicy) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

	currentVersion = strings.TrimSpace(currentVersion)
	result := InstallResult{CurrentVersion: currentVersion}
	logger.Info("[update] install start: source=%s current=%s target=%s/%s policy=%s",
		archiveSourceLabel(source.Kind), versionLabel(currentVersion), goos, goarch, restartPolicyLabel(policy))

	// --- Step 1: Determine target executable and restart configuration ---
	// Done before any network I/O so we fail fast (especially for RestartRequireMaster).
	targetPath, restartRuntimeInfo, err := resolveInstallTarget(policy)
	if err != nil {
		logger.Warn("[update] install target resolution failed: policy=%s err=%v", restartPolicyLabel(policy), err)
		return result, err
	}
	restartPID := 0
	if restartRuntimeInfo != nil {
		restartPID = restartRuntimeInfo.PID
	}
	logger.Info("[update] install target resolved: target=%s restart_pid=%d", targetPath, restartPID)

	// --- Step 2: Prepare archive ---
	// Remote: check release, skip if already up-to-date, download + verify.
	// Local: archive is already on disk; version and path are set in source.
	var archivePath string
	switch source.Kind {
	case ArchiveSourceRemote:
		check, err := c.CheckLatestRelease(currentVersion, goos, goarch)
		if err != nil {
			logger.Error("[update] install release check failed: current=%s target=%s/%s err=%v", versionLabel(currentVersion), goos, goarch, err)
			return result, err
		}
		result.LatestVersion = check.LatestVersion
		result.ReleaseURL = check.LatestReleaseURL
		if check.Target == nil {
			resolvedGOOS := goos
			resolvedGOARCH := goarch
			if strings.TrimSpace(resolvedGOOS) == "" {
				resolvedGOOS = runtime.GOOS
			}
			if strings.TrimSpace(resolvedGOARCH) == "" {
				resolvedGOARCH = runtime.GOARCH
			}
			logger.Warn("[update] install unsupported target: target=%s/%s", resolvedGOOS, resolvedGOARCH)
			return result, fmt.Errorf("automatic upgrade is not supported on %s/%s", resolvedGOOS, resolvedGOARCH)
		}
		result.ArchiveName = check.Target.Archive.Name
		source.Target = check.Target
		source.Version = check.LatestVersion
		logger.Info("[update] install release check result: latest=%s archive=%s reason=%s",
			versionLabel(result.LatestVersion), result.ArchiveName, strings.TrimSpace(check.Reason))

		if semverutil.IsStable(currentVersion) {
			cmp, err := semverutil.Compare(currentVersion, check.LatestVersion)
			if err != nil {
				logger.Error("[update] install semver compare failed: current=%s latest=%s err=%v", currentVersion, check.LatestVersion, err)
				return result, err
			}
			if cmp >= 0 {
				result.Reason = check.Reason
				logger.Info("[update] install skipped: current=%s latest=%s reason=%s", versionLabel(currentVersion), versionLabel(result.LatestVersion), strings.TrimSpace(result.Reason))
				return result, nil
			}
		}

	case ArchiveSourceLocal:
		result.ArchiveName = filepath.Base(source.ArchiveName)
		result.LatestVersion = source.Version
		archivePath = source.ArchivePath
		logger.Info("[update] install local archive: archive=%s version=%s path=%s", result.ArchiveName, result.LatestVersion, archivePath)
	}

	// --- Step 3: Create temp dir adjacent to the target executable ---
	tmpDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".swaves-upgrade-")
	if err != nil {
		logger.Error("[update] install create temp dir failed: target=%s err=%v", targetPath, err)
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	// --- Step 4: Download remote archive + checksum and verify ---
	if source.Kind == ArchiveSourceRemote {
		archivePath = filepath.Join(tmpDir, source.Target.Archive.Name)
		logger.Info("[update] install downloading archive: url=%s dst=%s", source.Target.Archive.DownloadURL, archivePath)
		if err := c.downloadToFile(source.Target.Archive.DownloadURL, archivePath); err != nil {
			logger.Error("[update] install download archive failed: url=%s err=%v", source.Target.Archive.DownloadURL, err)
			return result, err
		}
		checksumPath := filepath.Join(tmpDir, source.Target.Checksum.Name)
		logger.Info("[update] install downloading checksum: url=%s dst=%s", source.Target.Checksum.DownloadURL, checksumPath)
		if err := c.downloadToFile(source.Target.Checksum.DownloadURL, checksumPath); err != nil {
			logger.Error("[update] install download checksum failed: url=%s err=%v", source.Target.Checksum.DownloadURL, err)
			return result, err
		}
		if err := verifyChecksumFile(archivePath, checksumPath); err != nil {
			logger.Error("[update] install checksum verify failed: archive=%s err=%v", archivePath, err)
			return result, err
		}
		logger.Info("[update] install checksum verified: archive=%s", archivePath)
	}

	// --- Step 5: Extract binary and make it executable ---
	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, expectedReleaseBinaryName(result.ArchiveName))
	if err != nil {
		logger.Error("[update] install extract failed: archive=%s err=%v", archivePath, err)
		return result, err
	}
	logger.Info("[update] install extracted binary: path=%s", extractedPath)
	if err := os.Chmod(extractedPath, 0755); err != nil {
		logger.Error("[update] install chmod failed: path=%s err=%v", extractedPath, err)
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}

	// --- Step 6: Replace target executable (with rollback support) ---
	backupPath := filepath.Join(tmpDir, ".swaves-executable-backup")
	var rollback executableRollback
	if restartRuntimeInfo != nil {
		logger.Info("[update] install replacing executable via master: master_pid=%d target=%s", restartPID, targetPath)
		rollback, err = replaceExecutableWithRollback(extractedPath, *restartRuntimeInfo, backupPath)
	} else {
		logger.Info("[update] install replacing executable: target=%s", targetPath)
		rollback, err = replaceExecutableAtPathWithRollback(extractedPath, targetPath, backupPath)
	}
	if err != nil {
		logger.Error("[update] install replace executable failed: target=%s err=%v", targetPath, err)
		return result, err
	}

	// --- Step 7: Signal master restart when required ---
	if restartPID > 0 {
		if err := defaultSignalProcess(restartPID); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				logger.Error("[update] install restart signal failed and rollback failed: master_pid=%d err=%v rollback_err=%v", restartPID, err, rollbackErr)
				return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
			}
			logger.Error("[update] install restart signal failed: master_pid=%d err=%v", restartPID, err)
			return result, fmt.Errorf("signal master restart failed: %w", err)
		}
		result.RestartedPID = restartPID
		result.Reason = fmt.Sprintf("upgraded to %s", source.Version)
		logger.Info("[update] install success: version=%s master_pid=%d", versionLabel(result.LatestVersion), restartPID)
	} else {
		result.Reason = fmt.Sprintf("installed %s, restart required", source.Version)
		logger.Info("[update] install success: version=%s restart_required=true", versionLabel(result.LatestVersion))
	}
	result.Installed = true
	return result, nil
}

// resolveInstallTarget determines the target executable path and, when
// applicable, the RuntimeInfo of the master to restart. A non-nil RuntimeInfo
// in the return value means a restart is expected.
func resolveInstallTarget(policy RestartPolicy) (targetPath string, ri *RuntimeInfo, err error) {
	switch policy {
	case RestartRequireMaster:
		info, err := ReadActiveRuntimeInfo()
		if err != nil {
			return "", nil, fmt.Errorf("automatic upgrade requires daemon-mode=1 and an active master process: %w", err)
		}
		return info.Executable, &info, nil

	case RestartIfMatchingMaster:
		path, err := currentInstallExecutable()
		if err != nil {
			return "", nil, err
		}
		if info, ok := activeRuntimeForExecutable(path); ok {
			return path, &info, nil
		}
		return path, nil, nil

	case RestartWithMasterFallback:
		info, err := ReadActiveRuntimeInfo()
		if err == nil {
			return info.Executable, &info, nil
		}
		path, err := currentInstallExecutable()
		if err != nil {
			return "", nil, err
		}
		return path, nil, nil

	default:
		return "", nil, fmt.Errorf("unknown restart policy: %d", policy)
	}
}

func RestartActiveRuntime() (int, error) {
	runtimeInfo, err := ReadActiveRuntimeInfo()
	if err != nil {
		logger.Error("[update] restart active runtime failed to read active runtime: err=%v", err)
		return 0, err
	}
	logger.Info("[update] restart active runtime signaling master: master_pid=%d executable=%s", runtimeInfo.PID, runtimeInfo.Executable)
	if err := defaultSignalProcess(runtimeInfo.PID); err != nil {
		logger.Error("[update] restart active runtime signal failed: master_pid=%d err=%v", runtimeInfo.PID, err)
		return 0, fmt.Errorf("signal master restart failed: %w", err)
	}
	logger.Info("[update] restart active runtime signal sent: master_pid=%d", runtimeInfo.PID)
	return runtimeInfo.PID, nil
}

// InstallLatestRelease is the background auto-update entry point. It requires
// an active daemon master, downloads the latest stable release from GitHub, and
// signals the master to restart after installation.
func InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().InstallLatestRelease(currentVersion, goos, goarch)
}

func (c Client) InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	logger.Info("[update] auto install requested: current=%s target=%s/%s", versionLabel(currentVersion), goos, goarch)
	result, err := c.Install(InstallSource{Kind: ArchiveSourceRemote}, currentVersion, goos, goarch, RestartRequireMaster)
	if err != nil {
		logger.Error("[update] auto install failed: current=%s err=%v", versionLabel(currentVersion), err)
	} else {
		logger.Info("[update] auto install done: installed=%t master_pid=%d", result.Installed, result.RestartedPID)
	}
	return result, err
}

// InstallLatestReleaseCLI is the CLI upgrade entry point. It downloads the
// latest stable release from GitHub, installs it to the current executable, and
// restarts the daemon master if it is running the same executable.
func InstallLatestReleaseCLI(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().InstallLatestReleaseCLI(currentVersion, goos, goarch)
}

func (c Client) InstallLatestReleaseCLI(currentVersion string, goos string, goarch string) (InstallResult, error) {
	logger.Info("[update] cli install requested: current=%s target=%s/%s", versionLabel(currentVersion), goos, goarch)
	result, err := c.Install(InstallSource{Kind: ArchiveSourceRemote}, currentVersion, goos, goarch, RestartIfMatchingMaster)
	if err != nil {
		logger.Error("[update] cli install failed: current=%s err=%v", versionLabel(currentVersion), err)
	} else {
		logger.Info("[update] cli install done: installed=%t master_pid=%d", result.Installed, result.RestartedPID)
	}
	return result, err
}

// InstallLocalReleaseArchive is the admin upload entry point. It validates the
// uploaded archive name, then installs via the unified core: using the daemon
// master when available, otherwise replacing the current executable directly.
func InstallLocalReleaseArchive(archiveName string, archivePath string, currentVersion string, goos string, goarch string) (InstallResult, error) {
	archiveName = strings.TrimSpace(archiveName)
	archivePath = strings.TrimSpace(archivePath)
	result := InstallResult{
		CurrentVersion: strings.TrimSpace(currentVersion),
		ArchiveName:    filepath.Base(archiveName),
	}
	if archivePath == "" {
		logger.Warn("[update] manual install rejected: archive path is empty name=%s", result.ArchiveName)
		return result, fmt.Errorf("local release archive path is required")
	}
	logger.Info("[update] manual install requested: current=%s archive=%s target=%s/%s", versionLabel(result.CurrentVersion), result.ArchiveName, goos, goarch)

	version, err := validateLocalArchiveName(result.ArchiveName, goos, goarch)
	if err != nil {
		logger.Warn("[update] manual install validation failed: archive=%s target=%s/%s err=%v", result.ArchiveName, goos, goarch, err)
		return result, err
	}

	source := InstallSource{
		Kind:        ArchiveSourceLocal,
		ArchiveName: archiveName,
		ArchivePath: archivePath,
		Version:     version,
	}
	result, err = DefaultClient().Install(source, currentVersion, goos, goarch, RestartWithMasterFallback)
	if err != nil {
		logger.Error("[update] manual install failed: archive=%s err=%v", filepath.Base(archiveName), err)
	} else {
		logger.Info("[update] manual install done: installed=%t master_pid=%d", result.Installed, result.RestartedPID)
	}
	return result, err
}

func archiveSourceLabel(kind ArchiveSource) string {
	switch kind {
	case ArchiveSourceRemote:
		return "remote"
	case ArchiveSourceLocal:
		return "local"
	default:
		return "unknown"
	}
}

func restartPolicyLabel(policy RestartPolicy) string {
	switch policy {
	case RestartRequireMaster:
		return "require-master"
	case RestartIfMatchingMaster:
		return "if-matching-master"
	case RestartWithMasterFallback:
		return "with-master-fallback"
	default:
		return "unknown"
	}
}

func versionLabel(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return "unknown"
	}
	return version
}

func (c Client) downloadToFile(rawURL string, dstPath string) error {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fmt.Errorf("download url is required")
	}

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/octet-stream")
	req.Header.Set("User-Agent", "swaves/"+buildUserAgentVersion())

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed: status=%d url=%s", resp.StatusCode, rawURL)
	}

	file, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("create download file failed: %w", err)
	}
	defer func() { _ = file.Close() }()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("write download file failed: %w", err)
	}
	return nil
}

func verifyChecksumFile(archivePath string, checksumPath string) error {
	checksumData, err := os.ReadFile(checksumPath)
	if err != nil {
		return fmt.Errorf("read checksum file failed: %w", err)
	}
	fields := strings.Fields(string(checksumData))
	if len(fields) < 1 {
		return fmt.Errorf("checksum file is empty")
	}
	expected := strings.ToLower(strings.TrimSpace(fields[0]))
	if len(expected) != sha256.Size*2 {
		return fmt.Errorf("checksum value is invalid")
	}

	archiveData, err := os.ReadFile(archivePath)
	if err != nil {
		return fmt.Errorf("read archive file failed: %w", err)
	}
	actual := sha256.Sum256(archiveData)
	if hex.EncodeToString(actual[:]) != expected {
		return fmt.Errorf("archive checksum mismatch")
	}
	return nil
}

func extractReleaseBinary(archivePath string, dstDir string, expectedName string) (string, error) {
	archiveFile, err := os.Open(archivePath)
	if err != nil {
		return "", fmt.Errorf("open archive failed: %w", err)
	}
	defer func() { _ = archiveFile.Close() }()

	gzipReader, err := gzip.NewReader(archiveFile)
	if err != nil {
		return "", fmt.Errorf("open gzip archive failed: %w", err)
	}
	defer func() { _ = gzipReader.Close() }()

	expectedName = strings.TrimSpace(expectedName)
	if expectedName == "" {
		return "", fmt.Errorf("release binary name is required")
	}

	tarReader := tar.NewReader(gzipReader)
	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("read tar archive failed: %w", err)
		}
		if header == nil || header.Typeflag != tar.TypeReg {
			continue
		}

		name := filepath.Base(strings.TrimSpace(header.Name))
		if name == "" || name == "." || name == string(filepath.Separator) || name != expectedName {
			continue
		}
		dstPath := filepath.Join(dstDir, name)
		out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
		if err != nil {
			return "", fmt.Errorf("create extracted binary failed: %w", err)
		}
		if _, err := io.Copy(out, tarReader); err != nil {
			_ = out.Close()
			return "", fmt.Errorf("write extracted binary failed: %w", err)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("close extracted binary failed: %w", err)
		}
		return dstPath, nil
	}

	return "", fmt.Errorf("release binary %q not found in archive", expectedName)
}

func defaultSignalProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGHUP)
}

func currentInstallExecutable() (string, error) {
	path, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("resolve current executable failed: %w", err)
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("current executable is empty")
	}
	return path, nil
}

func activeRuntimeForExecutable(targetPath string) (RuntimeInfo, bool) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return RuntimeInfo{}, false
	}

	runtimeInfo, err := ReadActiveRuntimeInfo()
	if err != nil {
		return RuntimeInfo{}, false
	}
	if strings.TrimSpace(runtimeInfo.Executable) != targetPath {
		return RuntimeInfo{}, false
	}
	return runtimeInfo, true
}

func replaceExecutableAtPathWithRollback(nextPath string, targetPath string, backupPath string) (executableRollback, error) {
	nextPath = strings.TrimSpace(nextPath)
	targetPath = strings.TrimSpace(targetPath)
	backupPath = strings.TrimSpace(backupPath)
	if nextPath == "" || targetPath == "" || backupPath == "" {
		return nil, fmt.Errorf("replace executable paths are required")
	}

	if err := os.Rename(targetPath, backupPath); err != nil {
		return nil, fmt.Errorf("backup executable failed: %w", err)
	}

	restore := func() error {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove replaced executable failed: %w", err)
		}
		if err := os.Rename(backupPath, targetPath); err != nil {
			return fmt.Errorf("restore previous executable failed: %w", err)
		}
		return nil
	}

	if err := os.Rename(nextPath, targetPath); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return nil, fmt.Errorf("replace executable failed: %w (rollback failed: %v)", err, restoreErr)
		}
		return nil, fmt.Errorf("replace executable failed: %w", err)
	}

	return restore, nil
}

func replaceExecutableWithRollback(nextPath string, runtimeInfo RuntimeInfo, backupPath string) (executableRollback, error) {
	if err := ensureRuntimeInstallTarget(runtimeInfo); err != nil {
		return nil, err
	}
	return replaceExecutableAtPathWithRollback(nextPath, runtimeInfo.Executable, backupPath)
}

func ensureRuntimeInstallTarget(expected RuntimeInfo) error {
	expected.Executable = strings.TrimSpace(expected.Executable)
	if expected.PID <= 0 {
		return fmt.Errorf("runtime pid is required")
	}
	if expected.Executable == "" {
		return fmt.Errorf("runtime executable is required")
	}

	active, err := ReadActiveRuntimeInfo()
	if err != nil {
		return fmt.Errorf("revalidate active master failed: %w", err)
	}
	if active.PID != expected.PID {
		return fmt.Errorf("revalidate active master failed: runtime pid changed")
	}
	return nil
}

func validateLocalArchiveName(archiveName string, goos string, goarch string) (string, error) {
	archiveName = filepath.Base(strings.TrimSpace(archiveName))
	if archiveName == "" {
		return "", fmt.Errorf("local release archive name is required")
	}
	if strings.TrimSpace(goos) == "" {
		goos = runtime.GOOS
	}
	if strings.TrimSpace(goarch) == "" {
		goarch = runtime.GOARCH
	}
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("automatic upgrade is not supported on %s/%s", goos, goarch)
	}
	if !strings.HasPrefix(archiveName, "swaves_") || !strings.HasSuffix(archiveName, ".tar.gz") {
		return "", fmt.Errorf("invalid local archive name: %s", archiveName)
	}

	trimmed := strings.TrimSuffix(strings.TrimPrefix(archiveName, "swaves_"), ".tar.gz")
	parts := strings.Split(trimmed, "_")
	if len(parts) < 3 {
		return "", fmt.Errorf("invalid local archive name: %s", archiveName)
	}

	version := strings.Join(parts[:len(parts)-2], "_")
	archiveGOOS := parts[len(parts)-2]
	archiveGOARCH := parts[len(parts)-1]
	if !semverutil.IsStable(version) {
		return "", fmt.Errorf("local archive version must be a stable semver tag: %s", version)
	}
	if archiveGOOS != goos || archiveGOARCH != goarch {
		return "", fmt.Errorf("local archive %s does not match current platform %s/%s", archiveName, goos, goarch)
	}
	if archiveName != ReleaseArchiveName(version, goos, goarch) {
		return "", fmt.Errorf("invalid local archive name: %s", archiveName)
	}
	return version, nil
}

func expectedReleaseBinaryName(archiveName string) string {
	return strings.TrimSuffix(filepath.Base(strings.TrimSpace(archiveName)), ".tar.gz")
}
