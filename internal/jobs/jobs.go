package job

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"swaves/helper"
	"swaves/internal/db"
	"time"
)

const (
	settingBackupLocalDir         = "backup_local_dir"
	settingBackupLocalIntervalMin = "backup_local_interval_min"
	settingBackupLocalMaxCount    = "backup_local_max_count"
	clearEncryptedPostsNoopMsg    = "没有发现需要清理的文章"
)

func HelloJob() error {
	log.Println("Hello Job executed!")
	time.Sleep(2 * time.Second)
	return nil
}
func HelloJob1() error {
	log.Println("Hello Job1 executed!")
	time.Sleep(23 * time.Second)
	return errors.New("fdsa error")
}

// DatabaseBackupJob 数据库备份任务
func DatabaseBackupJob(reg *Registry) (string, error) {
	if reg == nil || reg.DB == nil {
		return "", errors.New("reg.DB is nil")
	}

	cfg := loadLocalBackupConfig(reg)
	backupDir := resolveBackupDir(cfg.Dir)

	if err := helper.EnsureDir(backupDir, 0755); err != nil {
		return "", fmt.Errorf("无法创建备份目录 %s: %w", backupDir, err)
	}

	latestAt, err := latestLocalBackupAt(backupDir)
	if err != nil {
		return "", fmt.Errorf("读取本地备份目录失败: %w", err)
	}
	if !latestAt.IsZero() && cfg.Interval > 0 {
		elapsed := time.Since(latestAt)
		if elapsed < cfg.Interval {
			remain := (cfg.Interval - elapsed).Round(time.Second)
			return fmt.Sprintf("skip local backup: interval=%s remaining=%s", cfg.Interval, remain), nil
		}
	}

	result, err := db.ExportSQLiteWithHash(reg.DB, backupDir)
	if err != nil {
		if strings.Contains(err.Error(), "无需重复导出") {
			return err.Error(), nil
		}
		return "", err
	}

	removedCount, err := pruneLocalBackupFiles(backupDir, cfg.MaxCount)
	if err != nil {
		return "", fmt.Errorf("本地备份完成但清理旧备份失败: %w", err)
	}

	return fmt.Sprintf("%v, dir=%s, pruned=%d", result, backupDir, removedCount), nil
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

	dir := strings.TrimSpace(readSettingString(reg.DB, settingBackupLocalDir, defaultDir))
	if dir == "" {
		dir = defaultDir
	}

	intervalMin := readSettingInt(reg.DB, settingBackupLocalIntervalMin, 1440)
	if intervalMin < 1 {
		intervalMin = 1
	}

	maxCount := readSettingInt(reg.DB, settingBackupLocalMaxCount, 30)
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
		log.Println("[backup] getwd error:", err)
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
func DeleteExpiredEncryptedPostsJob(reg *Registry) (string, error) {
	if reg == nil || reg.DB == nil {
		return "", errors.New("reg.DB is nil")
	}
	nowUnix := time.Now().Unix()
	n, err := db.SoftDeleteExpiredEncryptedPosts(reg.DB, nowUnix)
	if err != nil {
		return "", err
	}
	if n > 0 {
		return fmt.Sprintf("已软删除 %d 条过期加密文章\n", n), nil
	}

	return clearEncryptedPostsNoopMsg + "\n", nil
}
