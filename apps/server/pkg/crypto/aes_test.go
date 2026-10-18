package crypto

import (
	"encoding/base64"
	"strings"
	"testing"
)

func testKey(b byte) []byte {
	return []byte(strings.Repeat(string(b), 32))
}

// TestEncryptDecryptRoundTrip 核心路径：加密→解密还原明文。
func TestEncryptDecryptRoundTrip(t *testing.T) {
	c, err := NewAESGCMCrypto(testKey('a'))
	if err != nil {
		t.Fatalf("new crypto: %v", err)
	}
	plain := "ssh-password-p@ssw0rd!中文"
	enc, err := c.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if enc == "" || enc == plain {
		t.Fatalf("ciphertext should be non-empty base64 and differ from plaintext, got %q", enc)
	}
	dec, err := c.Decrypt(enc)
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	if dec != plain {
		t.Fatalf("round trip mismatch: got %q want %q", dec, plain)
	}
}

// TestKeyRotation 密钥轮换场景（SP-1.4 回归测试）：
// 旧钥密文新钥解不开；旧钥解密→新钥重加密→新钥可解。
func TestKeyRotation(t *testing.T) {
	oldC, err := NewAESGCMCrypto(testKey('o'))
	if err != nil {
		t.Fatalf("new old crypto: %v", err)
	}
	newC, err := NewAESGCMCrypto(testKey('n'))
	if err != nil {
		t.Fatalf("new new crypto: %v", err)
	}

	plain := "rotate-me"
	oldEnc, err := oldC.Encrypt(plain)
	if err != nil {
		t.Fatalf("encrypt old: %v", err)
	}

	// 新钥无法解旧密文（轮换前工具 dry-run 依赖此行为识别待轮换行）
	if _, err := newC.Decrypt(oldEnc); err == nil {
		t.Fatal("new key must not decrypt old ciphertext")
	}

	// 轮换：旧钥解 → 新钥加密 → 新钥可解
	dec, err := oldC.Decrypt(oldEnc)
	if err != nil {
		t.Fatalf("decrypt old: %v", err)
	}
	newEnc, err := newC.Encrypt(dec)
	if err != nil {
		t.Fatalf("encrypt new: %v", err)
	}
	got, err := newC.Decrypt(newEnc)
	if err != nil {
		t.Fatalf("decrypt new: %v", err)
	}
	if got != plain {
		t.Fatalf("rotated mismatch: got %q want %q", got, plain)
	}
}

// TestEncryptNonceRandom 同一明文两次加密产生不同密文（nonce 随机）。
func TestEncryptNonceRandom(t *testing.T) {
	c, _ := NewAESGCMCrypto(testKey('a'))
	e1, _ := c.Encrypt("same")
	e2, _ := c.Encrypt("same")
	if e1 == e2 {
		t.Fatal("nonce must be random: identical ciphertext for identical plaintext")
	}
}

// TestEmptyAndInvalidInput 边界：空串直通、非 32 字节密钥拒绝、坏 base64/短密文/篡改密文报错。
func TestEmptyAndInvalidInput(t *testing.T) {
	c, _ := NewAESGCMCrypto(testKey('a'))

	if enc, err := c.Encrypt(""); err != nil || enc != "" {
		t.Fatalf("encrypt empty should pass through, got %q err=%v", enc, err)
	}
	if dec, err := c.Decrypt(""); err != nil || dec != "" {
		t.Fatalf("decrypt empty should pass through, got %q err=%v", dec, err)
	}

	if _, err := NewAESGCMCrypto([]byte("short")); err == nil {
		t.Fatal("non-32-byte key must be rejected")
	}
	if _, err := c.Decrypt("!!!not-base64!!!"); err == nil {
		t.Fatal("invalid base64 must fail")
	}
	if _, err := c.Decrypt("aGVsbG8="); err == nil { // "hello"，短于 nonce
		t.Fatal("short ciphertext must fail")
	}
	enc, _ := c.Encrypt("tamper-target")
	// 篡改密文字节（确定性）：base64 段翻转可能只落在填充位导致偶发假阳性，
	// 故解码后翻转末字节——nonce||ciphertext 布局下末 16 字节是 GCM tag，翻转必验签失败。
	raw, derr := base64.StdEncoding.DecodeString(enc)
	if derr != nil {
		t.Fatalf("ciphertext must be valid base64: %v", derr)
	}
	raw[len(raw)-1] ^= 0x01
	tampered := base64.StdEncoding.EncodeToString(raw)
	if _, err := c.Decrypt(tampered); err == nil {
		t.Fatal("tampered ciphertext must fail GCM auth")
	}
}

// TestPtrHelpers 指针辅助函数：nil 直通，非 nil 往返。
func TestPtrHelpers(t *testing.T) {
	c, _ := NewAESGCMCrypto(testKey('a'))

	if p, err := c.EncryptPtr(nil); err != nil || p != nil {
		t.Fatalf("EncryptPtr(nil) should be nil,nil got %v,%v", p, err)
	}
	if p, err := c.DecryptPtr(nil); err != nil || p != nil {
		t.Fatalf("DecryptPtr(nil) should be nil,nil got %v,%v", p, err)
	}

	plain := "ptr-value"
	enc, err := c.EncryptPtr(&plain)
	if err != nil {
		t.Fatalf("EncryptPtr: %v", err)
	}
	dec, err := c.DecryptPtr(enc)
	if err != nil {
		t.Fatalf("DecryptPtr: %v", err)
	}
	if *dec != plain {
		t.Fatalf("ptr round trip mismatch: got %q want %q", *dec, plain)
	}
}
