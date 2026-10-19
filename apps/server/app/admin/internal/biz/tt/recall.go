package tt

import (
	"math/bits"
	"unicode"
)

// ============================================================================
// 话题聚类 Step2 召回的纯逻辑件（评审 B-1/A-1）：
//   - 标题 SimHash64 + 汉明相似度（Step2② 召回 / Step3 打分 s_simhash 通道）；
//   - 四段 pigeonhole 拆分（倒排桶查询的键；位段约定与 IC B-26 完全一致，
//     以便 repo 层复用同一查询形态）；
//   - 实体集合 Jaccard（Step2① 倒排命中后的 Step3 打分 s_entity 通道）。
//
// 为什么不用 algo/textutil 的 B-9 SimHash：那是长文档去重规格
// （≥40 token 且 ≥80 字符否则指纹无效），新闻标题普遍 <80 字符会全数失效。
// 本实现与 biz/mme/simhash.go 同族（rune bigram + FNV-1a + 无下限门槛），
// 但参数独立——TT 指纹只要求 TT 内部可比，跨系统可比性是伪需求。
// ============================================================================

// TitleSimHash64 计算标题 64 位 SimHash（字符 bigram 特征，标点/空白/符号滤除，
// 大小写归一；单字符标题退化为单特征，保证短标题也有信号）。
func TitleSimHash64(title string) uint64 {
	feats := titleBigrams(title)
	if len(feats) == 0 {
		return 0
	}
	var v [64]int
	for _, f := range feats {
		h := featureHash64(f)
		for i := 0; i < 64; i++ {
			if h&(1<<uint(i)) != 0 {
				v[i]++
			} else {
				v[i]--
			}
		}
	}
	var out uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			out |= 1 << uint(i)
		}
	}
	return out
}

// Hamming64 两指纹汉明距离。
func Hamming64(a, b uint64) int { return bits.OnesCount64(a ^ b) }

// SimHashSim 1 - Hamming/64（Step3 打分 s_simhash 通道输入）。
func SimHashSim(a, b uint64) float64 {
	return 1 - float64(Hamming64(a, b))/64.0
}

// SimHashSegments 四段 pigeonhole 拆分（B-26 位段约定）：
// seg0=[63:48] seg1=[47:32] seg2=[31:16] seg3=[15:0]，int16 直接截断。
// 召回成立条件：Hamming≤3 ⟹ 至少一段等值（鸽笼），故「任一等值」即候选。
func SimHashSegments(fp uint64) [4]int16 {
	return [4]int16{int16(fp >> 48), int16(fp >> 32), int16(fp >> 16), int16(fp)}
}

// HammingWithin 便捷判定：汉明 ≤ maxDist（A-1 召回阈值 3）。
func HammingWithin(a, b uint64, maxDist int) bool { return Hamming64(a, b) <= maxDist }

// JaccardEntities 实体集合 Jaccard（|A∩B| / |A∪B|）。
// 入参应为 (type,value) 规范化后的实体串（confidence≥0.6 过滤在调用方）。
// 双边皆空定义为 0（无信息态不得产出相似度，否则空话题互相虚假合并）。
func JaccardEntities(a, b []string) float64 {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	setA := make(map[string]struct{}, len(a))
	for _, e := range a {
		setA[e] = struct{}{}
	}
	inter := 0
	setB := make(map[string]struct{}, len(b))
	for _, e := range b {
		setB[e] = struct{}{}
		if _, ok := setA[e]; ok {
			inter++
		}
	}
	union := len(setA) + len(setB) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

// titleBigrams 抽取 rune 相邻二元组特征（滤标点/空白/符号，ToLower 归一）。
func titleBigrams(s string) []string {
	runes := make([]rune, 0, len(s))
	for _, r := range s {
		if unicode.IsPunct(r) || unicode.IsSpace(r) || unicode.IsSymbol(r) {
			continue
		}
		runes = append(runes, unicode.ToLower(r))
	}
	if len(runes) == 0 {
		return nil
	}
	if len(runes) == 1 {
		return []string{string(runes)}
	}
	feats := make([]string, 0, len(runes)-1)
	for i := 0; i+1 < len(runes); i++ {
		feats = append(feats, string(runes[i:i+2]))
	}
	return feats
}

// featureHash64 FNV-1a 64（确定性、无分配盐；与 mme 同参数但不耦合——各自冻结）。
func featureHash64(s string) uint64 {
	const (
		offset64 = 14695981039346656037
		prime64  = 1099511628211
	)
	h := uint64(offset64)
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= prime64
	}
	return h
}
