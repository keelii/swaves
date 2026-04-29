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

type InstallResult struct {
	CurrentVersion string
	LatestVersion  string
	ReleaseURL     string
	ArchiveName    string
	Installed      bool
	RestartedPID   int
	Reason         string
}

type installSourceKind string

const (
	installSourceLatestRelease installSourceKind = "latest_release"
	installSourceLocalArchive  installSourceKind = "local_archive"
)

type installSource struct {
	kind        installSourceKind
	archiveName string
	archivePath string
}

type restartMode string

const (
	restartNever               restartMode = "never"
	restartIfActiveTarget      restartMode = "if_active_target"
	restartRequireActiveTarget restartMode = "require_active_target"
)

type installRequest struct {
	currentVersion string
	goos           string
	goarch         string
	source         installSource
	restart        restartMode
}

type preparedInstall struct {
	latestVersion       string
	releaseURL          string
	archiveName         string
	archivePath         string
	expectedBinaryName  string
	reason              string
	downloadArchiveURL  string
	downloadChecksumURL string
}

var (
	installMu sync.Mutex

	readActiveRuntimeInfoFunc    = readActiveRuntimeInfo
	currentInstallExecutableFunc = currentInstallExecutable
	signalProcessFunc            = defaultSignalProcess
)

type executableRollback func() error

func InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().installRelease(installRequest{
		currentVersion: currentVersion,
		goos:           goos,
		goarch:         goarch,
		source:         installSource{kind: installSourceLatestRelease},
		restart:        restartRequireActiveTarget,
	})
}

func RestartActiveRuntime() (int, error) {
	runtimeInfo, err := readActiveRuntimeInfoFunc()
	if err != nil {
		logger.Error("[update] restart active runtime failed to read active runtime: err=%v", err)
		return 0, err
	}
	logger.Info("[update] restart active runtime signaling master: master_pid=%d executable=%s", runtimeInfo.PID, runtimeInfo.Executable)
	if err := signalProcessFunc(runtimeInfo.PID); err != nil {
		logger.Error("[update] restart active runtime signal failed: master_pid=%d err=%v", runtimeInfo.PID, err)
		return 0, fmt.Errorf("signal master restart failed: %w", err)
	}
	logger.Info("[update] restart active runtime signal sent: master_pid=%d", runtimeInfo.PID)
	return runtimeInfo.PID, nil
}

func InstallLatestReleaseCLI(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().installRelease(installRequest{
		currentVersion: currentVersion,
		goos:           goos,
		goarch:         goarch,
		source:         installSource{kind: installSourceLatestRelease},
		restart:        restartIfActiveTarget,
	})
}

func InstallLocalReleaseArchive(archiveName string, archivePath string, currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().installRelease(installRequest{
		currentVersion: currentVersion,
		goos:           goos,
		goarch:         goarch,
		source: installSource{
			kind:        installSourceLocalArchive,
			archiveName: archiveName,
			archivePath: archivePath,
		},
		restart: restartIfActiveTarget,
	})
}

func (c Client) InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return c.installRelease(installRequest{
		currentVersion: currentVersion,
		goos:           goos,
		goarch:         goarch,
		source:         installSource{kind: installSourceLatestRelease},
		restart:        restartRequireActiveTarget,
	})
}

func (c Client) InstallLatestReleaseCLI(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return c.installRelease(installRequest{
		currentVersion: currentVersion,
		goos:           goos,
		goarch:         goarch,
		source:         installSource{kind: installSourceLatestRelease},
		restart:        restartIfActiveTarget,
	})
}

