package dash

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"swaves/internal/platform/db"
	job "swaves/internal/platform/jobs"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/middleware"
	"swaves/internal/platform/store"
	"swaves/internal/platform/updater"
	"swaves/internal/shared/types"
	"time"

	"github.com/gofiber/fiber/v3"
	_ "github.com/mattn/go-sqlite3"
)

const (
	restoreSignalDelay        = 200 * time.Millisecond
	restoreUploadFormField    = "file"
	restoreBackupFileFormKey  = "backup_file"
	restoreStatusRefreshQuery = "refresh"
)

type restoreBackupFile struct {
	Name       string
	Size       int64
	ModifiedAt int64
	Path       string
}

type restoreSupportState struct {
	Enabled bool
	Message string
}

func (h *Handler) GetExportHandler(c fiber.Ctx) error {
	return h.redirectToDashRoute(c, "dash.import.show", nil, map[string]string{
		"tab": importExportTabExport,
	})
}

func (h *Handler) GetBackupRestoreHandler(c fiber.Ctx) error {
	return h.renderBackupRestoreView(c, nil)
}

func (h *Handler) GetBackupRestoreStatusHandler(c fiber.Ctx) error {
	status, err := updater.ReadRestoreStatus()
	if err != nil {
		logger.Error("[restore] read status failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"ok":    false,
			"error": err.Error(),
		})
	}
	return c.JSON(fiber.Map{
		"ok":         true,
		"status":     status.State,
		"label":      restoreStatusLabel(status.State),
		"message":    status.Message,
		"updated_at": status.UpdatedAt,
		"active":     isRestoreStatusActive(status.State),
	})
}

func (h *Handler) GetBackupRestoreDownloadHandler(c fiber.Ctx) error {
	sourceName := filepath.Base(c.Query(restoreBackupFileFormKey))
	if sourceName == "" || sourceName == "." {
		logger.Warn("[backup] local backup download rejected: ip=%s reason=missing_file", c.IP())
		return fiber.ErrBadRequest
	}

	sourcePath, err := findLocalRestoreSource(sourceName, h.Model.DSN)
	if err != nil {
		logger.Warn("[backup] local backup download failed: ip=%s file=%s err=%v", c.IP(), sourceName, err)
		return fiber.NewError(fiber.StatusNotFound, err.Error())
	}

	stream, err := openCleanupFileStream(sourcePath, nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			logger.Warn("[backup] local backup download failed: ip=%s file=%s err=%v", c.IP(), sourceName, err)
			return fiber.NewError(fiber.StatusNotFound, "未找到选中的本地备份文件。")
		}
		logger.Error("[backup] local backup open failed: ip=%s file=%s err=%v", c.IP(), sourceName, err)
		return fmt.Errorf("打开本地备份文件失败：%w", err)
	}

	logger.Info("[backup] local backup download requested: ip=%s file=%s", c.IP(), sourceName)
	c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", sourceName))
	c.Set("Content-Type", "application/x-sqlite3")
	return c.SendStream(stream)
}

func (h *Handler) PostExportRestoreLocalHandler(c fiber.Ctx) error {
	sourceName := filepath.Base(strings.TrimSpace(c.FormValue(restoreBackupFileFormKey)))
	if sourceName == "" || sourceName == "." {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, "请选择一个本地备份文件。")
	}

	sourcePath, err := findLocalRestoreSource(sourceName, h.Model.DSN)
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, err.Error())
	}

	if err := h.enqueueRestore(sourcePath); err != nil {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, err.Error())
	}

	return h.redirectToDashRouteWithNotice(c, "dash.backup_restore.show", nil, map[string]string{
		"refresh": strconv.Itoa(defaultRefreshDelaySeconds),
	}, "数据库恢复任务已提交，服务即将重启。")
}

func (h *Handler) PostExportRestoreUploadHandler(c fiber.Ctx) error {
	fileHeader, err := c.FormFile(restoreUploadFormField)
	if err != nil {
		statusCode := fiber.StatusBadRequest
		if errors.Is(err, fiber.ErrRequestEntityTooLarge) || strings.Contains(strings.ToLower(err.Error()), "request entity too large") {
			statusCode = fiber.StatusRequestEntityTooLarge
		}
		message := "读取上传文件失败：" + err.Error()
		if statusCode == fiber.StatusRequestEntityTooLarge {
			message = "上传文件过大，当前请求体大小限制为 10MB，请优先使用服务器本地备份恢复。"
		}
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, message)
	}

	sourcePath, err := h.saveRestoreUpload(fileHeader)
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, err.Error())
	}

	if err := h.enqueueRestore(sourcePath); err != nil {
		_ = os.Remove(sourcePath)
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, err.Error())
	}

	return h.redirectToDashRouteWithNotice(c, "dash.backup_restore.show", nil, map[string]string{
		"refresh": strconv.Itoa(defaultRefreshDelaySeconds),
	}, "数据库恢复任务已提交，服务即将重启。")
}

