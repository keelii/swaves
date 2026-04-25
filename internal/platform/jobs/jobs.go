package job

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"swaves/internal/platform/buildinfo"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/notify"
	"swaves/internal/platform/store"
	"swaves/internal/platform/updater"
	"swaves/internal/shared/helper"
	"swaves/internal/shared/types"
	"time"
)

const (
	settingBackupLocalDir         = "backup_local_dir"
	settingBackupLocalIntervalMin = "backup_local_interval_min"
	settingBackupLocalMaxCount    = "backup_local_max_count"
)

func jobMessage(message string) *string {
	return &message
}

var checkLatestAppRelease = updater.CheckLatestRelease

// DatabaseBackupJob 数据库备份任务
func DatabaseBackupJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}

	cfg := loadLocalBackupConfig(reg)
	return runLocalBackup(reg.DB, cfg, true)
}

func RunLocalBackupNow(dbx *db.DB) (*string, error) {
	if dbx == nil {
		return nil, errors.New("db is nil")
	}

	cfg := loadLocalBackupConfig(&Registry{Config: types.AppConfig{SqliteFile: dbx.DSN}})
	return runLocalBackup(dbx, cfg, false)
}

func RunRemoteBackupNow(dbx *db.DB) (*string, error) {
	if dbx == nil {
		return nil, errors.New("db is nil")
	}

	registryMu.RLock()
	reg := registry
	registryMu.RUnlock()
	if reg == nil {
		return nil, errors.New("task registry not initialized")
	}

	return PushSystemDataJob(&Registry{DB: dbx, Config: reg.Config})
}

func runLocalBackup(dbx *db.DB, cfg localBackupConfig, checkInterval bool) (*string, error) {
	backupDir := resolveBackupDir(cfg.Dir)
	logger.Info("[backup] local backup start: dir=%s interval=%s max_count=%d check_interval=%t", backupDir, cfg.Interval, cfg.MaxCount, checkInterval)

	if err := helper.EnsureDir(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建备份目录 %s: %w", backupDir, err)
	}

	if checkInterval {
		latestAt, err := latestLocalBackupAt(backupDir)
		if err != nil {
			return nil, fmt.Errorf("读取本地备份目录失败: %w", err)
		}
		if !latestAt.IsZero() && cfg.Interval > 0 {
			elapsed := time.Since(latestAt)
			if elapsed < cfg.Interval {
				remain := (cfg.Interval - elapsed).Round(time.Second)
				message := fmt.Sprintf("skip local backup: interval=%s remaining=%s", cfg.Interval, remain)
				logger.Info("[backup] local backup skipped: dir=%s reason=%s", backupDir, message)
				return jobMessage(message), nil
			}
		}
	}

	result, err := db.ExportSQLiteWithHash(dbx, backupDir)
	if err != nil {
		if strings.Contains(err.Error(), "无需重复导出") {
			logger.Info("[backup] local backup skipped: dir=%s reason=%s", backupDir, err.Error())
			return jobMessage(err.Error()), nil
		}
		logger.Error("[backup] local backup export failed: dir=%s err=%v", backupDir, err)
		return nil, err
	}

	removedCount, err := pruneLocalBackupFiles(backupDir, cfg.MaxCount)
	if err != nil {
		logger.Error("[backup] local backup prune failed: dir=%s err=%v", backupDir, err)
		return nil, fmt.Errorf("本地备份完成但清理旧备份失败: %w", err)
	}

	message := fmt.Sprintf("%v, dir=%s, pruned=%d", result, backupDir, removedCount)
	logger.Info("[backup] local backup completed: %s", message)
	return jobMessage(message), nil
}

type localBackupConfig struct {
	Dir      string
	Interval time.Duration
	MaxCount int
}

func loadLocalBackupConfig(reg *Registry) localBackupConfig {
	defaultDir := strings.TrimSpace(reg.Config.BackupDir)
	if defaultDir == "" {
		defaultDir = db.DefaultBackupDir
	}

	dir := strings.TrimSpace(store.GetSetting(settingBackupLocalDir))
	if dir == "" {
		dir = defaultDir
	}

	intervalMin := store.GetSettingInt(settingBackupLocalIntervalMin, 1440)
	if intervalMin < 1 {
		intervalMin = 1
	}

	maxCount := store.GetSettingInt(settingBackupLocalMaxCount, 30)
	if maxCount < 1 {
		maxCount = 1
	}

	return localBackupConfig{
		Dir:      dir,
		Interval: time.Duration(intervalMin) * time.Minute,
		MaxCount: maxCount,
	}
}

func ResolveLocalBackupDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = db.DefaultBackupDir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	wd, err := os.Getwd()
	if err != nil {
		logger.Warn("[backup] getwd error: %v", err)
		return dir
	}
	return filepath.Join(wd, dir)
}

