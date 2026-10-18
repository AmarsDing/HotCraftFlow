package mme

import (
	"context"

	"framework/pkg/mediacontract"
)

// ============ 查询对象 ============

// TaskQuery 任务列表查询。
type TaskQuery struct {
	State    string // 空 = 全部
	Page     int
	PageSize int
}

// JobQuery 渲染任务列表查询。
type JobQuery struct {
	State    string // 空 = 全部
	Page     int
	PageSize int
}

// ============ 仓储端口（data 层实现，只暴露领域类型） ============

// TaskRepo 加工任务仓储。
type TaskRepo interface {
	Create(ctx context.Context, t *Task) error
	Update(ctx context.Context, t *Task) error
	Get(ctx context.Context, id string) (*Task, error)
	List(ctx context.Context, q TaskQuery) ([]*Task, int, error)
	Delete(ctx context.Context, id string) error
}

// AssetRepo 素材仓储（FR-MIX-07 指纹去重的载体）。
type AssetRepo interface {
	Create(ctx context.Context, a *Asset) error
	Update(ctx context.Context, a *Asset) error
	Get(ctx context.Context, id string) (*Asset, error)
	// GetByFingerprint 按内容指纹查已有素材（去重判定）。
	GetByFingerprint(ctx context.Context, fingerprint string) (*Asset, error)
	// IncrRefCount 命中去重时引用计数 +1（返回新值）。
	IncrRefCount(ctx context.Context, id string) (int, error)
}

// CacheRepo 4 级分析缓存仓储（FR-MIX-02，详细设计 §2.2）。
type CacheRepo interface {
	// Get 按 cache_key 取条目；未命中/过期返回 nil,nil。
	Get(ctx context.Context, key string) (*CacheEntry, error)
	// Upsert 幂等写入（同 key 覆盖）。
	Upsert(ctx context.Context, e *CacheEntry) error
	// IncrHit 命中计数 +1（观测：验证缓存复用率）。
	IncrHit(ctx context.Context, key string) error
	// TotalBytes 当前缓存总字节（超限治理用）。
	TotalBytes(ctx context.Context) (int64, error)
}

// VariantSetRepo 变体簇仓储。
type VariantSetRepo interface {
	Create(ctx context.Context, s *VariantSet) error
	Update(ctx context.Context, s *VariantSet) error
	Get(ctx context.Context, id string) (*VariantSet, error)
	GetByTask(ctx context.Context, taskID string) (*VariantSet, error)
}

// VariantRepo 单变体仓储。
type VariantRepo interface {
	// CreateBatch 批量插入展开结果（一次事务）。
	CreateBatch(ctx context.Context, vs []*Variant) error
	Update(ctx context.Context, v *Variant) error
	Get(ctx context.Context, id string) (*Variant, error)
	// ListBySet 取簇内全部变体（矩阵网格/审核台）。
	ListBySet(ctx context.Context, setID string) ([]*Variant, error)
	// UpdateSelection 批量更新 selected（删减语义）。
	UpdateSelection(ctx context.Context, ids []string, selected bool) error
	// DeleteUnrenderedBySet 重建展开前清除旧行：仅删除 render_state=pending/failed 的变体
	// （done/running 变体已在上层被拒绝重建，双保险）。
	DeleteUnrenderedBySet(ctx context.Context, setID string) error
	// CountBySet 统计（总量/选中量）。
	CountBySet(ctx context.Context, setID string) (total int, selected int, err error)
}

// RenderJobRepo 渲染任务仓储。
type RenderJobRepo interface {
	Create(ctx context.Context, j *RenderJob) error
	Update(ctx context.Context, j *RenderJob) error
	Get(ctx context.Context, id string) (*RenderJob, error)
	GetByVariant(ctx context.Context, variantID string) (*RenderJob, error)
	List(ctx context.Context, q JobQuery) ([]*RenderJob, int, error)
	// ListBySet 取簇内全部 job（进度聚合用）。
	ListBySet(ctx context.Context, setID string) ([]*RenderJob, error)
}

// TemplateRepo 混剪模板仓储（FR-MIX-08）。
type TemplateRepo interface {
	Create(ctx context.Context, t *Template) error
	List(ctx context.Context, enabledOnly bool) ([]*Template, error)
	Get(ctx context.Context, id string) (*Template, error)
	// Count 模板数（启动种子判定：0 时写默认 3 套）。
	Count(ctx context.Context) (int, error)
}

// ============ 外部工具端口（data 层 CLI/HTTP 适配器实现） ============

