package updater

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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

// ArchiveSource 标识二进制归档的来源类型。
type ArchiveSource int

const (
	// ArchiveSourceRemote 从 GitHub Release 下载归档。
	ArchiveSourceRemote ArchiveSource = iota
	// ArchiveSourceLocal 使用已在磁盘上的本地归档文件。
	ArchiveSourceLocal
)

// InstallSource 描述一次安装所需的归档信息。
// ArchiveSourceRemote：只需设置 Kind，Release 目标在 Install 执行时从 GitHub 解析。
// ArchiveSourceLocal：需同时设置 Kind、ArchiveName、ArchivePath 和 Version。
type InstallSource struct {
	Kind        ArchiveSource
	ArchiveName string // 归档文件名，如 swaves_v1.2.3_linux_amd64.tar.gz
	ArchivePath string // 磁盘路径，ArchiveSourceLocal 时必填
	Version     string // 目标版本标签
}

// RestartPolicy 控制安装完成后如何处理正在运行的 daemon master。
type RestartPolicy int

const (
	// RestartRequireMaster 要求存在活跃的 daemon master，否则直接失败。
	// 安装到 master 的可执行文件路径，完成后始终发送重启信号。
	RestartRequireMaster RestartPolicy = iota
	// RestartIfMatchingMaster 安装到当前可执行文件路径。
	// 仅当活跃 master 的可执行路径与安装路径一致时才发送重启信号。
	RestartIfMatchingMaster
	// RestartWithMasterFallback 优先使用活跃 master：安装到 master 的可执行路径并重启；
	// 若无活跃 master，则安装到当前可执行文件路径，不触发重启。
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
	installMu             sync.Mutex
	installSignalProcess  = defaultSignalProcess
	installCurrentExePath = currentExecutablePath
)

type executableRollback func() error

type installTarget struct {
	Path        string
	RuntimeInfo *RuntimeInfo
}

func (t installTarget) RestartPID() int {
	if t.RuntimeInfo == nil {
		return 0
	}
	return t.RuntimeInfo.PID
}

type releasePackage struct {
	Source       ArchiveSource
	Version      string
	ArchiveName  string
	ArchivePath  string
	ArchiveURL   string
	ChecksumName string
	ChecksumURL  string
}

// Install 解析安装目标和发布包，然后执行解包、替换和按需重启。
func (c Client) Install(source InstallSource, currentVersion string, goos string, goarch string, policy RestartPolicy) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

	currentVersion = strings.TrimSpace(currentVersion)
	result := InstallResult{CurrentVersion: currentVersion}
	logger.Info("[update] install start: source=%s current=%s target=%s/%s policy=%s",
		archiveSourceLabel(source.Kind), versionLabel(currentVersion), goos, goarch, restartPolicyLabel(policy))

	targetPath, restartRuntimeInfo, err := resolveInstallTarget(policy)
	if err != nil {
		logger.Warn("[update] install target resolution failed: policy=%s err=%v", restartPolicyLabel(policy), err)
		return result, err
	}
	target := installTarget{Path: targetPath, RuntimeInfo: restartRuntimeInfo}
	logger.Info("[update] install target resolved: target=%s restart_pid=%d", target.Path, target.RestartPID())

	pkg, preparedResult, skipped, err := c.prepareInstallPackage(source, currentVersion, goos, goarch, result)
	if err != nil {
		return result, err
	}
	result = preparedResult
	if skipped {
		return result, nil
	}

	tmpDir, err := CreateUpgradeTempDir(".swaves-upgrade-")
	if err != nil {
		logger.Error("[update] install create temp dir failed: err=%v", err)
		return result, err
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	if pkg.Source == ArchiveSourceRemote {
		pkg, err = c.downloadReleasePackage(pkg, tmpDir)
		if err != nil {
			return result, err
		}
	}

	return installPreparedPackage(result, pkg, target, tmpDir)
}

func (c Client) prepareInstallPackage(source InstallSource, currentVersion string, goos string, goarch string, result InstallResult) (releasePackage, InstallResult, bool, error) {
	switch source.Kind {
	case ArchiveSourceRemote:
		return c.prepareRemotePackage(currentVersion, goos, goarch, result)
	case ArchiveSourceLocal:
		return prepareLocalPackage(source, result)
	default:
		return releasePackage{}, result, false, fmt.Errorf("unknown archive source: %d", source.Kind)
	}
}

