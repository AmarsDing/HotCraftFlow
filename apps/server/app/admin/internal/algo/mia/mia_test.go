package mia

import (
	"math"
	"testing"

	"framework/pkg/mediacontract"
)

func TestScoreImpactDirectionPreserved(t *testing.T) {
	// 评审修复：极好与极坏同分（|sentiment|），但方向不丢。
	in := ImpactInput{
		SentimentScore:      -0.9, // 极坏
		DistinctDomainCount: 4,
		SourceCredibility:   1.0,
		TopConfidences:      []float64{0.9, 0.8, 0.7},
		AgeHours:            1,
		TTLHours:            24,
	}
	out := ScoreImpact(in)
	if out.Direction != mediacontract.DirectionBearish {
		t.Fatalf("负情感应为 bearish，got %s", out.Direction)
	}
	if out.Score <= 0 {
		t.Fatalf("强信号应有正分，got %f", out.Score)
	}
	// 方向性独立：同 |score| 的正向应为 bullish。
	in.SentimentScore = 0.9
	if ScoreImpact(in).Direction != mediacontract.DirectionBullish {
		t.Fatalf("正情感应为 bullish")
	}
}

func TestScoreImpactNoMapping(t *testing.T) {
	// 无实体映射 → score=0 且 NoMapping（评审：不强行匹配）。
	out := ScoreImpact(ImpactInput{
		SentimentScore: 0.9, DistinctDomainCount: 4, SourceCredibility: 1,
		TopConfidences: nil, AgeHours: 1, TTLHours: 24,
	})
	if !out.NoMapping || out.Score != 0 {
		t.Fatalf("无映射应 score=0 + NoMapping，got %+v", out)
	}
}

func TestUnknownSentimentNotGuessed(t *testing.T) {
	// 引擎全失效 → EngineNone，score=0（禁止猜测）。
	if got := ChooseEngine("zh", false, false, false); got != mediacontract.EngineNone {
		t.Fatalf("引擎全失效应为 none，got %s", got)
	}
	// 中文走 LLM。
	if got := ChooseEngine("zh-CN", false, true, true); got != mediacontract.EngineLLM {
		t.Fatalf("中文应走 LLM，got %s", got)
	}
	// 英文非宏观走 FinBERT。
	if got := ChooseEngine("en", false, true, true); got != mediacontract.EngineFinBERT {
		t.Fatalf("英文非宏观应走 FinBERT，got %s", got)
	}
}

func TestTransmissionConfidenceCapped(t *testing.T) {
	rule := mediacontract.TransmissionRule{
		RuleID: "rule_x", ToAsset: "BTC/USDT", ToDirection: mediacontract.DirectionBearish,
		Confidence: 0.45, Note: "风险偏好假设",
	}
	parent := mediacontract.AffectedItem{Confidence: 0.9, Path: []string{"人物表态"}}
	out := ApplyTransmission(rule, parent)
	// 0.45 × 0.9 = 0.405 ≤ 0.6。
	if out.Confidence > 0.6 {
		t.Fatalf("传导置信度必须 ≤0.6，got %f", out.Confidence)
	}
	if !out.IsTransmission || len(out.Path) != 2 {
		t.Fatalf("传导项应有 is_transmission 且 path 追加")
	}
}

func TestEntityLinkLowConfidenceFiltered(t *testing.T) {
	// 契约硬约束：confidence < 0.6 不进 MIA。
	e := mediacontract.Entity{Type: mediacontract.EntityOrg, Value: "美联储", Confidence: 0.5}
	mappings := []EntityMapping{{EntityName: "美联储", Symbol: "DXY", AssetClass: mediacontract.AssetFX, BaseConfidence: 1.0}}
	if hits := LinkEntity(e, mappings); len(hits) != 0 {
		t.Fatalf("低置信实体不应链接，got %d 命中", len(hits))
	}
	// 高置信精确命中。
	e.Confidence = 0.9
	if hits := LinkEntity(e, mappings); len(hits) != 1 {
		t.Fatalf("高置信精确匹配应命中，got %d", len(hits))
	}
}

func TestTTLByAssetClass(t *testing.T) {
	if TTLForAssetClass(mediacontract.AssetCrypto) != 12 {
		t.Fatalf("加密 TTL 应为 12h")
	}
	if TTLForAssetClass(mediacontract.AssetEquity) != 24 {
		t.Fatalf("股票 TTL 应为 24h")
	}
}

func TestEventSimilarityWeights(t *testing.T) {
	// 权重 0.5/0.3/0.2；全满 → 1.0。
	if got := EventSimilarity(1, 1, 0); math.Abs(got-1.0) > 1e-9 {
		t.Fatalf("全满相似度应为 1.0，got %f", got)
	}
	if !ShouldMergeEvent(0.85) || ShouldMergeEvent(0.7) {
		t.Fatalf("并入阈值 0.82 判定错误")
	}
}
