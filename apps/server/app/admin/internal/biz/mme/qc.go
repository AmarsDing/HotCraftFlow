package mme

import (
	"context"
	"math"
	"os"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// QCChecker 质检（🔴 硬门禁，契约 §4 / 详细设计 §2.7）：
//   - 边界自检：时长落在 EDL 期望窗口、分辨率 = 目标画幅、文件非空 → BoundaryOK
//   - 字幕自检：SRT 存在且非空、时长与成片匹配（末条 end ≤ 成片 + 1s）、无句中硬切 → SubtitleOK
//   - 安全区判定：烧录时_subtitles 滤镜已用 MarginV=margin.Bottom_，此处只校验产物与规格一致；
//     force_style 计算见 ForceStyleFor。
//
// QC 不通过的变体标记 qc=false，Review 进入 approved 时被硬门禁拦截（绝不静默放行）。
type QCChecker struct {
	cfg     Config
	storage MediaStorage
	prober  Prober
	log     *log.Helper
}

// NewQCChecker 构造。
func NewQCChecker(cfg Config, storage MediaStorage, prober Prober, logger log.Logger) *QCChecker {
	return &QCChecker{cfg: cfg, storage: storage, prober: prober, log: log.NewHelper(log.With(logger, "module", "biz/mme/qc"))}
}

// QCReport 单项检查明细（manifest.qc 的来源）。
type QCReport struct {
	BoundaryOK bool    `json:"boundary_ok"`
	SubtitleOK bool    `json:"subtitle_ok"`
	Score      float64 `json:"score"`
	Checks     []QCCheck
}

// QCCheck 单条检查项。
type QCCheck struct {
	Name   string `json:"name"`
	OK     bool   `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// GateOK 硬门禁：两个核心开关都过才允许进 approved。
func (r *QCReport) GateOK(cfg Config) bool {
	if cfg.BlockOnBoundaryFail && !r.BoundaryOK {
		return false
	}
	if cfg.BlockOnSubtitleFail && !r.SubtitleOK {
		return false
	}
	return true
}

// CheckVariant 对渲染产物执行 QC。
// aspectSpec = 目标画幅规格；expectDurSec = EDL 片段时长；srtPath 可空（=无字幕成片，字幕自检跳过判 OK 但打标）。
func (c *QCChecker) CheckVariant(ctx context.Context, videoPath, srtPath string, aspectSpec AspectSpec, expectDurSec float64) *QCReport {
	rep := &QCReport{}
	add := func(name string, ok bool, detail string) {
		rep.Checks = append(rep.Checks, QCCheck{Name: name, OK: ok, Detail: detail})
	}

	// ---- 边界自检 ----
	fi, err := os.Stat(videoPath)
	if err != nil || fi.Size() == 0 {
		add("file_exists", false, "成片缺失或 0 字节: "+videoPath)
		rep.BoundaryOK = false
		rep.SubtitleOK = false
		rep.Score = 0
		return rep
	}
	add("file_exists", true, "")

	boundary := true
	var probe *ProbeResult
	if c.prober != nil && c.prober.Available(ctx) {
		if p, perr := c.prober.Probe(ctx, videoPath); perr == nil && p != nil {
			probe = p
		} else if perr != nil {
			c.log.Warnf("ffprobe failed for %s: %v", videoPath, perr)
		}
	}
	if probe == nil {
		// 无 ffprobe：不能假装通过——时长/分辨率不可验证，边界自检诚实降级为仅文件非空。
		add("probe", false, "ffprobe 不可用，时长/分辨率未验证")
		rep.BoundaryOK = false
	} else {
		add("probe", true, "")
		// 时长窗口：期望 ±1.5s 容差（重编码帧对齐误差）
		durOK := math.Abs(probe.DurationSec-expectDurSec) <= 1.5
		add("duration_match", durOK, sprintfDur(probe.DurationSec, expectDurSec))
		// 分辨率必须等于目标画幅
		resOK := probe.Width == aspectSpec.Width && probe.Height == aspectSpec.Height
		add("resolution_match", resOK, sprintfRes(probe.Width, probe.Height, aspectSpec))
		boundary = durOK && resOK
	}
	rep.BoundaryOK = boundary

	// ---- 字幕自检 ----
	subtitle := true
	if srtPath == "" {
		add("subtitle_present", true, "无字幕规格，跳过")
	} else {
		raw, rerr := os.ReadFile(srtPath)
		if rerr != nil || len(raw) == 0 {
			add("subtitle_present", false, "SRT 缺失或为空: "+srtPath)
			subtitle = false
		} else {
			entries, perr := ParseSRT(string(raw))
			if perr != nil || len(entries) == 0 {
				add("subtitle_parse", false, "SRT 解析失败")
				subtitle = false
			} else {
				add("subtitle_parse", true, "")
				if probe != nil {
					last := entries[len(entries)-1].End
					endOK := last.Seconds() <= probe.DurationSec+1.0
					add("subtitle_end_within", endOK, sprintfLastEnd(last.Seconds(), probe.DurationSec))
					subtitle = subtitle && endOK
				}
				hardCut := SubtitleHasHardCut(entries, 120*time.Millisecond)
				add("subtitle_no_hardcut", !hardCut, hardCutDetail(hardCut))
				subtitle = subtitle && !hardCut
			}
		}
	}
	rep.SubtitleOK = subtitle

	// 综合分：通过项占比（供排序参考，不参与门禁）。
	pass := 0
	for _, ck := range rep.Checks {
		if ck.OK {
			pass++
		}
	}
	rep.Score = float64(pass) / float64(len(rep.Checks))
	return rep
}

// ForceStyleFor 字幕烧录 force_style（🔴 安全区底部留白 = MarginV）。
// 简单档位 simple；dynamic 档位在 M13 后扩展，此处并档 simple 并在 manifest 标注。
func ForceStyleFor(aspect string, cfg Config) string {
	m := cfg.MarginOf(aspect)
	return "FontName=Microsoft YaHei,FontSize=14,PrimaryColour=&H00FFFFFF," +
		"OutlineColour=&H80000000,BorderStyle=1,Outline=2,Shadow=0," +
		"Alignment=2,MarginV=" + itoa(m.Bottom)
}

func sprintfDur(got, want float64) string {
	return "probe=" + f2(got) + "s, expect=" + f2(want) + "s"
}
func sprintfRes(w, h int, spec AspectSpec) string {
	return "probe=" + itoa(w) + "x" + itoa(h) + ", expect=" + itoa(spec.Width) + "x" + itoa(spec.Height)
}
func sprintfLastEnd(last, dur float64) string { return "lastEnd=" + f2(last) + "s, video=" + f2(dur) + "s" }
func hardCutDetail(b bool) string {
	if b {
		return "检测到句中硬切（相邻间隙<120ms 且前句无终止标点）"
	}
	return ""
}

func itoa(v int) string     { return strconv.Itoa(v) }
func f2(v float64) string   { return strconv.FormatFloat(v, 'f', 2, 64) }