func (h *Handler) PostBackupRestoreDeleteHandler(c fiber.Ctx) error {
	status, err := updater.ReadRestoreStatus()
	if err != nil {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, "读取恢复状态失败："+err.Error())
	}
	if isRestoreStatusActive(status.State) {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, "恢复任务执行中，暂时不能删除本地备份文件。")
	}

	sourceName := filepath.Base(strings.TrimSpace(c.FormValue(restoreBackupFileFormKey)))
	if sourceName == "" || sourceName == "." {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, "请选择要删除的本地备份文件。")
	}
	if err := deleteLocalRestoreBackup(sourceName, h.Model.DSN); err != nil {
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, err.Error())
	}

	return h.redirectToDashRouteWithNotice(c, "dash.backup_restore.show", nil, nil, "本地备份文件已删除。")
}

func (h *Handler) PostBackupRestoreBackupNowHandler(c fiber.Ctx) error {
	logger.Info("[backup] manual local backup requested: ip=%s", c.IP())
	message, err := job.RunLocalBackupNow(h.Model)
	if err != nil {
		logger.Error("[backup] run local backup now failed: %v", err)
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, "执行本地备份失败："+err.Error())
	}

	notice := "本地备份已完成。"
	if message != nil && strings.TrimSpace(*message) != "" {
		notice = strings.TrimSpace(*message)
	}
	logger.Info("[backup] manual local backup completed: ip=%s message=%s", c.IP(), notice)
	return h.redirectToDashRouteWithNotice(c, "dash.backup_restore.show", nil, nil, notice)
}

func (h *Handler) PostBackupRestoreRemoteBackupNowHandler(c fiber.Ctx) error {
	logger.Info("[backup] manual remote backup requested: ip=%s", c.IP())
	message, err := job.RunRemoteBackupNow(h.Model)
	if err != nil {
		logger.Error("[backup] run remote backup now failed: %v", err)
		return h.redirectToDashRouteWithError(c, "dash.backup_restore.show", nil, nil, "执行远程备份失败："+err.Error())
	}

	notice := "远程备份已完成。"
	if message != nil && strings.TrimSpace(*message) != "" {
		notice = strings.TrimSpace(*message)
	}
	logger.Info("[backup] manual remote backup completed: ip=%s message=%s", c.IP(), notice)
	return h.redirectToDashRouteWithNotice(c, "dash.backup_restore.show", nil, nil, notice)
}

func (h *Handler) renderBackupRestoreView(c fiber.Ctx, extra fiber.Map) error {
	viewData, err := buildBackupRestoreViewData(c, h.Model.DSN)
	if err != nil {
		return err
	}
	for key, value := range extra {
		viewData[key] = value
	}
	return RenderDashView(c, "dash/backup_restore.html", viewData, "")
}

func buildBackupRestoreViewData(c fiber.Ctx, sqliteFile string) (fiber.Map, error) {
	restoreStatus, err := updater.ReadRestoreStatus()
	if err != nil {
		return nil, err
	}

	restoreSupport := getRestoreSupportState()
	pager := middleware.GetPagination(c)
	backupFiles, backupDir, backupErr := listLocalRestoreBackups(sqliteFile)
	backupFiles = paginateRestoreBackups(backupFiles, &pager)
	updatedAt := ""
	if restoreStatus.UpdatedAt > 0 {
		updatedAt = time.Unix(restoreStatus.UpdatedAt, 0).Format("2006-01-02 15:04:05")
	}

	statusAPIURL, _ := c.GetRouteURL("dash.backup_restore.status", fiber.Map{})
	refreshDelay := parseRefreshDelaySeconds(c.Query(restoreStatusRefreshQuery))
	if refreshDelay <= 0 && isRestoreStatusActive(restoreStatus.State) {
		refreshDelay = defaultRefreshDelaySeconds
	}

	viewData := fiber.Map{
		"Title":                "备份恢复",
		"RestoreEnabled":       restoreSupport.Enabled,
		"RestoreMessage":       restoreSupport.Message,
		"RestoreStatus":        restoreStatus.State,
		"RestoreStatusKind":    restoreStatusKind(restoreStatus.State),
		"RestoreStatusLabel":   restoreStatusLabel(restoreStatus.State),
		"RestoreStatusMessage": strings.TrimSpace(restoreStatus.Message),
		"RestoreStatusUpdated": updatedAt,
		"RestoreRefreshDelay":  refreshDelay,
		"RestoreStatusAPIURL":  statusAPIURL,
		"LocalBackupFiles":     backupFiles,
		"LocalBackupDir":       backupDir,
		"Pager":                pager,
	}
	if backupErr != nil {
		viewData["BackupListError"] = backupErr.Error()
	}
	return viewData, nil
}

