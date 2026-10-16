// Package password 提供密码策略校验与复杂度规则。
package password

import (
	"errors"
	"fmt"
	"unicode"
)

// PolicyConfig 密码策略配置，由 system_configs 模块注入。
type PolicyConfig struct {
	MinLength         int  // 最小长度
	RequireUppercase  bool // 必须包含大写字母
	RequireLowercase  bool // 必须包含小写字母
	RequireDigit      bool // 必须包含数字
	RequireSpecial    bool // 必须包含特殊字符
	HistoryCheckCount int  // 历史密码校验数量（防止重复使用最近 N 次密码）
	ExpireDays        int  // 密码过期天数（0 表示不过期）
}

// DefaultPolicy 返回默认密码策略（与详细设计文档预置配置项一致）。
func DefaultPolicy() PolicyConfig {
	return PolicyConfig{
		MinLength:         8,
		RequireUppercase:  true,
		RequireLowercase:  true,
		RequireDigit:      true,
		RequireSpecial:    false,
		HistoryCheckCount: 5,
		ExpireDays:        90,
	}
}

// ValidateComplexity 校验密码复杂度，不满足则返回描述性错误。
func ValidateComplexity(pwd string, cfg PolicyConfig) error {
	// 按字符数（rune）计长，避免中文等多字节字符按字节计长导致长度判定偏松
	pwdLen := len([]rune(pwd))

	if pwdLen == 0 {
		return errors.New("密码不能为空")
	}

	if cfg.MinLength > 0 && pwdLen < cfg.MinLength {
		return fmt.Errorf("密码长度不能少于 %d 位", cfg.MinLength)
	}

	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, ch := range pwd {
		switch {
		case unicode.IsUpper(ch):
			hasUpper = true
		case unicode.IsLower(ch):
			hasLower = true
		case unicode.IsDigit(ch):
			hasDigit = true
		case unicode.IsPunct(ch) || unicode.IsSymbol(ch):
			hasSpecial = true
		}
	}

	if cfg.RequireUppercase && !hasUpper {
		return errors.New("密码必须包含大写字母")
	}
	if cfg.RequireLowercase && !hasLower {
		return errors.New("密码必须包含小写字母")
	}
	if cfg.RequireDigit && !hasDigit {
		return errors.New("密码必须包含数字")
	}
	if cfg.RequireSpecial && !hasSpecial {
		return errors.New("密码必须包含特殊字符")
	}

	return nil
}
