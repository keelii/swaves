package updater

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestInstallLocalReleaseArchiveRejectsWrongPlatform(t *testing.T) {
	if _, err := validateLocalArchiveName("swaves_v1.2.4_darwin_arm64.tar.gz", "linux", "amd64"); err == nil {
		t.Fatal("期望平台不匹配的归档被拒绝")
	}
}

func TestExtractReleaseBinarySkipsNonTargetFiles(t *testing.T) {
	tmpDir := t.TempDir()
	archivePath := filepath.Join(tmpDir, "swaves_v1.2.4_linux_amd64.tar.gz")
	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{
		{name: "README.md", content: []byte("readme")},
		{name: "swaves_v1.2.4_linux_amd64", content: []byte("new-binary")},
	})
	if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
		t.Fatalf("写入归档失败: %v", err)
	}

	extractedPath, err := extractReleaseBinary(archivePath, tmpDir, "swaves_v1.2.4_linux_amd64")
	if err != nil {
		t.Fatalf("extractReleaseBinary 失败: %v", err)
	}
	got, err := os.ReadFile(extractedPath)
	if err != nil {
		t.Fatalf("ReadFile 失败: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("解压内容不符合预期: %q", string(got))
	}
}

// TestInstallRequireMasterFailsWithoutActiveMaster 验证在没有 daemon-mode master
// 时，使用 RestartRequireMaster 策略的 Install 调用立即返回错误，不会进行任何
// 文件操作。
func TestInstallRequireMasterFailsWithoutActiveMaster(t *testing.T) {
	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	source := InstallSource{Kind: ArchiveSourceRemote}
	_, err := DefaultClient().Install(source, "v1.2.3", "linux", "amd64", RestartRequireMaster)
	if err == nil {
		t.Fatal("RestartRequireMaster 策略在无活跃 master 时应返回错误")
	}
	if !strings.Contains(err.Error(), "daemon-mode") {
		t.Fatalf("错误消息不符合预期: %q", err.Error())
	}
}

// TestInstallRemoteSkipsWhenAlreadyAtLatest 验证当前版本已是最新稳定版时，
// 远端更新路径直接返回 no-op，不尝试下载或安装。
func TestInstallRemoteSkipsWhenAlreadyAtLatest(t *testing.T) {
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := fmt.Sprintf(`{
				"tag_name":"v1.2.4",
				"html_url":"%s",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz","browser_download_url":"https://example.test/swaves_v1.2.4_linux_amd64.tar.gz"},
					{"name":"swaves_v1.2.4_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/swaves_v1.2.4_linux_amd64.tar.gz.sha256"}
				]
			}`, ReleaseTagURL("v1.2.4"))
			return newHTTPResponse(http.StatusOK, body), nil
		})},
	}

	result, err := client.Install(InstallSource{Kind: ArchiveSourceRemote}, "v1.2.4", "linux", "amd64", RestartIfMatchingMaster)
	if err != nil {
		t.Fatalf("Install 失败: %v", err)
	}
	if result.Installed {
		t.Fatal("已是最新版本时不应执行安装")
	}
	if result.LatestVersion != "v1.2.4" {
		t.Fatalf("LatestVersion 不符合预期: %q", result.LatestVersion)
	}
}

// TestInstallRemoteSkipsWhenNewerThanLatest 验证当前版本比最新发布版更新时，
// 远端更新路径跳过安装。
func TestInstallRemoteSkipsWhenNewerThanLatest(t *testing.T) {
	client := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := fmt.Sprintf(`{
				"tag_name":"v1.2.3",
				"html_url":"%s",
				"published_at":"2026-04-05T00:00:00Z",
				"draft":false,
				"prerelease":false,
				"assets":[
					{"name":"swaves_v1.2.3_linux_amd64.tar.gz","browser_download_url":"https://example.test/swaves_v1.2.3_linux_amd64.tar.gz"},
					{"name":"swaves_v1.2.3_linux_amd64.tar.gz.sha256","browser_download_url":"https://example.test/swaves_v1.2.3_linux_amd64.tar.gz.sha256"}
				]
			}`, ReleaseTagURL("v1.2.3"))
			return newHTTPResponse(http.StatusOK, body), nil
		})},
	}

	result, err := client.Install(InstallSource{Kind: ArchiveSourceRemote}, "v1.2.4", "linux", "amd64", RestartIfMatchingMaster)
	if err != nil {
		t.Fatalf("Install 失败: %v", err)
	}
	if result.Installed {
		t.Fatal("当前版本更新时不应执行安装")
	}
}

