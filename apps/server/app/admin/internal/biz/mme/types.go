package mme

import (
	"fmt"
	"time"

	"framework/app/admin/internal/conf"
)

// ============ 运行配置（conf.Mme 解析视图，默认值在此补齐） ============

// SafetyMargin 字幕安全区（概要设计 §5.2）。
type SafetyMargin struct {
	Bottom int `json:"bottom"`
	Top    int `json:"top"`
}

// Config MME 运行配置。
type Config struct {
	Enabled       bool
	PromptVersion string

	// 下载
	YtdlpPath       string
	CookiesPath     string
	DownloadTimeout time.Duration

	// 渲染
	RenderConcurrency      int
	FFmpegPath             string
	FFprobePath            string
	Preset                 string
	CRF                    int
	MaxDurSec              int
	MinDurSec              int
	KeepIntermediateOnFail bool

	// 矩阵
	MaxVariantsPerSet int
	DefaultAspects    []string
	DefaultHookCount  int

	// 缓存
	CacheEnabled       bool
	CacheTTLDays       int
	TranscriptTTLDays  int
	MaxCacheBytes      int64

	// 差异化
	DiffEnabled        bool
	DiffWarnThreshold  float64
	DiffHookThreshold  float64
	DiffCopyThreshold  float64
	DiffSampleFrames   int

	// QC
	SafetyMargins        map[string]SafetyMargin
	BlockOnBoundaryFail  bool
	BlockOnSubtitleFail  bool

	// ASR
	ASREngine     string
	ASRModel      string
	FunASREnabled bool
	CloudFallback bool

	// LLM
	LLMProvider  string
	LLMFallback  string
	OllamaURL    string
	OllamaModel  string
	LLMTimeout   time.Duration
	LLMDailyCap  int

	// 存储
	MediaDir      string
	ExportDir     string
	RetentionDays int

	// 剪映（本轮仅占位；M15 依赖 py 侧车部署）
	JianyingEnabled     bool
	PyDraftURL          string
	JianyingVersionNote string
}

