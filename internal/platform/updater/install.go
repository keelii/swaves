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

var (
	installMu     sync.Mutex
	signalProcess = defaultSignalProcess
)

type executableRollback func() error

func InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().InstallLatestRelease(currentVersion, goos, goarch)
}

func RestartActiveRuntime() (int, error) {
	runtimeInfo, err := ReadActiveRuntimeInfo()
	if err != nil {
		logger.Error("[update] restart active runtime failed to read active runtime: err=%v", err)
		return 0, err
	}
	logger.Info("[update] restart active runtime signaling master: master_pid=%d executable=%s", runtimeInfo.PID, runtimeInfo.Executable)
	if err := signalProcess(runtimeInfo.PID); err != nil {
		logger.Error("[update] restart active runtime signal failed: master_pid=%d err=%v", runtimeInfo.PID, err)
		return 0, fmt.Errorf("signal master restart failed: %w", err)
	}
	logger.Info("[update] restart active runtime signal sent: master_pid=%d", runtimeInfo.PID)
	return runtimeInfo.PID, nil
}

func InstallLatestReleaseCLI(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().InstallLatestReleaseCLI(currentVersion, goos, goarch)
}

func InstallLocalReleaseArchive(archiveName string, archivePath string, currentVersion string, goos string, goarch string) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

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
	logger.Info("[update] manual install start: current=%s archive=%s target=%s/%s", versionLabel(result.CurrentVersion), result.ArchiveName, goos, goarch)

	version, err := validateLocalArchiveName(result.ArchiveName, goos, goarch)
	if err != nil {
		logger.Warn("[update] manual install validation failed: archive=%s target=%s/%s err=%v", result.ArchiveName, goos, goarch, err)
		return result, err
	}
	result.LatestVersion = version
	logger.Info("[update] manual install archive validated: archive=%s version=%s", result.ArchiveName, result.LatestVersion)

	runtimeInfo, err := ReadActiveRuntimeInfo()
	hasActiveMaster := err == nil
	if hasActiveMaster {
		logger.Info("[update] manual install using active master: master_pid=%d executable=%s", runtimeInfo.PID, runtimeInfo.Executable)
	} else {
		logger.Info("[update] manual install without daemon-mode master: err=%v", err)
	}

	installTargetDir, err := installTargetDirectory(runtimeInfo, hasActiveMaster)
	if err != nil {
		logger.Error("[update] manual install resolve target dir failed: archive=%s err=%v", result.ArchiveName, err)
		return result, err
	}

	tmpDir, err := os.MkdirTemp(installTargetDir, ".swaves-upgrade-")
	if err != nil {
		logger.Error("[update] manual install create temp dir failed: archive=%s dir=%s err=%v", result.ArchiveName, installTargetDir, err)
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, expectedReleaseBinaryName(result.ArchiveName))
	if err != nil {
		logger.Error("[update] manual install extract failed: archive=%s err=%v", result.ArchiveName, err)
		return result, err
	}
	logger.Info("[update] manual install extracted binary: archive=%s extracted=%s", result.ArchiveName, extractedPath)
	if err := os.Chmod(extractedPath, 0755); err != nil {
		logger.Error("[update] manual install chmod failed: path=%s err=%v", extractedPath, err)
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}

	if hasActiveMaster {
		logger.Info("[update] manual install replacing executable via active master: master_pid=%d", runtimeInfo.PID)
		rollback, err := replaceExecutableWithRollback(extractedPath, runtimeInfo, filepath.Join(tmpDir, ".swaves-executable-backup"))
		if err != nil {
			logger.Error("[update] manual install replace executable failed: archive=%s master_pid=%d err=%v", result.ArchiveName, runtimeInfo.PID, err)
			return result, err
		}
		if err := signalProcess(runtimeInfo.PID); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				logger.Error("[update] manual install restart signal failed and rollback failed: archive=%s master_pid=%d err=%v rollback_err=%v", result.ArchiveName, runtimeInfo.PID, err, rollbackErr)
				return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
			}
			logger.Error("[update] manual install restart signal failed: archive=%s master_pid=%d err=%v", result.ArchiveName, runtimeInfo.PID, err)
			return result, fmt.Errorf("signal master restart failed: %w", err)
		}

		result.Installed = true
		result.RestartedPID = runtimeInfo.PID
		result.Reason = fmt.Sprintf("upgraded to %s", version)
		logger.Info("[update] manual install success: version=%s master_pid=%d", result.LatestVersion, result.RestartedPID)
		return result, nil
	}

	targetPath, err := currentInstallExecutable()
	if err != nil {
		logger.Error("[update] manual install resolve current executable failed: archive=%s err=%v", result.ArchiveName, err)
		return result, err
	}
	logger.Info("[update] manual install replacing current executable without daemon-mode: target=%s", targetPath)
	if _, err := replaceExecutableAtPathWithRollback(extractedPath, targetPath, filepath.Join(tmpDir, ".swaves-executable-backup")); err != nil {
		logger.Error("[update] manual install replace current executable failed: archive=%s target=%s err=%v", result.ArchiveName, targetPath, err)
		return result, err
	}
	result.Installed = true
	result.Reason = fmt.Sprintf("installed %s, restart required", version)
	logger.Info("[update] manual install success: version=%s restart_required=true", result.LatestVersion)
	return result, nil
}