func (c Client) installRelease(req installRequest) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

	result := InstallResult{CurrentVersion: strings.TrimSpace(req.currentVersion)}
	goos := strings.TrimSpace(req.goos)
	if goos == "" {
		goos = runtime.GOOS
	}
	goarch := strings.TrimSpace(req.goarch)
	if goarch == "" {
		goarch = runtime.GOARCH
	}

	logger.Info("[update] install start: source=%s current=%s target=%s/%s restart=%s", req.source.kind, versionLabel(result.CurrentVersion), goos, goarch, req.restart)
	prepared, err := c.prepareInstall(req.source, result.CurrentVersion, goos, goarch)
	if err != nil {
		logger.Error("[update] install prepare failed: source=%s target=%s/%s err=%v", req.source.kind, goos, goarch, err)
		return result, err
	}
	result.LatestVersion = prepared.latestVersion
	result.ReleaseURL = prepared.releaseURL
	result.ArchiveName = prepared.archiveName

	if prepared.reason != "" && prepared.archivePath == "" && prepared.downloadArchiveURL == "" {
		result.Reason = prepared.reason
		logger.Info("[update] install skipped: source=%s current=%s latest=%s reason=%s", req.source.kind, versionLabel(result.CurrentVersion), versionLabel(result.LatestVersion), prepared.reason)
		return result, nil
	}

	targetPath, err := currentInstallExecutableFunc()
	if err != nil {
		logger.Error("[update] install resolve current executable failed: err=%v", err)
		return result, err
	}

	activeRuntime, activeErr := activeRuntimeForTarget(targetPath)
	if req.restart == restartRequireActiveTarget && activeErr != nil {
		logger.Warn("[update] install requires active master: target=%s err=%v", targetPath, activeErr)
		return result, fmt.Errorf("automatic upgrade requires daemon-mode=1 and an active master process: %w", activeErr)
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".swaves-upgrade-")
	if err != nil {
		logger.Error("[update] install create temp dir failed: target=%s err=%v", targetPath, err)
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := strings.TrimSpace(prepared.archivePath)
	if strings.TrimSpace(prepared.downloadArchiveURL) != "" {
		archivePath = filepath.Join(tmpDir, prepared.archiveName)
		logger.Info("[update] install downloading archive: source=%s url=%s dst=%s", req.source.kind, prepared.downloadArchiveURL, archivePath)
		if err := c.downloadToFile(prepared.downloadArchiveURL, archivePath); err != nil {
			logger.Error("[update] install download archive failed: source=%s url=%s err=%v", req.source.kind, prepared.downloadArchiveURL, err)
			return result, err
		}

		checksumPath := filepath.Join(tmpDir, prepared.archiveName+".sha256")
		logger.Info("[update] install downloading checksum: source=%s url=%s dst=%s", req.source.kind, prepared.downloadChecksumURL, checksumPath)
		if err := c.downloadToFile(prepared.downloadChecksumURL, checksumPath); err != nil {
			logger.Error("[update] install download checksum failed: source=%s url=%s err=%v", req.source.kind, prepared.downloadChecksumURL, err)
			return result, err
		}
		if err := verifyChecksumFile(archivePath, checksumPath); err != nil {
			logger.Error("[update] install checksum verify failed: source=%s archive=%s checksum=%s err=%v", req.source.kind, archivePath, checksumPath, err)
			return result, err
		}
	}

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, prepared.expectedBinaryName)
	if err != nil {
		logger.Error("[update] install extract failed: source=%s archive=%s err=%v", req.source.kind, archivePath, err)
		return result, err
	}
	if err := os.Chmod(extractedPath, 0o755); err != nil {
		logger.Error("[update] install chmod failed: source=%s path=%s err=%v", req.source.kind, extractedPath, err)
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}

	logger.Info("[update] install replacing current executable: source=%s target=%s", req.source.kind, targetPath)
	rollback, err := replaceExecutableAtPathWithRollback(extractedPath, targetPath, filepath.Join(tmpDir, ".swaves-executable-backup"))
	if err != nil {
		logger.Error("[update] install replace current executable failed: source=%s target=%s err=%v", req.source.kind, targetPath, err)
		return result, err
	}

	result.Installed = true
	if req.restart == restartRequireActiveTarget || (req.restart == restartIfActiveTarget && activeErr == nil) {
		logger.Info("[update] install restarting active master: source=%s master_pid=%d target=%s", req.source.kind, activeRuntime.PID, targetPath)
		if err := signalProcessFunc(activeRuntime.PID); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				logger.Error("[update] install restart failed and rollback failed: source=%s master_pid=%d err=%v rollback_err=%v", req.source.kind, activeRuntime.PID, err, rollbackErr)
				return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
			}
			logger.Error("[update] install restart failed: source=%s master_pid=%d err=%v", req.source.kind, activeRuntime.PID, err)
			return result, fmt.Errorf("signal master restart failed: %w", err)
		}
		result.RestartedPID = activeRuntime.PID
	}

	result.Reason = installSuccessReason(req.source.kind, prepared.latestVersion, result.RestartedPID > 0)
	logger.Info("[update] install success: source=%s latest=%s target=%s master_pid=%d", req.source.kind, versionLabel(result.LatestVersion), targetPath, result.RestartedPID)
	return result, nil
}

