package mme

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// ToolUsecase 音视频工具执行编排（渲染 worker 调它跑单变体全流程）。
// 流程：reframe+cut → 字幕烧录 → 封面抽帧 → QC → manifest.json。
// 失败保留中间产物（keep_intermediate_on_fail），如实落 error。
type ToolUsecase struct {
	cfg      Config
	tasks    TaskRepo
	sets     VariantSetRepo
	variants VariantRepo
	jobs     RenderJobRepo
	assets   AssetRepo
	storage  MediaStorage
	renderer Renderer
	prober   Prober
	qc       *QCChecker
	log      *log.Helper
}

// NewToolUsecase 构造。
func NewToolUsecase(
	cfg Config,
	tasks TaskRepo,
	sets VariantSetRepo,
	variants VariantRepo,
	jobs RenderJobRepo,
	assets AssetRepo,
	storage MediaStorage,
	renderer Renderer,
	prober Prober,
	qc *QCChecker,
	logger log.Logger,
) *ToolUsecase {
	return &ToolUsecase{cfg: cfg, tasks: tasks, sets: sets, variants: variants, jobs: jobs, assets: assets, storage: storage, renderer: renderer, prober: prober, qc: qc, log: log.NewHelper(log.With(logger, "module", "biz/mme/tool"))}
}

// edlSegment 从 Variant.edl_ref JSON 解出的切点。
type edlSegment struct {
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
}

// parseEDL 解析 EDL（宽松：空 → 全长）。
func parseEDL(edlRef string, fallbackDur float64) edlSegment {
	var seg edlSegment
	if edlRef == "" {
		seg.EndSec = fallbackDur
		return seg
	}
	if err := json.Unmarshal([]byte(edlRef), &seg); err != nil || seg.EndSec <= seg.StartSec {
		seg = edlSegment{EndSec: fallbackDur}
	}
	return seg
}

// RunRenderJob asynq mme.render.variant handler 入口。
// 可重入：worker 崩溃后 job 仍 running，重启时 ResetStuckJobs 回 pending 重跑。
func (u *ToolUsecase) RunRenderJob(ctx context.Context, jobID string) error {
	job, err := u.jobs.Get(ctx, jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return ErrJobNotFound
	}
	v, err := u.variants.Get(ctx, job.VariantID)
	if err != nil {
		return err
	}
	if v == nil {
		return ErrVariantNotFound
	}
	set, err := u.sets.Get(ctx, v.SetID)
	if err != nil {
		return err
	}
	if set == nil {
		return ErrSetNotFound
	}
	asset, err := u.assets.Get(ctx, set.AssetID)
	if err != nil {
		return err
	}
	if asset == nil || asset.LocalPath == "" {
		return u.failRender(ctx, job, v, fmt.Errorf("%w: set %s asset %s 无源文件", ErrAssetMissing, set.ID, set.AssetID))
	}
	if !u.renderer.Available(ctx) {
		return u.failRender(ctx, job, v, fmt.Errorf("%w: ffmpeg not found (path=%s); install ffmpeg 后重试", ErrRenderFail, u.cfg.FFmpegPath))
	}

	job.State = RenderRunning
	job.StartedAt = time.Now().UTC()
	job.WorkerID = "asynq-mme"
	job.Progress = 0
	if err = u.jobs.Update(ctx, job); err != nil {
		return err
	}
	v.RenderState = RenderRunning
	v.RenderError = ""
	if err = u.variants.Update(ctx, v); err != nil {
		return err
	}
	if set.State != SetRendering {
		set.State = SetRendering
		_ = u.sets.Update(ctx, set)
	}

	varDir := u.storage.VariantDir(v.ID)
	if mkdirErr := os.MkdirAll(varDir, 0o755); mkdirErr != nil {
		return u.failRender(ctx, job, v, fmt.Errorf("mkdir variant dir: %w", mkdirErr))
	}
	spec, ok := LookupAspect(v.Aspect)
	if !ok {
		return u.failRender(ctx, job, v, fmt.Errorf("%w: variant %s aspect %q 无法识别", ErrRenderFail, v.ID, v.Aspect))
	}
	seg := parseEDL(v.EDLRef, asset.DurationSec)

	// 字幕：源字幕 → 平移到切点窗口；无源字幕 → 从 SRT 缓存（transcript 级）尝试；
	// 都没有 → 不烧录（QC subtitle_present 走"无字幕规格"分支）。
	srtOut := ""
	srcSRT := ""
	if asset.SubtitlePath != "" {
		srcSRT = asset.SubtitlePath
	}
	if srcSRT != "" {
		if entries, perr := readSRTFile(srcSRT); perr == nil && len(entries) > 0 {
			shifted := ShiftSRT(entries, -secDur(seg.StartSec), secDur(seg.StartSec), secDur(seg.EndSec))
			if len(shifted) > 0 {
				srtOut = filepath.Join(varDir, "burn.srt")
				if werr := os.WriteFile(srtOut, []byte(WriteSRT(shifted)), 0o644); werr != nil {
					u.log.Warnf("shifted srt write failed: %v (render without burn-in)", werr)
					srtOut = ""
				}
			}
		} else if perr != nil {
			u.log.Warnf("source srt parse failed: %v (render without burn-in)", perr)
		}
	}

	res, rerr := u.renderer.Render(ctx, RenderSpec{
		VariantID:   v.ID,
		SrcPath:     asset.LocalPath,
		DstPath:     filepath.Join(varDir, "video.mp4"),
		CoverPath:   filepath.Join(varDir, "cover.jpg"),
		SrtPath:     srtOut,
		ManifestDst: filepath.Join(varDir, "manifest.json"),
		Aspect:      spec,
		StartSec:    seg.StartSec,
		EndSec:      seg.EndSec,
		Margin:      u.cfg.MarginOf(v.Aspect),
		Preset:      u.cfg.Preset,
		CRF:         u.cfg.CRF,
	}, func(p float64) {
		updateJobProgress(ctx, u.jobs, job, p)
	})
	if rerr != nil {
		return u.failRender(ctx, job, v, rerr)
	}

	// ---- QC ----
	expectDur := seg.EndSec - seg.StartSec
	report := u.qc.CheckVariant(ctx, res.VideoPath, res.SrtPath, spec, expectDur)
	v.VideoPath = res.VideoPath
	v.CoverPath = res.CoverPath
	v.SrtPath = res.SrtPath
	v.DurationSec = res.DurationSec
	v.QCBoundaryOK = report.BoundaryOK
	v.QCSubtitleOK = report.SubtitleOK
	v.QCScore = report.Score

	// ---- manifest ----
	manifestPath := filepath.Join(varDir, "manifest.json")
	manifest := map[string]any{
		"variant_id":        v.ID,
		"set_id":            v.SetID,
		"content_version":   v.ContentVersion,
		"aspect":            v.Aspect,
		"hook_id":           v.HookID,
		"edl":               map[string]float64{"start_sec": seg.StartSec, "end_sec": seg.EndSec},
		"duration_sec":      res.DurationSec,
		"copy_degraded":     v.CopyDegraded,
		"ffmpeg_version":    u.renderer.Version(ctx),
		"reframe_method":    "crop_or_letterbox (无 GPU 智能重构)", // 🔴 诚实声明
		"caption_style":     v.CaptionStyle,
		"qc":                report,
		"rendered_at":       time.Now().UTC().Format(time.RFC3339),
	}
	if raw, werr := json.MarshalIndent(manifest, "", "  "); werr == nil {
		if os.WriteFile(manifestPath, raw, 0o644) == nil {
			v.ManifestPath = manifestPath
		} else {
			u.log.Warnf("manifest write failed: %s", manifestPath)
		}
	}

	// QC 不过 ≠ 渲染失败：产物落盘、qc=false 落库，RenderState 仍 done，
	// 🔴 门禁在 Review 批准时拦截（契约 §4），不在此处吞掉。
	v.RenderState = RenderDone
	if err = u.variants.Update(ctx, v); err != nil {
		return err
	}
	job.State = RenderDone
	job.Progress = 1
	job.FinishedAt = time.Now().UTC()
	if err = u.jobs.Update(ctx, job); err != nil {
		return err
	}
	u.maybeAdvanceSet(ctx, set.ID)
	u.log.Infof("variant %s rendered ok (qc boundary=%v subtitle=%v score=%.2f)", v.ID, report.BoundaryOK, report.SubtitleOK, report.Score)
	return nil
}

