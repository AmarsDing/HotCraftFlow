package tt

import (
	"strings"
	"testing"
)

// TestTitleSimHashDeterministic 指纹确定性（FNV 固定参数，漂移=全量历史指纹作废）。
func TestTitleSimHashDeterministic(t *testing.T) {
	title := "油价上调窗口开启 多地将迎新一轮涨价"
	a, b := TitleSimHash64(title), TitleSimHash64(title)
	if a != b {
		t.Fatal("SimHash 不确定")
	}
	if a == 0 {
		t.Fatal("正常标题不得为零指纹")
	}
}

// TestTitleSimHashShortTitle 短标题是 TT 主语料：单字符也有特征，不会全数失效
// （这是不用 B-9 textutil SimHash 的原因——其 ≥40token/≥80字符 门会判死所有标题）。
func TestTitleSimHashShortTitle(t *testing.T) {
	if fp := TitleSimHash64("震"); fp == 0 {
		t.Fatal("单字符标题应有非零指纹")
	}
	if fp := TitleSimHash64("央行降息"); fp == 0 {
		t.Fatal("四字标题应有非零指纹")
	}
}

// TestSimHashSim 相似度语义：同文=1；同事件微调高相似；无关话题低相似。
func TestSimHashSim(t *testing.T) {
	base := "新能源汽车补贴政策延续至明年 多车企跟进降价促销"
	a := TitleSimHash64(base)
	if s := SimHashSim(a, a); s != 1 {
		t.Fatalf("同文应 s=1: %v", s)
	}
	// 同事件改写（替换少量词）——明显高于无关话题。
	// 注：bigram SimHash 对短标题敏感，两字改写会翻转十余位（≈0.77），
	// 不能按长文档经验钉 0.8；与无关组拉开差距即可（Step2 召回是三路并集）。
	variant := TitleSimHash64("新能源汽车补贴政策延续到明年 多家车企跟进降价促销")
	if s := SimHashSim(a, variant); s < 0.70 {
		t.Fatalf("同事件微调应明显相似: %v", s)
	}
	// 无关话题——低相似（断言远低于合并阈值，不钉死具体值防脆弱）
	other := TitleSimHash64("欧冠决赛对阵出炉 两支球队将于下月会师伦敦")
	if s := SimHashSim(a, other); s > 0.70 {
		t.Fatalf("无关话题应低相似: %v", s)
	}
}

// TestSimHashSegments 位段约定（B-26 同款;int16 直接截断，含符号位翻转）。
func TestSimHashSegments(t *testing.T) {
	fp := uint64(0x0123456789ABCDEF)
	segs := SimHashSegments(fp)
	// int16 截断语义：0x89AB→35243-65536=-30293，0xCDEF→52719-65536=-12817。
	want := [4]int16{0x0123, 0x4567, -30293, -12817}
	if segs != want {
		t.Fatalf("segs=%v want %v", segs, want)
	}
}

// TestPigeonholeRecallProperty 鸽笼召回性质：Hamming≤3 ⟹ 至少一段等值。
// 这是「四段任一等值即候选」查询的正确性根基，全位级扫描验证。
func TestPigeonholeRecallProperty(t *testing.T) {
	fp := TitleSimHash64("央行宣布降准降息 释放流动性支持实体经济")
	for i := 0; i < 64; i++ { // 单比特
		assertSomeSegmentEqual(t, fp, fp^(1<<uint(i)))
		for j := i + 1; j < 64; j++ { // 双比特
			assertSomeSegmentEqual(t, fp, fp^(1<<uint(i))^(1<<uint(j)))
		}
	}
}

func assertSomeSegmentEqual(t *testing.T, a, b uint64) {
	t.Helper()
	sa, sb := SimHashSegments(a), SimHashSegments(b)
	eq := false
	for k := 0; k < 4; k++ {
		if sa[k] == sb[k] {
			eq = true
			break
		}
	}
	if !eq {
		t.Fatalf("Hamming≤3 但四段全异: %016x vs %016x", a, b)
	}
}

// TestHammingWithin A-1 召回阈值边界（≤3 召回，4 不召回）。
func TestHammingWithin(t *testing.T) {
	base := uint64(0x0000000000000000)
	d3 := base ^ 0b111  // 距 3
	d4 := base ^ 0b1111 // 距 4
	if !HammingWithin(base, d3, 3) {
		t.Fatal("距 3 应在召回窗内")
	}
	if HammingWithin(base, d4, 3) {
		t.Fatal("距 4 不应召回")
	}
}

// TestJaccardEntities 集合相似度语义与空集约定。
func TestJaccardEntities(t *testing.T) {
	if got := JaccardEntities(nil, nil); got != 0 {
		t.Fatalf("空∪空应 0（无信息态不得产出相似度）: %v", got)
	}
	if got := JaccardEntities([]string{"person:甲"}, nil); got != 0 {
		t.Fatalf("一侧为空应 0: %v", got)
	}
	if got := JaccardEntities([]string{"a"}, []string{"b"}); got != 0 {
		t.Fatalf("不相交应 0: %v", got)
	}
	if got := JaccardEntities([]string{"a", "b"}, []string{"b", "a"}); got != 1 {
		t.Fatalf("全同应 1: %v", got)
	}
	got := JaccardEntities([]string{"a", "b"}, []string{"b", "c"})
	if got < 0.333 || got > 0.334 {
		t.Fatalf("1/3 附近: %v", got)
	}
	// 去重语义：重复实体不影响结果
	if g2 := JaccardEntities([]string{"a", "a", "b"}, []string{"b", "c"}); g2 != got {
		t.Fatalf("重复元素不应改变 Jaccard: %v vs %v", g2, got)
	}
}

// TestSimHashLongTitleStability 长标题截断前不 panic 且同前缀高相似。
func TestSimHashLongTitleStability(t *testing.T) {
	long := strings.Repeat("国际原油价格大幅上涨影响全球市场", 8)
	a := TitleSimHash64(long)
	b := TitleSimHash64(long + "，专家称涨势或将延续")
	if s := SimHashSim(a, b); s < 0.70 {
		t.Fatalf("长标题加后缀应仍高相似: %v", s)
	}
}
