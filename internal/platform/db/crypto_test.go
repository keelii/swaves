package db

import (
	"errors"
	"strings"
	"testing"

	"swaves/internal/platform/config"
)

// withoutEncryptionKey 模拟未配置 SWAVES_ENCRYPTED_POST_KEY 的状态。
// config 包级变量仅在 init 时读取一次环境，故显式清空并在 cleanup 中恢复。
func withoutEncryptionKey(t *testing.T) {
	t.Helper()
	prevKey := config.EncryptedPostKey
	prevEnabled := config.EncryptedPostEnabled
	config.EncryptedPostKey = ""
	config.EncryptedPostEnabled = false
	t.Cleanup(func() {
		config.EncryptedPostKey = prevKey
		config.EncryptedPostEnabled = prevEnabled
	})
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	withTestEncryptionKey(t)

	cases := []string{
		"hello secret",
		" multiline\ncontent with 中文 \n and symbols <>\"'& ",
		strings.Repeat("a", 4096),
	}

	for _, plain := range cases {
		ciphertext, err := EncryptContent(plain)
		if err != nil {
			t.Fatalf("EncryptContent failed: %v", err)
		}
		if ciphertext == plain {
			t.Fatal("ciphertext should not equal plaintext")
		}
		if ciphertext == "" {
			t.Fatal("ciphertext should not be empty for non-empty plaintext")
		}

		got, err := DecryptContent(ciphertext)
		if err != nil {
			t.Fatalf("DecryptContent failed: %v", err)
		}
		if got != plain {
			t.Fatalf("round-trip mismatch: want %q, got %q", plain, got)
		}
	}
}

func TestEncryptContentEmptyInputIsNoop(t *testing.T) {
	withTestEncryptionKey(t)

	got, err := EncryptContent("")
	if err != nil {
		t.Fatalf("EncryptContent(\"\") unexpected error: %v", err)
	}
	if got != "" {
		t.Fatalf("EncryptContent(\"\") = %q, want empty", got)
	}
}

func TestEncryptContentKeyNotSetReturnsError(t *testing.T) {
	withoutEncryptionKey(t)

	_, err := EncryptContent("anything")
	if !errors.Is(err, ErrEncryptionKeyNotSet) {
		t.Fatalf("EncryptContent without key: want ErrEncryptionKeyNotSet, got %v", err)
	}
}

func TestDecryptContentKeyNotSetReturnsError(t *testing.T) {
	withoutEncryptionKey(t)

	// 用一个合法的 base64 串，确保错误来自密钥缺失而非格式问题。
	_, err := DecryptContent("bm9uY2UtcGxhY2Vob2xkZXI=")
	if !errors.Is(err, ErrEncryptionKeyNotSet) {
		t.Fatalf("DecryptContent without key: want ErrEncryptionKeyNotSet, got %v", err)
	}
}

func TestDecryptContentInvalidBase64(t *testing.T) {
	withTestEncryptionKey(t)

	_, err := DecryptContent("!!! not valid base64 !!!")
	if err == nil {
		t.Fatal("DecryptContent with invalid base64: want error, got nil")
	}
}

func TestDecryptContentTooShort(t *testing.T) {
	withTestEncryptionKey(t)

	// 3 字节 base64 解码后短于 GCM nonce（12 字节）。
	_, err := DecryptContent("YWJj")
	if err == nil {
		t.Fatal("DecryptContent with too-short ciphertext: want error, got nil")
	}
}
