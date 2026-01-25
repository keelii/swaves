package job

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
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
func DatabaseBackupJob(reg *Registry) error {
	if reg == nil || reg.DB == nil {
		return errors.New("reg.DB is nil")
	}

	// 生成备份文件名（包含时间戳）
	appName := "swaves"

	wd, err := os.Getwd()
	if err != nil {
		log.Println("[backup] getwd error:", err)
	}

	timestamp := time.Now().Format("2006-01-02-15-04-05")
	backupFilename := fmt.Sprintf("%s_backup_%s.sqlite", appName, timestamp)
	backupPath := filepath.Join(wd, reg.Config.BackupDir, backupFilename)

	log.Printf("[backup] starting database backup to: %s", backupPath)

	// 调用 ExportSQLiteDatabase 函数
	size, err := db.ExportSQLiteDatabase(reg.DB, backupPath)
	if err != nil {
		return fmt.Errorf("failed to export database: %v", err)
	}

	log.Printf("[backup] database backup completed: %s %d", backupPath, size)
	return nil
}
