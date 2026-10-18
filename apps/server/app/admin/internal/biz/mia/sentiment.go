package mia

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	algomia "framework/app/admin/internal/algo/mia"
	"framework/pkg/mediacontract"
)

// SentimentOrchestrator 情感引擎编排（详细设计 §2.2 引擎选择策略 + §5.3 降级路径）。
//
// 降级链：FinBERT → LLM（云）→ Ollama → unknown（🔴 禁止猜测，NFR-MIA-08）。
// 引擎不可配置（URL/Key 缺）视为「未配置」，直接跳过，不计失败；
// 运行期调用失败按顺序降级。
type SentimentOrchestrator struct {
	finbert SentimentEngine
	llm     SentimentEngine // 云 LLM（内部已实现云→Ollama 降级）
	ollama  SentimentEngine // 显式本地兜底（llm 为 nil 时仍可用）

	log *log.Helper
}

// NewSentimentOrchestrator 组装编排器。引擎可为 nil（未配置）。
func NewSentimentOrchestrator(finbert, llm, ollama SentimentEngine, logger log.Logger) *SentimentOrchestrator {
	return &SentimentOrchestrator{
		finbert: finbert,
		llm:     llm,
		ollama:  ollama,
		log:     log.NewHelper(log.With(logger, "module", "biz/mia/sentiment")),
	}
}

// Classify 按引擎选择策略分类。
// 全部引擎失效时返回 label=unknown、score=0、engine=none——🔴 禁止猜测。
func (o *SentimentOrchestrator) Classify(ctx context.Context, in SentimentInput) (SentimentResult, mediacontract.SentimentEngine) {
	// 1. 选择候选引擎顺序（详细设计 §2.2 选择策略 + §5.3 降级表）。
	finbertOK := o.finbert != nil && o.finbert.Available()
	llmOK := (o.llm != nil && o.llm.Available()) || (o.ollama != nil && o.ollama.Available())

	engine := algomia.ChooseEngine(in.Lang, in.IsMacro, finbertOK, llmOK)
	if engine == mediacontract.EngineNone {
		o.log.WithContext(ctx).Warnf("all sentiment engines unavailable (lang=%s isMacro=%v): unknown, no guessing", in.Lang, in.IsMacro)
		return unknownSentiment(), mediacontract.EngineNone
	}

	// 2. 按选择顺序尝试（失败降级）。
	var candidates []SentimentEngine
	if engine == mediacontract.EngineFinBERT {
		candidates = append(candidates, o.finbert)
		if o.llm != nil && o.llm.Available() {
			candidates = append(candidates, o.llm)
		}
		if o.ollama != nil && o.ollama.Available() {
			candidates = append(candidates, o.ollama)
		}
	} else { // EngineLLM
		if o.llm != nil && o.llm.Available() {
			candidates = append(candidates, o.llm)
		}
		if o.ollama != nil && o.ollama.Available() {
			candidates = append(candidates, o.ollama)
		}
		if finbertOK && in.Lang == "en" {
			// LLM 全挂时 FinBERT 仍可处理英文（§5.3 降级表）。
			candidates = append(candidates, o.finbert)
		}
	}

	for i, eng := range candidates {
		if eng == nil || !eng.Available() {
			continue
		}
		res, err := eng.Classify(ctx, in)
		if err != nil {
			o.log.WithContext(ctx).Warnf("sentiment engine %s failed (candidate %d/%d), degrading: %v", eng.Name(), i+1, len(candidates), err)
			continue
		}
		if !res.Label.Valid() || res.Label == mediacontract.SentimentUnknown {
			continue
		}
		out := SentimentResult{
			Label: res.Label,
			Score: clamp11(res.Score),
		}
		return out, contractEngineName(eng.Name())
	}

	o.log.WithContext(ctx).Warnf("all sentiment candidates failed (lang=%s): unknown, no guessing", in.Lang)
	return unknownSentiment(), mediacontract.EngineNone
}

// contractEngineName 适配器名 → 契约引擎名。
// 云 LLM 与 Ollama 都归为契约 llm（契约 §3：不同引擎分数不可混比，仅粗粒度二引擎）。
func contractEngineName(adapterName string) mediacontract.SentimentEngine {
	switch adapterName {
	case "finbert":
		return mediacontract.EngineFinBERT
	case "llm", "llm-cloud", "llm-ollama":
		return mediacontract.EngineLLM
	default:
		return mediacontract.EngineNone
	}
}

func unknownSentiment() SentimentResult {
	return SentimentResult{Label: mediacontract.SentimentUnknown, Score: 0}
}

func clamp11(v float64) float64 {
	if v < -1 {
		return -1
	}
	if v > 1 {
		return 1
	}
	return v
}
