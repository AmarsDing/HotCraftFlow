package iccollector

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// V1 降级适配器：embedding store=none / llm.enabled=false 的显式降级实现。
// 显式 > 隐式：端口总是非 nil，行为由配置决定（B-7 store=none = L3 停用并公告；
// 将来接 bge 推理服务时新建实现替换 provider，biz 零改动）。
// =============================================================================

// DisabledEmbedder 语义向量停用实现。
type DisabledEmbedder struct {
	store string
	log   *log.Helper
}

func NewEmbedder(cfg *biz.IcConfig, logger log.Logger) *DisabledEmbedder {
	e := &DisabledEmbedder{log: log.NewHelper(log.With(logger, "module", "ic/embedder"))}
	if cfg.EmbeddingStoreEnabled {
		// 配置要求 store 但 V1 无实现：开工即公告，L3 依旧停用（不允许静默假装在跑）。
		e.log.Warn("embedding store 已配置非 none，但 V1 推理适配器未交付——L3 按停用运行")
		e.store = "misconfigured"
	}
	return e
}

func (e *DisabledEmbedder) Embed(context.Context, string, string) (string, error) {
	return "", biz.ErrEmbeddingUnavailable
}

func (e *DisabledEmbedder) SearchSimilar(context.Context, string, float64, int32) ([]biz.EmbeddingNeighbor, error) {
	return nil, biz.ErrEmbeddingUnavailable
}

func (e *DisabledEmbedder) Enabled() bool { return false }

func (e *DisabledEmbedder) DailyRemaining(context.Context) int32 { return 0 }

var _ biz.EmbedderPort = (*DisabledEmbedder)(nil)

// DisabledLLMEnhancer LLM 停用实现（ADR-IC-07：纯规则理解层即降级态全貌）。
type DisabledLLMEnhancer struct{}

func NewLLMEnhancer(cfg *biz.IcConfig, logger log.Logger) *DisabledLLMEnhancer {
	if cfg.LLMEnabled {
		log.NewHelper(log.With(logger, "module", "ic/llm")).
			Warn("ic.llm.enabled=true 但 V1 LLM 适配器未交付——按规则版理解层运行（fallback_to_rule 语义）")
	}
	return &DisabledLLMEnhancer{}
}

func (DisabledLLMEnhancer) Enabled() bool { return false }

func (DisabledLLMEnhancer) Extract(context.Context, *contract.IntelItem) (*biz.ExtractSuggestion, error) {
	return nil, biz.ErrLLMUnavailable
}

var _ biz.LLMEnhancePort = (*DisabledLLMEnhancer)(nil)