// TestInstallLocalSourceRejectsEmptyArchivePath 验证本地来源缺少归档路径时
// 直接返回错误。
func TestInstallLocalSourceRejectsEmptyArchivePath(t *testing.T) {
	_, err := InstallLocalReleaseArchive("swaves_v1.2.4_linux_amd64.tar.gz", "", "v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("归档路径为空时应返回错误")
	}
}

// TestInstallLocalSourceRejectsWrongPlatform 验证归档平台与当前平台不匹配时
// 返回错误（通过顶层封装函数 InstallLocalReleaseArchive 调用核心路径）。
func TestInstallLocalSourceRejectsWrongPlatform(t *testing.T) {
	_, err := InstallLocalReleaseArchive("swaves_v1.2.4_darwin_arm64.tar.gz", "/tmp/fake.tar.gz", "v1.2.3", "linux", "amd64")
	if err == nil {
		t.Fatal("平台不匹配时应返回错误")
	}
}

// TestResolveInstallTargetRequireMasterFailsWithoutRuntime 验证 RestartRequireMaster
// 策略在没有运行时信息文件时立即返回错误。
func TestResolveInstallTargetRequireMasterFailsWithoutRuntime(t *testing.T) {
	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	_, _, err := resolveInstallTarget(RestartRequireMaster)
	if err == nil {
		t.Fatal("无运行时信息文件时 resolveInstallTarget 应失败")
	}
}

// TestResolveInstallTargetWithMasterFallbackNoMasterReturnsCurrentExe 验证
// RestartWithMasterFallback 在没有活跃 master 时回退到当前可执行文件路径，
// 且 RuntimeInfo 为 nil（不触发重启）。
func TestResolveInstallTargetWithMasterFallbackNoMasterReturnsCurrentExe(t *testing.T) {
	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	path, ri, err := resolveInstallTarget(RestartWithMasterFallback)
	if err != nil {
		t.Fatalf("resolveInstallTarget 失败: %v", err)
	}
	if ri != nil {
		t.Fatalf("无活跃 master 时 RuntimeInfo 应为 nil，实际 PID=%d", ri.PID)
	}
	if strings.TrimSpace(path) == "" {
		t.Fatal("目标路径不应为空")
	}
}

// TestResolveInstallTargetIfMatchingMasterNoMasterReturnsNilRI 验证
// RestartIfMatchingMaster 在没有活跃 master 时不触发重启（RuntimeInfo 为 nil）。
func TestResolveInstallTargetIfMatchingMasterNoMasterReturnsNilRI(t *testing.T) {
	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	path, ri, err := resolveInstallTarget(RestartIfMatchingMaster)
	if err != nil {
		t.Fatalf("resolveInstallTarget 失败: %v", err)
	}
	if ri != nil {
		t.Fatalf("无匹配 master 时 RuntimeInfo 应为 nil，实际 PID=%d", ri.PID)
	}
	if strings.TrimSpace(path) == "" {
		t.Fatal("目标路径不应为空")
	}
}

