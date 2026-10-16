package crypto

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"sort"
)

// License 签名模型（docs/开发文档/10-业务管理/03-详细设计 §2.6.0）：
// 私钥由厂商离线持有，永不进入客户部署环境；系统仅持公钥验签。
// 规范载荷（canonical payload）字段与键序固定，任何变更都会使既有签名失效。

var (
	ErrLicenseSignatureInvalid = errors.New("license signature invalid")
	ErrLicenseKeyInvalid       = errors.New("license key invalid")
)

// LicensePayload 许可证规范签名载荷。authorized_modules 规范化为升序去重。
type LicensePayload struct {
	LicenseNo          string   `json:"license_no"`
	CustomerName       string   `json:"customer_name"`
	AuthorizedModules  []string `json:"authorized_modules"`
	AuthorizedDevices  int      `json:"authorized_devices"`
	MaxUsers           int      `json:"max_users"`
	StartDate          string   `json:"start_date"` // YYYY-MM-DD
	ExpireDate         string   `json:"expire_date"`
	MachineFingerprint string   `json:"machine_fingerprint"` // 可空字符串（不绑定机器指纹时为空）
}

// CanonicalJSON 返回规范序列化：UTF-8、无空白字符、键序固定（struct 声明序）、模块升序去重。
func (p LicensePayload) CanonicalJSON() []byte {
	modules := make([]string, 0, len(p.AuthorizedModules))
	seen := make(map[string]struct{}, len(p.AuthorizedModules))
	for _, m := range p.AuthorizedModules {
		if _, ok := seen[m]; ok {
			continue
		}
		seen[m] = struct{}{}
		modules = append(modules, m)
	}
	sort.Strings(modules)
	p.AuthorizedModules = modules
	// encoding/json 按 struct 字段声明序输出，紧凑无空白，满足规范载荷约定。
	b, _ := json.Marshal(p)
	return b
}

// LoadRSAPublicKey 从 PEM 文件加载 RSA 公钥（PKIX 或 PKCS1 格式）。
func LoadRSAPublicKey(path string) (*rsa.PublicKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read public key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%w: pem decode failed", ErrLicenseKeyInvalid)
	}
	if pub, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		if rsaPub, ok := pub.(*rsa.PublicKey); ok {
			return rsaPub, nil
		}
		return nil, fmt.Errorf("%w: not an rsa public key", ErrLicenseKeyInvalid)
	}
	if rsaPub, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return rsaPub, nil
	}
	return nil, fmt.Errorf("%w: unsupported public key format", ErrLicenseKeyInvalid)
}

// LoadRSAPrivateKey 从 PEM 文件加载 RSA 私钥（PKCS1 或 PKCS8）。仅供厂商离线签名工具使用，客户部署环境不得包含私钥。
func LoadRSAPrivateKey(path string) (*rsa.PrivateKey, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read private key: %w", err)
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return nil, fmt.Errorf("%w: pem decode failed", ErrLicenseKeyInvalid)
	}
	if priv, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return priv, nil
	}
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: unsupported private key format", ErrLicenseKeyInvalid)
	}
	priv, ok := key.(*rsa.PrivateKey)
	if !ok {
		return nil, fmt.Errorf("%w: not an rsa private key", ErrLicenseKeyInvalid)
	}
	return priv, nil
}

// SignRSASHA256 对规范载荷做 RSA-SHA256 签名，返回 base64 编码签名。仅供厂商离线签名工具使用。
func SignRSASHA256(priv *rsa.PrivateKey, canonicalPayload []byte) (string, error) {
	if priv == nil {
		return "", fmt.Errorf("%w: nil private key", ErrLicenseKeyInvalid)
	}
	digest := sha256.Sum256(canonicalPayload)
	sig, err := rsa.SignPKCS1v15(nil, priv, crypto.SHA256, digest[:])
	if err != nil {
		return "", fmt.Errorf("sign: %w", err)
	}
	return base64.StdEncoding.EncodeToString(sig), nil
}

// VerifyRSASHA256 用厂商公钥验证规范载荷的 base64 签名；失败返回 ErrLicenseSignatureInvalid。
func VerifyRSASHA256(pub *rsa.PublicKey, canonicalPayload []byte, signatureB64 string) error {
	if pub == nil {
		return fmt.Errorf("%w: nil public key", ErrLicenseKeyInvalid)
	}
	sig, err := base64.StdEncoding.DecodeString(signatureB64)
	if err != nil {
		return fmt.Errorf("%w: signature base64 decode failed", ErrLicenseSignatureInvalid)
	}
	digest := sha256.Sum256(canonicalPayload)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		return ErrLicenseSignatureInvalid
	}
	return nil
}