func (c Client) InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

	result := InstallResult{CurrentVersion: strings.TrimSpace(currentVersion)}
	logger.Info("[update] auto install start: current=%s target=%s/%s", versionLabel(result.CurrentVersion), goos, goarch)
	runtimeInfo, err := ReadActiveRuntimeInfo()
	if err != nil {
		logger.Warn("[update] auto install unavailable: err=%v", err)
		return result, fmt.Errorf("automatic upgrade requires daemon-mode=1 and an active master process: %w", err)
	}
	logger.Info("[update] auto install active master detected: master_pid=%d executable=%s", runtimeInfo.PID, runtimeInfo.Executable)

	check, err := c.CheckLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		logger.Error("[update] auto install release check failed: current=%s target=%s/%s err=%v", versionLabel(result.CurrentVersion), goos, goarch, err)
		return result, err
	}
	result.LatestVersion = check.LatestVersion
	result.ReleaseURL = check.LatestReleaseURL
	logger.Info("[update] auto install release check result: latest=%s archive=%s reason=%s", versionLabel(result.LatestVersion), func() string {
		if check.Target == nil {
			return ""
		}
		return check.Target.Archive.Name
	}(), strings.TrimSpace(check.Reason))
	if check.Target == nil {
		if strings.TrimSpace(goos) == "" {
			goos = runtime.GOOS
		}
		if strings.TrimSpace(goarch) == "" {
			goarch = runtime.GOARCH
		}
		logger.Warn("[update] auto install unsupported target: target=%s/%s", goos, goarch)
		return result, fmt.Errorf("automatic upgrade is not supported on %s/%s", goos, goarch)
	}
	result.ArchiveName = check.Target.Archive.Name

	stableCurrent := semverutil.IsStable(currentVersion)
	if stableCurrent {
		cmp, err := semverutil.Compare(currentVersion, check.LatestVersion)
		if err != nil {
			logger.Error("[update] auto install semver compare failed: current=%s latest=%s err=%v", currentVersion, check.LatestVersion, err)
			return result, err
		}
		if cmp >= 0 {
			result.Reason = check.Reason
			logger.Info("[update] auto install skipped: current=%s latest=%s reason=%s", versionLabel(result.CurrentVersion), versionLabel(result.LatestVersion), strings.TrimSpace(result.Reason))
			return result, nil
		}
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(runtimeInfo.Executable), ".swaves-upgrade-")
	if err != nil {
		logger.Error("[update] auto install create temp dir failed: executable=%s err=%v", runtimeInfo.Executable, err)
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := filepath.Join(tmpDir, check.Target.Archive.Name)
	logger.Info("[update] auto install downloading archive: url=%s dst=%s", check.Target.Archive.DownloadURL, archivePath)
	if err := c.downloadToFile(check.Target.Archive.DownloadURL, archivePath); err != nil {
		logger.Error("[update] auto install download archive failed: url=%s err=%v", check.Target.Archive.DownloadURL, err)
		return result, err
	}

	checksumPath := filepath.Join(tmpDir, check.Target.Checksum.Name)
	logger.Info("[update] auto install downloading checksum: url=%s dst=%s", check.Target.Checksum.DownloadURL, checksumPath)
	if err := c.downloadToFile(check.Target.Checksum.DownloadURL, checksumPath); err != nil {
		logger.Error("[update] auto install download checksum failed: url=%s err=%v", check.Target.Checksum.DownloadURL, err)
		return result, err
	}
	if err := verifyChecksumFile(archivePath, checksumPath); err != nil {
		logger.Error("[update] auto install checksum verify failed: archive=%s checksum=%s err=%v", archivePath, checksumPath, err)
		return result, err
	}
	logger.Info("[update] auto install checksum verified: archive=%s", archivePath)

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, expectedReleaseBinaryName(check.Target.Archive.Name))
	if err != nil {
		logger.Error("[update] auto install extract failed: archive=%s err=%v", archivePath, err)
		return result, err
	}
	logger.Info("[update] auto install extracted binary: path=%s", extractedPath)
	if err := os.Chmod(extractedPath, 0755); err != nil {
		logger.Error("[update] auto install chmod failed: path=%s err=%v", extractedPath, err)
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}
	logger.Info("[update] auto install replacing executable: master_pid=%d target=%s", runtimeInfo.PID, runtimeInfo.Executable)
	rollback, err := replaceExecutableWithRollback(extractedPath, runtimeInfo, filepath.Join(tmpDir, ".swaves-executable-backup"))
	if err != nil {
		logger.Error("[update] auto install replace executable failed: master_pid=%d err=%v", runtimeInfo.PID, err)
		return result, err
	}
	if err := signalProcess(runtimeInfo.PID); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			logger.Error("[update] auto install restart signal failed and rollback failed: master_pid=%d err=%v rollback_err=%v", runtimeInfo.PID, err, rollbackErr)
			return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
		}
		logger.Error("[update] auto install restart signal failed: master_pid=%d err=%v", runtimeInfo.PID, err)
		return result, fmt.Errorf("signal master restart failed: %w", err)
	}

	result.Installed = true
	result.RestartedPID = runtimeInfo.PID
	result.Reason = fmt.Sprintf("upgraded to %s", check.LatestVersion)
	logger.Info("[update] auto install success: version=%s master_pid=%d", versionLabel(result.LatestVersion), result.RestartedPID)
	return result, nil
}