func (c Client) prepareRemotePackage(currentVersion string, goos string, goarch string, result InstallResult) (releasePackage, InstallResult, bool, error) {
	check, err := c.CheckLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		logger.Error("[update] install release check failed: current=%s target=%s/%s err=%v", versionLabel(currentVersion), goos, goarch, err)
		return releasePackage{}, result, false, err
	}

	result.LatestVersion = check.LatestVersion
	result.ReleaseURL = check.LatestReleaseURL
	if check.Target == nil {
		resolvedGOOS, resolvedGOARCH := releasePlatform(goos, goarch)
		logger.Warn("[update] install unsupported target: target=%s/%s", resolvedGOOS, resolvedGOARCH)
		return releasePackage{}, result, false, fmt.Errorf("automatic upgrade is not supported on %s/%s", resolvedGOOS, resolvedGOARCH)
	}

	result.ArchiveName = check.Target.Archive.Name
	logger.Info("[update] install release check result: latest=%s archive=%s reason=%s",
		versionLabel(result.LatestVersion), result.ArchiveName, strings.TrimSpace(check.Reason))

	if !check.HasUpgrade {
		result.Reason = check.Reason
		if restartedPID := restartAlreadyInstalledRuntime(); restartedPID > 0 {
			result.RestartedPID = restartedPID
			result.Reason = "current executable is already upgraded, restart requested"
		}
		logger.Info("[update] install skipped: current=%s latest=%s reason=%s", versionLabel(currentVersion), versionLabel(result.LatestVersion), strings.TrimSpace(result.Reason))
		return releasePackage{}, result, true, nil
	}

	return releasePackage{
		Source:       ArchiveSourceRemote,
		Version:      check.LatestVersion,
		ArchiveName:  check.Target.Archive.Name,
		ArchiveURL:   check.Target.Archive.DownloadURL,
		ChecksumName: check.Target.Checksum.Name,
		ChecksumURL:  check.Target.Checksum.DownloadURL,
	}, result, false, nil
}

func prepareLocalPackage(source InstallSource, result InstallResult) (releasePackage, InstallResult, bool, error) {
	pkg := releasePackage{
		Source:      ArchiveSourceLocal,
		Version:     source.Version,
		ArchiveName: filepath.Base(source.ArchiveName),
		ArchivePath: source.ArchivePath,
	}
	result.ArchiveName = pkg.ArchiveName
	result.LatestVersion = pkg.Version
	logger.Info("[update] install local archive: archive=%s version=%s path=%s", result.ArchiveName, result.LatestVersion, pkg.ArchivePath)
	return pkg, result, false, nil
}

func (c Client) downloadReleasePackage(pkg releasePackage, tmpDir string) (releasePackage, error) {
	archivePath := filepath.Join(tmpDir, pkg.ArchiveName)
	logger.Info("[update] install downloading archive: url=%s dst=%s", pkg.ArchiveURL, archivePath)
	if err := c.downloadToFile(pkg.ArchiveURL, archivePath); err != nil {
		logger.Error("[update] install download archive failed: url=%s err=%v", pkg.ArchiveURL, err)
		return pkg, err
	}

	checksumPath := filepath.Join(tmpDir, pkg.ChecksumName)
	logger.Info("[update] install downloading checksum: url=%s dst=%s", pkg.ChecksumURL, checksumPath)
	if err := c.downloadToFile(pkg.ChecksumURL, checksumPath); err != nil {
		logger.Error("[update] install download checksum failed: url=%s err=%v", pkg.ChecksumURL, err)
		return pkg, err
	}
	if err := verifyChecksumFile(archivePath, checksumPath); err != nil {
		logger.Error("[update] install checksum verify failed: archive=%s err=%v", archivePath, err)
		return pkg, err
	}
	logger.Info("[update] install checksum verified: archive=%s", archivePath)

	pkg.ArchivePath = archivePath
	return pkg, nil
}