// ResolveConfig 把 conf.Bootstrap.GetMme() 解析为运行时 Config（零值补默认值）。
func ResolveConfig(bc *conf.Bootstrap) Config {
	c := Config{
		Enabled:                true,
		PromptVersion:          "v1.0",
		YtdlpPath:              "yt-dlp",
		DownloadTimeout:        30 * time.Minute,
		RenderConcurrency:      2,
		FFmpegPath:             "ffmpeg",
		FFprobePath:            "ffprobe",
		Preset:                 "medium",
		CRF:                    23,
		MaxDurSec:              300,
		MinDurSec:              5,
		KeepIntermediateOnFail: true,
		MaxVariantsPerSet:      60,
		DefaultAspects:         []string{"9:16", "1:1", "16:9"},
		DefaultHookCount:       2,
		CacheEnabled:           true,
		CacheTTLDays:           90,
		TranscriptTTLDays:      180,
		MaxCacheBytes:          50 << 30,
		DiffEnabled:            true,
		DiffWarnThreshold:      0.90,
		DiffHookThreshold:      0.85,
		DiffCopyThreshold:      0.92,
		DiffSampleFrames:       12,
		SafetyMargins: map[string]SafetyMargin{
			"9:16": {Bottom: 380, Top: 130},
			"1:1":  {Bottom: 180, Top: 100},
			"16:9": {Bottom: 120, Top: 80},
		},
		BlockOnBoundaryFail: true,
		BlockOnSubtitleFail: true,
		ASREngine:           "whisper",
		ASRModel:            "base",
		LLMProvider:         "cloud",
		LLMFallback:         "ollama",
		OllamaURL:           "http://127.0.0.1:11434",
		OllamaModel:         "qwen2.5:7b",
		LLMTimeout:          120 * time.Second,
		LLMDailyCap:         800,
		MediaDir:            "./storage/mme/media",
		ExportDir:           "./storage/mme/export",
		RetentionDays:       180,
		JianyingEnabled:     true,
		PyDraftURL:          "http://127.0.0.1:8901",
		JianyingVersionNote: "[待测] 剪映版本兼容性未全面验证",
	}
	if bc == nil || bc.GetMme() == nil {
		return c
	}
	m := bc.GetMme()
	if !m.GetEnabled() {
		c.Enabled = false
	}
	if v := m.GetPromptVersion(); v != "" {
		c.PromptVersion = v
	}
	if d := m.GetDownload(); d != nil {
		if v := d.GetYtdlpPath(); v != "" {
			c.YtdlpPath = v
		}
		c.CookiesPath = d.GetCookiesPath()
		if d.GetTimeout() != nil && d.GetTimeout().AsDuration() > 0 {
			c.DownloadTimeout = d.GetTimeout().AsDuration()
		}
	}
	if r := m.GetRender(); r != nil {
		if r.GetConcurrency() > 0 {
			c.RenderConcurrency = int(r.GetConcurrency())
		}
		if v := r.GetFfmpegPath(); v != "" {
			c.FFmpegPath = v
		}
		if v := r.GetFfprobePath(); v != "" {
			c.FFprobePath = v
		}
		if v := r.GetPreset(); v != "" {
			c.Preset = v
		}
		if r.GetCrf() > 0 {
			c.CRF = int(r.GetCrf())
		}
		if r.GetMaxDurationSec() > 0 {
			c.MaxDurSec = int(r.GetMaxDurationSec())
		}
		if r.GetMinDurationSec() > 0 {
			c.MinDurSec = int(r.GetMinDurationSec())
		}
		c.KeepIntermediateOnFail = r.GetKeepIntermediateOnFail()
	}
	if mx := m.GetMatrix(); mx != nil {
		if mx.GetMaxVariantsPerSet() > 0 {
			c.MaxVariantsPerSet = int(mx.GetMaxVariantsPerSet())
		}
		if len(mx.GetDefaultAspects()) > 0 {
			c.DefaultAspects = append([]string(nil), mx.GetDefaultAspects()...)
		}
		if mx.GetDefaultHookCount() > 0 {
			c.DefaultHookCount = int(mx.GetDefaultHookCount())
		}
	}
	if ca := m.GetCache(); ca != nil {
		c.CacheEnabled = ca.GetEnabled()
		if ca.GetTtlDays() > 0 {
			c.CacheTTLDays = int(ca.GetTtlDays())
		}
		if ca.GetTranscriptTtlDays() > 0 {
			c.TranscriptTTLDays = int(ca.GetTranscriptTtlDays())
		}
		if ca.GetMaxCacheBytes() > 0 {
			c.MaxCacheBytes = ca.GetMaxCacheBytes()
		}
	}
	if dc := m.GetDiffCheck(); dc != nil {
		c.DiffEnabled = dc.GetEnabled()
		if dc.GetWarnThreshold() > 0 {
			c.DiffWarnThreshold = dc.GetWarnThreshold()
		}
		if dc.GetHookThreshold() > 0 {
			c.DiffHookThreshold = dc.GetHookThreshold()
		}
		if dc.GetCopyThreshold() > 0 {
			c.DiffCopyThreshold = dc.GetCopyThreshold()
		}
		if dc.GetSampleFrames() > 0 {
			c.DiffSampleFrames = int(dc.GetSampleFrames())
		}
	}
	if q := m.GetQc(); q != nil {
		for k, v := range q.GetSafetyMargin() {
			c.SafetyMargins[k] = SafetyMargin{Bottom: int(v.GetBottom()), Top: int(v.GetTop())}
		}
		c.BlockOnBoundaryFail = q.GetBlockOnBoundaryFail()
		c.BlockOnSubtitleFail = q.GetBlockOnSubtitleFail()
	}
	if a := m.GetAsr(); a != nil {
		if v := a.GetEngine(); v != "" {
			c.ASREngine = v
		}
		if v := a.GetModel(); v != "" {
			c.ASRModel = v
		}
		c.FunASREnabled = a.GetFunasrEnabled()
		c.CloudFallback = a.GetCloudFallback()
	}
	if l := m.GetLlm(); l != nil {
		if v := l.GetProvider(); v != "" {
			c.LLMProvider = v
		}
		if v := l.GetFallback(); v != "" {
			c.LLMFallback = v
		}
		if v := l.GetOllamaUrl(); v != "" {
			c.OllamaURL = v
		}
		if v := l.GetOllamaModel(); v != "" {
			c.OllamaModel = v
		}
		if l.GetTimeout() != nil && l.GetTimeout().AsDuration() > 0 {
			c.LLMTimeout = l.GetTimeout().AsDuration()
		}
		if l.GetDailyCallCap() > 0 {
			c.LLMDailyCap = int(l.GetDailyCallCap())
		}
	}
	if s := m.GetStorage(); s != nil {
		if v := s.GetMediaDir(); v != "" {
			c.MediaDir = v
		}
		if v := s.GetExportDir(); v != "" {
			c.ExportDir = v
		}
		if s.GetRetentionDays() > 0 {
			c.RetentionDays = int(s.GetRetentionDays())
		}
	}
	if j := m.GetJianying(); j != nil {
		c.JianyingEnabled = j.GetEnabled()
		if v := j.GetPyDraftUrl(); v != "" {
			c.PyDraftURL = v
		}
		if v := j.GetVersionNote(); v != "" {
			c.JianyingVersionNote = v
		}
	}
	return c
}