func (c Client) prepareInstall(source installSource, currentVersion string, goos string, goarch string) (preparedInstall, error) {
	switch source.kind {
	case installSourceLatestRelease:
		return c.prepareLatestRelease(currentVersion, goos, goarch)
	case installSourceLocalArchive:
		return prepareLocalArchive(source.archiveName, source.archivePath, goos, goarch)
	default:
		return preparedInstall{}, fmt.Errorf("unsupported install source: %s", source.kind)
	}
}

func (c Client) prepareLatestRelease(currentVersion string, goos string, goarch string) (preparedInstall, error) {
	check, err := c.CheckLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		return preparedInstall{}, err
	}

	prepared := preparedInstall{
		latestVersion: check.LatestVersion,
		releaseURL:    check.LatestReleaseURL,
		reason:        strings.TrimSpace(check.Reason),
	}
	if check.Target == nil {
		return prepared, fmt.Errorf("automatic upgrade is not supported on %s/%s", goos, goarch)
	}
	prepared.archiveName = check.Target.Archive.Name
	prepared.expectedBinaryName = expectedReleaseBinaryName(check.Target.Archive.Name)

	if semverutil.IsStable(currentVersion) {
		cmp, err := semverutil.Compare(currentVersion, check.LatestVersion)
		if err != nil {
			return prepared, err
		}
		if cmp >= 0 {
			return prepared, nil
		}
	}

	prepared.downloadArchiveURL = check.Target.Archive.DownloadURL
	prepared.downloadChecksumURL = check.Target.Checksum.DownloadURL
	return prepared, nil
}

func prepareLocalArchive(archiveName string, archivePath string, goos string, goarch string) (preparedInstall, error) {
	archiveName = filepath.Base(strings.TrimSpace(archiveName))
	archivePath = strings.TrimSpace(archivePath)
	if archivePath == "" {
		return preparedInstall{archiveName: archiveName}, fmt.Errorf("local release archive path is required")
	}

	version, err := validateLocalArchiveName(archiveName, goos, goarch)
	if err != nil {
		return preparedInstall{archiveName: archiveName}, err
	}
	return preparedInstall{
		latestVersion:      version,
		archiveName:        archiveName,
		archivePath:        archivePath,
		expectedBinaryName: expectedReleaseBinaryName(archiveName),
	}, nil
}

func installSuccessReason(sourceKind installSourceKind, latestVersion string, restarted bool) string {
	latestVersion = strings.TrimSpace(latestVersion)
	if latestVersion == "" {
		latestVersion = "unknown version"
	}
	if restarted {
		return fmt.Sprintf("upgraded to %s", latestVersion)
	}
	if sourceKind == installSourceLatestRelease {
		return fmt.Sprintf("installed %s to current executable", latestVersion)
	}
	return fmt.Sprintf("installed %s to current executable", latestVersion)
}

func activeRuntimeForTarget(targetPath string) (RuntimeInfo, error) {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return RuntimeInfo{}, fmt.Errorf("install target is required")
	}

	runtimeInfo, err := readActiveRuntimeInfoFunc()
	if err != nil {
		return RuntimeInfo{}, err
	}
	if strings.TrimSpace(runtimeInfo.Executable) != targetPath {
		return RuntimeInfo{}, fmt.Errorf("active master executable does not match install target")
	}
	return runtimeInfo, nil
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
		out, err := os.OpenFile(dstPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
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