func installPreparedPackage(result InstallResult, pkg releasePackage, target installTarget, tmpDir string) (InstallResult, error) {
	extractedPath, err := extractReleaseBinary(pkg.ArchivePath, tmpDir, expectedReleaseBinaryName(pkg.ArchiveName))
	if err != nil {
		logger.Error("[update] install extract failed: archive=%s err=%v", pkg.ArchivePath, err)
		return result, err
	}
	logger.Info("[update] install extracted binary: path=%s", extractedPath)
	if err := os.Chmod(extractedPath, 0755); err != nil {
		logger.Error("[update] install chmod failed: path=%s err=%v", extractedPath, err)
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}

	backupPath := filepath.Join(tmpDir, ".swaves-executable-backup")
	var rollback executableRollback
	if target.RuntimeInfo != nil {
		logger.Info("[update] install replacing executable via master: master_pid=%d target=%s", target.RestartPID(), target.Path)
		rollback, err = replaceExecutableWithRollback(extractedPath, *target.RuntimeInfo, backupPath)
	} else {
		logger.Info("[update] install replacing executable: target=%s", target.Path)
		rollback, err = replaceExecutableAtPathWithRollback(extractedPath, target.Path, backupPath)
	}
	if err != nil {
		logger.Error("[update] install replace executable failed: target=%s err=%v", target.Path, err)
		return result, err
	}

	restartPID := target.RestartPID()
	if restartPID > 0 {
		if err := installSignalProcess(restartPID); err != nil {
			if rollbackErr := rollback(); rollbackErr != nil {
				logger.Error("[update] install restart signal failed and rollback failed: master_pid=%d err=%v rollback_err=%v", restartPID, err, rollbackErr)
				return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
			}
			logger.Error("[update] install restart signal failed: master_pid=%d err=%v", restartPID, err)
			return result, fmt.Errorf("signal master restart failed: %w", err)
		}
		result.RestartedPID = restartPID
		result.Reason = fmt.Sprintf("upgraded to %s", pkg.Version)
		logger.Info("[update] install success: version=%s master_pid=%d", versionLabel(result.LatestVersion), restartPID)
	} else {
		result.Reason = fmt.Sprintf("installed %s, restart required", pkg.Version)
		logger.Info("[update] install success: version=%s restart_required=true", versionLabel(result.LatestVersion))
	}
	result.Installed = true
	return result, nil
}

func restartAlreadyInstalledRuntime() int {
	info, err := ReadActiveRuntimeInfo()
	if err != nil {
		return 0
	}
	currentPath, err := currentInstallExecutable()
	if err != nil {
		return 0
	}
	if !sameExecutablePath(info.Executable, currentPath) {
		return 0
	}
	if err := installSignalProcess(info.PID); err != nil {
		logger.Warn("[update] already upgraded runtime restart failed: master_pid=%d err=%v", info.PID, err)
		return 0
	}
	logger.Info("[update] already upgraded runtime restart requested: master_pid=%d", info.PID)
	return info.PID
}

func releasePlatform(goos string, goarch string) (string, string) {
	goos = strings.TrimSpace(goos)
	goarch = strings.TrimSpace(goarch)
	if goos == "" {
		goos = runtime.GOOS
	}
	if goarch == "" {
		goarch = runtime.GOARCH
	}
	return goos, goarch
}

