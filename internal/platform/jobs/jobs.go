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
	"swaves/internal/shared/pathutil"
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

// DatabaseBackupJob 数据库备份任务
func DatabaseBackupJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}

	cfg := loadLocalBackupConfig(reg)
	ret, noOp, err := runLocalBackup(reg.DB, cfg, true)
	if err != nil {
		return nil, err
	}
	if noOp {
		return nil, nil
	}
	return ret, nil
}

func RunLocalBackupNow(dbx *db.DB) (*string, error) {
	if dbx == nil {
		return nil, errors.New("db is nil")
	}

	cfg := loadLocalBackupConfig(&Registry{Config: types.AppConfig{SqliteFile: dbx.DSN}})
	ret, _, err := runLocalBackup(dbx, cfg, false)
	return ret, err
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

	return runRemoteBackupJob(&Registry{DB: dbx, Config: reg.Config}, false)
}

func runLocalBackup(dbx *db.DB, cfg localBackupConfig, checkInterval bool) (*string, bool, error) {
	backupDir := resolveBackupDirForSQLite(cfg.Dir, cfg.SqliteFile)
	logger.Info("[backup] local backup start: dir=%s interval=%s max_count=%d check_interval=%t", backupDir, cfg.Interval, cfg.MaxCount, checkInterval)

	if err := helper.EnsureDir(backupDir, 0755); err != nil {
		return nil, false, fmt.Errorf("无法创建备份目录 %s: %w", backupDir, err)
	}

	if checkInterval {
		latestAt, err := latestLocalBackupAt(backupDir)
		if err != nil {
			return nil, false, fmt.Errorf("读取本地备份目录失败: %w", err)
		}
		if !latestAt.IsZero() && cfg.Interval > 0 {
			elapsed := time.Since(latestAt)
			if elapsed < cfg.Interval {
				return nil, true, nil
			}
		}
	}

	result, err := db.ExportSQLiteWithHash(dbx, backupDir)
	if err != nil {
		if strings.Contains(err.Error(), "无需重复导出") {
			return jobMessage(err.Error()), true, nil
		}
		logger.Error("[backup] local backup export failed: dir=%s err=%v", backupDir, err)
		return nil, false, err
	}

	removedCount, err := pruneLocalBackupFiles(backupDir, cfg.MaxCount)
	if err != nil {
		logger.Error("[backup] local backup prune failed: dir=%s err=%v", backupDir, err)
		return nil, false, fmt.Errorf("本地备份完成但清理旧备份失败: %w", err)
	}

	message := fmt.Sprintf("%v, dir=%s, pruned=%d", result, backupDir, removedCount)
	logger.Info("[backup] local backup completed: %s", message)
	return jobMessage(message), false, nil
}

type localBackupConfig struct {
	Dir        string
	SqliteFile string
	Interval   time.Duration
	MaxCount   int
}

func loadLocalBackupConfig(reg *Registry) localBackupConfig {
	defaultDir := strings.TrimSpace(reg.Config.BackupDir)
	if defaultDir == "" {
		defaultDir = updater.DefaultBackupDir
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
		Dir:        dir,
		SqliteFile: reg.Config.SqliteFile,
		Interval:   time.Duration(intervalMin) * time.Minute,
		MaxCount:   maxCount,
	}
}

func ResolveLocalBackupDir(dir string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = updater.DefaultBackupDir
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

func ResolveLocalBackupDirForSQLite(dir string, sqliteFile string) string {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = updater.DefaultBackupDir
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	cacheRoot, err := pathutil.ResolveDatabaseCacheRoot(sqliteFile)
	if err != nil {
		logger.Warn("[backup] resolve sqlite cache root error: sqlite=%s err=%v", strings.TrimSpace(sqliteFile), err)
		return ResolveLocalBackupDir(dir)
	}

	rel := filepath.Clean(dir)
	if rel == ".cache" {
		return cacheRoot
	}
	rel = strings.TrimPrefix(rel, ".cache"+string(os.PathSeparator))
	if rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		logger.Warn("[backup] relative backup dir outside cache root ignored: dir=%s cache_root=%s", dir, cacheRoot)
		return filepath.Join(cacheRoot, "backups")
	}
	return filepath.Join(cacheRoot, rel)
}

func resolveBackupDirForSQLite(dir string, sqliteFile string) string {
	return ResolveLocalBackupDirForSQLite(dir, sqliteFile)
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

	return nil, nil
}

// CheckAppUpdateJob 检查当前 swaves 版本是否落后于最新稳定 release。
func CheckAppUpdateJob(reg *Registry) (*string, error) {
	if reg == nil || reg.DB == nil {
		return nil, errors.New("reg.DB is nil")
	}
	if !buildinfo.IsReleaseVersion() {
		return nil, nil
	}

	result, err := updater.CheckLatestRelease(buildinfo.Version, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return nil, err
	}
	if !result.HasUpgrade || result.Target == nil {
		return nil, nil
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
	if deletedCount == 0 {
		return nil, nil
	}

	retentionDays := notify.NotificationRetentionDays()
	return jobMessage(fmt.Sprintf("清理通知完成：deleted=%d retention_days=%d cutoff=%d", deletedCount, retentionDays, cutoffUnix)), nil
}
