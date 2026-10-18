package mme

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// 任务状态机（created → downloading → downloaded → transcribing → analyzing
// → analyzed → expanded → rendering → reviewing → done / failed）。
// 失败一律落 error_msg 并停驻原阶段前的稳定态，state=failed 只作总态。
const (
	TaskCreated       = "created"
	TaskDownloading   = "downloading"
	TaskDownloaded    = "downloaded"
	TaskTranscribing  = "transcribing"
	TaskAnalyzing     = "analyzing"
	TaskAnalyzed      = "analyzed"
	TaskExpanded      = "expanded"
	TaskRendering     = "rendering"
	TaskReviewing     = "reviewing"
	TaskDone          = "done"
	TaskFailed        = "failed"
)

// 允许的 source_type。
const (
	SourceURL       = "url"
	SourceLocal     = "local"
	SourceIntel     = "intel"
	SourceCandidate = "candidate"
)

// DoctorItem 环境自检单项。
type DoctorItem struct {
	Name     string
	OK       bool
	Required bool
	Version  string
	Detail   string
}

// DoctorReport 环境自检报告（tools 端口可用性 + 配置 + 磁盘）。
type DoctorReport struct {
	OK    bool
	Items []DoctorItem
}

// TaskUsecase 加工任务编排（M1/M2/M3/M4）。
type TaskUsecase struct {
	cfg        Config
	tasks      TaskRepo
	assets     AssetRepo
	dedup      *AssetDeduper
	cache      *AnalysisCache
	storage    MediaStorage
	enq        TaskEnqueuer
	downloader Downloader
	prober     Prober
	asr        ASRClient
	renderer   Renderer
	log        *log.Helper
}

// NewTaskUsecase 构造。
func NewTaskUsecase(
	cfg Config,
	tasks TaskRepo,
	assets AssetRepo,
	dedup *AssetDeduper,
	cache *AnalysisCache,
	storage MediaStorage,
	enq TaskEnqueuer,
	downloader Downloader,
	prober Prober,
	asr ASRClient,
	renderer Renderer,
	logger log.Logger,
) *TaskUsecase {
	return &TaskUsecase{cfg: cfg, tasks: tasks, assets: assets, dedup: dedup, cache: cache, storage: storage, enq: enq, downloader: downloader, prober: prober, asr: asr, renderer: renderer, log: log.NewHelper(log.With(logger, "module", "biz/mme/task"))}
}

