package exectree

import "regexp"

// redactRules 进程输出的敏感串脱敏词表（HF token、Bearer、OpenAI key 等）。
// 词表从严：宁可误伤日志可读性，不外泄凭据（调研报告 §subprocess_backend
// stdout/stderr 过滤 HF token 的同款纪律）。
var redactRules = []*regexp.Regexp{
	regexp.MustCompile(`hf_[A-Za-z0-9]{12,}`),
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9._\-]{8,}`),
	regexp.MustCompile(`sk-[A-Za-z0-9]{16,}`),
	regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password)=["']?[A-Za-z0-9._\-]{8,}`),
	regexp.MustCompile(`(?i)authorization:\s*\S+.*`),
}

// Redact 对输出行做脱敏。空字符串原样返回。
func Redact(s string) string {
	if s == "" {
		return s
	}
	for _, re := range redactRules {
		s = re.ReplaceAllString(s, "***REDACTED***")
	}
	return s
}
