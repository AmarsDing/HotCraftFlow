package data

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/pkg/exectree"
)

// FfprobeProber ffprobe CLI 探测器（QC 边界自检 + 元数据补全）。
type FfprobeProber struct {
	cfg bizmme.Config
	log *log.Helper
}

// NewFfprobeProber 构造。
func NewFfprobeProber(cfg bizmme.Config, logger log.Logger) bizmme.Prober {
	return &FfprobeProber{cfg: cfg, log: log.NewHelper(log.With(logger, "module", "mme/ffprobe"))}
}

func (p *FfprobeProber) Available(context.Context) bool {
	_, err := exectree.LookPath(p.cfg.FFprobePath)
	return err == nil
}

// ffprobeOut -print_format json 输出最小子集。
type ffprobeOut struct {
	Format struct {
		Duration string `json:"duration"`
		Size     string `json:"size"`
	} `json:"format"`
	Streams []struct {
		CodecType string `json:"codec_type"`
		Width     int    `json:"width"`
		Height    int    `json:"height"`
	} `json:"streams"`
}

// Probe 探测媒体文件时长/分辨率/字节数。失败返回 error（QC 据此诚实降级）。
func (p *FfprobeProber) Probe(ctx context.Context, path string) (*bizmme.ProbeResult, error) {
	if !p.Available(ctx) {
		return nil, bizmme.ErrInvalidState
	}
	res := exectree.Run(ctx, exectree.RunSpec{
		Bin: p.cfg.FFprobePath,
		Args: []string{
			"-v", "quiet",
			"-print_format", "json",
			"-show_format", "-show_streams",
			"--", path,
		},
		Timeout:     60 * time.Second,
		OutputLines: 4000, // ffprobe JSON 需完整（多流 mkv 行数大）
	})
	if !res.Success() {
		return nil, &probeError{attribution: res.Attribution, stderr: res.StderrTail}
	}
	var out ffprobeOut
	if err := json.Unmarshal([]byte(res.StdoutTail), &out); err != nil {
		return nil, err
	}
	pr := &bizmme.ProbeResult{}
	if v, err := strconv.ParseFloat(out.Format.Duration, 64); err == nil {
		pr.DurationSec = v
	}
	if v, err := strconv.ParseInt(out.Format.Size, 10, 64); err == nil {
		pr.SizeBytes = v
	}
	for _, s := range out.Streams {
		if s.CodecType == "video" && pr.Width == 0 {
			pr.Width, pr.Height = s.Width, s.Height
			break
		}
	}
	return pr, nil
}

// probeError 探测失败。
type probeError struct{ attribution, stderr string }

func (e *probeError) Error() string {
	return "ffprobe failed: " + e.attribution + " | " + tail(e.stderr, 200)
}
