package password

import (
	"strings"
	"testing"
)

func TestValidateComplexity(t *testing.T) {
	tests := []struct {
		name    string
		pwd     string
		cfg     PolicyConfig
		wantErr string // 期望错误信息包含的子串，为空表示期望通过
	}{
		{
			name:    "空密码",
			pwd:     "",
			cfg:     PolicyConfig{MinLength: 8},
			wantErr: "密码不能为空",
		},
		{
			name:    "ASCII 长度不足",
			pwd:     "Ab1",
			cfg:     PolicyConfig{MinLength: 8},
			wantErr: "密码长度不能少于 8 位",
		},
		{
			name: "中文密码按字符计长-不足",
			// "中文ab12" 共 6 个字符（10 字节），按 rune 计长应判定不足 8 位；
			// 若按字节计长（10 >= 8）会误判通过
			pwd:     "中文ab12",
			cfg:     PolicyConfig{MinLength: 8},
			wantErr: "密码长度不能少于 8 位",
		},
		{
			name:    "中文密码按字符计长-满足",
			pwd:     "中文密码测试Abcd1234",
			cfg:     PolicyConfig{MinLength: 8},
			wantErr: "",
		},
		{
			name: "满足全部复杂度要求",
			pwd:  "Abcdefg1",
			cfg: PolicyConfig{
				MinLength:        8,
				RequireUppercase: true,
				RequireLowercase: true,
				RequireDigit:     true,
			},
			wantErr: "",
		},
		{
			name:    "缺少大写字母",
			pwd:     "abcdefg1",
			cfg:     PolicyConfig{MinLength: 8, RequireUppercase: true},
			wantErr: "密码必须包含大写字母",
		},
		{
			name:    "缺少数字",
			pwd:     "Abcdefgh",
			cfg:     PolicyConfig{MinLength: 8, RequireDigit: true},
			wantErr: "密码必须包含数字",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateComplexity(tt.pwd, tt.cfg)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("期望通过，实际返回错误: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("期望错误 %q，实际通过", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("期望错误包含 %q，实际为 %q", tt.wantErr, err.Error())
			}
		})
	}
}
