package job

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"swaves/helper"
	"swaves/internal/db"
	"time"
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
	err := helper.EnsureDir(reg.Config.BackupDir, 0755)
	if err != nil {
		log.Printf("无法创建备份目录: %v\n", err)
	}

	if reg == nil || reg.DB == nil {
		return "", errors.New("reg.DB is nil")
	}

	wd, err := os.Getwd()
	if err != nil {
		log.Println("[backup] getwd error:", err)
	}

	backupDir := filepath.Join(wd, reg.Config.BackupDir)

	// 调用 ExportSQLiteDatabase 函数
	result, err := db.ExportSQLiteWithHash(reg.DB, backupDir)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%v", result), nil
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

	return "没有发现需要清理的文章\n", nil
}
