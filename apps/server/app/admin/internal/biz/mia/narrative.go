package mia

import (
	"strings"

	algomia "framework/app/admin/internal/algo/mia"
)

// NarrativeHit 叙事命中结果（FR-MIA-02）。
type NarrativeHit struct {
	Item          NarrativeItem // 命中的映射行
	MatchedKeyword string        // 命中的关键词（审计）
}

// matchNarratives 在标题+正文中按映射表 B 的关键词做包含匹配（大小写不敏感）。
// 命中多个关键词只取第一个记证（MatchedKeyword 用于审计追溯——命中可追溯，FR-MIA-01 验收）。
func matchNarratives(text string, items []*NarrativeItem) []NarrativeHit {
	lower := strings.ToLower(text)
	out := make([]NarrativeHit, 0, 4)
	for _, it := range items {
		if it == nil || !it.Enabled {
			continue
		}
		for _, kw := range it.Keywords {
			k := strings.ToLower(strings.TrimSpace(kw))
			if k == "" {
				continue
			}
			// 标题全文或叙事名本身命中都算命中。
			if strings.Contains(lower, k) {
				out = append(out, NarrativeHit{Item: *it, MatchedKeyword: kw})
				break // 同一映射行只记一次
			}
		}
	}
	return out
}

// toAlgoMappings biz 视图映射行 → 算法层映射结构（algo/mia 不依赖 biz 类型）。
func toAlgoMappings(items []*MappingItem) []algomia.EntityMapping {
	out := make([]algomia.EntityMapping, 0, len(items))
	for _, it := range items {
		if it == nil || !it.Enabled {
			continue
		}
		out = append(out, algomia.EntityMapping{
			EntityName:     it.EntityName,
			Aliases:        it.EntityAlias,
			Symbol:         it.Symbol,
			AssetClass:     it.AssetClass,
			DirectionBias:  it.DirectionBias,
			BaseConfidence: it.Confidence,
			RuleID:         "mapping/" + it.EntityName,
			Notes:          it.Notes,
		})
	}
	return out
}
