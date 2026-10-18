package mme

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-kratos/kratos/v2/log"
)

// 平台文案长度硬约束（需求规格 FR-MIX-04；超长截断不报错）。
const (
	copyTitleMaxDouble   = 55  // 抖音标题
	copyDescMaxDouyin    = 300 // 抖音简介
	copyTitleMaxXHS      = 20  // 小红书标题
	copyDescMaxXHS       = 1000
	copyTitleMaxBili     = 80 // B 站标题
	copyDescMaxBili      = 2000
	copyTitleMaxYTS      = 100 // YouTube Shorts
	copyDescMaxYTS       = 5000
)

// 支持的平台集合——🔴 键必须与 MAP 平台 registry 的 platform_key 一致。
var copyPlatforms = []string{"douyin", "xiaohongshu", "bilibili", "youtube_shorts"}

// platformLimits 平台约束表。
func platformLimits(platform string) (titleMax, descMax int) {
	switch platform {
	case "douyin":
		return copyTitleMaxDouble, copyDescMaxDouyin
	case "xiaohongshu":
		return copyTitleMaxXHS, copyDescMaxXHS
	case "bilibili":
		return copyTitleMaxBili, copyDescMaxBili
	case "youtube_shorts":
		return copyTitleMaxYTS, copyDescMaxYTS
	}
	return 80, 2000
}

// truncateRunes 按 rune 截断。
func truncateRunes(s string, n int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= n {
		return string(r)
	}
	return string(r[:n])
}

// CopyGenerator 平台差异化文案生成器（FR-MIX-04）。
//
// 通路：LLM 可用 → 按平台 prompt 生成（产出须过长度校验截断）；
// LLM 不可用 → 🔴 模板降级，绝不编造：用素材标题/用户意图套固定句式，
// 并在返回中标 degraded=true（落到 Variant.copy_degraded 与 manifest）。
type CopyGenerator struct {
	cfg Config
	llm LLMClient
	log *log.Helper
}

// NewCopyGenerator 构造；llm 可为 nil（= 纯模板模式）。
func NewCopyGenerator(cfg Config, llm LLMClient, logger log.Logger) *CopyGenerator {
	return &CopyGenerator{cfg: cfg, llm: llm, log: log.NewHelper(log.With(logger, "module", "biz/mme/copy"))}
}

// CopyInput 生成输入。
type CopyInput struct {
	AssetTitle  string // 素材原始标题
	UserIntent  string // 用户意图（可空）
	HookText    string // Hook 文案（可空）
	Transcript  string // 转写摘要（可空）
	ContentNote string // 内容版本备注（可空）
}

// CopyOutput 生成输出。
type CopyOutput struct {
	Copy     map[string]PlatformCopy
	Degraded bool
}

// llmPrompt 平台 prompt 模板（键名与输出 key 严格一致）。
func llmPrompt(platform string, in CopyInput) string {
	limits1, limits2 := platformLimits(platform)
	return fmt.Sprintf(`你是短视频多平台运营文案写手。根据素材信息，为平台 %s 生成文案。
要求：
- title: 不超过 %d 字，符合该平台风格，不得夸大事实，不得出现素材中不存在的内容
- desc: 不超过 %d 字
- tags: 3-8 个话题标签（不含 # 前缀）
- 只输出 JSON：{"title":"...","desc":"...","tags":["..."]}，不要输出其他内容

素材标题: %s
用户意图: %s
Hook 文案: %s
内容备注: %s
转写摘要: %s`, platform, limits1, limits2, in.AssetTitle, in.UserIntent, in.HookText, in.ContentNote, truncateRunes(in.Transcript, 500))
}

// templateCopy 降级模板：只用已知事实组句（🔴 不编造卖点）。
func templateCopy(platform string, in CopyInput) PlatformCopy {
	tm, dm := platformLimits(platform)
	title := strings.TrimSpace(in.AssetTitle)
	if title == "" {
		title = "短视频"
	}
	if in.HookText != "" {
		title = in.HookText
	}
	desc := title
	if in.ContentNote != "" {
		desc = title + "｜" + in.ContentNote
	}
	return PlatformCopy{
		Title: truncateRunes(title, tm),
		Desc:  truncateRunes(desc, dm),
		Tags:  []string{"矩阵混剪"},
	}
}

// parseLLMJSON 从 LLM 输出提取文案（宽松：找首个 { 到末个 }）。
func parseLLMJSON(raw string, platform string) (PlatformCopy, bool) {
	i := strings.Index(raw, "{")
	j := strings.LastIndex(raw, "}")
	if i < 0 || j <= i {
		return PlatformCopy{}, false
	}
	tm, dm := platformLimits(platform)
	m, err := parseSimpleCopyJSON(raw[i : j+1])
	if err != nil {
		return PlatformCopy{}, false
	}
	m.Title = truncateRunes(m.Title, tm)
	m.Desc = truncateRunes(m.Desc, dm)
	return m, true
}

// Generate 生成四平台文案。LLM 单平台失败 → 该平台单项降级（不拖垮整体）。
func (g *CopyGenerator) Generate(ctx context.Context, in CopyInput) (*CopyOutput, error) {
	out := &CopyOutput{Copy: map[string]PlatformCopy{}}
	if g.llm == nil || !g.llm.Available(ctx) {
		g.log.Warnf("[%s] LLM unavailable, use template fallback (copy_degraded=true)", g.cfg.PromptVersion)
		for _, p := range copyPlatforms {
			out.Copy[p] = templateCopy(p, in)
		}
		out.Degraded = true
		return out, nil
	}
	degraded := false
	for _, p := range copyPlatforms {
		raw, err := g.llm.Generate(ctx, llmPrompt(p, in))
		if err != nil {
			g.log.Warnf("LLM generate failed for %s: %v; fallback to template", p, err)
			out.Copy[p] = templateCopy(p, in)
			degraded = true
			continue
		}
		pc, ok := parseLLMJSON(raw, p)
		if !ok {
			g.log.Warnf("LLM output unparsable for %s; fallback to template", p)
			out.Copy[p] = templateCopy(p, in)
			degraded = true
			continue
		}
		out.Copy[p] = pc
	}
	out.Degraded = degraded
	return out, nil
}