// resolveInstallTarget 根据重启策略解析目标可执行文件路径及（可选的）待重启
// master 的 RuntimeInfo。返回值中 RuntimeInfo 非 nil 表示需要触发重启。
func resolveInstallTarget(policy RestartPolicy) (targetPath string, ri *RuntimeInfo, err error) {
	switch policy {
	case RestartRequireMaster:
		info, err := ReadActiveRuntimeInfo()
		if err != nil {
			if errors.Is(err, ErrRuntimeInfoNotFound) {
				return "", nil, fmt.Errorf("automatic upgrade requires daemon-mode=1: no active master found: %w", err)
			}
			if errors.Is(err, ErrMasterNotRunning) {
				return "", nil, fmt.Errorf("automatic upgrade requires an active master process: master has stopped: %w", err)
			}
			return "", nil, fmt.Errorf("automatic upgrade failed to read active master: %w", err)
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
		logger.Warn("[update] active master unavailable, fallback to current executable: err=%v", err)
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
	if err := installSignalProcess(runtimeInfo.PID); err != nil {
		logger.Error("[update] restart active runtime signal failed: master_pid=%d err=%v", runtimeInfo.PID, err)
		return 0, fmt.Errorf("signal master restart failed: %w", err)
	}
	logger.Info("[update] restart active runtime signal sent: master_pid=%d", runtimeInfo.PID)
	return runtimeInfo.PID, nil
}

// InstallLatestRelease 是后台自动更新入口。从 GitHub 下载最新稳定版本；
// 有活跃 master 时安装后重启，否则安装到当前可执行文件并要求手动重启。
func InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	return DefaultClient().InstallLatestRelease(currentVersion, goos, goarch)
}

func (c Client) InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	logger.Info("[update] auto install requested: current=%s target=%s/%s", versionLabel(currentVersion), goos, goarch)
	result, err := c.Install(InstallSource{Kind: ArchiveSourceRemote}, currentVersion, goos, goarch, RestartWithMasterFallback)
	if err != nil {
		logger.Error("[update] auto install failed: current=%s err=%v", versionLabel(currentVersion), err)
	} else {
		logger.Info("[update] auto install done: installed=%t master_pid=%d", result.Installed, result.RestartedPID)
	}
	return result, err
}

// InstallLatestReleaseCLI 是命令行升级入口。从 GitHub 下载最新稳定版本，
// 安装到当前可执行文件路径；若活跃 master 的可执行路径与之一致，则同时重启 master。
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

// InstallLocalReleaseArchive 是管理后台上传更新入口。先校验上传的归档文件名，
// 再调用统一核心：有活跃 master 时通过 master 安装，否则直接替换当前可执行文件。
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
	path, err := installCurrentExePath()
	if err != nil {
		return "", fmt.Errorf("resolve current executable failed: %w", err)
	}
	path = cleanExecutablePath(path)
	if path == "" {
		return "", fmt.Errorf("current executable is empty")
	}
	return path, nil
}

func currentExecutablePath() (string, error) {
	return os.Executable()
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
	if !sameExecutablePath(runtimeInfo.Executable, targetPath) {
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

	if err := copyFile(targetPath, backupPath); err != nil {
		return nil, fmt.Errorf("backup executable failed: %w", err)
	}

	restore := func() error {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove replaced executable failed: %w", err)
		}
		if err := moveFile(backupPath, targetPath); err != nil {
			return fmt.Errorf("restore previous executable failed: %w", err)
		}
		return nil
	}

	if err := moveFile(nextPath, targetPath); err != nil {
		if restoreErr := restore(); restoreErr != nil {
			return nil, fmt.Errorf("replace executable failed: %w (rollback failed: %v)", err, restoreErr)
		}
		return nil, fmt.Errorf("replace executable failed: %w", err)
	}

	return restore, nil
}

func moveFile(srcPath string, dstPath string) error {
	if err := os.Rename(srcPath, dstPath); err != nil {
		if !errors.Is(err, syscall.EXDEV) {
			return err
		}
		tmpPath, err := copyFileToTargetDir(srcPath, dstPath)
		if err != nil {
			return err
		}
		if err := os.Rename(tmpPath, dstPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		if err := os.Remove(srcPath); err != nil {
			return err
		}
	}
	return nil
}

func copyFileToTargetDir(srcPath string, dstPath string) (string, error) {
	dstDir := filepath.Dir(dstPath)
	tmp, err := os.CreateTemp(dstDir, ".swaves-copy-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	if err := copyFile(srcPath, tmpPath); err != nil {
		_ = os.Remove(tmpPath)
		return "", err
	}
	return tmpPath, nil
}

func copyFile(srcPath string, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()

	info, err := src.Stat()
	if err != nil {
		return err
	}
	dst, err := os.OpenFile(dstPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return err
	}
	if err := dst.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dstPath, info.Mode()); err != nil {
		return err
	}
	if err := os.Chtimes(dstPath, info.ModTime(), info.ModTime()); err != nil {
		return err
	}
	return nil
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
	if !sameExecutablePath(active.Executable, expected.Executable) {
		return fmt.Errorf("revalidate active master failed: runtime executable changed")
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
