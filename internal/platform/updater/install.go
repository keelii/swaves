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
		return result, fmt.Errorf("local release archive path is required")
	}

	runtimeInfo, err := ReadActiveRuntimeInfo()
	if err != nil {
		return result, fmt.Errorf("automatic upgrade requires daemon-mode=1 and an active master process: %w", err)
	}

	version, err := validateLocalArchiveName(result.ArchiveName, goos, goarch)
	if err != nil {
		return result, err
	}
	result.LatestVersion = version

	tmpDir, err := os.MkdirTemp(filepath.Dir(runtimeInfo.Executable), ".swaves-upgrade-")
	if err != nil {
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, expectedReleaseBinaryName(result.ArchiveName))
	if err != nil {
		return result, err
	}
	if err := os.Chmod(extractedPath, 0755); err != nil {
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}
	rollback, err := replaceExecutableWithRollback(extractedPath, runtimeInfo, filepath.Join(tmpDir, ".swaves-executable-backup"))
	if err != nil {
		return result, err
	}
	if err := signalProcess(runtimeInfo.PID); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
		}
		return result, fmt.Errorf("signal master restart failed: %w", err)
	}

	result.Installed = true
	result.RestartedPID = runtimeInfo.PID
	result.Reason = fmt.Sprintf("upgraded to %s", version)
	return result, nil
}

func (c Client) InstallLatestRelease(currentVersion string, goos string, goarch string) (InstallResult, error) {
	installMu.Lock()
	defer installMu.Unlock()

	result := InstallResult{CurrentVersion: strings.TrimSpace(currentVersion)}
	runtimeInfo, err := ReadActiveRuntimeInfo()
	if err != nil {
		return result, fmt.Errorf("automatic upgrade requires daemon-mode=1 and an active master process: %w", err)
	}

	check, err := c.CheckLatestRelease(currentVersion, goos, goarch)
	if err != nil {
		return result, err
	}
	result.LatestVersion = check.LatestVersion
	result.ReleaseURL = check.LatestReleaseURL
	if check.Target == nil {
		if strings.TrimSpace(goos) == "" {
			goos = runtime.GOOS
		}
		if strings.TrimSpace(goarch) == "" {
			goarch = runtime.GOARCH
		}
		return result, fmt.Errorf("automatic upgrade is not supported on %s/%s", goos, goarch)
	}
	result.ArchiveName = check.Target.Archive.Name

	stableCurrent := semverutil.IsStable(currentVersion)
	if stableCurrent {
		cmp, err := semverutil.Compare(currentVersion, check.LatestVersion)
		if err != nil {
			return result, err
		}
		if cmp >= 0 {
			result.Reason = check.Reason
			return result, nil
		}
	}

	tmpDir, err := os.MkdirTemp(filepath.Dir(runtimeInfo.Executable), ".swaves-upgrade-")
	if err != nil {
		return result, fmt.Errorf("create upgrade temp dir failed: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	archivePath := filepath.Join(tmpDir, check.Target.Archive.Name)
	if err := c.downloadToFile(check.Target.Archive.DownloadURL, archivePath); err != nil {
		return result, err
	}

	checksumPath := filepath.Join(tmpDir, check.Target.Checksum.Name)
	if err := c.downloadToFile(check.Target.Checksum.DownloadURL, checksumPath); err != nil {
		return result, err
	}
	if err := verifyChecksumFile(archivePath, checksumPath); err != nil {
		return result, err
	}

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, expectedReleaseBinaryName(check.Target.Archive.Name))
	if err != nil {
		return result, err
	}
	if err := os.Chmod(extractedPath, 0755); err != nil {
		return result, fmt.Errorf("chmod extracted binary failed: %w", err)
	}
	rollback, err := replaceExecutableWithRollback(extractedPath, runtimeInfo, filepath.Join(tmpDir, ".swaves-executable-backup"))
	if err != nil {
		return result, err
	}
	if err := signalProcess(runtimeInfo.PID); err != nil {
		if rollbackErr := rollback(); rollbackErr != nil {
			return result, fmt.Errorf("signal master restart failed: %w (rollback failed: %v)", err, rollbackErr)
		}
		return result, fmt.Errorf("signal master restart failed: %w", err)
	}

	result.Installed = true
	result.RestartedPID = runtimeInfo.PID
	result.Reason = fmt.Sprintf("upgraded to %s", check.LatestVersion)
	return result, nil
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

func replaceExecutableWithRollback(nextPath string, runtimeInfo RuntimeInfo, backupPath string) (executableRollback, error) {
	if err := ensureRuntimeInstallTarget(runtimeInfo); err != nil {
		return nil, err
	}

	nextPath = strings.TrimSpace(nextPath)
	targetPath := strings.TrimSpace(runtimeInfo.Executable)
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
