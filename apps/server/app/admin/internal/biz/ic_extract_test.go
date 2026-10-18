package biz

import (
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 理解层规则版黄金测试（B-18：entities 词表/正则、category D>C>A>B、
// workability 公式与四词表 reason——降级纪律的主路径必须有回归保险）
// =============================================================================

func hasEntity(ents []contract.Entity, typ contract.EntityType, value string) bool {
	for _, e := range ents {
		if e.Type == typ && e.Value == value {
			return true
		}
	}
	return false
}

func TestExtractEntitiesRules(t *testing.T) {
	// 《》→ policy(0.9)：词典/正则确定性断言
	ents := extractEntities("", "监管部门发布《网络平台内容审核规定》要求落实")
	if !hasEntity(ents, contract.EntityPolicy, "网络平台内容审核规定") {
		t.Fatalf("《》 应抽 policy 实体: %+v", ents)
	}
	// tech 词典 → tech(0.9)（政策/市场词典是 category 信号而非实体，刻意不抽——
	// 词短极易脏实体表；category 断言见 TestClassifyCategoryPrecedence）
	ents = extractEntities("", "OpenAI 发布新一代大模型，GPT 系列再升级")
	if !hasEntity(ents, contract.EntityTech, "OpenAI") {
		t.Fatalf("tech 词典词应抽 tech 实体: %+v", ents)
	}
	// 置信度下限：规则均 ≥0.6（B-18 丢弃线）
	for _, e := range ents {
		if e.Confidence < 0.6 {
			t.Fatalf("实体置信度跌破下限: %+v", e)
		}
	}
}

func TestClassifyCategoryPrecedence(t *testing.T) {
	m := &contract.IntelMetrics{}
	// D：政策词典
	if got := classifyCategory(m, &contract.IntelPayload{Title: "新规对平台处罚做出规定"}); got != contract.CategoryDCompliance {
		t.Fatalf("政策文应 D: %s", got)
	}
	// D 与 C 同现 → D 优先（合规信号判成热点代价不对称）
	p := &contract.IntelPayload{Title: "新规出台，央行随即表态，美联储财报季临近", Text: ""}
	if got := classifyCategory(m, p); got != contract.CategoryDCompliance {
		t.Fatalf("D 与 C 同现应 D 优先: %s", got)
	}
	// C：市场词典
	if got := classifyCategory(m, &contract.IntelPayload{Title: "美联储今日降息，美股大涨财报季开启"}); got != contract.CategoryCMarket {
		t.Fatalf("市场文应 C: %s", got)
	}
	// A：rank/rank_total ≤0.3（且不含 C/D 词）
	ma := &contract.IntelMetrics{Rank: 3, RankTotal: 10}
	if got := classifyCategory(ma, &contract.IntelPayload{Title: "某明星出席活动引热议"}); got != contract.CategoryAHot {
		t.Fatalf("热榜前 30%% 应 A: %s", got)
	}
	// B：兜底
	mb := &contract.IntelMetrics{Rank: 8, RankTotal: 10}
	if got := classifyCategory(mb, &contract.IntelPayload{Title: "某纪录片片段流出"}); got != contract.CategoryBMaterial {
		t.Fatalf("默认应 B 兜底: %s", got)
	}
}

// TestScoreWorkabilityFormula B-18 公式逐项锚定，
// score 偏差即表示实现漂移（公式改动必须同步此断言）。
func TestScoreWorkabilityFormula(t *testing.T) {
	ents := make([]contract.Entity, 5) // 5 实体 → hasViewpoint=1
	p := &contract.IntelPayload{
		MediaRefs: []string{"asset://a", "asset://b", "asset://c"}, // 3 素材 → hasMaterial=1
		Text:      strings.Repeat("文", 3000),                       // 3000 字 → 1
		Lang:      "zh",
	}
	score, reason := scoreWorkability(ents, p, 0.5)
	if d := score - 1.0; d > 1e-9 || d < -1e-9 {
		t.Fatalf("全满项应得 ≈1.0: %f", score)
	}
	if reason != "" {
		t.Fatalf("过线不应有 reason: %q", reason)
	}

	// 精确半分：hasMaterial=0.5（1.5 素材 → clamp 后 1/3）不重；取 2 素材 = 2/3
	p2 := &contract.IntelPayload{
		MediaRefs: []string{"asset://a"},     // 1/3
		Text:      strings.Repeat("文", 1500), // 1500/3000 = 0.5（zh → 系数1）
		Lang:      "en-us",
	}
	score, reason = scoreWorkability(make([]contract.Entity, 0), p2, 0.5)
	want := 0.4*(1.0/3.0) + 0.3*0.0 + 0.3*0.5
	if diff := score - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("公式偏差: got %f want %f", score, want)
	}
	if reason == "" {
		t.Fatal("低于 0.5 必须有 reason")
	}
}

func TestScoreWorkabilityReasonWordlist(t *testing.T) {
	// 语言不支持：fr + 低分
	fr := &contract.IntelPayload{Text: strings.Repeat("a", 3000), Lang: "fr"}
	_, reason := scoreWorkability(nil, fr, 0.5)
	if reason != "语言不支持" {
		t.Fatalf("fr 应判语言不支持: %q", reason)
	}
	// 素材不足：zh + 无素材 + 短文
	zh := &contract.IntelPayload{Text: strings.Repeat("文", 900), Lang: "zh"}
	_, reason = scoreWorkability(nil, zh, 0.5)
	if reason != "素材不足" {
		t.Fatalf("无素材应判素材不足(第一主缺口): %q", reason)
	}
	// 时长不足：有素材有观点但 <800 字（minScore 拉高触发 reason 分支）
	ents := make([]contract.Entity, 5)
	short := &contract.IntelPayload{
		MediaRefs: []string{"asset://a", "asset://b", "asset://c"},
		Text:      strings.Repeat("文", 500),
		Lang:      "zh",
	}
	_, reason = scoreWorkability(ents, short, 0.9)
	if reason != "时长不足" {
		t.Fatalf("短文本应判时长不足: %q", reason)
	}
	// 无明确观点：素材/时长足但无实体
	noView := &contract.IntelPayload{
		MediaRefs: []string{"asset://a", "asset://b", "asset://c"},
		Text:      strings.Repeat("文", 3000),
		Lang:      "zh",
	}
	score, reason := scoreWorkability(nil, noView, 0.5)
	// 0.4*1 + 0.3*0 + 0.3*1 = 0.7 ≥ 0.5 → 过线空 reason；
	// 构造不能过线情形：minScore 拉高
	if score < 0.69 || score > 0.71 {
		t.Fatalf("公式漂移: %f", score)
	}
	_, reason = scoreWorkability(nil, noView, 0.8)
	if reason != "无明确观点" {
		t.Fatalf("应判无明确观点: %q", reason)
	}
}

func TestExtractUsecaseRuleOnly(t *testing.T) {
	// llm=nil 纯规则降级态（ADR-IC-07 默认态）：提取结果 determinism 断言
	uc := NewIcExtractUsecase(&IcConfig{WorkabilityMinScore: 0.5}, nil, log.DefaultLogger)
	item := &contract.IntelItem{
		Payload: contract.IntelPayload{
			Title: "监管部门发布《网络平台内容审核规定》",
			Text:  strings.Repeat("监管要求平台加强内容治理，对内部分类分级给出时限。", 20),
			Lang:  "zh",
		},
	}
	res := uc.Extract(t.Context(), item)
	if res == nil {
		t.Fatal("pure rules extraction returned nil")
	}
	if len(res.Entities) == 0 {
		t.Fatal("rule path must produce entities for policy text")
	}
}
