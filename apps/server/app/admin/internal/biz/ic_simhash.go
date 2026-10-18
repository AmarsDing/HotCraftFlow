package biz

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

// =============================================================================
// SimHash 语义指纹（评审 B-9 可执行规格，历史指纹不可比=全量作废 → 严禁改参数）
//  1. NFKC；去 HTML 标签；URL/邮箱→占位符；连续空白折叠
//  2. CJK 占比 > 0.3 → 中文路径；中英混排 → 两段词流合并统一加权
//  3. 权重 tf·idf（idf 表 data/ic/idf.json 周任务产出；冷启动内置默认表）
//  4. fnv64a 固定（禁止更换；更换 = 全量重算）
// 边界：tokens < 40 或文本 < 80 字符 → fingerprint_valid=false，L2 跳过直进 L3
// 阈值换算：64 位汉明 ≤3 ≈ 相似度 ≥ 0.953（仅长文本可靠 → tokens>=40 前置）
// [M-13] 选型注记：SimHash 而非 MinHash —— O(1) 存储 + 汉明比对，V1 日产 3000-8000
// 条量级收益不抵 LSH 分桶复杂度（B-9 原文结论）。
// =============================================================================

const (
	simhashMaxCorpusRunes = 20000 // 超出截断（防超大文档拖垮权重）
	simhashMinTokens      = 40    // 低于无效指纹
	simhashMinCorpusRunes = 80    // 低于无效指纹
)

// SimHashResult 计算产物。
type SimHashResult struct {
	Fingerprint uint64
	Hex         string // 16 位小写 hex（契约 dedup.fingerprint 为 string，评审 C-4）
	Segments    [4]int16
	Valid       bool   // fingerprint_valid（B-9 边界）
	Tokens      int
	Lang        string // zh|en|und（S-25 兜底判定结果，und 时由调用方按占比回写）
}

// IDFTable tf·idf 权重表（周任务 worker/ic/idf_job 产出 data/ic/idf.json；冷启动内置）。
type IDFTable struct {
	Weights  map[string]float64
	Fallback float64 // 表外词权重：按“相对稀有”假设给常量（B-9 未指定 → 写死并注释）
}

// GetIDF 查词权重。
func (t *IDFTable) GetIDF(token string) float64 {
	if t == nil {
		return builtinIDF.Fallback
	}
	if w, ok := t.Weights[token]; ok {
		return w
	}
	return t.Fallback
}

// LoadIDFTable 载入周任务产物；路径为空或读失败 → 内置默认表。
func LoadIDFTable(path string) *IDFTable {
	if path == "" {
		return builtinIDF
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return builtinIDF
	}
	var weights map[string]float64
	if err := json.Unmarshal(raw, &weights); err != nil || len(weights) == 0 {
		return builtinIDF
	}
	return &IDFTable{Weights: weights, Fallback: builtinIDF.Fallback}
}

var (
	htmlTagPattern = regexp.MustCompile(`<[^>]*>`)
	urlPattern     = regexp.MustCompile(`https?://\S+|www\.\S+`)
	emailPattern   = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
	spacePattern   = regexp.MustCompile(`\s+`)
)

