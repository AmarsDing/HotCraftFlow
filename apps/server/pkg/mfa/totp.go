// Package mfa 提供 MFA(TOTP) 相关工具：密钥生成、验证码校验、otpauth URL 构造。
package mfa

import (
	"fmt"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	// Issuer TOTP 颁发者名称（显示在认证器 App 中）
	Issuer = "Framework"
	// Period TOTP 周期（秒）
	Period = 30
	// Digits TOTP 位数
	Digits = 6
	// Algorithm TOTP 算法
	Algorithm = "SHA1"
)

// GenerateSecret 为指定账户生成新的 TOTP 密钥（base32 编码字符串）。
// account 通常是用户名或 user_id，用于在认证器 App 中区分条目。
func GenerateSecret(account string) (secret string, url string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      Issuer,
		AccountName: account,
		Period:      Period,
		Digits:      otp.Digits(Digits),
		Algorithm:   otp.AlgorithmSHA1,
	})
	if err != nil {
		return "", "", fmt.Errorf("generate totp key failed: %w", err)
	}
	return key.Secret(), key.URL(), nil
}

// ValidateCode 校验 TOTP 验证码是否有效（含 ±1 周期容差，防时钟漂移）。
func ValidateCode(code, secret string) bool {
	valid, err := totp.ValidateCustom(code, secret, time.Now(),
		totp.ValidateOpts{
			Period:    Period,
			Skew:      1, // 允许前后各 1 个周期（共 90s）容差
			Digits:    otp.Digits(Digits),
			Algorithm: otp.AlgorithmSHA1,
		},
	)
	if err != nil {
		return false
	}
	return valid
}
