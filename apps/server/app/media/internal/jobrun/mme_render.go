// mme.render：MME 变体渲染的进程执行面。
//
// 语义与 admin 本地 FFmpegRenderer 严格对齐（委托模式下的行为等价性）：
//   - 主命令一条过：切（-ss/-to）+ reframe（aspect.filter）+ 烧录（subtitles
//     force_style，MarginV=安全区底部）；libx264/preset/crf/yuv420p/faststart；
//   - 封面在 EDL 1/3 处抽帧，失败不判整变体失败（cover_path 置空）；
//   - 输出时长 < 预期一半 → 判失败（源缺段防御）；
//   - intro/outro 未接线：如实记 warn 继续（与 admin 现状一致，不静默也不报错）；
//   - manifest 不落盘：manifest 由 admin biz 在 QC 后撰写（tool_usecase.go），
//     本地渲染器同样不写；本 handler 忽略 manifest_dst 字段。
//
// 差异点（仅观测面更强，不改变产物）：进度经 -progress pipe:2 实时上抛
// （本地实现是完成后一次性 0.7→1.0；委托模式下 biz 侧收到平滑进度）。
package jobrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"framework/app/media/internal/jobstore"
	"framework/pkg/exectree"
	"framework/pkg/mmesub"
)

// mmeRenderPayload mme.render 的载荷契约（镜像 admin biz/mme.RenderSpec）。
type mmeRenderPayload struct {
	VariantID   string `json:"variant_id,omitempty"`
	SrcPath     string `json:"src_path"`
	DstPath     string `json:"dst_path"`
	CoverPath   string `json:"cover_path,omitempty"`
	SrtPath     string `json:"srt_path,omitempty"`
	ManifestDst string `json:"manifest_dst,omitempty"` // biz 层自有；本 handler 忽略（见包注释）
	Aspect      struct {
		Key    string `json:"key,omitempty"`
		Width  int    `json:"width,omitempty"`
		Height int    `json:"height,omitempty"`
		Filter string `json:"filter"`
	} `json:"aspect"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Margin   struct {
		Bottom int `json:"bottom,omitempty"`
		Top    int `json:"top,omitempty"`
	} `json:"margin,omitempty"`
	IntroPath string `json:"intro_path,omitempty"`
	OutroPath string `json:"outro_path,omitempty"`
	Preset    string `json:"preset,omitempty"`
	CRF       int    `json:"crf"`
}

// normalize 校验前的去歧义规整：
//   - M4 修复：CRF=0 在 x264 是无损（~10x 码率），并非「沿用默认」。调用方省略
//     crf 字段时 JSON 反序列化得 0 恰好穿过范围检查，300s 1080p 中间产物从几十
//     MB 变数 GB，与 mme.yaml 默认 23 静默不一致——0 一律回填默认 23（与 admin
//     本地渲染器对齐）；
//   - Preset 缺省 medium。
//
// 注意：CRF<0/>51 不在此遮盖，交回调用点的范围检查判 BAD_SPEC。
func (p *mmeRenderPayload) normalize() {
	if p.CRF == 0 {
		p.CRF = 23
	}
	if p.Preset == "" {
		p.Preset = "medium"
	}
}

// MMERenderDeps mme.render handler 的启动期依赖。
type MMERenderDeps struct {
	FFmpegPath string       // 已解析路径或 "ffmpeg"（LookPath 判存）
	Log        *slog.Logger // 可空
}

// 渲染错误公开码（classifyError 经 ErrorCode() 透出到 job.error_code）。
var (
	errRenderBadSpec  = errors.New("invalid render spec")
	errRenderNoFFmpeg = errors.New("ffmpeg not available")
	errRenderFailed   = errors.New("ffmpeg render failed")
)

// mmeCodedError 携稳定公开码的 handler 错误。
type mmeCodedError struct {
	code string
	err  error
}

func (e *mmeCodedError) Error() string     { return e.err.Error() }
func (e *mmeCodedError) Unwrap() error     { return e.err }
func (e *mmeCodedError) ErrorCode() string { return e.code }

func mmeErr(code string, sentinel error, format string, a ...any) error {
	return &mmeCodedError{code: code, err: fmt.Errorf("%s: %w", fmt.Sprintf(format, a...), sentinel)}
}

// outTimeMsRe "-progress pipe:2" 的 out_time_ms=xxx 行（单位实为微秒）。
var outTimeMsRe = regexp.MustCompile(`^out_time_ms=(\d+)$`)

// subtitleForceStyle 烧录样式（M5 修复：唯一真相迁往 pkg/mmesub，本常量仅作
// 调用点薄壳——样式串任何变更在 admin/media 双执行面编译期同步，不再手抄漂移）。
func subtitleForceStyle(marginV int) string { return mmesub.ForceStyle(marginV) }

// MMERenderHandler 注册类型 mme.render。
func MMERenderHandler(dep MMERenderDeps) Handler {
	if dep.Log == nil {
		dep.Log = slog.Default()
	}
	if dep.FFmpegPath == "" {
		dep.FFmpegPath = "ffmpeg"
	}
	return func(ctx context.Context, job jobstore.Job, payload json.RawMessage, emit func(progress float64)) (json.RawMessage, error) {
		var p mmeRenderPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, mmeErr("MEDIA_RENDER_BAD_SPEC", errRenderBadSpec, "payload unmarshal: %v", err)
		}
		if strings.TrimSpace(p.SrcPath) == "" || strings.TrimSpace(p.DstPath) == "" {
			return nil, mmeErr("MEDIA_RENDER_BAD_SPEC", errRenderBadSpec, "src_path/dst_path required")
		}
		if strings.TrimSpace(p.Aspect.Filter) == "" {
			return nil, mmeErr("MEDIA_RENDER_BAD_SPEC", errRenderBadSpec, "aspect.filter required（画幅数学由编排侧给出）")
		}
		dur := p.EndSec - p.StartSec
		if dur <= 0 {
			return nil, mmeErr("MEDIA_RENDER_BAD_SPEC", errRenderBadSpec, "invalid segment [%.3f,%.3f)", p.StartSec, p.EndSec)
		}
		p.normalize() // CRF=0 回填 / preset 缺省（语义注释在方法上）
		if p.CRF < 0 || p.CRF > 51 {
			return nil, mmeErr("MEDIA_RENDER_BAD_SPEC", errRenderBadSpec, "crf %d out of [0,51]", p.CRF)
		}
		if _, err := exectree.LookPath(dep.FFmpegPath); err != nil {
			return nil, mmeErr("MEDIA_RENDER_NO_FFMPEG", errRenderNoFFmpeg,
				"path=%s；设置 MEDIA_FFMPEG_PATH 或安装 ffmpeg 后重试", dep.FFmpegPath)
		}
		if p.IntroPath != "" || p.OutroPath != "" {
			// 与 admin FFmpegRenderer 同一语义：模板片头片尾尚未接线，切+降级直出。
			dep.Log.Warn("mme.render: intro/outro not wired (cut-only, parity with local renderer)",
				"job", job.ID, "variant", p.VariantID)
		}
		if err := os.MkdirAll(filepath.Dir(p.DstPath), 0o755); err != nil {
			return nil, mmeErr("MEDIA_RENDER_FAIL", errRenderFailed, "mkdir dst dir: %v", err)
		}

		// ---- 1) 切 + reframe(+ 烧录) → video ----
		vf := p.Aspect.Filter
		if p.SrtPath != "" {
			vf = fmt.Sprintf("%s,subtitles='%s':force_style='%s'",
				vf, escapeSubFilterPath(p.SrtPath), subtitleForceStyle(p.Margin.Bottom))
		}
		args := []string{
			"-hide_banner", "-nostats",
			"-progress", "pipe:2", // 实时进度走 stderr，Spawn 逐行回调
			"-ss", strconv.FormatFloat(p.StartSec, 'f', 3, 64),
			"-to", strconv.FormatFloat(p.EndSec, 'f', 3, 64),
			"-i", p.SrcPath,
			"-vf", vf,
			"-c:v", "libx264", "-preset", p.Preset, "-crf", strconv.Itoa(p.CRF),
			"-pix_fmt", "yuv420p", // 兼容性（部分播放器不认 yuv444）
			"-movflags", "+faststart",
			"-map", "0:v:0", "-map", "0:a?", "-c:a", "aac",
			"-map_metadata", "-1", "-y", p.DstPath,
		}

		// 看门狗与 admin 同纪律：预期时长 ×20，下限 10 分钟。
		watchDur := time.Duration(dur*20) * time.Second
		if watchDur < 10*time.Minute {
			watchDur = 10 * time.Minute
		}
		runCtx, stopWatch := context.WithTimeout(ctx, watchDur)
		defer stopWatch()

		// stderr 消费：进度行 → emit；其余行入尾环（报错携带）。
		var (
			tailMu   sync.Mutex
			tail     []string
			lastOutS float64
		)
		stderrLine := func(line string) {
			if m := outTimeMsRe.FindStringSubmatch(line); m != nil {
				if us, err := strconv.ParseInt(m[1], 10, 64); err == nil {
					lastOutS = float64(us) / 1e6
					// 主渲染占 0..0.7（与 admin 的 0.7 节点对齐）。
					if frac := lastOutS / dur; frac > 0 {
						if frac > 1 {
							frac = 1
						}
						emit(frac * 0.7)
					}
				}
				return
			}
			tailMu.Lock()
			if len(tail) >= 40 {
				tail = tail[1:]
			}
			tail = append(tail, line)
			tailMu.Unlock()
		}

		pipes, err := exectree.Spawn(runCtx, exectree.RunSpec{
			Bin:  dep.FFmpegPath,
			Args: args,
			Dir:  filepath.Dir(p.DstPath),
		}, stderrLine)
		if err != nil {
			return nil, mmeErr("MEDIA_RENDER_FAIL", errRenderFailed, "spawn ffmpeg: %v", err)
		}
		waitErr := pipes.Wait()
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err() // 取消：执行器归因 MEDIA_JOB_CANCELLED（树杀由 exectree 完成）
		case runCtx.Err() == context.DeadlineExceeded:
			return nil, mmeErr("MEDIA_RENDER_FAIL", errRenderFailed,
				"watchdog %s exceeded (dur=%.1fs)", watchDur, dur)
		case waitErr != nil:
			tailMu.Lock()
			t := strings.Join(tail, "\n")
			tailMu.Unlock()
			return nil, mmeErr("MEDIA_RENDER_FAIL", errRenderFailed,
				"ffmpeg exit: %v | stderr tail: %s", waitErr, t)
		}

		// 防御：输出异常短 → 源缺段，判失败（与 admin 同阈值）。
		if lastOutS > 0 && lastOutS < dur*0.5 {
			return nil, mmeErr("MEDIA_RENDER_FAIL", errRenderFailed,
				"output %.1fs < half of expected %.1fs (source may lack segment)", lastOutS, dur)
		}
		emit(0.7)

		// ---- 2) 封面抽帧（EDL 1/3 处，防黑帧开头；失败不判整变体失败） ----
		coverOut := p.CoverPath
		if coverOut != "" {
			_ = os.MkdirAll(filepath.Dir(coverOut), 0o755)
			coverRes := exectree.Run(ctx, exectree.RunSpec{
				Bin: dep.FFmpegPath,
				Args: []string{
					"-hide_banner",
					"-ss", strconv.FormatFloat(p.StartSec+dur*0.33, 'f', 3, 64),
					"-i", p.SrcPath,
					"-vf", p.Aspect.Filter,
					"-frames:v", "1", "-q:v", "3",
					"-y", coverOut,
				},
				Timeout: watchDur / 2,
			})
			if !coverRes.Success() {
				dep.Log.Warn("mme.render: cover extract failed (variant keeps empty cover)",
					"job", job.ID, "variant", p.VariantID, "attribution", coverRes.Attribution)
				coverOut = ""
			} else if fi, statErr := os.Stat(coverOut); statErr != nil || fi.Size() == 0 {
				coverOut = ""
			}
		}
		emit(1)

		durationOut := dur
		if lastOutS > 0 {
			durationOut = lastOutS
		}
		return json.Marshal(map[string]any{
			"video_path":   p.DstPath,
			"cover_path":   coverOut,
			"srt_path":     p.SrtPath,
			"duration_sec": durationOut,
		})
	}
}

// escapeSubFilterPath subtitles 滤镜路径转义（M5 修复：唯一真相迁往 pkg/mmesub，
// 本函数仅作调用点薄壳）。
func escapeSubFilterPath(p string) string { return mmesub.EscapeFilterPath(p) }
