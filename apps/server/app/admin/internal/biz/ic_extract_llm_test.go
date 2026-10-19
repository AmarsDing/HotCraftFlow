package biz

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// L4 提取 LLM 增强链（ADR-IC-07：默认规则版；开启失败回退规则；B-18 融合纪律）
// =============================================================================

// stubLLM 可编程 LLM 端口。
type stubLLM struct {
	enabled bool
	sug     *ExtractSuggestion
	err     error
	calls   int
}

func (s *stubLLM) Enabled() bool { return s.enabled }
func (s *stubLLM) Extract(_ context.Context, _ *contract.IntelItem) (*ExtractSuggestion, error) {
	s.calls++
	return s.sug, s.err
}

func llmCfg() *IcConfig {
	cfg := ResolveIcConfig(nil)
	cfg.LLMEnabled = true
	return cfg
}

func extractItem() *contract.IntelItem {
	return &contract.IntelItem{
		APIVersion: "1.0",
		IntelID:    contract.NewIntelItemID(),
		Source:     contract.IntelSource{Platform: "weibo", Region: contract.RegionCN, CollectMethod: contract.CollectHotlist},
		Payload:    contract.IntelPayload{Title: "工信部发布新能源补贴政策", Text: "财政部联合发布新一轮新能源汽车购置补贴政策，涉及多个省市试点。", Lang: "zh"},
	}
}

// TestExtractLLMMerge B-18 融合：实体按 (type,value) 保最高、置信钳制 [0.6,0.95]、
// 分类规则为准、workability 仅贴阈值带内采信 LLM。
func TestExtractLLMMerge(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)
	item := extractItem()

	// 先跑规则基线（LLM 关），拿真实 workability 分供贴带构造。
	base := NewIcExtractUsecase(ResolveIcConfig(nil), nil, logger).Extract(ctx, item)
	if base.Category == "" {
		t.Fatal("规则分类不能为空")
	}

	llm := &stubLLM{enabled: true, sug: &ExtractSuggestion{
		Entities: []contract.Entity{
			{Type: contract.EntityPolicy, Value: "LLM独有实体", Confidence: 0.8},
			{Type: contract.EntityOrg, Value: "置信超上限", Confidence: 0.99}, // 钳 0.95（B-18）
			{Type: contract.EntityPerson, Value: "低置信应丢弃", Confidence: 0.3},
			{Type: contract.EntityType("bogus"), Value: "非法类型应丢弃", Confidence: 0.9},
		},
		Category:          "C_market", // 与规则大概率不同 → 须保持规则
		WorkabilityScore:  base.WorkabilityScore + 0.04,
		WorkabilityReason: "LLM 复核建议",
	}}
	res := NewIcExtractUsecase(llmCfg(), llm, logger).Extract(ctx, item)

	if res.LLMDegraded {
		t.Fatal("LLM 正常返回不得标降级")
	}
	if llm.calls != 1 {
		t.Fatalf("LLM 调用计数: %d", llm.calls)
	}
	// 分类以规则为准（B-18）
	if res.Category != base.Category {
		t.Fatalf("分类漂移: rule=%s got=%s —— B-18 纪律失守", base.Category, res.Category)
	}
	// workability 贴带采信
	if res.WorkabilityScore != base.WorkabilityScore+0.04 {
		t.Fatalf("贴带采信失败: base=%f got=%f", base.WorkabilityScore, res.WorkabilityScore)
	}
	// 实体融合断言
	byValue := map[string]float64{}
	for _, e := range res.Entities {
		byValue[e.Value] = e.Confidence
	}
	if byValue["LLM独有实体"] != 0.8 {
		t.Fatalf("LLM 新实体未并入: %v", res.Entities)
	}
	if byValue["置信超上限"] != 0.95 {
		t.Fatalf("B-18 上限钳制失败: %v", byValue["置信超上限"])
	}
	if _, ok := byValue["低置信应丢弃"]; ok {
		t.Fatal("低置信实体应被丢掉（不进 MIA 链接）")
	}
	if _, ok := byValue["非法类型应丢弃"]; ok {
		t.Fatal("非法实体类型应被丢掉")
	}
	// 规则独有实体必须保留（融合是 max-merge 不是替换）
	for _, e := range base.Entities {
		if _, ok := byValue[e.Value]; !ok {
			t.Fatalf("规则实体 %s 被 LLM 融合吞掉", e.Value)
		}
	}
}