func getRestoreSupportState() restoreSupportState {
	if runtime.GOOS == "windows" {
		return restoreSupportState{
			Message: "Windows 暂不支持数据库恢复。",
		}
	}
	if _, err := updater.ReadActiveRuntimeInfo(); err != nil {
		return restoreSupportState{
			Message: "daemon-mode 未启用时，数据库恢复不可用。",
		}
	}
	return restoreSupportState{Enabled: true}
}

func listLocalRestoreBackups(sqliteFile string) ([]restoreBackupFile, string, error) {
	backupDir := resolveRestoreBackupDir(store.GetSetting("backup_local_dir"), sqliteFile)
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []restoreBackupFile{}, backupDir, nil
		}
		return nil, backupDir, fmt.Errorf("读取本地备份目录失败：%w", err)
	}

	backups := make([]restoreBackupFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sqlite") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, backupDir, fmt.Errorf("读取备份文件信息失败：%w", err)
		}
		backups = append(backups, restoreBackupFile{
			Name:       entry.Name(),
			Size:       info.Size(),
			ModifiedAt: info.ModTime().Unix(),
			Path:       filepath.Join(backupDir, entry.Name()),
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].ModifiedAt > backups[j].ModifiedAt
	})
	return backups, backupDir, nil
}

func paginateRestoreBackups(backups []restoreBackupFile, pager *types.Pagination) []restoreBackupFile {
	if pager == nil {
		return backups
	}

	pager.Total = len(backups)
	if pager.PageSize <= 0 {
		pager.PageSize = 10
	}
	if pager.Total == 0 {
		pager.Page = 1
		pager.Num = 0
		return []restoreBackupFile{}
	}

	pager.Num = (pager.Total + pager.PageSize - 1) / pager.PageSize
	if pager.Page <= 0 {
		pager.Page = 1
	}
	if pager.Page > pager.Num {
		pager.Page = pager.Num
	}

	start := (pager.Page - 1) * pager.PageSize
	if start >= len(backups) {
		return []restoreBackupFile{}
	}
	end := start + pager.PageSize
	if end > len(backups) {
		end = len(backups)
	}
	return backups[start:end]
}

func resolveRestoreBackupDir(dir, sqliteFile string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = db.DefaultBackupDir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	sqliteFile = strings.TrimSpace(sqliteFile)
	if sqliteFile != "" {
		absFile, err := filepath.Abs(sqliteFile)
		if err != nil {
			logger.Warn("[backup] resolve sqlite abs path failed, falling back to cwd: file=%s err=%v", sqliteFile, err)
		} else {
			return filepath.Join(filepath.Dir(absFile), dir)
		}
	}
	wd, err := os.Getwd()
	if err != nil {
		return dir
	}
	return filepath.Join(wd, dir)
}

func findLocalRestoreSource(name, sqliteFile string) (string, error) {
	backups, _, err := listLocalRestoreBackups(sqliteFile)
	if err != nil {
		return "", err
	}
	for _, backup := range backups {
		if backup.Name == name {
			return backup.Path, nil
		}
	}
	return "", fmt.Errorf("未找到选中的本地备份文件：%s", name)
}

func deleteLocalRestoreBackup(name, sqliteFile string) error {
	sourcePath, err := findLocalRestoreSource(name, sqliteFile)
	if err != nil {
		return err
	}
	if err := os.Remove(sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("未找到选中的本地备份文件：%s", name)
		}
		return fmt.Errorf("删除本地备份文件失败：%w", err)
	}
	return nil
}