// failRender 统一渲染失败处理：保留中间产物 + 落可读错误。
func (u *ToolUsecase) failRender(ctx context.Context, job *RenderJob, v *Variant, cause error) error {
	job.State = RenderFailed
	job.ErrorCode = codeOf(cause)
	job.ErrorMsg = cause.Error()
	job.FinishedAt = time.Now().UTC()
	if !u.cfg.KeepIntermediateOnFail {
		job.IntermediatePath = "" // 尊重配置；默认 true 保留现场
	}
	_ = u.jobs.Update(ctx, job)
	v.RenderState = RenderFailed
	v.RenderError = cause.Error()
	_ = u.variants.Update(ctx, v)
	u.log.Errorf("render job %s variant %s failed: %v", job.ID, v.ID, cause)
	return nil // asynq 不应重复投递——失败已落库，重试走显式 Retry
}

// maybeAdvanceSet 全部 job 终态后把簇推进到 review（可人工审核的最早时机）。
func (u *ToolUsecase) maybeAdvanceSet(ctx context.Context, setID string) {
	jobs, err := u.jobs.ListBySet(ctx, setID)
	if err != nil || len(jobs) == 0 {
		return
	}
	for _, j := range jobs {
		if j.State == RenderPending || j.State == RenderRunning {
			return
		}
	}
	set, err := u.sets.Get(ctx, setID)
	if err != nil || set == nil {
		return
	}
	if set.State == SetRendering {
		set.State = SetReview
		if err = u.sets.Update(ctx, set); err != nil {
			u.log.Warnf("advance set %s to review failed: %v", setID, err)
		}
	}
}

// updateJobProgress 进度回写（0~1 钳制；失败静默——不让进度错误打死渲染）。
func updateJobProgress(ctx context.Context, jobs RenderJobRepo, job *RenderJob, p float64) {
	p = clamp01(p)
	if p < job.Progress+0.02 && p < 1 { // 限频：>2% 才写库
		return
	}
	job.Progress = p
	_ = jobs.Update(ctx, job)
}

// codeOf 从包装错误里找最近的哨兵错误码。
func codeOf(err error) string {
	for _, e := range []error{ErrRenderFail, ErrAssetMissing, ErrQCFailed, ErrStorageFull} {
		if isSentinel(err, e) {
			return e.Error()
		}
	}
	return ErrRenderFail.Error()
}

// isSentinel errors.Is 的薄封装（哨兵错误比较）。
func isSentinel(err, sentinel error) bool {
	for err != nil {
		if err == sentinel {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

func secDur(v float64) time.Duration { return time.Duration(v * float64(time.Second)) }

func readSRTFile(path string) ([]SRTEntry, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseSRT(string(raw))
}