func (c Client) InstallLatestReleaseCLI(currentVersion string, goos string, goarch string) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

	result := InstallResult{CurrentVersion: strings.TrimSpace(currentVersion)}
	logger.Info("[update] cli install start: current=%s target=%s/%s", versionLabel(result.CurrentVersion), goos, goarch)

	check, err := c.CheckLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		logger.Error("[update] cli install release check failed: current=%s target=%s/%s err=%v", versionLabel(result.CurrentVersion), goos, goarch, err)
		return result, err
	}
	result.LatestVersion = check.LatestVersion
	result.ReleaseURL = check.LatestReleaseURL
	logger.Info("[update] cli install release check result: latest=%s archive=%s reason=%s", versionLabel(result.LatestVersion), func() string {
		if check.Target == nil {
			return ""
		}
		return check.Target.Archive.Name
	}(), strings.TrimSpace(check.Reason))
	if check.Target == nil {
		if strings.TrimSpace(goos) == "" {
			goos = runtime.GOOS
		}
		if strings.TrimSpace(goarch) == "" {
			goarch = runtime.GOARCH
		}
		logger.Warn("[update] cli install unsupported target: target=%s/%s", goos, goarch)
		return result, fmt.Errorf("automatic upgrade is not supported on %s/%s", goos, goarch)
	}
	result.ArchiveName = check.Target.Archive.Name

	stableCurrent := semverutil.IsStable(currentVersion)
	if stableCurrent {
		cmp, err := semverutil.Compare(currentVersion, check.LatestVersion)
		if err != nil {
			logger.Error("[update] cli install semver compare failed: current=%s latest=%s err=%v", currentVersion, check.LatestVersion, err)
			return result, err
		}
		if cmp >= 0 {
			result.Reason = check.Reason
			logger.Info("[update] cli install skipped: current=%s latest=%s reason=%s", versionLabel(result.CurrentVersion), versionLabel(result.LatestVersion), strings.TrimSpace(result.Reason))
			return result, nil
		}
	}

	targetPath, err := currentInstallExecutable()
	if err != nil {
		logger.Error("[update] cli install resolve current executable failed: err=%v", err)
		return result, err
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(targetPath), ".swaves-upgrade-")
	if err != nil {
		logger.Error("[update] cli install create temp dir failed: target=%s err=%v", targetPath, err)
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := filepath.Join(tmpDir, check.Target.Archive.Name)
	logger.Info("[update] cli install downloading archive: url=%s dst=%s", check.Target.Archive.DownloadURL, archivePath)
	if err := c.downloadToFile(check.Target.Archive.DownloadURL, archivePath); err != nil {
		logger.Error("[update] cli install download archive failed: url=%s err=%v", check.Target.Archive.DownloadURL, err)
		return result, err
	}

	checksumPath := filepath.Join(tmpDir, check.Target.Checksum.Name)
	logger.Info("[update] cli install downloading checksum: url=%s dst=%s", check.Target.Checksum.DownloadURL, checksumPath)
	if err := c.downloadToFile(check.Target.Checksum.DownloadURL, checksumPath); err != nil {
		logger.Error("[update] cli install download checksum failed: url=%s err=%v", check.Target.Checksum.DownloadURL, err)
		return result, err
	}
	if err := verifyChecksumFile(archivePath, checksumPath); err != nil {
		logger.Error("[update] cli install checksum verify failed: archive=%s checksum=%s err=%v", archivePath, checksumPath, err)
		return result, err
	}

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, expectedReleaseBinaryName(check.Target.Archive.Name))
	if err != nil {
		logger.Error("[update] cli install extract failed: archive=%s err=%v", archivePath, err)
		return result, err
	}
	if err := os.Chmod(extractedPath, 0755); err != nil {
		logger.Error("[update] cli install chmod failed: path=%s err=%v", extractedPath, err)
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}
	logger.Info("[update] cli install replacing current executable: target=%s", targetPath)
	if _, err := replaceExecutableAtPathWithRollback(extractedPath, targetPath, filepath.Join(tmpDir, ".swaves-executable-backup")); err != nil {
		logger.Error("[update] cli install replace current executable failed: target=%s err=%v", targetPath, err)
		return result, err
	}

	result.Installed = true
	result.Reason = fmt.Sprintf("installed %s to current executable", check.LatestVersion)
	logger.Info("[update] cli install success: version=%s target=%s", versionLabel(result.LatestVersion), targetPath)
	return result, nil
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

func installTargetDirectory(runtimeInfo RuntimeInfo, hasActiveMaster bool) (string, error) {
	if hasActiveMaster {
		return filepath.Dir(runtimeInfo.Executable), nil
	}
	targetPath, err := currentInstallExecutable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(targetPath), nil
}

func currentInstallExecutable() (string, error) {
	path, err := currentExecutable()
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