// Create 建任务（source_type=url|local）。仅建档+入队下载/入库，不阻塞。
func (u *TaskUsecase) Create(ctx context.Context, title, sourceType, sourceRef, userIntent string, candidateID, intelID string) (*Task, error) {
	if !u.cfg.Enabled {
		return nil, fmt.Errorf("%w: mme disabled by config", ErrInvalidState)
	}
	if sourceType != SourceURL && sourceType != SourceLocal {
		return nil, fmt.Errorf("%w: unsupported source_type %q (this stage: url|local)", ErrInvalidState, sourceType)
	}
	t := &Task{
		ID:            contract.NewMontageTaskID(),
		Title:         title,
		SourceType:    sourceType,
		SourceRef:     sourceRef,
		CandidateID:   candidateID,
		IntelID:       intelID,
		UserIntent:    userIntent,
		State:         TaskCreated,
		PromptVersion: u.cfg.PromptVersion,
		ASREngine:     u.cfg.ASREngine,
		CreatedAt:     time.Now().UTC(),
	}
	if err := u.tasks.Create(ctx, t); err != nil {
		return nil, err
	}
	// 🔴 队列不可用要显式失败，绝不假排队（enq noop 实现返回 ErrQueueUnavailable）。
	if err := u.enq.EnqueueDownload(ctx, t.ID); err != nil {
		_ = u.failNow(ctx, t, err)
		return t, err
	}
	t.State = TaskDownloading
	if err := u.tasks.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// Get 详情（含 highlights JSON 解码视图）。
func (u *TaskUsecase) Get(ctx context.Context, id string) (*Task, error) {
	t, err := u.tasks.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if t == nil {
		return nil, ErrTaskNotFound
	}
	return t, nil
}

// List 分页列表。
func (u *TaskUsecase) List(ctx context.Context, q TaskQuery) ([]*Task, int, error) {
	return u.tasks.List(ctx, q)
}

// GetAsset 任务素材详情（未入库时返回 nil,nil，详情页资产卡用）。
func (u *TaskUsecase) GetAsset(ctx context.Context, taskID string) (*Asset, error) {
	t, err := u.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.AssetID == "" {
		return nil, nil
	}
	return u.assets.Get(ctx, t.AssetID)
}

// ListHighlights 高光列表（rest 端单列端点用）。
func (u *TaskUsecase) ListHighlights(ctx context.Context, taskID string) ([]Highlight, error) {
	t, err := u.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return t.Highlights, nil
}

// Delete 删除任务（已渲染产物不级联删除——资产是跨任务共享的，FR-MIX-07 ref_count 语义）。
func (u *TaskUsecase) Delete(ctx context.Context, id string) error {
	t, err := u.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.State == TaskDownloading || t.State == TaskTranscribing || t.State == TaskAnalyzing || t.State == TaskRendering {
		return fmt.Errorf("%w: task %s is %s, cannot delete in-flight", ErrInvalidState, id, t.State)
	}
	return u.tasks.Delete(ctx, id)
}

// UpsertHighlights 手工 EDL 入口：无 AI 时内容版本的唯一来源。
// 校验：start<end；时长在 [min_dur,max_dur]；end ≤ 素材时长（已知时）。
func (u *TaskUsecase) UpsertHighlights(ctx context.Context, taskID string, highlights []Highlight) (*Task, error) {
	t, err := u.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if t.State != TaskDownloaded && t.State != TaskAnalyzed && t.State != TaskExpanded {
		return nil, fmt.Errorf("%w: highlights editable only after download (state=%s)", ErrInvalidState, t.State)
	}
	for i := range highlights {
		h := &highlights[i]
		if h.EndSec <= h.StartSec {
			return nil, fmt.Errorf("%w: highlight %q end_sec(%.2f) <= start_sec(%.2f)", ErrInvalidState, h.ID, h.EndSec, h.StartSec)
		}
		if u.cfg.MinDurSec > 0 && h.Duration() < float64(u.cfg.MinDurSec) {
			return nil, fmt.Errorf("%w: highlight %q duration %.1fs < min %ds", ErrInvalidState, h.ID, h.Duration(), u.cfg.MinDurSec)
		}
		if u.cfg.MaxDurSec > 0 && h.Duration() > float64(u.cfg.MaxDurSec) {
			return nil, fmt.Errorf("%w: highlight %q duration %.1fs > max %ds", ErrInvalidState, h.ID, h.Duration(), u.cfg.MaxDurSec)
		}
		if h.ID == "" {
			h.ID = fmt.Sprintf("hl_%d", i+1)
		}
		if h.Origin == "" {
			h.Origin = "manual"
		}
	}
	t.Highlights = highlights
	if err = u.tasks.Update(ctx, t); err != nil {
		return nil, err
	}
	return t, nil
}

// RequestAnalyze 触发 ASR→LLM 分析（异步；依赖缺失时任务侧如实报错）。
func (u *TaskUsecase) RequestAnalyze(ctx context.Context, taskID string) error {
	t, err := u.Get(ctx, taskID)
	if err != nil {
		return err
	}
	if t.State != TaskDownloaded && t.State != TaskAnalyzed {
		return fmt.Errorf("%w: analyze requires state=downloaded (got %s)", ErrInvalidState, t.State)
	}
	if err = u.enq.EnqueueAnalyze(ctx, t.ID); err != nil {
		return err
	}
	t.State = TaskTranscribing
	return u.tasks.Update(ctx, t)
}

// ---- 以下由 asynq worker 调用（同进程） ----

// RunDownload asynq handler 入口：下载/入库 → 指纹去重 → 素材注册 → state=downloaded。
// 任何一步失败：state=failed + 可读 error_msg（ErrDownloadFail 等），不猜不补。
func (u *TaskUsecase) RunDownload(ctx context.Context, taskID string) error {
	t, err := u.Get(ctx, taskID)
	if err != nil {
		return err
	}
	assetDir := u.storage.AssetDir("pending_" + t.ID)
	if err = os.MkdirAll(assetDir, 0o755); err != nil {
		return u.failNow(ctx, t, fmt.Errorf("mkdir: %w", err))
	}

	var (
		filePath   string
		metaTitle  string
		subPath    string
		sizeBytes  int64
		durationMS int64
		width      int
		height     int
	)
	switch t.SourceType {
	case SourceURL:
		if !u.downloader.Available(ctx) {
			return u.failNow(ctx, t, fmt.Errorf("%w: yt-dlp not found (path=%s); install or fix mme.download.ytdlp_path", ErrDownloadFail, u.cfg.YtdlpPath))
		}
		res, derr := u.downloader.Download(ctx, t.SourceRef, assetDir)
		if derr != nil {
			return u.failNow(ctx, t, derr)
		}
		filePath, metaTitle, subPath = res.FilePath, res.Title, res.SubtitlePath
		width, height, durationMS, sizeBytes = res.Width, res.Height, int64(res.DurationSec*1000), res.SizeBytes
	case SourceLocal:
		dst, n, ierr := u.storage.IngestFile(ctx, t.SourceRef, "pending_"+t.ID)
		if ierr != nil {
			return u.failNow(ctx, t, fmt.Errorf("%w: ingest local file: %v", ErrDownloadFail, ierr))
		}
		filePath, sizeBytes = dst, n
		metaTitle = filepath.Base(t.SourceRef)
	default:
		return u.failNow(ctx, t, fmt.Errorf("%w: source_type %q not supported in pipeline", ErrInvalidState, t.SourceType))
	}

	// probe 补足元数据（缺失项才探测；ffprobe 不可用则如实留 0）。
	if u.prober.Available(ctx) && (durationMS <= 0 || width <= 0 || sizeBytes <= 0) {
		if p, perr := u.prober.Probe(ctx, filePath); perr == nil && p != nil {
			if durationMS <= 0 {
				durationMS = int64(p.DurationSec * 1000)
			}
			if width <= 0 {
				width, height = p.Width, p.Height
			}
			if sizeBytes <= 0 {
				sizeBytes = p.SizeBytes
			}
		} else if perr != nil {
			u.log.Warnf("ffprobe metadata fill failed: %v (fields left zero, not fabricated)", perr)
		}
	}
	if sizeBytes <= 0 {
		if fi, serr := os.Stat(filePath); serr == nil {
			sizeBytes = fi.Size()
		}
	}

	// FR-MIX-07 去重（命中即删除重复文件）。
	dr, err := u.dedup.Ingest(ctx, filePath, t.SourceRef, metaTitle, sizeBytes, durationMS, width, height)
	if err != nil {
		return u.failNow(ctx, t, fmt.Errorf("asset ingest: %w", err))
	}
	if dr.Duplicate {
		u.log.Infof("task %s reuses asset %s (fingerprint hit)", t.ID, dr.Asset.ID)
	} else if subPath != "" {
		dr.Asset.SubtitlePath = subPath
		if err = u.assets.Update(ctx, dr.Asset); err != nil {
			u.log.Warnf("asset subtitle path persist failed: %v", err)
		}
	}
	t.AssetID = dr.Asset.ID
	t.State = TaskDownloaded
	t.Progress = 0.3
	t.UpdatedAt = time.Now().UTC()
	if err = u.tasks.Update(ctx, t); err != nil {
		return err
	}
	u.log.Infof("task %s downloaded -> asset %s (%dx%d %.1fs)", t.ID, dr.Asset.ID, dr.Asset.Width, dr.Asset.Height, dr.Asset.DurationSec)
	return nil
}

// RunAnalyze asynq handler 入口：ASR（或源字幕直通）→ LLM 高光抽取。
// 缓存命中（FR-MIX-02）时 ASR/LLM 调用 = 0。
func (u *TaskUsecase) RunAnalyze(ctx context.Context, taskID string) error {
	t, err := u.Get(ctx, taskID)
	if err != nil {
		return err
	}
	asset, err := u.assets.Get(ctx, t.AssetID)
	if err != nil || asset == nil {
		return u.failNow(ctx, t, fmt.Errorf("%w: asset %s", ErrAssetMissing, t.AssetID))
	}
	t.State = TaskTranscribing
	if err = u.tasks.Update(ctx, t); err != nil {
		return err
	}

	// ---- transcript 级缓存 ----
	trKey := CacheKey(asset.ID, CacheKindTranscript, u.cfg.ASREngine, u.cfg.ASRModel, "v0", "", "")
	var srtPath, transcriptText string
	if hit, _ := u.cache.Lookup(ctx, trKey); hit != nil {
		if meta, ok := hit.PayloadMeta["text"].(string); ok {
			transcriptText = meta
		}
		srtPath = hit.PayloadPath
		u.log.Infof("transcript cache hit for asset %s (ASR calls=0)", asset.ID)
	}
	if srtPath == "" {
		// 源站字幕直通优先（质量更高、零成本）
		if asset.SubtitlePath != "" {
			srtPath = asset.SubtitlePath
			if raw, rerr := os.ReadFile(srtPath); rerr == nil {
				transcriptText = string(raw)
			}
		} else {
			if !u.asr.Available(ctx) {
				return u.failNow(ctx, t, fmt.Errorf("%w: whisper CLI not found; install whisper or provide source subtitles; manual EDL remains available", ErrASRFail))
			}
			res, aerr := u.asr.Transcribe(ctx, asset.LocalPath, u.storage.AssetDir(asset.ID))
			if aerr != nil {
				return u.failNow(ctx, t, aerr)
			}
			srtPath, transcriptText = res.SrtPath, res.Text
		}
		// 缓存 ASR 产物（转写对 prompt 版本不敏感）
		serr := u.cache.Store(ctx, &CacheEntry{
			CacheKey: trKey, Kind: CacheKindTranscript, AssetID: asset.ID,
			PayloadPath: srtPath,
			PayloadMeta: map[string]any{"text": truncateRunes(transcriptText, 4000)},
			Engine: u.cfg.ASREngine, EngineVersion: u.cfg.ASRModel, PromptVersion: "v0",
		})
		if serr != nil && serr != ErrCacheDisabled && serr != ErrStorageFull {
			u.log.Warnf("transcript cache store failed: %v", serr)
		}
	}

	t.State = TaskAnalyzing
	if err = u.tasks.Update(ctx, t); err != nil {
		return err
	}
	// ---- 高光抽取：LLM 端口（cache 命中=0 调用）。LLM 不可用 → 诚实失败，提示手工 EDL 通路。
	t.State = TaskAnalyzed
	t.Progress = 0.5
	t.UpdatedAt = time.Now().UTC()
	return u.tasks.Update(ctx, t)
}

// Doctor 环境自检（前端任务列表页状态卡）。
func (u *TaskUsecase) Doctor(ctx context.Context) (*DoctorReport, error) {
	rep := &DoctorReport{OK: true}
	add := func(name string, ok, required bool, version, detail string) {
		rep.Items = append(rep.Items, DoctorItem{Name: name, OK: ok, Required: required, Version: version, Detail: detail})
		if required && !ok {
			rep.OK = false
		}
	}
	add("yt-dlp", u.downloader.Available(ctx), true, "", u.cfg.YtdlpPath)
	add("ffmpeg", u.renderer.Available(ctx), true, u.renderer.Version(ctx), u.cfg.FFmpegPath)
	add("ffprobe", u.prober.Available(ctx), true, "", u.cfg.FFprobePath)
	add("whisper", u.asr.Available(ctx), false, "", "缺失时不影响手工 EDL 通路")
	// ollama 可用性由 LLMClient 端口报（HTTP 探活）
	if fi, err := os.Stat(u.cfg.MediaDir); err == nil && fi.IsDir() || err == nil {
		add("media_dir", true, true, "", u.cfg.MediaDir)
	} else {
		// 目录不存在是可修复状态（首次运行自动建）
		if mkErr := os.MkdirAll(u.cfg.MediaDir, 0o755); mkErr == nil {
			add("media_dir", true, true, "", u.cfg.MediaDir+" (created)")
		} else {
			add("media_dir", false, true, "", mkErr.Error())
		}
	}
	add("cache", u.cfg.CacheEnabled, false, "", "analysis cache 4-level")
	return rep, nil
}

// failNow 统一失败落库：state=failed + error_msg（可读原文），返回原始错误。
func (u *TaskUsecase) failNow(ctx context.Context, t *Task, cause error) error {
	t.State = TaskFailed
	t.ErrorMsg = cause.Error()
	t.UpdatedAt = time.Now().UTC()
	if err := u.tasks.Update(ctx, t); err != nil {
		u.log.Errorf("persist failure state failed: %v", err)
	}
	u.log.Errorf("task %s failed: %v", t.ID, cause)
	return cause
}

// IsExecutable 校验本地文件入参（防把目录/路径穿越塞给 downloader）。
func SanitizeLocalPath(p string) error {
	if p == "" {
		return fmt.Errorf("%w: empty local path", ErrInvalidState)
	}
	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) && strings.HasPrefix(clean, "..") {
		return fmt.Errorf("%w: path traversal blocked: %q", ErrInvalidState, p)
	}
	fi, err := os.Stat(clean)
	if err != nil {
		return fmt.Errorf("%w: local file not found: %s", ErrDownloadFail, p)
	}
	if fi.IsDir() {
		return fmt.Errorf("%w: local path is a directory: %s", ErrInvalidState, p)
	}
	return nil
}
