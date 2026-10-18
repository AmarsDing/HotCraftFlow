package mia

import (
	"fmt"

	algomia "framework/app/admin/internal/algo/mia"
	"framework/pkg/mediacontract"
)

// 影响路径生成（FR-MIA-09；详细设计 §2.6）。
// 规则：模板化 + 映射表 notes 拼接，不做 LLM 自由生成——避免编造因果链。
// 直接映射：path = [实体名, "→ 资产类别 符号", "（映射依据：<notes>）"]
// 传导项：  path = 父路径 + ["→ " + rule.Note]

// ExplainMappedPath 生成直接映射的影响路径。
// h.RuleID 形如 "mapping/<entity_name>"（narrative.go 组装）；notes 取映射表的理由说明。
func ExplainMappedPath(h algomia.AssetHit, direction mediacontract.Direction) []string {
	entityName := entityNameOfRule(h.RuleID)
	verb := "指向"
	switch direction {
	case mediacontract.DirectionBullish:
		verb = "利好"
	case mediacontract.DirectionBearish:
		verb = "利空"
	}
	return []string{
		fmt.Sprintf("事件涉实体「%s」", entityName),
		fmt.Sprintf("→ %s %s %s（默认偏向 %s，映射置信度 %.0f%%）",
			assetClassCN(h.AssetClass), h.Symbol, verb, direction, h.Confidence*100),
		"（映射依据：" + notesOrFallback(h.Notes, h.RuleID) + "）",
	}
}

// notesOrFallback 映射路径的「映射依据」段优先用映射行 notes（详细设计 §2.6 模板）。
func notesOrFallback(notes, ruleID string) string {
	if notes != "" {
		return notes
	}
	return fmt.Sprintf("见映射表规则 %s", ruleID)
}

// ExplainNarrativePath 生成叙事映射的影响路径。
func ExplainNarrativePath(h NarrativeHit) []string {
	return []string{
		fmt.Sprintf("命中加密叙事「%s」（触发关键词：%s）", h.Item.Narrative, h.MatchedKeyword),
		fmt.Sprintf("→ 关联币种 %s（默认氛围 %s）", joinCN(h.Item.Coins), h.Item.DefaultSentimentHint),
		fmt.Sprintf("→ 影响按 %dh TTL 衰减", h.Item.TTLHours),
	}
}

// 约定：映射行 RuleID 用 "mapping/<entity_name>"（narrative.go 拼装）。
func entityNameOfRule(ruleID string) string {
	const prefix = "mapping/"
	if n := len(prefix); len(ruleID) > n && ruleID[:n] == prefix {
		return ruleID[n:]
	}
	return ruleID
}

// assetClassCN 资产类别文案。
func assetClassCN(ac mediacontract.AssetClass) string {
	switch ac {
	case mediacontract.AssetEquity:
		return "股票"
	case mediacontract.AssetIndex:
		return "股指"
	case mediacontract.AssetFX:
		return "汇率"
	case mediacontract.AssetCommodity:
		return "商品"
	case mediacontract.AssetCrypto:
		return "加密"
	default:
		return string(ac)
	}
}

func joinCN(ss []string) string {
	if len(ss) == 0 {
		return "-"
	}
	out := ss[0]
	for i := 1; i < len(ss); i++ {
		out += "、" + ss[i]
	}
	return out
}
