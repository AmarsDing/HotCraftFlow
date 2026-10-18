package mme

import (
	"math/bits"
	"strings"
	"unicode"
)

// simhash 64 位文本指纹 + 汉明距离，供 FR-MIX-05 文案/Hook 差异化。
// 纯 Go、无外部依赖；对中英文混排按 rune 二元组（bigram）取特征。

// tokenize 抽取 bigram 特征：过滤标点/空白，把连续可见 rune 串切成相邻对。
// 单 rune 文本退化为单特征，保证短标题也有信号。
func tokenize(s string) []string {
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
	features := make([]string, 0, len(runes)-1)
	for i := 0; i+1 < len(runes); i++ {
		features = append(features, string(runes[i:i+2]))
	}
	return features
}

// featureHash FNV-1a 64（确定性、无分配盐）。
func featureHash(s string) uint64 {
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

// Simhash64 计算文本 64 位指纹。
func Simhash64(text string) uint64 {
	feats := tokenize(text)
	if len(feats) == 0 {
		return 0
	}
	var v [64]int
	for _, f := range feats {
		h := featureHash(f)
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

// SimhashSimilarity 两文本相似度：1 - hamming/64。空文本相等定义为 1（都无信息）。
func SimhashSimilarity(a, b string) float64 {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" && b == "" {
		return 1
	}
	ha, hb := Simhash64(a), Simhash64(b)
	return 1 - float64(bits.OnesCount64(ha^hb))/64.0
}
