package textutil

import (
	"regexp"
	"strings"
	"unicode"
)

// SimHash 64 位语义指纹（评审 IC B-9 的可执行规格）。
//
// 设计约束（写死，禁止更改——更改等于全量历史指纹作废）：
//  - 哈希函数固定 FNV-1a 64。
//  - 权重 tf-idf；idf 表由周任务从近 30 天语料统计，冷启动用内置默认表。
//  - CJK 走 2-gram，非 CJK 走词法切分。
//  - tokens < 40 或文本 < 80 字符 → 指纹不稳定，valid=false（调用方跳过 L2 直接进 L3）。

// SimHash 最小稳定规模（评审 B-9）。
const (
	SimHashMinTokens = 40
	SimHashMinChars  = 80
	SimHashMaxChars  = 20000 // 超大文档截断，防拖垮权重
)

var (
	reHTMLTag  = regexp.MustCompile(`<[^>]+>`)
	reURL      = regexp.MustCompile(`https?://\S+`)
	reEmail    = regexp.MustCompile(`\S+@\S+\.\S+`)
	reSpace    = regexp.MustCompile(`\s+`)
)

// Fingerprint 是 SimHash 计算结果。
type Fingerprint struct {
	Value uint64 // 64 位指纹
	Hex   string // 16 位小写 hex（契约 dedup.fingerprint 用 string）
	Valid bool   // 指纹是否稳定（样本量足够）
}

// SimHash 计算文本的 64 位语义指纹。
// idf 为词 → idf 权重表；为 nil 时用均匀权重（tf 即可，冷启动降级）。
// lang 提示语言；"und" 时按 CJK 占比自动判定。
func SimHash(text, lang string, idf map[string]float64) Fingerprint {
	tokens := tokenize(normalize(text), lang)
	if len(tokens) < SimHashMinTokens || len(text) < SimHashMinChars {
		return Fingerprint{Value: 0, Hex: "0000000000000000", Valid: false}
	}

	// tf 统计。
	tf := map[string]float64{}
	for _, tok := range tokens {
		tf[tok]++
	}

	// 按位加权累加。
	var v [64]float64
	for w, cnt := range tf {
		weight := cnt
		if idf != nil {
			if w8, ok := idf[w]; ok {
				weight = cnt * w8
			}
		}
		h := FNV64a(w)
		for i := 0; i < 64; i++ {
			if (h>>uint(i))&1 == 1 {
				v[i] += weight
			} else {
				v[i] -= weight
			}
		}
	}

	// 降维：正值置 1。
	var f uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			f |= 1 << uint(i)
		}
	}
	return Fingerprint{Value: f, Hex: toHex64(f), Valid: true}
}

// Hamming 计算两个指纹的汉明距离。
func Hamming(a, b uint64) int {
	x := a ^ b
	n := 0
	for x != 0 {
		n++
		x &= x - 1
	}
	return n
}

// HammingSimilar 判断汉明距离 ≤3（64 位下 ≈ 相似度 ≥0.953，仅对长文本可靠）。
// 这就是为什么必须有 tokens>=40 的前置条件。
func HammingSimilar(a, b uint64) bool {
	return Hamming(a, b) <= 3
}

// Split4 把 64 位指纹拆成 4 段 16 位（评审 IC B-26：PostgreSQL 无无符号整型，拆 smallint 存储）。
// 返回 [b0, b1, b2, b3]，b0 为最高 16 位。鸽巢原理：汉明 ≤3 时必有一段完全相同。
func Split4(f uint64) [4]int16 {
	return [4]int16{
		int16((f >> 48) & 0xFFFF),
		int16((f >> 32) & 0xFFFF),
		int16((f >> 16) & 0xFFFF),
		int16(f & 0xFFFF),
	}
}

// Join4 把 4 段 smallint 还原为 64 位指纹（Split4 的逆运算）。
func Join4(b [4]int16) uint64 {
	return uint64(uint16(b[0]))<<48 | uint64(uint16(b[1]))<<32 |
		uint64(uint16(b[2]))<<16 | uint64(uint16(b[3]))
}

func toHex64(v uint64) string {
	const digits = "0123456789abcdef"
	var b [16]byte
	for i := 15; i >= 0; i-- {
		b[i] = digits[v&0xF]
		v >>= 4
	}
	return string(b[:])
}

// normalize 文本归一化：去 HTML、URL/邮箱占位符、连续空白折叠、全角转半角。
func normalize(s string) string {
	s = reHTMLTag.ReplaceAllString(s, " ")
	s = reURL.ReplaceAllString(s, " URL ")
	s = reEmail.ReplaceAllString(s, " EMAIL ")
	s = reSpace.ReplaceAllString(s, " ")
	if len(s) > SimHashMaxChars {
		s = s[:SimHashMaxChars]
	}
	return strings.TrimSpace(s)
}

// tokenize 分词：CJK 走 2-gram，非 CJK 走词法切分；中英混排合并。
func tokenize(s, lang string) []string {
	var tokens []string
	var word []rune
	flushWord := func() {
		if len(word) > 0 {
			tokens = append(tokens, strings.ToLower(string(word)))
			word = word[:0]
		}
	}
	var cjkRun []rune
	flushCJK := func() {
		// CJK 连续段做 2-gram。
		if len(cjkRun) >= 2 {
			for i := 0; i+1 < len(cjkRun); i++ {
				tokens = append(tokens, string(cjkRun[i:i+2]))
			}
		} else if len(cjkRun) == 1 {
			tokens = append(tokens, string(cjkRun))
		}
		cjkRun = cjkRun[:0]
	}

	for _, r := range s {
		switch {
		case isCJKRune(r):
			flushWord()
			cjkRun = append(cjkRun, r)
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			flushCJK()
			word = append(word, r)
		default:
			flushWord()
			flushCJK()
		}
	}
	flushWord()
	flushCJK()
	return tokens
}

func isCJKRune(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r)
}
