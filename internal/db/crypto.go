package db

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

const (
	// 系统加密密钥（用于 EncryptedPost 的内容加密）
	// 这是固定密钥，确保所有 EncryptedPost 使用相同的密钥加密
	// 未来可以从 settings 表中读取或使用环境变量
	DefaultEncryptedPostKey = "swaves-encrypted-post-key-2024"
)

// getEncryptionKey 获取系统加密密钥
// 返回: 32 字节的 AES-256 密钥
func getEncryptionKey() [32]byte {
	// 使用系统密钥生成 32 字节的 AES-256 密钥
	// 可以从 settings 表中读取 admin_password 的哈希值作为密钥的一部分
	// 这里使用固定密钥，确保所有 EncryptedPost 使用相同的密钥
	return sha256.Sum256([]byte(DefaultEncryptedPostKey))
}

// EncryptContent 使用 AES-256-GCM 加密内容
// plaintext: 要加密的明文内容
// 返回: base64 编码的加密数据（格式：nonce|encrypted_data|tag）
func EncryptContent(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	// 获取系统加密密钥
	key := getEncryptionKey()

	// 创建 AES cipher
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	// 创建 GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// 生成随机 nonce（12 字节用于 GCM）
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	// 加密
	ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)

	// 返回 base64 编码
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// DecryptContent 使用 AES-256-GCM 解密内容
// ciphertext: base64 编码的加密数据
// 返回: 解密后的明文
func DecryptContent(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	// 解码 base64
	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		// 如果 base64 解码失败，可能是旧数据（未加密），直接返回
		// 或者返回错误，取决于是否需要向后兼容
		return "", errors.New("invalid encrypted content format")
	}

	// 检查数据长度
	if len(data) < 32 {
		return "", errors.New("encrypted content too short")
	}

	// 获取系统加密密钥
	key := getEncryptionKey()

	// 创建 AES cipher
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}

	// 创建 GCM
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	// 检查数据长度
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	// 提取 nonce 和 ciphertext
	nonce, encrypted := data[:nonceSize], data[nonceSize:]

	// 解密
	plaintext, err := gcm.Open(nil, nonce, encrypted, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}
