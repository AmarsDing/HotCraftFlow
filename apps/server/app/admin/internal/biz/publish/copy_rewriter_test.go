package publish

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// 超长标题截断 + 警告留证（中文 rune 安全，不得斩半个汉字）。
func TestRewrite_TitleTruncation_RuneSafe(t *testing.T) {
	title := strings.Repeat("热", 150) // 150 个汉字 = 150 runes
	r := RewriteForPlatform(Capability{TitleMaxLen: 100}, title, "", nil)
	if !utf8.ValidString(r.Title) {
		t.Fatal("截断产物必须是合法 UTF-8")
	}
	if got := utf8.RuneCountInString(r.Title); got > 100 {
		t.Fatalf("截断后超过上限: %d runes", got)
	}
	if !strings.HasSuffix(r.Title, "…") {
		t.Fatalf("截断应补省略号: %q", tail(r.Title, 5))
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "标题") {
		t.Fatalf("应有 1 条标题降级警告，实际 %+v", r.Warnings)
	}
}

// 标点回看：断头附近有标点时优先在标点处收刀（英文句子场景）。
func TestTruncateRunes_PunctLookback(t *testing.T) {
	// max=30：断头 29，回看窗内最后一个空格收刀。
	s := "uploads schedule matrix build ok for you"
	out, changed := truncateRunes(s, 30)
	if !changed {
		t.Fatal("应触发截断")
	}
	if utf8.RuneCountInString(out) > 30 {
		t.Fatalf("超过上限: %d", utf8.RuneCountInString(out))
	}
	if !strings.HasSuffix(out, "…") {
		t.Fatalf("缺少省略号: %q", out)
	}
	core := strings.TrimSuffix(out, "…")
	if core == s[:29] {
		t.Fatalf("未发生标点收刀（直接硬切）: %q", out)
	}
}

// 未超限不变更；超长描述同样留证；max<=0 视为不限。
func TestRewrite_NoChangeWhenFit(t *testing.T) {
	r := RewriteForPlatform(Capability{TitleMaxLen: 100, DescMaxLen: 280}, "短标题", "短描述", []string{"a"})
	if r.Title != "短标题" || r.Desc != "短描述" || len(r.Warnings) != 0 {
		t.Fatalf("未超限不应变更: %+v", r)
	}
	// 无上限（0/缺省）→ 永不截断。
	r2 := RewriteForPlatform(Capability{}, strings.Repeat("x", 9999), strings.Repeat("y", 9999), nil)
	if r2.Title != strings.Repeat("x", 9999) || r2.Desc != strings.Repeat("y", 9999) {
		t.Fatal("无上限平台不应截断")
	}
}

// 描述截断独立留证。
func TestRewrite_DescTruncation(t *testing.T) {
	desc := strings.Repeat("描写。", 600) // 1800 runes
	r := RewriteForPlatform(Capability{DescMaxLen: 1000}, "", desc, nil)
	if utf8.RuneCountInString(r.Desc) > 1000 {
		t.Fatalf("描述截断后超限: %d", utf8.RuneCountInString(r.Desc))
	}
	if len(r.Warnings) != 1 || !strings.Contains(r.Warnings[0], "描述") {
		t.Fatalf("应有 1 条描述降级警告，实际 %+v", r.Warnings)
	}
}

func tail(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[len(r)-n:])
}
