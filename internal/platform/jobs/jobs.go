package job

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"swaves/internal/platform/db"
	"swaves/internal/platform/logger"
	"swaves/internal/platform/notify"
	"swaves/internal/platform/store"
	"swaves/internal/shared/helper"
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
	backupDir := resolveBackupDir(cfg.Dir)

	if err := helper.EnsureDir(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("无法创建备份目录 %s: %w", backupDir, err)
	}

	latestAt, err := latestLocalBackupAt(backupDir)
	if err != nil {
		return nil, fmt.Errorf("读取本地备份目录失败: %w", err)
	}
	if !latestAt.IsZero() && cfg.Interval > 0 {
		elapsed := time.Since(latestAt)
		if elapsed < cfg.Interval {
			remain := (cfg.Interval - elapsed).Round(time.Second)
			return jobMessage(fmt.Sprintf("skip local backup: interval=%s remaining=%s", cfg.Interval, remain)), nil
		}
	}

	result, err := db.ExportSQLiteWithHash(reg.DB, backupDir)
	if err != nil {
		if strings.Contains(err.Error(), "无需重复导出") {
			return jobMessage(err.Error()), nil
		}
		return nil, err
	}

	removedCount, err := pruneLocalBackupFiles(backupDir, cfg.MaxCount)
	if err != nil {
		return nil, fmt.Errorf("本地备份完成但清理旧备份失败: %w", err)
	}

	return jobMessage(fmt.Sprintf("%v, dir=%s, pruned=%d", result, backupDir, removedCount)), nil
}

type localBackupConfig struct {
	Dir      string
	Interval time.Duration
	MaxCount int
}

func loadLocalBackupConfig(reg *Registry) localBackupConfig {
	defaultDir := strings.TrimSpace(reg.Config.BackupDir)
	if defaultDir == "" {
		defaultDir = "backups"
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

func resolveBackupDir(dir string) string {
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
