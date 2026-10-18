// Package engines 引擎抽象层（原创 Go 实现，架构思路参考调研报告 §2.1/§引擎抽象层）：
// 声明式能力位 + 急切/惰性双注册 + 不静默降级路由。
//
// 纪律：
//   - 能力位是引擎的自我声明（注册时静态给出），不是运行时探测；
//   - Select 拒绝 routing=unavailable 的引擎（400 + 稳定公开 reason）；
//   - 硬件与引擎声明不匹配时绝不悄悄跑 CPU——要么 400，要么显式 cpu_fallback 透出。
package engines

import (
	"context"
	"errors"
)

// Kind 引擎类别。
type Kind string

const (
	KindTTS Kind = "tts"
	KindASR Kind = "asr"
	KindLLM Kind = "llm"
)

// Caps 引擎声明式能力位。
type Caps struct {
	GPUCompat        []string // 可执行加速环境：cuda|rocm|metal|cpu（空=未定，按 cpu_only 处理）
	MinVRAMGB        float64
	SupportsCloning  bool
	SupportsEmotion  bool
	InstructVocab    []string // instruct 词汇白名单（客户端清洗用；空=不支持 instruct）
}

// SynthesisRequest 一次合成请求。
type SynthesisRequest struct {
	Text         string
	Instruct     string
	Language     string
	RefAudioPath string
	// Seconds 显式时长（0=由引擎自行决定，echo 引擎按文本长度估算）。
	Seconds float64
}

// ChunkSink 分块输出回调：pcm 为一段音频数据，progress 为 0–1。
// 返回 error 即中止合成（取消语义的落点）。
type ChunkSink func(pcm []byte, progress float64) error

// SynthesisResult 合成结果。
type SynthesisResult struct {
	DurationMs   int
	Samples      int
	Routing      Routing // 实际执行的路由（供响应元数据透出）
	ArtifactHint string  // 建议产物文件名（含扩展名）
}

// Engine 引擎接口。Available 必须幂等、快速、异常不外泄原文。
type Engine interface {
	ID() string
	DisplayName() string
	Kind() Kind
	Caps() Caps
	// Available 自述可用；reason 为一句稳定公开原因（内部诊断绝不出此边界）。
	Available(ctx context.Context) (ok bool, reason string)
	Synthesize(ctx context.Context, req SynthesisRequest, sink ChunkSink) (SynthesisResult, error)
}

// ---------------------------------------------------------------- 错误

// 稳定公开错误词表（对外只给分类，内部细节进日志）。
var (
	ErrEngineNotFound    = errors.New("engine not found")
	ErrEngineUnavailable = errors.New("engine unavailable on this host")
	ErrEngineNoCPUFallbk = errors.New("engine requires accelerator; silent cpu fallback is forbidden")
	ErrEngineLicense     = errors.New("engine license not accepted")
	ErrSynthesisFailed   = errors.New("synthesis failed")
)