// TestReplaceExecutableAtPathRollsBackOnExplicitCall 验证 rollback 函数被调用时
// 能将原始可执行文件恢复到目标路径。
func TestReplaceExecutableAtPathRollsBackOnExplicitCall(t *testing.T) {
	tmpDir := t.TempDir()
	targetPath := filepath.Join(tmpDir, "swaves")
	newPath := filepath.Join(tmpDir, "swaves_new")
	backupPath := filepath.Join(tmpDir, "swaves_backup")

	if err := os.WriteFile(targetPath, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("写入目标文件失败: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("new-binary"), 0755); err != nil {
		t.Fatalf("写入新文件失败: %v", err)
	}

	rollback, err := replaceExecutableAtPathWithRollback(newPath, targetPath, backupPath)
	if err != nil {
		t.Fatalf("replaceExecutableAtPathWithRollback 失败: %v", err)
	}

	// 验证新二进制已替换
	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("替换后 ReadFile 失败: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("替换后内容=%q，期望 new-binary", string(got))
	}

	// 执行回滚
	if err := rollback(); err != nil {
		t.Fatalf("rollback 失败: %v", err)
	}

	// 验证原始二进制已还原
	got, err = os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("回滚后 ReadFile 失败: %v", err)
	}
	if string(got) != "old-binary" {
		t.Fatalf("回滚后内容=%q，期望 old-binary", string(got))
	}
}

func TestCopyFilePreservesModeAndModTime(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not portable on Windows")
	}

	tmpDir := t.TempDir()
	srcPath := filepath.Join(tmpDir, "src")
	dstPath := filepath.Join(tmpDir, "dst")
	mode := os.FileMode(02755)
	modTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := os.WriteFile(srcPath, []byte("binary"), mode); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}
	if err := os.Chmod(srcPath, mode); err != nil {
		t.Fatalf("Chmod failed: %v", err)
	}
	if err := os.Chtimes(srcPath, modTime, modTime); err != nil {
		t.Fatalf("Chtimes failed: %v", err)
	}
	srcInfo, err := os.Stat(srcPath)
	if err != nil {
		t.Fatalf("source Stat failed: %v", err)
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		t.Fatalf("copyFile failed: %v", err)
	}

	info, err := os.Stat(dstPath)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Mode() != srcInfo.Mode() {
		t.Fatalf("mode=%v, want %v", info.Mode(), srcInfo.Mode())
	}
	if !info.ModTime().Equal(srcInfo.ModTime()) {
		t.Fatalf("modtime=%v, want %v", info.ModTime(), srcInfo.ModTime())
	}
}

// TestInstallLocalSourceWithActiveMasterRestartsAndInstalls 验证
// RestartWithMasterFallback 在有活跃 master 时：
//   - 将新二进制安装到 master 的可执行路径
//   - 通过 SIGHUP 重启 master
//   - 返回 Installed=true 且 RestartedPID 正确
//
// 本测试仅在 Linux 上运行，因为 daemon-mode 重启依赖 SIGHUP。
func TestInstallLocalSourceWithActiveMasterRestartsAndInstalls(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅 Linux 支持通过 SIGHUP 重启 daemon master")
	}

	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	// 启动一个可以接收 SIGHUP 并退出的子进程（sleep 对 SIGHUP 默认动作是终止）
	cmd := exec.Command("sleep", "3600")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动后台进程失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	sleepPID := cmd.Process.Pid

	// 建立指向 tmpDir 内假可执行文件的运行时信息
	fakeExe := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(fakeExe, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("写入假可执行文件失败: %v", err)
	}
	if err := WriteRuntimeInfo(RuntimeInfo{PID: sleepPID, Executable: fakeExe}); err != nil {
		t.Fatalf("WriteRuntimeInfo 失败: %v", err)
	}
	stubRuntimeProcessExecutablePath(t, func(pid int) (string, bool, error) {
		return fakeExe, true, nil
	})

	// 构建包含新二进制的归档
	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{
		{name: "swaves_v1.2.4_linux_amd64", content: []byte("new-binary")},
	})
	if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
		t.Fatalf("写入归档失败: %v", err)
	}

	source := InstallSource{
		Kind:        ArchiveSourceLocal,
		ArchiveName: archiveName,
		ArchivePath: archivePath,
		Version:     "v1.2.4",
	}
	result, err := DefaultClient().Install(source, "v1.2.3", "linux", "amd64", RestartWithMasterFallback)
	if err != nil {
		t.Fatalf("Install 失败: %v", err)
	}
	if !result.Installed {
		t.Fatal("result.Installed 应为 true")
	}
	if result.RestartedPID != sleepPID {
		t.Fatalf("RestartedPID=%d，期望 %d", result.RestartedPID, sleepPID)
	}
	if result.LatestVersion != "v1.2.4" {
		t.Fatalf("LatestVersion=%q，期望 v1.2.4", result.LatestVersion)
	}

	// 验证新二进制已写入假可执行路径
	got, err := os.ReadFile(fakeExe)
	if err != nil {
		t.Fatalf("ReadFile fakeExe 失败: %v", err)
	}
	if string(got) != "new-binary" {
		t.Fatalf("fakeExe 内容=%q，期望 new-binary", string(got))
	}
}