// ============ 画幅规格（概要设计 §5.1 / 详细设计 §2.7） ============

// AspectSpec 画幅规格：目标分辨率与 reframing 滤镜表达式。
type AspectSpec struct {
	Key    string // 9:16 | 1:1 | 16:9
	Width  int
	Height int
	// Filter 供 ffmpeg -vf 使用。诚实声明：无 GPU 时 9:16/1:1 是中心裁切，
	// 不是智能主体追踪；16:9 非等比来源走 letterbox 保持画面完整。
	Filter string
}

// AspectSpecs 全量画幅表（map + 稳定次序）。
var AspectSpecs = map[string]AspectSpec{
	"9:16": {Key: "9:16", Width: 1080, Height: 1920, Filter: "crop=ih*9/16:ih,scale=1080:1920:flags=lanczos"},
	"1:1":  {Key: "1:1", Width: 1080, Height: 1080, Filter: "crop=ih:ih,scale=1080:1080:flags=lanczos"},
	"16:9": {Key: "16:9", Width: 1920, Height: 1080, Filter: "scale=1920:1080:force_original_aspect_ratio=decrease,pad=1920:1080:(ow-iw)/2:(oh-ih)/2"},
}

// LookupAspect 解析画幅 key；非法 key 返回 false。
func LookupAspect(key string) (AspectSpec, bool) {
	s, ok := AspectSpecs[key]
	return s, ok
}

// MarginOf 取画幅字幕安全区；未知画幅退最保守值。
func (c Config) MarginOf(aspect string) SafetyMargin {
	if m, ok := c.SafetyMargins[aspect]; ok {
		return m
	}
	return SafetyMargin{Bottom: 380, Top: 130}
}

// ============ 领域类型 ============

// Highlight 高光候选 / 内容版本（B 维度）。存于 MontageTask.highlights JSON。
type Highlight struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	StartSec float64 `json:"start_sec"`
	EndSec   float64 `json:"end_sec"`
	Note     string  `json:"note"`
	Origin   string  `json:"origin"` // manual | auto
}

// Duration 时长。
func (h Highlight) Duration() float64 { return h.EndSec - h.StartSec }

// EDLRef 切点引用（存 Variant.edl_ref 的 JSON）。
func (h Highlight) EDLRef() string {
	return fmt.Sprintf(`{"start_sec":%.3f,"end_sec":%.3f}`, h.StartSec, h.EndSec)
}

// HookSpec Hook（前 3s）规格（C 维度）。
type HookSpec struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Text  string `json:"text"`
}

