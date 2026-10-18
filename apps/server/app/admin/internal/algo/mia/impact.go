// Package mia 实现 ③ MIA 市场影响分析的核心算法。
// 对应 docs/systems/03-MIA/03-详细设计.md §2，吸收评审报告修复。
//
// 诚实声明（评审 §4）：消息→资产价格影响是弱信号（有效市场假说下边际信息有限）。
// 本包输出的 impact_score 是「关注优先级」而非「涨跌概率」，方向性独立于分数。
package mia

import (
	"math"
	"sort"

	"framework/pkg/mediacontract"
)

// ---------- 情感（评审 B：FinBERT 仅英文，中文走 LLM；分数换算明确） ----------

// FinBERTScoreToSigned 把 FinBERT 三分类概率映射到连续值 [-1,1]。
// score = P(positive) - P(negative)。
func FinBERTScoreToSigned(pPositive, pNegative float64) float64 {
	return clamp(pPositive-pNegative, -1, 1)
}

// SentimentLabelFromScore 由连续值得标签（阈值 ±0.15）。
func SentimentLabelFromScore(score float64) mediacontract.SentimentLabel {
	switch {
	case score > 0.15:
		return mediacontract.SentimentPositive
	case score < -0.15:
		return mediacontract.SentimentNegative
	default:
		return mediacontract.SentimentNeutral
	}
}

// ChooseEngine 引擎选择策略（评审修复：lang/isMacro 分路，全失效返回 none 禁止猜测）。
func ChooseEngine(lang string, isMacro, finbertOK, llmOK bool) mediacontract.SentimentEngine {
	isEN := lang == "en"
	isZH := lang == "zh" || lang == "zh-CN"
	switch {
	case isEN && !isMacro && finbertOK:
		return mediacontract.EngineFinBERT
	case (isZH || isMacro) && llmOK:
		return mediacontract.EngineLLM
	case finbertOK:
		return mediacontract.EngineFinBERT // 降级
	case llmOK:
		return mediacontract.EngineLLM // 降级
	default:
		return mediacontract.EngineNone // 🔴 禁止猜测
	}
}

// ---------- 影响面评分（FR-MIA-06） ----------

// ImpactInput 影响面评分输入。
type ImpactInput struct {
	SentimentScore      float64 // -1 ~ 1；unknown 时传 0（不猜）
	DistinctDomainCount int     // 按不同域名去重后的来源数
	SourceCredibility   float64 // 信源可信度基准（官方/央行/交易所=1.0，自媒体=0.6）
	TopConfidences      []float64 // 实体映射置信度（取 top3 均值）
	AgeHours            float64 // 事件年龄（样本时间口径）
	TTLHours            int     // 股票/宏观 24，加密 12，叙事表可覆盖 6
}

// ImpactOutput 影响面评分结果（含四因子明细，契约 impact.factors 强制）。
type ImpactOutput struct {
	Score     float64
	Direction mediacontract.Direction
	Factors   mediacontract.ImpactFactors
	NoMapping bool // 无实体映射（impact_score 记 0，不强行匹配）
}

// SourceWeight 计算信源权重：min(1, 0.4 + 0.15×log2(distinct_domains)) × credibility。
// 1 源≈0.4，2 源≈0.55，4 源≈0.7，8 源≈0.85（再乘可信度）。
func SourceWeight(distinctDomains int, credibility float64) float64 {
	if distinctDomains < 1 {
		distinctDomains = 1
	}
	if credibility <= 0 {
		credibility = 0.6
	}
	base := 0.4 + 0.15*math.Log2(float64(distinctDomains))
	if base > 1 {
		base = 1
	}
	return clamp01(base * credibility)
}

// Relevance 实体映射相关度：top3 置信度均值；无映射返回 0。
func Relevance(confidences []float64) float64 {
	if len(confidences) == 0 {
		return 0
	}
	s := append([]float64(nil), confidences...)
	sort.Sort(sort.Reverse(sort.Float64Slice(s)))
	n := 3
	if len(s) < n {
		n = len(s)
	}
	var sum float64
	for i := 0; i < n; i++ {
		sum += clamp01(s[i])
	}
	return sum / float64(n)
}

// Decay 时间衰减：exp(-age_hours / ttl_hours)。
func Decay(ageHours float64, ttlHours int) float64 {
	if ttlHours <= 0 {
		ttlHours = 24
	}
	if ageHours < 0 {
		ageHours = 0
	}
	return math.Exp(-ageHours / float64(ttlHours))
}

// ScoreImpact 计算影响面评分。
//
// impact = 100 × |sentiment| × source_weight × relevance × decay
//
// ⚠️ 评审修复与诚实声明：
//   - 方向性独立于分数：|sentiment| 让极好/极坏同分，direction 单独输出（不丢失方向）。
//   - 乘积公式使分数普遍偏小（四因子均 <1），score 应理解为「相对关注优先级」，
//     跨卡排序用其在同窗口内的分位，不要用绝对阈值断言「重要」。
//   - 无实体映射（relevance=0）时 score=0 且 NoMapping=true，不强行匹配。
func ScoreImpact(in ImpactInput) ImpactOutput {
	sentimentFactor := math.Abs(clamp(in.SentimentScore, -1, 1))
	sourceW := SourceWeight(in.DistinctDomainCount, in.SourceCredibility)
	relevance := Relevance(in.TopConfidences)
	decay := Decay(in.AgeHours, in.TTLHours)

	out := ImpactOutput{
		Factors: mediacontract.ImpactFactors{
			Sentiment:    sentimentFactor,
			SourceWeight: sourceW,
			Relevance:    relevance,
			Decay:        decay,
		},
	}
	if relevance == 0 {
		out.NoMapping = true
		out.Score = 0
	} else {
		out.Score = 100 * sentimentFactor * sourceW * relevance * decay
	}

	// 方向性独立于分数（评审修复：极好/极坏虽同分，方向不丢）。
	switch {
	case in.SentimentScore > 0:
		out.Direction = mediacontract.DirectionBullish
	case in.SentimentScore < 0:
		out.Direction = mediacontract.DirectionBearish
	default:
		out.Direction = mediacontract.DirectionNeutral
	}
	return out
}

// TTLForAssetClass 各类资产的默认 TTL（小时）。叙事表可覆盖加密为 6h。
func TTLForAssetClass(ac mediacontract.AssetClass) int {
	switch ac {
	case mediacontract.AssetCrypto:
		return 12
	default: // equity / index / fx / commodity
		return 24
	}
}

// ---------- 跨资产传导（FR-MIA-08） ----------

// ApplyTransmission 对一条已映射结论应用传导规则，生成传导项。
// 传导置信度 = rule.Confidence × parentConfidence（必 ≤0.6，契约强制）。
func ApplyTransmission(rule mediacontract.TransmissionRule, parent mediacontract.AffectedItem) mediacontract.AffectedItem {
	conf := rule.Confidence * parent.Confidence
	if conf > 0.6 {
		conf = 0.6 // 双保险：即使上层绕过校验也强制封顶
	}
	path := append(append([]string(nil), parent.Path...), "→ "+rule.Note)
	return mediacontract.AffectedItem{
		Symbol:         rule.ToAsset,
		Direction:      rule.ToDirection,
		Confidence:     conf,
		Path:           path,
		RuleID:         rule.RuleID,
		IsTransmission: true,
	}
}

// clamp 夹到 [lo,hi]。
func clamp(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func clamp01(v float64) float64 { return clamp(v, 0, 1) }
