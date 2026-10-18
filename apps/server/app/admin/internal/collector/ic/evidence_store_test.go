package iccollector

import (
	"strings"
	"testing"
)

// =============================================================================
// PII 脱敏守卫（PIPL 红线 + RE2 无 look-around 的等价实现回归）。
// 历史事故：look-around 写法 MustCompile 于 init panic，整栈起不来——
// 本文件的存在即为「本包 init 安全」的活证据（导入即跑正则编译）。
// =============================================================================

func TestRedactPII(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"邮箱", "联系 foo@bar.com 或 a+tag@sub.x.cn 询价",
			"联系 [email-redacted] 或 [email-redacted] 询价"},
		{"手机号首字符", "13812345678 下单",
			"[phone-redacted] 下单"},
		{"手机号中嵌", "手机号 13998765432 微信同号",
			"手机号 [phone-redacted] 微信同号"},
		{"邻近同型共享边界（两遍兜底）", "13812345678,13998765432",
			"[phone-redacted],[phone-redacted]"},
		{"超长数字串内不红act", "订单号1381234567890勿改",
			"订单号1381234567890勿改"},
		{"身份证", "打款至账户，身份证620123199001011234办结",
			"打款至账户，身份证[id-redacted]办结"},
		{"身份证X结尾", "62012319900101123X",
			"[id-redacted]"},
		{"17位非证件不误伤", "编号12345678901234567有序",
			"编号12345678901234567有序"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(redactPII([]byte(c.in))); got != c.want {
				t.Fatalf("redactPII(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestRedactPIIPreservesChainBaseline 脱敏改变字节 ⇒ hash 随字节走：
// 与 B-17 链语义的既定口径（链提交的是「在档字节」而非线上原文）锁死。
func TestRedactPIIDeterministic(t *testing.T) {
	raw := []byte(strings.Repeat("页脚电话 13898701234，邮箱 care@corp.cn。", 100))
	a, b := redactPII(raw), redactPII(raw)
	if string(a) != string(b) {
		t.Fatal("redactPII 必须确定性（同入同出，证据链可复核）")
	}
}
