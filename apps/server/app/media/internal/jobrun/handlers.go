package jobrun

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"framework/app/media/internal/engines"
	"framework/app/media/internal/engines/echosynth"
	"framework/pkg/exectree"
	"framework/app/media/internal/jobstore"
)

// ---------------------------------------------------------------- echo.synthesize

// echoSynthesizePayload echo.synthesize 的载荷契约。
type echoSynthesizePayload struct {
	Text         string  `json:"text"`
	Instruct     string  `json:"instruct,omitempty"`
	Language     string  `json:"language,omitempty"`
	RefAudioPath string  `json:"ref_audio_path,omitempty"`
	Seconds      float64 `json:"seconds,omitempty"`
	EngineID     string  `json:"engine_id,omitempty"` // 显式指定引擎；空走激活/默认
}

// EchoDeps echo.synthesize handler 的启动期依赖。
type EchoDeps struct {
	Registry      *engines.Registry
	DefaultEngine string
	ArtifactDir   string
}

// 产物落盘边界纪律：单文件大小上限（30s × 22050Hz × 2B ≈ 1.3MB，放宽到 64MB）。
const maxArtifactBytes = 64 << 20

// EchoSynthesizeHandler 注册类型 echo.synthesize：
// 解析载荷 → 引擎路由门禁 → 分块合成（sink 累积 PCM + progress 事件）→
// WAV 封装落盘 → result_json（artifact 相对路径，下载走 artifacts 裸端点）。
func EchoSynthesizeHandler(dep EchoDeps) Handler {
	return func(ctx context.Context, job jobstore.Job, payload json.RawMessage, emit func(progress float64)) (json.RawMessage, error) {
		var p echoSynthesizePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("payload unmarshal: %w", err)
		}
		if strings.TrimSpace(p.Text) == "" && p.Seconds <= 0 {
			return nil, errors.New("text is required (or explicit seconds)")
		}

		// 引擎选择：显式 id 或当前激活/默认；routing 门禁在此落闸。
		engineID := p.EngineID
		var (
			eng engines.Engine
			rt  engines.Routing
			err error
		)
		if engineID != "" {
			eng, rt, err = pickEngine(dep.Registry, engineID)
		} else {
			eng, rt, err = dep.Registry.Active(dep.DefaultEngine)
		}
		if err != nil {
			return nil, engines.WithPublicCode(err)
		}

		var buf bytes.Buffer
		buf.Grow(512 << 10)
		sink := func(pcm []byte, progress float64) error {
			if buf.Len()+len(pcm) > maxArtifactBytes {
				return errors.New("artifact size cap exceeded")
			}
			buf.Write(pcm)
			emit(progress)
			return nil
		}

		res, err := eng.Synthesize(ctx, engines.SynthesisRequest{
			Text:         p.Text,
			Instruct:     p.Instruct,
			Language:     p.Language,
			RefAudioPath: p.RefAudioPath,
			Seconds:      p.Seconds,
		}, sink)
		if err != nil {
			return nil, engines.WithPublicCode(err)
		}

		// WAV 封装 + 落盘。
		pcm := make([]int16, buf.Len()/2)
		for i := range pcm {
			pcm[i] = int16(buf.Bytes()[i*2]) | int16(buf.Bytes()[i*2+1])<<8
		}
		wav := echosynth.EncodeWAV(pcm)

		name := res.ArtifactHint
		if name == "" {
			name = "speech.wav"
		}
		rel, err := writeArtifact(dep.ArtifactDir, job.ID, name, wav)
		if err != nil {
			return nil, err
		}
		emit(1)

		return json.Marshal(map[string]any{
			"artifact":    rel,
			"duration_ms": res.DurationMs,
			"samples":     res.Samples,
			"routing":     string(rt),
			"engine":      eng.ID(),
		})
	}
}

// pickEngine 显式 id 选择 + 路由门禁（给 handler 内部用；Registry 自身
// 也做 Select 校验，这里是"只取不改激活"的轻量路径）。
func pickEngine(reg *engines.Registry, id string) (engines.Engine, engines.Routing, error) {
	// resolve + 路由判定：借 Active 语义不合适（会影响全局激活），
	// 直接走 List 视图拿 routing，再 resolve 实例。
	for _, v := range reg.List(context.Background()) {
		if v.ID == id {
			if err := engines.GateError(v.Routing); err != nil {
				return nil, v.Routing, err
			}
			eng, err := reg.EngineFor(id)
			return eng, v.Routing, err
		}
	}
	return nil, "", engines.ErrEngineNotFound
}

// writeArtifact 产物落盘：artifact_dir/<job_id>/<name>，返回相对路径
// （artifacts 裸端点以同规则回读；名称在此净化，防目录穿越）。
func writeArtifact(baseDir, jobID, name string, data []byte) (string, error) {
	clean := filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if clean == "." || clean == "/" || clean == "" {
		return "", errors.New("invalid artifact name")
	}
	dir := filepath.Join(baseDir, jobID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("artifact dir: %w", err)
	}
	full := filepath.Join(dir, clean)
	if err := os.WriteFile(full, data, 0o644); err != nil {
		return "", fmt.Errorf("artifact write: %w", err)
	}
	return clean, nil
}

// ---------------------------------------------------------------- exec.probe

// probePayload exec.probe 的载荷契约：受控跑一次外部可执行文件，
// 验证 exectree 进程容器链路（杀树/超时/脱敏/归因）。
type probePayload struct {
	Bin      string   `json:"bin"`
	Args     []string `json:"args,omitempty"`
	TimeoutS int      `json:"timeout_s,omitempty"` // 3–120s，默认 10s
}

// ExecProbeHandler 注册类型 exec.probe（诊断链路占位；生产 handler
// 在 MME 开工时以同模式注册进本执行器）。
func ExecProbeHandler() Handler {
	return func(ctx context.Context, job jobstore.Job, payload json.RawMessage, emit func(progress float64)) (json.RawMessage, error) {
		var p probePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("payload unmarshal: %w", err)
		}
		if p.Bin == "" {
			return nil, errors.New("bin is required")
		}
		if p.TimeoutS <= 0 {
			p.TimeoutS = 10
		}
		if p.TimeoutS > 120 {
			p.TimeoutS = 120
		}
		if p.TimeoutS < 3 {
			p.TimeoutS = 3
		}

		emit(0.1)
		res := exectree.Run(ctx, exectree.RunSpec{
			Bin:     p.Bin,
			Args:    p.Args,
			Timeout: time.Duration(p.TimeoutS) * time.Second,
		})
		emit(1)

		return json.Marshal(map[string]any{
			"exit_code":   res.ExitCode,
			"timed_out":   res.TimedOut,
			"cancelled":   res.Cancelled,
			"duration_ms": res.Duration.Milliseconds(),
			"attribution": res.Attribution,
			"stdout_tail": res.StdoutTail,
			"stderr_tail": res.StderrTail,
		})
	}
}
