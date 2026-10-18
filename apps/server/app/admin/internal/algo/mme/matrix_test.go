package mme

import (
	"strings"
	"testing"

	"framework/app/admin/internal/algo/textutil"
	"framework/pkg/mediacontract"
)

func TestExpandCartesian(t *testing.T) {
	// 2 内容版本 × 3 画幅 × 2 Hook = 12 变体。
	set, err := Expand(ExpandRequest{
		ContentVersions: []ContentVersion{{Label: "v1", EDLRef: "e1"}, {Label: "v2", EDLRef: "e2"}},
		Aspects:         []string{"9:16", "1:1", "16:9"},
		Hooks:           []Hook{{ID: "h1"}, {ID: "h2"}},
	})
	if err != nil {
		t.Fatalf("Expand: %v", err)
	}
	if len(set.Variants) != 12 {
		t.Fatalf("应为 12 变体，got %d", len(set.Variants))
	}
	if err := mediacontract.ValidateIDFormat(set.VariantSetID, mediacontract.PrefixVariantSet); err != nil {
		t.Fatalf("variant_set_id 前缀非法：%v", err)
	}
}

func TestExpandScaleGuard(t *testing.T) {
	// 规模保护：超过 60 拒绝展开（评审确认）。
	cvs := make([]ContentVersion, 11) // 11 × 3 × 2 = 66 > 60
	for i := range cvs {
		cvs[i] = ContentVersion{Label: "v"}
	}
	_, err := Expand(ExpandRequest{
		ContentVersions: cvs,
		Aspects:         []string{"9:16", "1:1", "16:9"},
		Hooks:           []Hook{{ID: "h1"}, {ID: "h2"}},
	})
	if err == nil {
		t.Fatalf("超限应拒绝展开")
	}
	if _, ok := err.(*ExpandError); !ok {
		t.Fatalf("应返回 ExpandError，got %T", err)
	}
}

func TestCacheKeyIncludesIntent(t *testing.T) {
	// 评审：user_intent 不同 → key 不同（缓存复用会导致结果错误）。
	k1 := CacheKey("as_1", CacheChapters, "whisper", "1.0", "p1", "zh", "intent-A")
	k2 := CacheKey("as_1", CacheChapters, "whisper", "1.0", "p1", "zh", "intent-B")
	if k1 == k2 {
		t.Fatalf("不同 intent 必须产生不同缓存 key")
	}
	// 同参数幂等。
	if CacheKey("as_1", CacheChapters, "whisper", "1.0", "p1", "zh", "intent-A") != k1 {
		t.Fatalf("同参数缓存 key 应幂等")
	}
}

func TestDiffWarn(t *testing.T) {
	// 综合相似度 ≥0.90 → warn。
	res := EvaluateDiff([]PairSimilarity{
		{A: "v1", B: "v2", Visual: 0.95, Hook: 0.5, Copy: 0.5},
	})
	if !res.Warn {
		t.Fatalf("视觉 0.95 ≥0.90 应告警")
	}
	if res.MinPairSimilarity != 0.95 {
		t.Fatalf("最小相似度应为 0.95，got %f", res.MinPairSimilarity)
	}
	// 不告警。
	res2 := EvaluateDiff([]PairSimilarity{{A: "v1", B: "v2", Visual: 0.5, Hook: 0.4, Copy: 0.3}})
	if res2.Warn {
		t.Fatalf("低相似度不应告警")
	}
}

func TestSimHashStability(t *testing.T) {
	// 评审 IC B-9：同文本指纹稳定可比。
	text := strings.Repeat("美联储宣布降息二十五个基点以应对经济下行压力。", 10)
	a := textutil.SimHash(text, "zh", nil)
	b := textutil.SimHash(text, "zh", nil)
	if !a.Valid || a.Value != b.Value {
		t.Fatalf("同文本指纹应稳定一致")
	}
	if len(a.Hex) != 16 {
		t.Fatalf("hex 应为 16 位，got %s", a.Hex)
	}
	// 相似文本汉明距离应小。
	text2 := strings.Repeat("美联储宣布降息二十五个基点以应对经济下行压力增大。", 10)
	c := textutil.SimHash(text2, "zh", nil)
	if textutil.Hamming(a.Value, c.Value) > 10 {
		t.Fatalf("相似文本汉明距离应较小，got %d", textutil.Hamming(a.Value, c.Value))
	}
	// 完全不同文本距离应大。
	d := textutil.SimHash(strings.Repeat(" Completely unrelated english text about cooking recipes and sports.", 10), "en", nil)
	if textutil.Hamming(a.Value, d.Value) < 15 {
		t.Fatalf("不同文本汉明距离应较大，got %d", textutil.Hamming(a.Value, d.Value))
	}
}

func TestSimHashShortTextInvalid(t *testing.T) {
	// 短文本指纹不稳定（评审 B-9 边界）。
	fp := textutil.SimHash("短", "zh", nil)
	if fp.Valid {
		t.Fatalf("超短文本指纹应标记 invalid")
	}
}

func TestSimHashSplitJoin(t *testing.T) {
	// 评审 IC B-26：4 段 smallint 拆分/还原。
	fp := textutil.SimHash(strings.Repeat("测试文本内容用于验证指纹拆分。", 10), "zh", nil)
	parts := textutil.Split4(fp.Value)
	if textutil.Join4(parts) != fp.Value {
		t.Fatalf("Split4/Join4 必须互逆")
	}
}

func TestDedupKeyAndCycleBucket(t *testing.T) {
	// 评审 IC B-6：幂等键与周期桶。
	b := textutil.CycleBucket(3600*10+1234, 30) // 30 分钟桶
	if b != 3600*10 {
		t.Fatalf("周期桶计算错误，got %d", b)
	}
	k1 := textutil.DedupKey("url1", "src1", b)
	k2 := textutil.DedupKey("url1", "src1", b)
	if k1 != k2 {
		t.Fatalf("同周期同 URL 幂等键应一致")
	}
	k3 := textutil.DedupKey("url1", "src1", b+1800) // 下一周期
	if k1 == k3 {
		t.Fatalf("不同周期幂等键应不同（同 URL 每轮更新快照）")
	}
}
