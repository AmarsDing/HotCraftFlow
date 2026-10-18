package mediacontract

import (
	"strings"
	"testing"
	"time"
)

func TestULIDFormatAndMonotonic(t *testing.T) {
	a := ULID()
	b := ULID()
	if len(a) != 26 || len(b) != 26 {
		t.Fatalf("ULID 长度应为 26，got %d/%d", len(a), len(b))
	}
	// 字典序 == 生成顺序（同一毫秒内单调递增）。
	if b < a {
		t.Fatalf("ULID 应单调递增：%s 后于 %s 生成却更小", b, a)
	}
	for _, c := range a {
		if !isCrockford(byte(c)) {
			t.Fatalf("ULID 含非法字符 %q", string(c))
		}
	}
}

func TestNewIDPrefix(t *testing.T) {
	id := NewIntelItemID()
	if !strings.HasPrefix(id, "it_") {
		t.Fatalf("IntelItem ID 应为 it_ 前缀，got %s", id)
	}
	if err := ValidateIDFormat(id, PrefixIntelItem); err != nil {
		t.Fatalf("自生成 ID 应通过格式校验：%v", err)
	}
	if err := ValidateIDFormat(id, PrefixTopic); err == nil {
		t.Fatalf("错误前缀应校验失败")
	}
}

func TestIntelItemValidate(t *testing.T) {
	now := UTCNow()
	item := &IntelItem{
		APIVersion: "1.0",
		IntelID:    NewIntelItemID(),
		Source: IntelSource{
			Platform:      "weibo",
			Region:        RegionCN,
			CollectMethod: CollectHotlist,
			CollectedAt:   now,
		},
		Category:   CategoryAHot,
		Compliance: IntelCompliance{Gate: GatePass},
	}
	if err := item.Validate(); err != nil {
		t.Fatalf("合法 IntelItem 应通过：%v", err)
	}

	// 非法枚举必须失败。
	bad := *item
	bad.Category = "HOT" // 大写非法（契约要求 A_hot）
	if err := bad.Validate(); err == nil {
		t.Fatalf("非法 category 应校验失败")
	}

	// D_compliance 必须阻断自动流程。
	blk := *item
	blk.Category = CategoryDCompliance
	if !blk.BlocksAutoFlow() {
		t.Fatalf("D_compliance 必须阻断自动流程")
	}
}

func TestImpactCardDisclaimerMandatory(t *testing.T) {
	card := &ImpactCard{
		APIVersion: "1.0",
		ImpactID:   NewImpactCardID(),
		TopicID:    NewTopicID(),
		Sentiment:  Sentiment{Label: SentimentPositive, Score: 0.5, Engine: EngineFinBERT},
		Impact:     Impact{TTLHours: 24},
		Disclaimer: "", // 空免责声明
	}
	if err := card.Validate(); err == nil {
		t.Fatalf("disclaimer 为空必须校验失败（契约 §3）")
	}
	card.Disclaimer = ImpactDisclaimer
	if err := card.Validate(); err != nil {
		t.Fatalf("补免责声明后应通过：%v", err)
	}
}

func TestTransmissionConfidenceCap(t *testing.T) {
	rule := &TransmissionRule{
		RuleID:      NewID(PrefixRule),
		ToAsset:     "BTC/USDT",
		ToDirection: DirectionBearish,
		Confidence:  0.9, // 超过 0.6 上限
		Note:        "风险偏好假设",
	}
	if err := rule.Validate(); err == nil {
		t.Fatalf("传导规则 confidence >0.6 必须失败（假设不是因果）")
	}
	rule.Confidence = 0.45
	if err := rule.Validate(); err != nil {
		t.Fatalf("confidence ≤0.6 应通过：%v", err)
	}
}

func TestBurstNeedsTwoMethods(t *testing.T) {
	ts := &TrendSeries{
		APIVersion: "1.0",
		TopicID:    NewTopicID(),
		Category:   CategoryAHot,
		Stage:      StageRising,
		Burst: Burst{
			IsBurst:    true,
			MethodsHit: []BurstMethod{BurstZScore}, // 只有 1 个方法
		},
	}
	if err := ts.Validate(); err == nil {
		t.Fatalf("单方法命中不得 is_burst=true（契约 §2）")
	}
}

func TestEnvelopeRoundTrip(t *testing.T) {
	item := &IntelItem{
		APIVersion: "1.0",
		IntelID:    NewIntelItemID(),
		Category:   CategoryAHot,
		Compliance: IntelCompliance{Gate: GatePass},
	}
	env, err := NewEnvelope("ic", SubjectIntelNormalized, "", item)
	if err != nil {
		t.Fatalf("NewEnvelope: %v", err)
	}
	if err := ValidateIDFormat(env.EventID, PrefixEvent); err != nil {
		t.Fatalf("event_id 前缀非法：%v", err)
	}
	if env.TraceID == "" {
		t.Fatalf("trace_id 应自动生成")
	}

	b, _ := env.Encode()
	got, err := DecodeEnvelope(b)
	if err != nil {
		t.Fatalf("DecodeEnvelope: %v", err)
	}
	var back IntelItem
	if err := got.DecodePayload(&back); err != nil {
		t.Fatalf("DecodePayload: %v", err)
	}
	if back.IntelID != item.IntelID {
		t.Fatalf("往返后 intel_id 不一致")
	}
}

func TestUTCTimeFormat(t *testing.T) {
	s := FormatRFC3339UTC(time.Date(2026, 10, 17, 9, 0, 0, 0, time.FixedZone("CST", 8*3600)))
	if !strings.HasSuffix(s, "Z") {
		t.Fatalf("必须输出 UTC 带 Z，got %s", s)
	}
	if strings.Contains(s, "+08:00") {
		t.Fatalf("禁止 +08:00 偏移，got %s", s)
	}
}
