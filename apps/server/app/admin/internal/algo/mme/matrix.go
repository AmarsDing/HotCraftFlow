// Package mme 实现 ④ MME 短视频矩阵混剪的核心算法。
// 对应 docs/systems/04-MME/03-详细设计.md §2。
package mme

import (
	"fmt"

	"framework/app/admin/internal/algo/textutil"
	"framework/pkg/mediacontract"
)

// MaxVariantsPerSet 单 VariantSet 的变体上限（规模保护，评审确认）。
// 渲染是算力密集操作，无上限的笛卡尔积会瞬间打满队列。
const MaxVariantsPerSet = 60

// ContentVersion B 维度：内容版本（来自高光候选）。
type ContentVersion struct {
	Label  string
	EDLRef string // 切点在 B 维度确定
}

// Hook C 维度：开头钩子（默认 ≥2）。
type Hook struct {
	ID string
}

// ExpandRequest 矩阵展开请求（FR-MIX-01）。
type ExpandRequest struct {
	TaskID          string
	ContentVersions []ContentVersion // B 维度
	Aspects         []string         // A 维度，默认 [9:16, 1:1, 16:9]
	Hooks           []Hook           // C 维度
	TemplateID      string
	Origin          mediacontract.VariantOrigin
}

// ExpandError 展开错误（规模超限）。
type ExpandError struct{ Count int }

func (e *ExpandError) Error() string {
	return fmt.Sprintf("组合数 %d 超过上限 %d，请减少内容版本或 Hook 数量", e.Count, MaxVariantsPerSet)
}

// Expand 执行矩阵展开（先展开后渲染，评审确认的设计）。
// 只做笛卡尔积的**元数据**展开，不渲染（渲染是算力密集，单独调度）。
// 规模超过 MaxVariantsPerSet 时拒绝展开。
func Expand(req ExpandRequest) (*mediacontract.VariantSet, error) {
	aspects := req.Aspects
	if len(aspects) == 0 {
		aspects = []string{"9:16", "1:1", "16:9"}
	}
	count := len(req.ContentVersions) * len(aspects) * len(req.Hooks)
	if count > MaxVariantsPerSet {
		return nil, &ExpandError{Count: count}
	}
	if count == 0 {
		return nil, fmt.Errorf("mme: 展开为空（内容版本/画幅/Hook 至少各一）")
	}

	set := &mediacontract.VariantSet{
		APIVersion:   "1.0",
		VariantSetID: mediacontract.NewVariantSetID(),
		Origin:       req.Origin,
	}
	for _, cv := range req.ContentVersions {
		for _, asp := range aspects {
			for _, hk := range req.Hooks {
				set.Variants = append(set.Variants, mediacontract.Variant{
					VariantID:      mediacontract.NewVariantID(),
					Aspect:         asp,
					ContentVersion: cv.Label,
					HookID:         hk.ID,
					EDLRef:         cv.EDLRef,
					Copy:           map[string]mediacontract.PlatformCopy{},
					QC:             mediacontract.QC{}, // 待渲染自检填充
				})
			}
		}
	}
	return set, nil
}

// ---------- 缓存 Key（FR-MIX-02，详细设计 §2.2） ----------

// CacheKind 缓存产物类型。
type CacheKind string

const (
	CacheTranscript CacheKind = "transcript"
	CacheChapters   CacheKind = "chapters"
	CacheReport     CacheKind = "report"
	CacheKeyframes  CacheKind = "keyframes"
	CacheReframed   CacheKind = "reframed"
)

// CacheKey 计算产物缓存 Key（FR-MIX-02）。
// user_intent 纳入 key 的原因：不同意图产出不同章节/报告，缓存复用会导致结果错误。
func CacheKey(assetID string, kind CacheKind, engine, engineVersion, promptVersion, lang, userIntentHash string) string {
	// 用 | 分隔，各段先经 sha256 防注入歧义。
	return textutil.SHA256Hex(
		assetID, string(kind), engine, engineVersion, promptVersion, lang, userIntentHash,
	)
}

// ---------- 差异化度检查（FR-MIX-05，🔴） ----------

// 差异化告警阈值（详细设计 §2.5）。
const (
	DiffVisualThreshold = 0.90 // 视觉相似度
	DiffHookThreshold   = 0.85 // Hook 相似度
	DiffCopyThreshold   = 0.92 // 文案相似度
	DiffOverallWarn     = 0.90 // 综合告警
)

// PairSimilarity 一对变体的多维相似度。
type PairSimilarity struct {
	A, B   string
	Visual float64
	Hook   float64
	Copy   float64
	Audio  float64
}

// Max 返回该对的综合相似度（各维度最大值）。
func (p PairSimilarity) Max() float64 {
	m := p.Visual
	if p.Hook > m {
		m = p.Hook
	}
	if p.Copy > m {
		m = p.Copy
	}
	if p.Audio > m {
		m = p.Audio
	}
	return m
}

// DiffResult 差异化检查结果。
type DiffResult struct {
	MinPairSimilarity float64
	Warn              bool
	WorstPair         *PairSimilarity
}

// EvaluateDiff 评估一组两两相似度，给出最小值与是否告警。
// 告警阈值：综合（各维度 max）≥ 0.90 → warn=true（不阻断送审，人工决策）。
func EvaluateDiff(pairs []PairSimilarity) DiffResult {
	res := DiffResult{MinPairSimilarity: 0}
	if len(pairs) == 0 {
		return res
	}
	min := 1.0
	var worst *PairSimilarity
	for i := range pairs {
		ov := pairs[i].Max()
		if ov < min {
			min = ov
			worst = &pairs[i]
		}
		if ov >= DiffOverallWarn {
			res.Warn = true
		}
	}
	res.MinPairSimilarity = min
	res.WorstPair = worst
	return res
}
