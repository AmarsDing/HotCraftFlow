package mme

import "errors"

// MME 错误码（文档 03-详细设计 §5）。消息体即错误码串，service 层转 HTTP 400/409/502。
var (
	// ErrDownloadFail 下载失败（风控/需登录/404/网络）——提示 Cookie 配置。
	ErrDownloadFail = errors.New("MME_DOWNLOAD_FAIL")
	// ErrASRFail 转写失败——保留中间产物；可换引擎重试。
	ErrASRFail = errors.New("MME_ASR_FAIL")
	// ErrLLMUnavailable 云与 Ollama 均不可用——任务暂停（不猜）。
	ErrLLMUnavailable = errors.New("MME_LLM_UNAVAILABLE")
	// ErrExpandTooLarge 笛卡尔组合数超上限——拒绝展开并提示减少维度。
	ErrExpandTooLarge = errors.New("MME_EXPAND_TOO_LARGE")
	// ErrRenderFail 渲染失败——保留中间产物；可重试。
	ErrRenderFail = errors.New("MME_RENDER_FAIL")
	// ErrQCFailed 🔴 质检不通过——标记 qc=false，阻断送审。
	ErrQCFailed = errors.New("MME_QC_FAILED")
	// ErrQCBlock 🔴 批准的变体被 QC 硬门禁拦下——列出清单（不静默放行）。
	ErrQCBlock = errors.New("MME_QC_BLOCK")
	// ErrRejectReasonEmpty 🔴 驳回未填原因——拒绝提交（FR-MIX-06 强约束）。
	ErrRejectReasonEmpty = errors.New("MME_REJECT_REASON_EMPTY")
	// ErrStorageFull 存储超限——阻断新渲染并提示清理。
	ErrStorageFull = errors.New("MME_STORAGE_FULL")
	// ErrCacheDisabled 缓存被配置关闭（信息性，不应向上冒泡）。
	ErrCacheDisabled = errors.New("MME_CACHE_DISABLED")
	// ErrQueueUnavailable asynq 未配置——入队失败（任务不会静默假排队）。
	ErrQueueUnavailable = errors.New("MME_QUEUE_UNAVAILABLE")
	// ErrTaskNotFound 任务不存在。
	ErrTaskNotFound = errors.New("MME_TASK_NOT_FOUND")
	// ErrSetNotFound 变体簇不存在。
	ErrSetNotFound = errors.New("MME_SET_NOT_FOUND")
	// ErrVariantNotFound 变体不存在。
	ErrVariantNotFound = errors.New("MME_VARIANT_NOT_FOUND")
	// ErrJobNotFound 渲染任务不存在。
	ErrJobNotFound = errors.New("MME_JOB_NOT_FOUND")
	// ErrInvalidState 状态机非法流转（如未下载就展开）。
	ErrInvalidState = errors.New("MME_INVALID_STATE")
	// ErrAssetMissing 任务尚无素材（下载未完成）。
	ErrAssetMissing = errors.New("MME_ASSET_MISSING")
	// ErrHighlightsEmpty 无内容版本（手工 EDL 或分析高光为空）——无法展开。
	ErrHighlightsEmpty = errors.New("MME_HIGHLIGHTS_EMPTY")
)