func resolveBackupDir(dir string) string {
	return ResolveLocalBackupDir(dir)
}

// migrateBackupDir moves .sqlite backup files from the legacy default "backups/"
// directory to the current default ".cache/backups/" when no custom backup directory
// has been configured by the user.
func migrateBackupDir() {
	if strings.TrimSpace(store.GetSetting(settingBackupLocalDir)) == db.DefaultBackupDir {
		return
	}

	oldDir := resolveBackupDir(db.LegacyBackupDir)
	newDir := resolveBackupDir(db.DefaultBackupDir)
	if oldDir == newDir {
		return
	}

	entries, err := os.ReadDir(oldDir)
	if err != nil {
		logger.Warn("[backup] migrate: read legacy dir failed: dir=%s err=%v", oldDir, err)
		return
	}

	logger.Warn("[backup] migrate: old dir=%s dir=%s entries=%s", oldDir, newDir, entries)

	var filesToMove []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(strings.ToLower(entry.Name()), ".sqlite") {
			filesToMove = append(filesToMove, entry.Name())
		}
	}
	if len(filesToMove) == 0 {
		return
	}

	if err := helper.EnsureDir(newDir, 0755); err != nil {
		logger.Warn("[backup] migrate: create target dir failed: dir=%s err=%v", newDir, err)
		return
	}

	moved := 0
	for _, name := range filesToMove {
		src := filepath.Join(oldDir, name)
		dst := filepath.Join(newDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			logger.Warn("[backup] migrate: move file failed: src=%s dst=%s err=%v", src, dst, err)
			continue
		}
		moved++
	}

	if moved > 0 {
		logger.Info("[backup] migrate: moved %d backup file(s) from %s to %s", moved, oldDir, newDir)
		// delete old dir
		err := os.RemoveAll(oldDir)
		if err != nil {
			logger.Warn("[backup] migrate: remove old dir failed: dir=%s err=%v", oldDir, err)
		}
	}
}

func latestLocalBackupAt(backupDir string) (time.Time, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return time.Time{}, err
	}

	latest := time.Time{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sqlite") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return time.Time{}, err
		}
		if info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}

	return latest, nil
}

func pruneLocalBackupFiles(backupDir string, maxCount int) (int, error) {
	if maxCount <= 0 {
		return 0, nil
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return 0, err
	}

	type backupFile struct {
		path    string
		modTime time.Time
	}

	files := make([]backupFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".sqlite") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return 0, err
		}
		files = append(files, backupFile{
			path:    filepath.Join(backupDir, entry.Name()),
			modTime: info.ModTime(),
		})
	}

	if len(files) <= maxCount {
		return 0, nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	removed := 0
	for _, f := range files[maxCount:] {
		if err := os.Remove(f.path); err != nil {
			return removed, err
		}
		removed++
	}

	return removed, nil
}

// DeleteExpiredEncryptedPostsJob 软删除已过期的加密文章（expires_at < 当前时间）
func DeleteExpiredEncryptedPostsJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}
	nowUnix := time.Now().Unix()
	n, err := db.SoftDeleteExpiredEncryptedPosts(reg.DB, nowUnix)
	if err != nil {
		return nil, err
	}
	if n > 0 {
		return jobMessage(fmt.Sprintf("已软删除 %d 条过期加密文章", n)), nil
	}

	return jobMessage("暂无过期加密文章，无需处理"), nil
}

// CheckAppUpdateJob 检查当前 swaves 版本是否落后于最新稳定 release。
func CheckAppUpdateJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}
	if !buildinfo.IsReleaseVersion() {
		return jobMessage("非发布版本，跳过更新检查"), nil
	}

	result, err := checkLatestAppRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	if !result.HasUpgrade || result.Target == nil {
		return jobMessage(fmt.Sprintf("当前已是最新版本：%s", result.CurrentVersion)), nil
	}

	nowUnix := time.Now().Unix()
	if err := notify.CreateAppUpdateNotification(reg.DB, result.CurrentVersion, result.LatestVersion, result.LatestReleaseURL, nowUnix); err != nil {
		return nil, err
	}
	return jobMessage(fmt.Sprintf("app update available: %s -> %s", result.CurrentVersion, result.LatestVersion)), nil
}

// ClearExpiredNotificationsJob 清理过期通知（updated_at < now-retention_days）
func ClearExpiredNotificationsJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}

	nowTime := time.Now()
	cutoffUnix := notify.ExpiredBeforeUnix(nowTime)
	deletedCount, err := db.DeleteExpiredNotifications(reg.DB, cutoffUnix)
	if err != nil {
		return nil, err
	}

	retentionDays := notify.NotificationRetentionDays()
	return jobMessage(fmt.Sprintf("清理通知完成：deleted=%d retention_days=%d cutoff=%d", deletedCount, retentionDays, cutoffUnix)), nil
}
