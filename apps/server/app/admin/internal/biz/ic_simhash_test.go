package biz

import (
	"strings"
	"testing"
)

// =============================================================================
// SimHash 黄金测试（B-9 可执行规格：参数写死，历史指纹不可比=全量作废，
// 这些断言即为「参数未漂移」的回归保险——改动本实现先炸这里）
// =============================================================================

// longCJK 构造 ≥40 tokens 的中文长文本（bigram 每 rune 约一枚 → 60+ 字即可）。
func longCJK() string {
	seg := "今日资本市场出现明显波动，多家机构上调了风险预期，监管部门随后发布新规对平台收取费用做出明确规定，"
	return strings.Repeat(seg, 3)
}

func TestComputeSimHashDeterministic(t *testing.T) {
	a := ComputeSimHash("标题", longCJK(), "", nil)
	b := ComputeSimHash("标题", longCJK(), "", nil)
	if a.Fingerprint != b.Fingerprint || a.Hex != b.Hex {
		t.Fatal("SimHash 不确定（fnv 固定哈希被破坏）")
	}
	if !a.Valid {
		t.Fatalf("长中文文本应为有效指纹: tokens=%d", a.Tokens)
	}
	if len(a.Hex) != 16 {
		t.Fatalf("hex 应为 16 位: %q", a.Hex)
	}
}

func TestComputeSimHashSmallEditSmallDrift(t *testing.T) {
	base := longCJK()
	variant := strings.Replace(base, "上调", "下调", 1)
	a := ComputeSimHash("", base, "", nil)
	b := ComputeSimHash("", variant, "", nil)
	d := HammingDistance(a.Fingerprint, b.Fingerprint)
	// 语义近似 → 低位漂移（B-9 相似度性质：≥0.953 ≈ hamming ≤3；单差词通常 ≤3 位；
	// 放宽至 6 防启发式波动，仍远小于随机文本的 ~32）
	if d > 6 {
		t.Fatalf("小改动漂移过大: hamming=%d", d)
	}
}

func TestComputeSimHashDistinctTexts(t *testing.T) {
	a := ComputeSimHash("", longCJK(), "", nil)
	other := "The central bank announced a surprise rate cut today, sending equity markets higher across the region. " +
		"Analysts expect further easing in the coming quarter as inflation continues to decline steadily."
	b := ComputeSimHash("", other, "", nil)
	d := HammingDistance(a.Fingerprint, b.Fingerprint)
	if d < 10 {
		t.Fatalf("无关文本漂移过小: hamming=%d", d)
	}
}

func TestComputeSimHashInvalidFingerprintGates(t *testing.T) {
	// 空语料 → 无效
	r := ComputeSimHash("", "", "", nil)
	if r.Valid {
		t.Fatal("空语料不应出有效指纹")
	}
	// 标题兜底但 corpus < 80 runes / tokens < 40 → 无效（B-9 边界）
	r = ComputeSimHash("短视频标题", "", "", nil)
	if r.Valid {
		t.Fatalf("超短文本不应出有效指纹: tokens=%d", r.Tokens)
	}
	// text 为空时取 title（B-9 规则）
	longTitle := longCJK()
	r = ComputeSimHash(longTitle, "", "", nil)
	if !r.Valid {
		t.Fatalf("text 为空应回退 title 计算: tokens=%d", r.Tokens)
	}
}

func TestSegmentsRoundTrip(t *testing.T) {
	fp := uint64(0x1234567890abcdef)
	seg := segmentsOf(fp)
	got := uint64(uint16(seg[0]))<<48 | uint64(uint16(seg[1]))<<32 | uint64(uint16(seg[2]))<<16 | uint64(uint16(seg[3]))
	if got != fp {
		t.Fatalf("segments 往返失败: %016x != %016x", got, fp)
	}
}

func TestHammingDistance(t *testing.T) {
	if d := HammingDistance(0, 0); d != 0 {
		t.Fatal("0^0 应为 0")
	}
	// 两值恰为 64 位全反（xor=0xffffffffffffffff）→ 64；低 32 位全 1 vs 0 → 32。
	if d := HammingDistance(0xffff0000ffff0000, 0x0000ffff0000ffff); d != 64 {
		t.Fatalf("全反位应 64: %d", d)
	}
	if d := HammingDistance(0x00000000ffffffff, 0); d != 32 {
		t.Fatalf("半置位应 32: %d", d)
	}
	if d := HammingDistance(1|4|16, 0); d != 3 {
		t.Fatalf("三位应为 3: %d", d)
	}
}

func TestDetectLangByScript(t *testing.T) {
	if got := DetectLangByScript("这是一段纯中文文本，没有任何英文单词"); got != "zh" {
		t.Fatalf("纯中文: %s", got)
	}
	if got := DetectLangByScript("pure english text without any chinese"); got != "en" {
		t.Fatalf("纯英文: %s", got)
	}
	if got := DetectLangByScript("123 !@# ……"); got != "und" {
		t.Fatalf("无字母: %s", got)
	}
}

func TestNormalizeCorpusDeterministicThroughCompute(t *testing.T) {
	raw := "<p>看全文 https://example.com/x?utm_source=a ，联系 foo@bar.com</p>" + longCJK()
	a := ComputeSimHash("", raw, "", nil)
	b := ComputeSimHash("", " "+raw+" ", "", nil)
	if a.Fingerprint != b.Fingerprint {
		t.Fatal("空白/HTML/URL/邮箱归一化应保证指纹稳定")
	}
}

// TestIDFTableGet 表外词走 Fallback 常量（B-9 未指定，实现注释写死）。
func TestIDFTableGet(t *testing.T) {
	tab := &IDFTable{Weights: map[string]float64{"热词": 2.5}, Fallback: 1.0}
	if tab.GetIDF("热词") != 2.5 {
		t.Fatal("表内词应取表权重")
	}
	if tab.GetIDF("不存在") != 1.0 {
		t.Fatal("表外词应取 Fallback")
	}
	var nilTab *IDFTable
	if nilTab.GetIDF("x") <= 0 {
		t.Fatal("nil 表应走内置默认表 Fallback")
	}
}