// DownloadResult 下载产物（元数据尽量取全；取不到的字段为 0 值，不猜）。
type DownloadResult struct {
	FilePath     string
	Title        string
	DurationSec  float64
	Width        int
	Height       int
	SizeBytes    int64
	SubtitlePath string // 源站自带字幕（优先于 ASR）
}

// Downloader yt-dlp 端口（FR-MIX M1/M2）。
type Downloader interface {
	// Available yt-dlp 可执行（doctor 用）。
	Available(ctx context.Context) bool
	Download(ctx context.Context, url, destDir string) (*DownloadResult, error)
}

// ProbeResult ffprobe 探测结果。
type ProbeResult struct {
	DurationSec float64
	Width       int
	Height      int
	SizeBytes   int64
}

// Prober ffprobe 端口（QC 边界自检依赖）。
type Prober interface {
	Available(ctx context.Context) bool
	Probe(ctx context.Context, path string) (*ProbeResult, error)
}

// RenderSpec 单变体渲染规格（详细设计 §2.7）。
type RenderSpec struct {
	VariantID   string
	SrcPath     string // 源素材（已裁 cut 前）
	DstPath     string // 成片 mp4
	CoverPath   string // 封面 jpg
	SrtPath     string // 烧录字幕（可空 = 不烧录）
	ManifestDst string
	Aspect      AspectSpec
	StartSec    float64
	EndSec      float64
	Margin      SafetyMargin
	IntroPath   string // 模板片头（可空）
	OutroPath   string // 模板片尾（可空）
	Preset      string
	CRF         int
}

// RenderResult 渲染产物路径。
type RenderResult struct {
	VideoPath  string
	CoverPath  string
	SrtPath    string
	DurationSec float64
}

// Renderer FFmpeg 渲染器端口（🔴 缺失时必须报可读错误，不产出假结果）。
type Renderer interface {
	Available(ctx context.Context) bool
	// Version 版本串（manifest.ffmpeg_version；不可得为空串）。
	Version(ctx context.Context) string
	// Render 执行渲染；onProgress 回传 0~1（按 out_time/duration 估算）。
	Render(ctx context.Context, spec RenderSpec, onProgress func(p float64)) (*RenderResult, error)
}

// TranscriptResult ASR 转写产物。
type TranscriptResult struct {
	SrtPath  string
	Text     string
	Language string
}

// ASRClient 语音识别端口（whisper CLI；缺失报 ErrASRFail，不猜）。
type ASRClient interface {
	Available(ctx context.Context) bool
	Transcribe(ctx context.Context, mediaPath, outDir string) (*TranscriptResult, error)
}

// LLMClient 大模型端口（Ollama HTTP；不可用报 ErrLLMUnavailable，不编）。
type LLMClient interface {
	Available(ctx context.Context) bool
	Generate(ctx context.Context, prompt string) (string, error)
}

// ============ 基础设施端口 ============

// TaskEnqueuer asynq 入队端口（渲染/下载异步化）。
// 🔴 noop 实现返回 ErrQueueUnavailable——任务不会静默假排队。
type TaskEnqueuer interface {
	EnqueueDownload(ctx context.Context, taskID string) error
	EnqueueAnalyze(ctx context.Context, taskID string) error
	EnqueueRender(ctx context.Context, jobID string) error
}

// EventPublisher NATS 事件端口（Dispatch 发 mm.mme.variant.ready.v1）。
type EventPublisher interface {
	PublishVariantReady(ctx context.Context, set *mediacontract.VariantSet, traceID string) error
}

// MediaStorage 媒体文件接触面（唯一写文件的组件；media_dir 布局归它管）。
type MediaStorage interface {
	// MediaDir 媒体根目录。
	MediaDir() string
	// AssetDir 素材目录 <media_dir>/assets/<asset_id>/。
	AssetDir(assetID string) string
	// VariantDir 变体目录 <media_dir>/variants/<variant_id>/。
	VariantDir(variantID string) string
	// WorkDir 中间产物目录（渲染失败保留现场）。
	WorkDir(jobID string) string
	// IngestFile 把本地文件纳入资产库（返回库内绝对路径与字节数）。
	IngestFile(ctx context.Context, srcPath, assetID string) (dstPath string, sizeBytes int64, err error)
	// ExportZip 打包簇内全部 selected 产物 + manifest，返回 zip 绝对路径。
	ExportZip(ctx context.Context, setID string, variants []*Variant) (zipPath string, err error)
	// Remove 删除文件（去重命中时删重复下载副本用）。
	Remove(ctx context.Context, path string) error
}
