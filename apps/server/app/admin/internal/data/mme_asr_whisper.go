package data

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/pkg/exectree"
)

// WhisperASR whisper CLI 转写器（openai-whisper / faster-whisper CLI 二选一探测）。
//
// 🔴 不可用（未安装）时 Available=false，调用方报 MME_ASR_FAIL 并保留中间产物；
// 绝不编造转写文本。产物：outDir/<basename>.srt + .txt。
type WhisperASR struct {
	cfg bizmme.Config
	log *log.Helper
}

// NewWhisperASR 构造。
func NewWhisperASR(cfg bizmme.Config, logger log.Logger) bizmme.ASRClient {
	return &WhisperASR{cfg: cfg, log: log.NewHelper(log.With(logger, "module", "mme/whisper"))}
}

// whisperBin 探测可执行：faster-whisper 包常装为 "whisper-ctranslate2"，
// openai-whisper 为 "whisper"。
func (w *WhisperASR) whisperBin() string {
	for _, bin := range []string{"whisper", "whisper-ctranslate2"} {
		if p, err := exectree.LookPath(bin); err == nil {
			return p
		}
	}
	return ""
}

func (w *WhisperASR) Available(context.Context) bool { return w.whisperBin() != "" }

// Transcribe 转写 mediaPath → SRT。超时：媒体分钟数 ×4 + 10min 起步（粗看门狗）。
func (w *WhisperASR) Transcribe(ctx context.Context, mediaPath, outDir string) (*bizmme.TranscriptResult, error) {
	bin := w.whisperBin()
	if bin == "" {
		return nil, fmt.Errorf("%w: whisper CLI not found; install openai-whisper/faster-whisper 后重试（手工 EDL 通路不受影响）", bizmme.ErrASRFail)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: mkdir %s: %v", bizmme.ErrASRFail, outDir, err)
	}
	args := []string{
		mediaPath,
		"--model", w.cfg.ASRModel,
		"--output_format", "srt",
		"--output_dir", outDir,
		"--verbose", "False",
	}
	// whisper 中文为主场景留 language 自动检测；强制 zh 属产品化取舍，先不锁
	res := exectree.Run(ctx, exectree.RunSpec{
		Bin:     bin,
		Args:    args,
		Timeout: 30 * time.Minute,
	})
	if !res.Success() {
		return nil, fmt.Errorf("%w: %s | stderr: %s", bizmme.ErrASRFail, res.Attribution, tail(res.StderrTail, 400))
	}
	// whisper 输出 <basename>.srt
	base := strings.TrimSuffix(filepath.Base(mediaPath), filepath.Ext(mediaPath))
	srtPath := filepath.Join(outDir, base+".srt")
	raw, err := os.ReadFile(filepath.Clean(srtPath))
	if err != nil || len(raw) == 0 {
		return nil, fmt.Errorf("%w: whisper success but no srt at %s", bizmme.ErrASRFail, srtPath)
	}
	w.log.Infof("whisper transcribed %s -> %s (%d bytes)", mediaPath, srtPath, len(raw))
	return &bizmme.TranscriptResult{SrtPath: srtPath, Text: string(raw), Language: "auto"}, nil
}
