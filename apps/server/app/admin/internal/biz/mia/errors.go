package mia

import "errors"

// MIA 错误码（详细设计 §5）。业务失败按此抛出，service 层转换为 HTTP 错误。
var (
	// ErrMappingNotFound 实体无映射——诚实返回空 affected，不强行匹配。
	ErrMappingNotFound = errors.New("MIA_MAPPING_NOT_FOUND")
	// ErrSentimentUnavailable 情感引擎全不可用——返回 unknown，禁止猜测。
	ErrSentimentUnavailable = errors.New("MIA_SENTIMENT_UNAVAILABLE")
	// ErrDisclaimerEmpty 🔴 免责声明为空——拒绝落库与推送（硬失败）。
	ErrDisclaimerEmpty = errors.New("MIA_DISCLAIMER_EMPTY")
	// ErrPathEmpty 🔴 affected 项 path 为空——拒绝落库（硬失败）。
	ErrPathEmpty = errors.New("MIA_PATH_EMPTY")
	// ErrConfidenceInvalid 🔴 confidence 越界或缺失——拒绝落库（硬失败）。
	ErrConfidenceInvalid = errors.New("MIA_CONFIDENCE_INVALID")
	// ErrQuoteReadFail 行情拉取失败——跳过校验，不影响评分产出。
	ErrQuoteReadFail = errors.New("MIA_QUOTE_READ_FAIL")
	// ErrOpenBBLicenseUnverified OpenBB 许可证未复核——拒绝接入（阻断式）。
	ErrOpenBBLicenseUnverified = errors.New("MIA_OPENBB_LICENSE_UNVERIFIED")
	// ErrLLMCapExceeded 云 LLM 超日调用上限——熔断至 Ollama。
	ErrLLMCapExceeded = errors.New("MIA_LLM_CAP_EXCEEDED")
	// ErrTradingBlocked 🔴 检测到交易类调用——立即拒绝并 critical 告警。
	ErrTradingBlocked = errors.New("MIA_TRADING_BLOCKED")
	// ErrTransmissionConfidenceTooHigh 传导规则置信度 >0.6（假设非因果，标 0.9 是自欺欺人）。
	ErrTransmissionConfidenceTooHigh = errors.New("MIA_TRANSMISSION_CONFIDENCE_TOO_HIGH")
)