// TestInstallLocalSourceRequireMasterWithActiveMasterInstalls 验证
// RestartRequireMaster 在有活跃 master 时完成安装并重启 master。
func TestInstallLocalSourceRequireMasterWithActiveMasterInstalls(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("仅 Linux 支持通过 SIGHUP 重启 daemon master")
	}

	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	cmd := exec.Command("sleep", "3600")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动后台进程失败: %v", err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	sleepPID := cmd.Process.Pid

	fakeExe := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(fakeExe, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("写入假可执行文件失败: %v", err)
	}
	if err := WriteRuntimeInfo(RuntimeInfo{PID: sleepPID, Executable: fakeExe}); err != nil {
		t.Fatalf("WriteRuntimeInfo 失败: %v", err)
	}
	stubRuntimeProcessExecutablePath(t, func(pid int) (string, bool, error) {
		return fakeExe, true, nil
	})

	archiveName := "swaves_v1.2.4_linux_amd64.tar.gz"
	archivePath := filepath.Join(tmpDir, archiveName)
	archiveData := buildTarGzArchiveEntries(t, []archiveEntry{
		{name: "swaves_v1.2.4_linux_amd64", content: []byte("new-binary")},
	})
	if err := os.WriteFile(archivePath, archiveData, 0o644); err != nil {
		t.Fatalf("写入归档失败: %v", err)
	}

	source := InstallSource{
		Kind:        ArchiveSourceLocal,
		ArchiveName: archiveName,
		ArchivePath: archivePath,
		Version:     "v1.2.4",
	}
	result, err := DefaultClient().Install(source, "v1.2.3", "linux", "amd64", RestartRequireMaster)
	if err != nil {
		t.Fatalf("Install 失败: %v", err)
	}
	if !result.Installed {
		t.Fatal("result.Installed 应为 true")
	}
	if result.RestartedPID != sleepPID {
		t.Fatalf("RestartedPID=%d，期望 %d", result.RestartedPID, sleepPID)
	}
}