func (h *Handler) enqueueRestore(sourcePath string) error {
	supportState := getRestoreSupportState()
	if !supportState.Enabled {
		return errors.New(supportState.Message)
	}

	if err := validateRestoreSQLiteFile(sourcePath); err != nil {
		return err
	}
	if err := createRestoreSafetyBackup(h.Model); err != nil {
		return err
	}

	status, err := updater.ReadRestoreStatus()
	if err != nil {
		return err
	}
	if isRestoreStatusActive(status.State) {
		return fmt.Errorf("已有数据库恢复任务正在执行，请稍后再试")
	}

	if err := updater.WriteRestoreRequest(updater.RestoreRequest{Source: sourcePath}); err != nil {
		return err
	}
	if err := updater.WriteRestoreStatus(updater.RestoreStatus{
		State:   updater.RestoreStatusPending,
		Message: "恢复任务已提交，等待 master 处理。",
	}); err != nil {
		return err
	}

	go triggerQueuedRestore()
	return nil
}

func createRestoreSafetyBackup(model *db.DB) error {
	backupDir := resolveRestoreBackupDir(store.GetSetting("backup_local_dir"), model.DSN)
	_, err := db.ExportSQLiteWithHash(model, backupDir)
	if err == nil {
		return nil
	}
	if strings.Contains(err.Error(), "无需重复导出") {
		return nil
	}
	return fmt.Errorf("创建恢复前备份失败：%w", err)
}

func validateRestoreSQLiteFile(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("恢复文件路径不能为空")
	}
	if !strings.HasSuffix(strings.ToLower(path), ".sqlite") {
		return fmt.Errorf("仅支持恢复 .sqlite 文件")
	}

	header := make([]byte, 16)
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("打开恢复文件失败：%w", err)
	}
	if _, err := io.ReadFull(file, header); err != nil {
		_ = file.Close()
		return fmt.Errorf("读取恢复文件头失败：%w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("关闭恢复文件失败：%w", err)
	}
	if string(header) != "SQLite format 3\x00" {
		return fmt.Errorf("恢复文件不是有效的 SQLite 数据库")
	}

	database, err := openReadOnlySQLite(path)
	if err != nil {
		return fmt.Errorf("打开恢复数据库失败：%w", err)
	}
	defer func() { _ = database.Close() }()

	var integrity string
	if err := database.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil {
		return fmt.Errorf("校验恢复数据库失败：%w", err)
	}
	if strings.TrimSpace(strings.ToLower(integrity)) != "ok" {
		return fmt.Errorf("恢复数据库完整性校验失败：%s", integrity)
	}

	rows, err := database.Query("SELECT name FROM sqlite_master WHERE type='table'")
	if err != nil {
		return fmt.Errorf("读取恢复数据库表结构失败：%w", err)
	}
	defer func() { _ = rows.Close() }()

	requiredTables := map[string]bool{
		"posts":      false,
		"settings":   false,
		"categories": false,
		"tags":       false,
		"comments":   false,
		"assets":     false,
		"sessions":   false,
	}
	requiredTableNames := map[string]string{
		string(db.TablePosts):      "posts",
		string(db.TableSettings):   "settings",
		string(db.TableCategories): "categories",
		string(db.TableTags):       "tags",
		string(db.TableComments):   "comments",
		string(db.TableAssets):     "assets",
		string(db.TableSessions):   "sessions",
	}
	foundRequiredTables := map[string]string{}

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return fmt.Errorf("读取恢复数据库表失败：%w", err)
		}
		if _, ok := requiredTables[name]; ok {
			requiredTables[name] = true
			foundRequiredTables[name] = name
			continue
		}
		if logicalName, ok := requiredTableNames[name]; ok {
			requiredTables[logicalName] = true
			foundRequiredTables[logicalName] = name
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("遍历恢复数据库表失败：%w", err)
	}
	for table, ok := range requiredTables {
		if !ok {
			return fmt.Errorf("恢复数据库缺少必要数据表：%s", table)
		}
	}
	if err := validateRestoreSettingsData(database, foundRequiredTables["settings"]); err != nil {
		return err
	}
	return nil
}