// Task 加工任务领域视图。
type Task struct {
	ID, Title, SourceType, SourceRef string
	CandidateID, IntelID, AssetID    string
	UserIntent, State                string
	PromptVersion, ASREngine         string
	ErrorMsg                         string
	Highlights                       []Highlight
	Progress                         float64
	CreatedAt, UpdatedAt             time.Time
}

// Asset 素材领域视图。
type Asset struct {
	ID, Fingerprint string
	SourceURL       string
	LocalPath       string
	DurationSec     float64
	Width, Height   int
	SizeBytes       int64
	RefCount        int
	MetaTitle       string
	SubtitlePath    string
	CreatedAt       time.Time
}

// CacheEntry 分析缓存条目。
type CacheEntry struct {
	ID                                  int64
	CacheKey, Kind, AssetID             string
	PayloadPath                         string
	PayloadMeta                         map[string]any
	Engine, EngineVersion, PromptVersion string
	HitCount, SizeBytes                 int64
	ExpiresAt                           time.Time
}

// PairSimilarity 两变体相似度（FR-MIX-05）。
type PairSimilarity struct {
	VariantA string  `json:"variant_a"`
	VariantB string  `json:"variant_b"`
	Visual   float64 `json:"visual"`
	Hook     float64 `json:"hook"`
	Copy     float64 `json:"copy"`
	Overall  float64 `json:"overall"`
}

// VariantSet 变体簇领域视图。
type VariantSet struct {
	ID, TaskID, CandidateID, AssetID string
	TranscriptRef, State             string
	VariantCount                     int
	DiffMinSimilarity                float64
	DiffWarn                         bool
	DiffPairs                        []PairSimilarity
	TemplateID                       string
	CreatedAt                        time.Time
}

// VariantState 渲染状态常量。
const (
	RenderPending = "pending"
	RenderRunning = "running"
	RenderDone    = "done"
	RenderFailed  = "failed"

	ReviewPending  = "pending"
	ReviewApproved = "approved"
	ReviewRejected = "rejected"

	SetPlanned   = "planned"
	SetRendering = "rendering"
	SetReview    = "review"
	SetApproved  = "approved"
	SetRejected  = "rejected"
)

// Variant 单变体领域视图。
type Variant struct {
	ID, SetID, ContentVersion, Aspect string
	HookID, CoverID                   string
	EDLRef                            string
	DurationSec                       float64
	VideoPath, CoverPath              string
	CaptionStyle, SrtPath             string
	Copy                              map[string]PlatformCopy
	CopyDegraded                      bool
	ManifestPath                      string
	QCBoundaryOK, QCSubtitleOK        bool
	QCScore                           float64
	RenderState, RenderError          string
	Selected                          bool
	ReviewState, ReviewReason         string
}

// PlatformCopy 平台差异化文案（与契约 §4 copy.<platform> 同形）。
type PlatformCopy struct {
	Title string   `json:"title"`
	Desc  string   `json:"desc"`
	Tags  []string `json:"tags,omitempty"`
}

// RenderJob 渲染任务领域视图。
type RenderJob struct {
	ID, VariantID, State       string
	Progress                   float64
	StartedAt, FinishedAt      time.Time
	WorkerID                   string
	ErrorCode, ErrorMsg        string
	IntermediatePath           string
	Attempt                    int
	CreatedAt                  time.Time
}

// Template 混剪模板。
type Template struct {
	ID, Name                 string
	IntroPath, OutroPath     string
	Transition               string
	CaptionSkin              map[string]any
	Enabled                  bool
}

// DefaultHooks 生成默认 Hook 组（C 维度，数量来自配置）。
func DefaultHooks(c Config) []HookSpec {
	n := c.DefaultHookCount
	if n < 2 {
		n = 2
	}
	out := make([]HookSpec, 0, n)
	labels := []string{"Hook A", "Hook B", "Hook C", "Hook D"}
	ids := []string{"hook_a", "hook_b", "hook_c", "hook_d"}
	for i := 0; i < n && i < len(ids); i++ {
		out = append(out, HookSpec{ID: ids[i], Label: labels[i]})
	}
	return out
}
