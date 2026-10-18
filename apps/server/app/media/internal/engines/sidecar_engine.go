package engines

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"framework/pkg/exectree"
	"framework/app/media/internal/sidecars"
)

// SidecarSpec sidecar 类引擎的注册描述（catalog 预留下的最小形态）。
type SidecarSpec struct {
	ID          string
	DisplayName string
	Kind        Kind
	Caps        Caps
	// Bin sidecar 可执行文件（绝对路径或 PATH 名）。
	Bin string
	// DriverCfg 超时/看门狗/env 白名单（来自 media.yaml sidecar 段）。
	DriverCfg sidecars.Config
	// Env 该引擎特有的追加环境（覆盖在 DriverCfg.Env 之后）。
	Env []string
	// Log 日志（nil 时用默认 logger）。
	Log *slog.Logger
}

// SidecarEngine 子进程 sidecar 的 Engine 适配器：
// 把 sidecars.Driver 的生命周期/帧协议语义收编到引擎抽象层。
type SidecarEngine struct {
	spec SidecarSpec
	drv  *sidecars.Driver
}

// NewSidecarEngine 构造适配器（进程惰性拉起——首次 Synthesize 才 spawn）。
func NewSidecarEngine(spec SidecarSpec) *SidecarEngine {
	log := spec.Log
	if log == nil {
		log = slog.Default()
	}
	var env []string
	env = append(env, spec.DriverCfg.Env...)
	env = append(env, spec.Env...)
	cfg := spec.DriverCfg
	cfg.Env = env
	return &SidecarEngine{
		spec: spec,
		drv:  sidecars.NewDriver(spec.Bin, cfg, log.With("engine", spec.ID)),
	}
}

// Driver 暴露底层驱动（Registry onEvict 收割回调用）。
func (e *SidecarEngine) Driver() *sidecars.Driver { return e.drv }

func (e *SidecarEngine) ID() string          { return e.spec.ID }
func (e *SidecarEngine) DisplayName() string { return e.spec.DisplayName }
func (e *SidecarEngine) Kind() Kind          { return e.spec.Kind }
func (e *SidecarEngine) Caps() Caps          { return e.spec.Caps }

// Available v1 判据：二进制可执行存在（模型文件级探测下期接入 catalog）。
func (e *SidecarEngine) Available(context.Context) (bool, string) {
	if _, err := exectree.LookPath(e.spec.Bin); err != nil {
		return false, "sidecar 可执行文件缺失：" + e.spec.Bin
	}
	return true, ""
}

// sidecarSynthesisRequest 下发给 sidecar 的 JSON 载荷（协议层契约）。
type sidecarSynthesisRequest struct {
	Text         string  `json:"text"`
	Instruct     string  `json:"instruct,omitempty"`
	Language     string  `json:"language,omitempty"`
	RefAudioPath string  `json:"ref_audio_path,omitempty"`
	Seconds      float64 `json:"seconds,omitempty"`
}

// sidecarSynthesisResult sidecar result 帧的 JSON 载荷（协议层契约）。
type sidecarSynthesisResult struct {
	DurationMs   int    `json:"duration_ms"`
	Samples      int    `json:"samples"`
	ArtifactHint string `json:"artifact_hint,omitempty"`
}

// Synthesize 经 sidecar 协议合成：
// 帧 chunk → sink（进度取最近一次 progress 帧）；帧 result → 结果。
// 错误映射：看门狗/协议/进程死 → 带归因包装；sidecar 业务错误透传 Code。
func (e *SidecarEngine) Synthesize(ctx context.Context, req SynthesisRequest, sink ChunkSink) (SynthesisResult, error) {
	var lastProgress float64
	onChunk := func(pcm []byte) {
		if sink != nil {
			// chunk 不带进度时沿用最近 progress；sink 错误经 done 通道回溯。
			_ = sink(pcm, lastProgress)
		}
	}
	evs := sidecars.Events{
		OnProgress: func(p float64) {
			if p >= 0 && p <= 1 {
				lastProgress = p
			}
		},
		OnChunk: onChunk,
		OnLog: func(line string) {
			slog.Default().Debug("sidecar line", "engine", e.spec.ID, "line", line)
		},
	}

	raw, err := e.drv.RunGeneration(ctx, sidecarSynthesisRequest{
		Text:         req.Text,
		Instruct:     req.Instruct,
		Language:     req.Language,
		RefAudioPath: req.RefAudioPath,
		Seconds:      req.Seconds,
	}, evs)
	if err != nil {
		var se *sidecars.SidecarError
		if errors.As(err, &se) {
			return SynthesisResult{}, fmt.Errorf("%w: %s", ErrSynthesisFailed, se.Error())
		}
		return SynthesisResult{}, err
	}

	var r sidecarSynthesisResult
	if len(raw) > 0 {
		if jerr := json.Unmarshal(raw, &r); jerr != nil {
			return SynthesisResult{}, fmt.Errorf("%w: result payload: %v", ErrSynthesisFailed, jerr)
		}
	}
	return SynthesisResult{
		DurationMs:   r.DurationMs,
		Samples:      r.Samples,
		ArtifactHint: r.ArtifactHint,
	}, nil
}
