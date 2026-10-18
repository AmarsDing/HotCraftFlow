package mia

import (
	"math"
	"strings"
	"unicode"

	"framework/pkg/mediacontract"
)

// ---------- 实体归一化与链接（FR-MIA-01，详细设计 §2.1） ----------

// NormalizeEntity 归一化实体名：去空白、大小写折叠、全半角归一。
func NormalizeEntity(s string) string {
	s = strings.TrimSpace(s)
	s = strings.ToLower(s)
	// 全角转半角（常见全角字母数字与空格）。
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'Ａ' && r <= 'Ｚ':
			b.WriteRune(r - 'Ａ' + 'a')
		case r >= '０' && r <= '９':
			b.WriteRune(r - '０' + '0')
		case r == '　':
			b.WriteRune(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// AssetHit 是实体链接的命中结果。
type AssetHit struct {
	Symbol        string
	AssetClass    mediacontract.AssetClass
	DirectionBias mediacontract.Direction
	Confidence    float64
	RuleID        string
}

// EntityMapping 是映射表的一条记录（实体 → 标的）。
type EntityMapping struct {
	EntityName    string // 归一化后的主名
	Aliases       []string
	Symbol        string
	AssetClass    mediacontract.AssetClass
	DirectionBias mediacontract.Direction
	BaseConfidence float64
	RuleID        string
}

// LinkConfidenceThreshold 低于此置信度的实体不进 MIA 链接（契约 §1：0.6）。
const LinkConfidenceThreshold = 0.6

// FuzzyThreshold 模糊匹配相似度阈值；命中则 confidence ×0.8。
const FuzzyThreshold = 0.85

// LinkEntity 把契约实体链接到标的（评审：未命中返回空，不强行匹配）。
// mappings 为映射表（调用方从 DB 加载）。返回命中的 AssetHit（可能为空）。
func LinkEntity(e mediacontract.Entity, mappings []EntityMapping) []AssetHit {
	// 契约硬约束：confidence < 0.6 不进 MIA。
	if e.Confidence < LinkConfidenceThreshold {
		return nil
	}
	name := NormalizeEntity(e.Value)
	if name == "" {
		return nil
	}
	var hits []AssetHit
	for _, m := range mappings {
		conf := 0.0
		if NormalizeEntity(m.EntityName) == name {
			conf = m.BaseConfidence // 精确匹配
		} else {
			for _, al := range m.Aliases {
				if NormalizeEntity(al) == name {
					conf = m.BaseConfidence * 0.95 // 别名匹配略降
					break
				}
			}
			if conf == 0 {
				if sim := jaccard(name, NormalizeEntity(m.EntityName)); sim >= FuzzyThreshold {
					conf = m.BaseConfidence * 0.8 // 模糊匹配降权
				}
			}
		}
		if conf > 0 {
			hits = append(hits, AssetHit{
				Symbol:        m.Symbol,
				AssetClass:    m.AssetClass,
				DirectionBias: m.DirectionBias,
				Confidence:    conf * e.Confidence, // 乘上实体自身置信度
				RuleID:        m.RuleID,
			})
		}
	}
	return hits
}

// jaccard 计算两个字符串的字符 bigram Jaccard 相似度（轻量模糊匹配）。
func jaccard(a, b string) float64 {
	ga, gb := bigrams(a), bigrams(b)
	if len(ga) == 0 || len(gb) == 0 {
		return 0
	}
	inter := 0
	for g := range ga {
		if gb[g] {
			inter++
		}
	}
	union := len(ga) + len(gb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func bigrams(s string) map[string]bool {
	r := []rune(s)
	out := map[string]bool{}
	for i := 0; i+1 < len(r); i++ {
		out[string(r[i:i+2])] = true
	}
	if len(r) == 1 {
		out[string(r)] = true
	}
	return out
}

// ---------- 事件聚类（FR-MIA-05，详细设计 §2.3） ----------

// EventSimilarity 计算新条目与既有事件卡的相似度。
// sim = 0.5·Jaccard(实体) + 0.3·标题语义余弦 + 0.2·时间接近度。
// titleSim 由调用方提供（embedding 余弦，本包不依赖模型）。
func EventSimilarity(entityJaccard, titleSim, hoursApart float64) float64 {
	// 时间接近度：24h 内线性衰减到 0。
	timeProximity := math.Max(0, 1-hoursApart/24.0)
	return 0.5*clamp01(entityJaccard) + 0.3*clamp01(titleSim) + 0.2*clamp01(timeProximity)
}

// EventMergeThreshold 事件并入阈值。
const EventMergeThreshold = 0.82

// ShouldMergeEvent 判断是否并入既有事件卡。
func ShouldMergeEvent(similarity float64) bool {
	return similarity >= EventMergeThreshold
}

// EntitySetJaccard 计算两个实体集合的 Jaccard 重合度（按 type+value 归一化）。
func EntitySetJaccard(a, b []mediacontract.Entity) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	key := func(e mediacontract.Entity) string {
		return string(e.Type) + ":" + NormalizeEntity(e.Value)
	}
	sa, sb := map[string]bool{}, map[string]bool{}
	for _, e := range a {
		if e.Confidence >= LinkConfidenceThreshold {
			sa[key(e)] = true
		}
	}
	for _, e := range b {
		if e.Confidence >= LinkConfidenceThreshold {
			sb[key(e)] = true
		}
	}
	if len(sa) == 0 || len(sb) == 0 {
		return 0
	}
	inter := 0
	for k := range sa {
		if sb[k] {
			inter++
		}
	}
	union := len(sa) + len(sb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// IsCJK 判断文本是否以中日韩字符为主（供语言/分词路径判定）。
func IsCJK(s string) bool {
	if s == "" {
		return false
	}
	var cjk, total int
	for _, r := range s {
		if unicode.IsLetter(r) {
			total++
			if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
				unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
				cjk++
			}
		}
	}
	return total > 0 && float64(cjk)/float64(total) > 0.3
}