// TestExtractLLMOutOfBand workability 离带（>±0.05）不采信——避免 LLM 漂移改写门禁。
func TestExtractLLMOutOfBand(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)
	item := extractItem()
	base := NewIcExtractUsecase(ResolveIcConfig(nil), nil, logger).Extract(ctx, item)

	llm := &stubLLM{enabled: true, sug: &ExtractSuggestion{
		WorkabilityScore: base.WorkabilityScore + 0.3, // 出带
	}}
	res := NewIcExtractUsecase(llmCfg(), llm, logger).Extract(ctx, item)
	if res.WorkabilityScore != base.WorkabilityScore {
		t.Fatalf("离带分数被采信: base=%f got=%f —— LLM 不得漂移门禁分", base.WorkabilityScore, res.WorkabilityScore)
	}
}

// TestExtractLLMDegrades 故障/空建议 → 规则回退（降级标记可观测）。
func TestExtractLLMDegrades(t *testing.T) {
	ctx := context.Background()
	logger := log.NewStdLogger(io.Discard)
	item := extractItem()
	base := NewIcExtractUsecase(ResolveIcConfig(nil), nil, logger).Extract(ctx, item)

	t.Run("服务故障回退规则", func(t *testing.T) {
		llm := &stubLLM{enabled: true, err: errors.New("llm timeout")}
		res := NewIcExtractUsecase(llmCfg(), llm, logger).Extract(ctx, item)
		if !res.LLMDegraded {
			t.Fatal("故障必须标 LLMDegraded（观测）")
		}
		if res.Category != base.Category || res.WorkabilityScore != base.WorkabilityScore {
			t.Fatal("回退结果必须与纯规则一致")
		}
	})

	t.Run("空建议回退规则", func(t *testing.T) {
		llm := &stubLLM{enabled: true, sug: nil}
		res := NewIcExtractUsecase(llmCfg(), llm, logger).Extract(ctx, item)
		if !res.LLMDegraded || res.Category != base.Category {
			t.Fatal("空建议回退异常")
		}
	})

	t.Run("端口报不可用 → 不调用且非降级", func(t *testing.T) {
		llm := &stubLLM{enabled: false}
		res := NewIcExtractUsecase(llmCfg(), llm, logger).Extract(ctx, item)
		if llm.calls != 0 {
			t.Fatal("Enabled=false 不得发起调用")
		}
		if res.LLMDegraded {
			t.Fatal("未启用不是降级（口径区分）")
		}
	})

	t.Run("配置开关关闭 → 规则版", func(t *testing.T) {
		llm := &stubLLM{enabled: true, sug: &ExtractSuggestion{Category: "C_market"}}
		res := NewIcExtractUsecase(ResolveIcConfig(nil), llm, logger).Extract(ctx, item)
		if llm.calls != 0 || res.Category != base.Category {
			t.Fatal("ADR-IC-07 默认关：配置关闭时规则为唯一事实源")
		}
	})
}

// TestMergeLLMEntities 纯函数融合纪律：非法/低置信丢、上限钳、同键保高。
func TestMergeLLMEntities(t *testing.T) {
	rule := []contract.Entity{
		{Type: contract.EntityOrg, Value: "财政部", Confidence: 0.7},
	}
	got := mergeLLMEntities(rule, []contract.Entity{
		{Type: contract.EntityOrg, Value: "财政部", Confidence: 0.72},  // 同键更高 → 覆盖规则的 0.7
		{Type: contract.EntityOrg, Value: "财政部", Confidence: 0.61},  // 较低且晚到 → 保留 0.72
		{Type: contract.EntityTopic, Value: "新能源", Confidence: 0.60}, // 贴下限舍入并入
	})
	var confMoF, hasTopic float64
	var topicFound bool
	for _, e := range got {
		if e.Value == "财政部" {
			confMoF = e.Confidence
		}
		if e.Value == "新能源" {
			hasTopic = e.Confidence
			topicFound = true
		}
	}
	if confMoF != 0.72 {
		t.Fatalf("同键未保最高置信: %v (got %f)", got, confMoF)
	}
	if !topicFound || hasTopic != 0.60 {
		t.Fatalf("下限实体并入异常: found=%v conf=%f", topicFound, hasTopic)
	}
}
