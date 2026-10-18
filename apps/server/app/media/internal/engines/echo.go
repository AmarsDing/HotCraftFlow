package engines

import (
	"context"
	"encoding/binary"
	"errors"
	"time"
	"unicode/utf8"

	"framework/app/media/internal/engines/echosynth"
)

// echoEngineID echo 占位引擎的注册 id。
const echoEngineID = "echo"

// echo 引擎的时长估算规则（占位物语义：文本越长音频越长）。
const (
	echoMinSeconds = 0.5
	echoMaxSeconds = 30.0
	// secondsPerRune 每字符折合秒数（中英文混排都按 rune 计，粗略但可预期）。
	secondsPerRune = 0.012
	// chunkMs 每块时长，驱动 SSE 进度事件的粒度。
	chunkMs = 200
	// throttle每块之间的微小停顿，避免秒级任务瞬间结束、UI 来不及渲染。
	// 占位物刻意模拟真实 TTS 的"合成耗时"观感；真实引擎接入后删除。
	chunkPaceMs = 30
)

// echoEngine 进程内占位引擎：按文本长度生成正弦 WAV，分块回调驱动进度。
type echoEngine struct{}

// NewEchoEngine 构造 echo 占位引擎（急切注册用）。
func NewEchoEngine() Engine { return &echoEngine{} }

// EchoEngineID 暴露注册 id（cmd 组装默认值用）。
func EchoEngineID() string { return echoEngineID }

func (e *echoEngine) ID() string          { return echoEngineID }
func (e *echoEngine) DisplayName() string { return "Echo Sine（占位引擎）" }
func (e *echoEngine) Kind() Kind          { return KindTTS }

// Caps echo 纯 CPU 执行：GPUCompat 只声明 cpu（任何宿主都可路由为 cpu_only）。
func (e *echoEngine) Caps() Caps {
	return Caps{
		GPUCompat:       []string{"cpu"},
		SupportsCloning: false,
		SupportsEmotion: false,
	}
}

// Available 进程内引擎恒可用（只要有 CPU）。
func (e *echoEngine) Available(context.Context) (bool, string) { return true, "" }

// EstimateSeconds 时长估算（导出给 echo 与 sidecar 示范共用）：
// 显式 seconds > 0 直接采用（钳到上限）；否则按 rune 数折算。
func EstimateSeconds(text string, explicit float64) float64 {
	sec := explicit
	if sec <= 0 {
		n := utf8.RuneCountInString(text)
		sec = echoMinSeconds + float64(n)*secondsPerRune
	}
	if sec < echoMinSeconds {
		sec = echoMinSeconds
	}
	if sec > echoMaxSeconds {
		sec = echoMaxSeconds
	}
	return sec
}

// Synthesize 分块合成：每 chunkMs 一块，先 sink 原始 PCM 字节，
// 由调用方（jobrun handler）负责把块追加拼装并最终 wav 封装。
// 取消语义：sink 返回 error 或 ctx 取消时立即中止。
func (e *echoEngine) Synthesize(ctx context.Context, req SynthesisRequest, sink ChunkSink) (SynthesisResult, error) {
	sec := EstimateSeconds(req.Text, req.Seconds)
	total := echosynth.SamplesForSeconds(sec)
	if total <= 0 {
		return SynthesisResult{}, ErrSynthesisFailed
	}
	pcm := echosynth.PCM(total)

	step := echosynth.ChunkSamples(chunkMs)
	if step <= 0 {
		step = total
	}
	var err error
	for off := 0; off < total; off += step {
		end := off + step
		if end > total {
			end = total
		}
		// PCM16 LE 编码该块。
		chunk := make([]byte, (end-off)*2)
		for i, s := range pcm[off:end] {
			binary.LittleEndian.PutUint16(chunk[i*2:], uint16(s))
		}
		progress := float64(end) / float64(total)
		// 占位物节奏控制：模拟逐块合成耗时。
		select {
		case <-ctx.Done():
			return SynthesisResult{}, errors.Join(ErrSynthesisFailed, ctx.Err())
		default:
		}
		if err = sink(chunk, progress); err != nil {
			return SynthesisResult{}, errors.Join(ErrSynthesisFailed, err)
		}
		sleepCtx(ctx, chunkPaceMs)
	}

	return SynthesisResult{
		DurationMs:   int(sec * 1000),
		Samples:      total,
		ArtifactHint: "speech.wav",
	}, nil
}

// sleepCtx 可被 ctx 打断的小睡（节奏控制）。
func sleepCtx(ctx context.Context, ms int) {
	t := time.NewTimer(time.Duration(ms) * time.Millisecond)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