func validateRestoreSettingsData(database *sql.DB, tableName string) error {
	if database == nil {
		return fmt.Errorf("恢复数据库连接不能为空")
	}
	if tableName == "" {
		return fmt.Errorf("恢复数据库缺少必要数据表：settings")
	}

	quotedTable := quoteSQLiteIdentifier(tableName)

	var count int
	if err := database.QueryRow("SELECT COUNT(1) FROM " + quotedTable).Scan(&count); err != nil {
		return fmt.Errorf("读取恢复数据库配置失败：%w", err)
	}
	if count == 0 {
		return fmt.Errorf("恢复数据库未完成初始化，缺少必要配置数据：settings")
	}

	var passwordCount int
	if err := database.QueryRow("SELECT COUNT(1) FROM "+quotedTable+" WHERE code = ?", "dash_password").Scan(&passwordCount); err != nil {
		return fmt.Errorf("读取恢复数据库配置项失败：%w", err)
	}
	if passwordCount == 0 {
		return fmt.Errorf("恢复数据库缺少必要配置项：dash_password")
	}

	return nil
}

func quoteSQLiteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func openReadOnlySQLite(path string) (*sql.DB, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve restore database path failed: %w", err)
	}
	uri := url.URL{Scheme: "file", Path: path}
	query := uri.Query()
	query.Set("mode", "ro")
	query.Set("_busy_timeout", "5000")
	uri.RawQuery = query.Encode()
	uri.Path = absPath
	return sql.Open("sqlite3", uri.String())
}

func (h *Handler) saveRestoreUpload(fileHeader *multipart.FileHeader) (string, error) {
	src, err := fileHeader.Open()
	if err != nil {
		return "", fmt.Errorf("打开上传文件失败：%w", err)
	}
	defer func() { _ = src.Close() }()

	name := filepath.Base(fileHeader.Filename)
	if !strings.HasSuffix(strings.ToLower(name), ".sqlite") {
		return "", fmt.Errorf("仅支持上传 .sqlite 文件")
	}

	dbPath := strings.TrimSpace(h.Model.DSN)
	if dbPath == "" {
		return "", fmt.Errorf("无法定位当前数据库文件")
	}

	tmpDir := filepath.Dir(dbPath)
	if tmpDir == "" {
		tmpDir = "."
	}
	tmpFile, err := os.CreateTemp(tmpDir, ".swaves-restore-*.sqlite")
	if err != nil {
		return "", fmt.Errorf("创建上传临时文件失败：%w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(tmpFile.Name())
		}
	}()

	if _, err = io.Copy(tmpFile, src); err != nil {
		_ = tmpFile.Close()
		return "", fmt.Errorf("保存上传文件失败：%w", err)
	}
	if err = tmpFile.Close(); err != nil {
		return "", fmt.Errorf("关闭上传文件失败：%w", err)
	}
	return tmpFile.Name(), nil
}

func triggerQueuedRestore() {
	time.Sleep(restoreSignalDelay)

	pid, err := updater.RestartActiveRuntime()
	if err != nil {
		logger.Error("[restore] signal master failed: %v", err)
		_ = updater.RemoveRestoreRequest()
		_ = updater.WriteRestoreStatus(updater.RestoreStatus{
			State:   updater.RestoreStatusFailed,
			Message: "通知 master 执行恢复失败：" + err.Error(),
		})
		return
	}
	logger.Info("[restore] restore signal sent to master pid=%d", pid)
}

func restoreStatusLabel(status string) string {
	switch status {
	case updater.RestoreStatusPending:
		return "等待执行"
	case updater.RestoreStatusStoppingWorker:
		return "停止旧进程"
	case updater.RestoreStatusReplacingDB:
		return "替换数据库"
	case updater.RestoreStatusStartingWorker:
		return "启动新进程"
	case updater.RestoreStatusSuccess:
		return "恢复成功"
	case updater.RestoreStatusRolledBack:
		return "已回滚"
	case updater.RestoreStatusFailed:
		return "恢复失败"
	default:
		return "空闲"
	}
}

func isRestoreStatusActive(status string) bool {
	switch status {
	case updater.RestoreStatusPending, updater.RestoreStatusStoppingWorker, updater.RestoreStatusReplacingDB, updater.RestoreStatusStartingWorker:
		return true
	default:
		return false
	}
}

func restoreStatusKind(status string) string {
	switch status {
	case updater.RestoreStatusFailed, updater.RestoreStatusRolledBack:
		return "danger"
	case updater.RestoreStatusSuccess:
		return "success"
	case updater.RestoreStatusPending, updater.RestoreStatusStoppingWorker, updater.RestoreStatusReplacingDB, updater.RestoreStatusStartingWorker:
		return "info"
	default:
		return "info"
	}
}
