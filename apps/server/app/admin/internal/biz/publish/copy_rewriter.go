package publish

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// CopyRewriteResult 平台文案重写结论（FR-PUB-03）。截断只降级不阻断，
// 每次实际发生的降级都在 Warnings 留证（随发布响 response 返回，前端展示）。
type CopyRewriteResult struct {
	Title    string
	Desc     string
	Tags     []string
	Warnings []string
}

// RewriteForPlatform 按平台能力声明裁剪文案。
//
// V1 仅做**确定性截断**（rune 安全 + 省略号）；语气改写（学 OD /ai/adapt）为
// Phase 3 能力，TODO 标注，不伪造「已改写」语义。
func RewriteForPlatform(cap Capability, title, desc string, tags []string) CopyRewriteResult {
	r := CopyRewriteResult{Title: title, Desc: desc, Tags: tags}
	if cap.TitleMaxLen > 0 {
		if newTitle, changed := truncateRunes(title, cap.TitleMaxLen); changed {
			r.Title = newTitle
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("标题超出平台上限 %d 字，已截断（%d → %d）", cap.TitleMaxLen, utf8.RuneCountInString(title), cap.TitleMaxLen))
		}
	}
	if cap.DescMaxLen > 0 {
		if newDesc, changed := truncateRunes(desc, cap.DescMaxLen); changed {
			r.Desc = newDesc
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("描述超出平台上限 %d 字，已截断（%d → %d）", cap.DescMaxLen, utf8.RuneCountInString(desc), cap.DescMaxLen))
		}
	}
	// TODO(Phase3): 平台语气/标签风格改写（参考 OD /ai/adapt 接口）。当前标签原样透传。
	return r
}

// truncateRunes rune 安全截断；max<=0 视为不限，不触发截断。
// 截断时优先在末尾标点/空白处收刀，再补「…」（省略号计入上限的近似：先斩到 max-1）。
func truncateRunes(s string, max int) (string, bool) {
	if max <= 0 || utf8.RuneCountInString(s) <= max {
		return s, false
	}
	runes := []rune(s)
	cut := max - 1 // 留一个位置给省略号
	if cut < 1 {
		cut = 1
	}
	// 从断头往回找标点/空白，最多回看 12 字，避免截掉半个词（对拉丁语系更友好）。
	lower := cut - 12
	if lower < cut/2 {
		lower = cut / 2
	}
	for i := cut; i > lower; i-- {
		c := runes[i-1]
		if c == ' ' || c == '\n' || strings.ContainsRune(",.;:!?、。，；：！？-)）]】", c) {
			cut = i - 1
			break
		}
	}
	return strings.TrimRight(string(runes[:cut]), " \n") + "…", true
}
