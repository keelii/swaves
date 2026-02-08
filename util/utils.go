package util

import (
	"fmt"
	"os"
)

func EnsureDir(dirPath string, perm os.FileMode) error {
	// 检查路径是否存在
	info, err := os.Stat(dirPath)
	if err == nil {
		// 路径存在，检查是否是目录
		if !info.IsDir() {
			return fmt.Errorf("路径存在但不是目录: %s", dirPath)
		}
		return nil // 目录已存在
	}

	// 如果错误是"不存在"，则创建目录
	if os.IsNotExist(err) {
		err = os.MkdirAll(dirPath, perm)
		if err != nil {
			return fmt.Errorf("创建目录失败: %w", err)
		}
		return nil
	}

	// 其他错误（权限问题等）
	return fmt.Errorf("检查目录失败: %w", err)
}