// normalizeCorpus NFKC + 去 HTML + 占位符 + 空白折叠（B-9 第 1 步）。
func normalizeCorpus(s string) string {
	s = norm.NFKC.String(s)
	s = htmlTagPattern.ReplaceAllString(s, " ")
	s = urlPattern.ReplaceAllString(s, " urlref ")
	s = emailPattern.ReplaceAllString(s, " emailref ")
	s = spacePattern.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// truncateRunes 复用 radar_insight.go 的包级定义（rune 截断，防多字节切断）。

// isCJK 中文/日文/韩文及部分兼容区段走 CJK 路径。
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

// DetectLangByScript S-25 兜底判定：CJK 占比 > 0.3 → zh；否则 en；无字母 → und。
func DetectLangByScript(s string) string {
	var cjk, latin int
	for _, r := range s {
		switch {
		case isCJK(r):
			cjk++
		case unicode.IsLetter(r) && r < 128:
			latin++
		}
	}
	total := cjk + latin
	if total == 0 {
		return "und"
	}
	if float64(cjk)/float64(total) > 0.3 {
		return "zh"
	}
	return "en"
}

// cjkBigrams 按 CJK 连续段切 2-gram；先删字符级停用词的。
func cjkBigrams(s string, out []string) []string {
	runes := []rune(s)
	var run []rune
	flush := func() {
		for i := 0; i+1 < len(run); i++ {
			out = append(out, string(run[i:i+2]))
		}
		run = run[:0]
	}
	for _, r := range runes {
		if isCJK(r) {
			if _, stop := cjkStopChars[r]; stop {
				flush()
				continue
			}
			run = append(run, r)
			continue
		}
		flush()
	}
	flush()
	return out
}

// latinWords Unicode 词法切分 + lowercase + 去停用词（B-9 非 CJK 路径）。
func latinWords(s string, out []string) []string {
	var b strings.Builder
	flush := func() {
		if b.Len() == 0 {
			return
		}
		w := b.String()
		b.Reset()
		if utf8.RuneCountInString(w) < 2 {
			return
		}
		if _, stop := enStopWords[w]; stop {
			return
		}
		out = append(out, w)
	}
	for _, r := range s {
		if isCJK(r) {
			flush() // CJK 字符归另一支处理
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		flush()
	}
	flush()
	return out
}

// fnv64a 固定哈希（B-9 第 4 步）。
func fnv64a(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// ComputeSimHash 主入口。idf 传 nil 用内置默认表。
func ComputeSimHash(title, text, langHint string, idf *IDFTable) SimHashResult {
	if idf == nil {
		idf = builtinIDF
	}
	// 边界：text 为空 → 用 title；两者皆空 → 0 且无效（B-9 规则，忠实实现）
	corpus := text
	if strings.TrimSpace(corpus) == "" {
		corpus = title
	}
	corpus = normalizeCorpus(corpus)
	corpus = truncateRunes(corpus, simhashMaxCorpusRunes)
	if corpus == "" {
		return SimHashResult{Hex: fmt.Sprintf("%016x", uint64(0)), Segments: segmentsOf(0), Valid: false, Lang: "und"}
	}

	// S-25：上游 lang 缺失 / und → 按区块统计判定并回写
	lang := langHint
	if lang == "" || lang == "und" {
		lang = DetectLangByScript(corpus)
	}

	// 分词：中英两支都跑，合并词流（B-9 「中英混排 → 两段分词结果合并后统一加权」）
	tokens := make([]string, 0, 64)
	tokens = cjkBigrams(corpus, tokens)
	tokens = latinWords(corpus, tokens)

	// tf 统计
	tf := make(map[string]int, len(tokens))
	for _, t := range tokens {
		tf[t]++
	}

	// v[0..63] 累加（B-9 第 5 步）
	var v [64]float64
	for token, count := range tf {
		w := float64(count) * idf.GetIDF(token)
		h := fnv64a(token)
		for i := 0; i < 64; i++ {
			if (h>>uint(i))&1 == 1 {
				v[i] += w
			} else {
				v[i] -= w
			}
		}
	}
	// 第 6 步：降维
	var fp uint64
	for i := 0; i < 64; i++ {
		if v[i] > 0 {
			fp |= 1 << uint(i)
		}
	}

	valid := len(tokens) >= simhashMinTokens && utf8.RuneCountInString(corpus) >= simhashMinCorpusRunes
	return SimHashResult{
		Fingerprint: fp,
		Hex:         fmt.Sprintf("%016x", fp),
		Segments:    segmentsOf(fp),
		Valid:       valid,
		Tokens:      len(tokens),
		Lang:        lang,
	}
}

// segmentsOf 切 4×16 位段 [63:48]..[15:0]（B-26 pigeonhole 倒排）。
func segmentsOf(fp uint64) [4]int16 {
	return [4]int16{
		int16(uint16(fp >> 48)),
		int16(uint16(fp >> 32)),
		int16(uint16(fp >> 16)),
		int16(uint16(fp)),
	}
}

// HammingDistance 64 位种群计数（俗称 popcount，L2 判值 ≤3）。
func HammingDistance(a, b uint64) int {
	x := a ^ b
	n := 0
	for x != 0 {
		x &= x - 1
		n++
	}
	return n
}