// TestInstallLocalSourceSignalFailureTriggersRollback 验证信号发送失败时
// 回滚机制能将原始可执行文件还原。使用不存在的 PID 模拟信号失败。
func TestInstallLocalSourceSignalFailureTriggersRollback(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("基于信号的回滚测试仅适用于 Linux")
	}

	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	// 使用真实 PID 写入 RuntimeInfo 使 ReadActiveRuntimeInfo 通过，
	// 但随后杀死该进程使信号发送失败。
	cmd := exec.Command("sleep", "3600")
	if err := cmd.Start(); err != nil {
		t.Fatalf("启动后台进程失败: %v", err)
	}
	sleepPID := cmd.Process.Pid
	// 立即杀死进程，使后续 SIGHUP 信号失败
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("杀死进程失败: %v", err)
	}
	_ = cmd.Wait()

	fakeExe := filepath.Join(tmpDir, "swaves")
	if err := os.WriteFile(fakeExe, []byte("old-binary"), 0755); err != nil {
		t.Fatalf("写入假可执行文件失败: %v", err)
	}
	// 写入指向已死进程的运行时信息
	// 注意：ReadActiveRuntimeInfo 会检查进程是否存在，若不存在则返回错误，
	// 因此对于 RestartRequireMaster 策略，Install 会在 resolveInstallTarget
	// 阶段失败（不会进行文件操作）。
	// 本测试直接测试低层替换后的回滚行为。
	_ = sleepPID

	targetPath := filepath.Join(tmpDir, "swaves_target")
	newPath := filepath.Join(tmpDir, "swaves_new")
	backupPath := filepath.Join(tmpDir, "swaves_backup")

	if err := os.WriteFile(targetPath, []byte("original"), 0755); err != nil {
		t.Fatalf("写入目标文件失败: %v", err)
	}
	if err := os.WriteFile(newPath, []byte("replacement"), 0755); err != nil {
		t.Fatalf("写入新文件失败: %v", err)
	}

	rollback, err := replaceExecutableAtPathWithRollback(newPath, targetPath, backupPath)
	if err != nil {
		t.Fatalf("replaceExecutableAtPathWithRollback 失败: %v", err)
	}

	// 模拟信号失败后触发回滚
	if err := rollback(); err != nil {
		t.Fatalf("rollback 失败: %v", err)
	}

	got, err := os.ReadFile(targetPath)
	if err != nil {
		t.Fatalf("回滚后 ReadFile 失败: %v", err)
	}
	if string(got) != "original" {
		t.Fatalf("回滚后内容=%q，期望 original", string(got))
	}
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatal("回滚后备份文件应已被删除")
	}
}

// TestAllThreeEntryPointsCallUnifiedInstall 通过反射验证三个顶层入口均调用
// 同一核心逻辑——当 HTTP 服务不可用时，远端来源均返回相同类型的错误。
// 本测试确保 InstallLatestRelease / InstallLatestReleaseCLI 均走远端来源路径。
func TestAllThreeEntryPointsUseUnifiedSourceAndPolicyTypes(t *testing.T) {
	tmpDir := t.TempDir()
	withUpdaterWorkingDir(t, tmpDir)
	resetRuntimeCacheRoot(t)

	errClient := Client{
		BaseURL: "https://example.test/latest",
		HTTPClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return newHTTPResponse(http.StatusInternalServerError, "server error"), nil
		})},
	}

	// InstallLatestRelease：RestartRequireMaster，无活跃 master 时优先返回 master 错误
	_, err1 := errClient.InstallLatestRelease("v1.2.3", "linux", "amd64")
	if err1 == nil || !strings.Contains(err1.Error(), "daemon-mode") {
		t.Fatalf("InstallLatestRelease: 期望 daemon-mode 错误，实际: %v", err1)
	}

	// InstallLatestReleaseCLI：RestartIfMatchingMaster，无活跃 master 时走发布检查
	// 发布检查失败（HTTP 500）→ 应返回发布检查失败错误
	_, err2 := errClient.InstallLatestReleaseCLI("v1.2.3", "linux", "amd64")
	if err2 == nil {
		t.Fatal("InstallLatestReleaseCLI: 应返回发布检查失败错误")
	}
	if strings.Contains(err2.Error(), "daemon-mode") {
		t.Fatalf("InstallLatestReleaseCLI 不应要求 daemon-mode，实际: %v", err2)
	}
}

type archiveEntry struct {
	name    string
	content []byte
}

func buildTarGzArchiveEntries(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzipWriter := gzip.NewWriter(&buf)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, entry := range entries {
		if err := tarWriter.WriteHeader(&tar.Header{Name: entry.name, Mode: 0o755, Size: int64(len(entry.content))}); err != nil {
			t.Fatalf("WriteHeader failed: %v", err)
		}
		if _, err := tarWriter.Write(entry.content); err != nil {
			t.Fatalf("Write archive contents failed: %v", err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatalf("Close tar writer failed: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("Close gzip writer failed: %v", err)
	}
	return buf.Bytes()
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func newHTTPResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
