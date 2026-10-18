package mme

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// HookTextResolver 由 hook_id 解析 Hook 文本（任务侧提供：hook 文本可能来自用户输入或模板）。
type HookTextResolver func(hookID string) string

// DiffChecker 差异化检查（FR-MIX-05）：同簇两两比对，综合相似度超阈值 → warn 强提示。
//
// 🔴 这是风控措施，不硬阻断审核（ADR-MME-04）；告警必须在审核台可见。
// 维度：
//   - 视觉视觉：依赖抽帧/pHash，本轮规划为 ffmpeg 抽帧后 aHash 比对；
//     渲染产物未就绪时该维度留空（不参与综合分，结果中注明 skipped）。
//   - Hook：Simhash(title+首 3s 文案)。
//   - 文案：Simhash(平台标题+简介)（copy_degraded 的模板文案天生高相似，
//     这正是需要告警的场景，不能豁免）。
type DiffChecker struct {
	cfg Config
	log *log.Helper
}

// NewDiffChecker 构造。
func NewDiffChecker(cfg Config, logger log.Logger) *DiffChecker {
	return &DiffChecker{cfg: cfg, log: log.NewHelper(log.With(logger, "module", "biz/mme/diff"))}
}

// DiffOutcome 检查结果。
type DiffOutcome struct {
	MinSimilarity float64
	Warn          bool
	Pairs         []PairSimilarity
	Summary       string
}

// overallSimilarity 综合分 = 三维度有效值取 max（任一维度过热都要提示）。
func overallSimilarity(visual, hook, copy float64, hasVisual bool) float64 {
	m := hook
	if copy > m {
		m = copy
	}
	if hasVisual && visual > m {
		m = visual
	}
	return m
}

// Check 对一组（通常同 content_version 内跨 aspect/hook 的）变体做两两比对。
// visualSim 可为 nil（无产物时）：传 nil 表示该维度跳过。
// resolveHook 解析 Hook 文案；platform 为文案比对目标平台（空 = douyin）。
func (c *DiffChecker) Check(_ context.Context, variants []*Variant, resolveHook HookTextResolver, platform string, visualSim func(a, b *Variant) (float64, bool)) *DiffOutcome {
	if platform == "" {
		platform = "douyin"
	}
	out := &DiffOutcome{MinSimilarity: 1}
	if !c.cfg.DiffEnabled || len(variants) < 2 {
		out.MinSimilarity = 0
		out.Summary = "差异化检查未启用或变体不足 2 个"
		return out
	}
	for i := 0; i < len(variants); i++ {
		for j := i + 1; j < len(variants); j++ {
			a, b := variants[i], variants[j]
			// 内容版本（B 维度）不同的变体天然差异（不同 EDL 切点），不比对
			if a.ContentVersion != b.ContentVersion {
				continue
			}
			pair := PairSimilarity{VariantA: a.ID, VariantB: b.ID}
			hasVisual := false
			if visualSim != nil {
				if v, ok := visualSim(a, b); ok {
					pair.Visual = v
					hasVisual = true
				}
			}
			pair.Hook = SimhashSimilarity(resolveHook(a.HookID), resolveHook(b.HookID))
			pair.Copy = SimhashSimilarity(copyText(a, platform), copyText(b, platform))
			pair.Overall = overallSimilarity(pair.Visual, pair.Hook, pair.Copy, hasVisual)
			out.Pairs = append(out.Pairs, pair)
		}
	}
	if len(out.Pairs) == 0 {
		out.MinSimilarity = 0
		out.Summary = "无同内容版本的变体对，无需告警"
		return out
	}
	SortPairs(out.Pairs)
	out.MinSimilarity = out.Pairs[0].Overall
	// 阈值判定：综合 ≥ warn_threshold → 告警
	if out.MinSimilarity >= c.cfg.DiffWarnThreshold {
		out.Warn = true
		for _, p := range out.Pairs {
			if p.Overall >= c.cfg.DiffWarnThreshold {
				c.log.Warnf("diff warn: %s ~ %s overall=%.2f (hook=%.2f copy=%.2f)",
					p.VariantA, p.VariantB, p.Overall, p.Hook, p.Copy)
			}
		}
	}
	if out.Warn {
		out.Summary = sprintfDiff(out.MinSimilarity, c.cfg.DiffWarnThreshold)
	} else {
		out.Summary = sprintfOK(out.MinSimilarity)
	}
	return out
}

// copyText 平台文案拼接（标题+简介）。
func copyText(v *Variant, platform string) string {
	if v.Copy == nil {
		return ""
	}
	if pc, ok := v.Copy[platform]; ok {
		return pc.Title + "\n" + pc.Desc
	}
	return ""
}

func sprintfDiff(min, th float64) string {
	return "检测到高相似变体对（max overall=" + f2(min) + " ≥ 阈值 " + f2(th) + "），建议在审核台复核 Hook/文案差异"
}
func sprintfOK(min float64) string {
	return "差异化检查通过（max overall=" + f2(min) + " < 阈值）"
}
