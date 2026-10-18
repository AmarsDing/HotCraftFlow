package biz

import "errors"

// =============================================================================
// ① IC 领域错误（biz 内部流控；对外映射 ic_error.proto 错误码责任在 service/biz 边界）
// =============================================================================

var (
	// ErrEmbeddingCap 语义向量当日配额耗尽（B-7 硬熔断：超限跳过 L3，按需告警）。
	ErrEmbeddingCap = errors.New("IC_EMBEDDING_DAILY_CAP")
	// ErrEmbeddingUnavailable store=none 或推理端点不可达（L3 停用并公告）。
	ErrEmbeddingUnavailable = errors.New("IC_EMBEDDING_UNAVAILABLE")
	// ErrLLMUnavailable llm.enabled=false 或推理端点不可达（ADR-IC-07 回退规则）。
	ErrLLMUnavailable = errors.New("IC_LLM_UNAVAILABLE")
	// ErrURLBlocked SSRF 私网拦截（S-6；对外 IC_URL_BLOCKED）。
	ErrURLBlocked = errors.New("IC_URL_BLOCKED")
	// ErrRobotsDenied robots 禁止（对外 IC_ROBOTS_DISALLOWED）。
	ErrRobotsDenied = errors.New("IC_ROBOTS_DENIED")
	// ErrEvidenceChainBroken 取证链校验失败（对外 IC_EVIDENCE_CHAIN_BROKEN，critical）。
	ErrEvidenceChainBroken = errors.New("IC_EVIDENCE_CHAIN_BROKEN")
)
