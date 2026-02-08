package job

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"swaves/internal/db"
	"swaves/util"
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
	err := util.EnsureDir(reg.Config.BackupDir, 0755)
	if err != nil {
		log.Fatalf("无法创建备份目录: %v", err)
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
