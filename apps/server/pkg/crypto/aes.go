package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"
)

const (
	// defaultKey 开发环境默认密钥（32 bytes = AES-256）。生产环境必须通过 ASSET_ENCRYPTION_KEY 环境变量覆盖。
	defaultKey = "Framework2026DevEncryptionKey!!!"
	envKey     = "ASSET_ENCRYPTION_KEY"
)

var (
	ErrEncryptionFailed = errors.New("encryption failed")
	ErrDecryptionFailed = errors.New("decryption failed")
	ErrInvalidKey       = errors.New("invalid encryption key")

	instance *AESGCMCrypto
	once     sync.Once
)

// AESGCMCrypto 提供 AES-256-GCM 加解密能力。
type AESGCMCrypto struct {
	gcm cipher.AEAD
}

// GetCrypto 返回单例（首次调用时从环境变量或默认值初始化密钥）。
func GetCrypto() *AESGCMCrypto {
	once.Do(func() {
		c, err := NewAESGCMCrypto(getKey())
		if err != nil {
			panic(fmt.Sprintf("init AES crypto failed: %v", err))
		}
		instance = c
	})
	return instance
}

func getKey() []byte {
	if k := os.Getenv(envKey); len(k) == 32 {
		return []byte(k)
	}
	if k := os.Getenv(envKey); len(k) > 0 {
		// 非 32 字节的 key 做 padding/truncate 到 32 字节
		log.Printf("[Warn] 环境变量 %s 长度 %d 字节，非 32 字节（AES-256），将做零 padding/截断处理，建议配置 32 字节密钥", envKey, len(k))
		b := make([]byte, 32)
		copy(b, k)
		return b
	}
	log.Printf("[Warn] 未设置环境变量 %s，使用内置开发密钥，生产环境必须设置环境变量 %s", envKey, envKey)
	return []byte(defaultKey)
}

// NewAESGCMCrypto 使用 32 字节密钥创建 AES-256-GCM 加密器。
func NewAESGCMCrypto(key []byte) (*AESGCMCrypto, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("%w: key must be 32 bytes, got %d", ErrInvalidKey, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return &AESGCMCrypto{gcm: gcm}, nil
}

// Encrypt 加密明文，返回 base64 编码的密文（nonce + ciphertext）。
// 空字符串返回空字符串（不加密）。
func (c *AESGCMCrypto) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	nonce := make([]byte, c.gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("%w: generate nonce: %v", ErrEncryptionFailed, err)
	}

	ciphertext := c.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密 base64 编码的密文，返回明文。
// 空字符串返回空字符串。
func (c *AESGCMCrypto) Decrypt(ciphertext string) (string, error) {
	if ciphertext == "" {
		return "", nil
	}

	data, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("%w: base64 decode: %v", ErrDecryptionFailed, err)
	}

	nonceSize := c.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("%w: ciphertext too short", ErrDecryptionFailed)
	}

	nonce, ciphertextBytes := data[:nonceSize], data[nonceSize:]
	plaintext, err := c.gcm.Open(nil, nonce, ciphertextBytes, nil)
	if err != nil {
		return "", fmt.Errorf("%w: gcm open: %v", ErrDecryptionFailed, err)
	}

	return string(plaintext), nil
}

// EncryptPtr 加密 *string 指针指向的值，返回加密后的新指针。
// 输入 nil 返回 nil。
func (c *AESGCMCrypto) EncryptPtr(plain *string) (*string, error) {
	if plain == nil {
		return nil, nil
	}
	encrypted, err := c.Encrypt(*plain)
	if err != nil {
		return nil, err
	}
	return &encrypted, nil
}

// DecryptPtr 解密 *string 指针指向的值，返回解密后的新指针。
// 输入 nil 返回 nil。
func (c *AESGCMCrypto) DecryptPtr(cipher *string) (*string, error) {
	if cipher == nil {
		return nil, nil
	}
	decrypted, err := c.Decrypt(*cipher)
	if err != nil {
		return nil, err
	}
	return &decrypted, nil
}
