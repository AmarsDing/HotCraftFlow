package data

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/pkg/exectree"
)

// YtdlpDownloader yt-dlp CLI 下载器（M1/M2）。
// 产物：destDir/src.<ext> 主视频 + 可选源字幕 + info.json。
// 🔴 失败返回 ErrDownloadFail 包裹的可读归因（exectree Attribution + stderr 尾部），
// 提示 cookie 配置方向，不猜测原因。
type YtdlpDownloader struct {
	cfg bizmme.Config
	log *log.Helper
}

// NewYtdlpDownloader 构造。
func NewYtdlpDownloader(cfg bizmme.Config, logger log.Logger) bizmme.Downloader {
	return &YtdlpDownloader{cfg: cfg, log: log.NewHelper(log.With(logger, "module", "mme/ytdlp"))}
}

func (d *YtdlpDownloader) Available(context.Context) bool {
	_, err := exectree.LookPath(d.cfg.YtdlpPath)
	return err == nil
}

// ytdlpInfo --print-json 输出的最小子集（只取需要字段；多余字段忽略）。
type ytdlpInfo struct {
	ID       string  `json:"id"`
	Title    string  `json:"title"`
	Duration float64 `json:"duration"`
	Width    int     `json:"width"`
	Height   int     `json:"height"`
	FilePath string  `json:"_filename"`
}

// validateSourceURL 防 argv 注入：只允许 http/https。
func validateSourceURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("%w: invalid url %q (http/https only)", bizmme.ErrDownloadFail, raw)
	}
	return nil
}

// Download 下载 URL → destDir。ctx 带 DownloadTimeout 看门狗（usecase 层已配）。
func (d *YtdlpDownloader) Download(ctx context.Context, sourceURL, destDir string) (*bizmme.DownloadResult, error) {
	if err := validateSourceURL(sourceURL); err != nil {
		return nil, err
	}
	if !d.Available(ctx) {
		return nil, fmt.Errorf("%w: yt-dlp not found (path=%s); install yt-dlp 后重试", bizmme.ErrDownloadFail, d.cfg.YtdlpPath)
	}
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("%w: mkdir %s: %v", bizmme.ErrDownloadFail, destDir, err)
	}
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"--newline",
		// 主视频落 src.<ext>；容器按站点最优（mp4 优先，不转码）
		"-o", filepath.Join(destDir, "src.%(ext)s"),
		"-f", "bv*+ba/b",
		"--merge-output-format", "mp4",
		// 元数据与源字幕（zh 优先 en 兜底；--write-subs 不烧录只落盘）
		"--write-info-json",
		"--write-subs",
		"--sub-langs", "zh.*,zh-Hans,en",
		// 下载完成后把最终文件路径与元数据以 JSON 打到 stdout
		"--print", "after_move:%(_filename)j",
		"--", sourceURL,
	}
	if d.cfg.CookiesPath != "" {
		args = append([]string{"--cookies", d.cfg.CookiesPath}, args...)
	}
	d.log.Infof("yt-dlp download: %s -> %s", sourceURL, destDir)
	res := exectree.Run(ctx, exectree.RunSpec{
		Bin:     d.cfg.YtdlpPath,
		Args:    args,
		Timeout: d.cfg.DownloadTimeout,
	})
	if !res.Success() {
		return nil, fmt.Errorf("%w: %s | stderr: %s",
			bizmme.ErrDownloadFail, res.Attribution, tail(res.StderrTail, 600))
	}
	// --print after_move 输出最后一行应是 JSON 路径串
	filePath := strings.TrimSpace(lastLine(res.StdoutTail))
	filePath = strings.Trim(filePath, `"`)
	if filePath == "" || !strings.HasSuffix(filepath.Dir(filePath), filepath.Clean(destDir)) {
		// fallback：在 destDir 里找 src.* 主文件
		filePath = findSrcFile(destDir)
	}
	if filePath == "" {
		return nil, fmt.Errorf("%w: yt-dlp success but no src file in %s", bizmme.ErrDownloadFail, destDir)
	}
	fi, err := os.Stat(filePath)
	if err != nil || fi.Size() == 0 {
		return nil, fmt.Errorf("%w: downloaded file empty: %s", bizmme.ErrDownloadFail, filePath)
	}
	out := &bizmme.DownloadResult{FilePath: filePath, SizeBytes: fi.Size()}
	// info.json 补标题/时长/分辨率（能读多少是多少，缺项留 0，不猜）
	if info := readInfoJSON(destDir); info != nil {
		out.Title = info.Title
		if out.DurationSec <= 0 {
			out.DurationSec = info.Duration
		}
		out.Width, out.Height = info.Width, info.Height
	}
	// 源字幕（yt-dlp --write-subs 产物 src.*.vtt/srt 等）
	out.SubtitlePath = findSubtitle(destDir)
	return out, nil
}

// findSrcFile 在目录里找 src. 前缀主视频文件。
func findSrcFile(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, "src.") && !strings.HasSuffix(name, ".json") &&
			!strings.Contains(name, ".zh") && !strings.Contains(name, ".en") {
			return filepath.Join(dir, name)
		}
	}
	return ""
}

// findSubtitle 找源字幕文件（.srt/.vtt，含语言中缀）。
func findSubtitle(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		name := strings.ToLower(e.Name())
		if strings.HasPrefix(name, "src.") && (strings.HasSuffix(name, ".srt") ||
			(strings.HasSuffix(name, ".vtt") && (strings.Contains(name, ".zh") || strings.Contains(name, ".en")))) {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// readInfoJSON 读 <dir>/src.info.json 最小子集。
func readInfoJSON(dir string) *ytdlpInfo {
	matches, err := filepath.Glob(filepath.Join(dir, "src.info.json"))
	if err != nil || len(matches) == 0 {
		return nil
	}
	raw, err := os.ReadFile(filepath.Clean(matches[0]))
	if err != nil {
		return nil
	}
	var info ytdlpInfo
	if err = json.Unmarshal(raw, &info); err != nil {
		return nil
	}
	return &info
}

func lastLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if i := strings.LastIndex(s, "\n"); i >= 0 {
		return s[i+1:]
	}
	return s
}

func tail(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}
