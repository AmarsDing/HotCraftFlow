// Package doctor 环境体检：按配置名单探测外部可执行文件（ffmpeg/yt-dlp/…），
// 解析路径与版本，给出一句修复指引。前端体检页与任务失败归因共用这份报告。
package doctor

import (
	"context"
	"strings"
	"time"

	"framework/pkg/exectree"
)

// ProbeResult 单项探测结果。
type ProbeResult struct {
	Name    string
	OK      bool
	Path    string
	Version string // --version 输出的首个非空行（截断）
	Hint    string // 失败时的修复指引（稳定公开话术，不泄露内部细节）
}

const versionLineMax = 120

// Report 逐项探测。timeout 为单项执行上限（防 `--version` 挂死拖垮报告）。
func Report(ctx context.Context, probes []string, timeout time.Duration) []ProbeResult {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	out := make([]ProbeResult, 0, len(probes))
	for _, name := range probes {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, probe(ctx, name, timeout))
	}
	return out
}

func probe(ctx context.Context, name string, timeout time.Duration) ProbeResult {
	r := ProbeResult{Name: name}

	path, err := exectree.LookPath(name)
	if err != nil {
		r.Hint = "未找到可执行文件：请安装并加入 PATH，或在配置中写入绝对路径"
		return r
	}
	r.Path = path

	res := exectree.Run(ctx, exectree.RunSpec{
		Bin:     path,
		Args:    []string{"--version"},
		Timeout: timeout,
	})
	if !res.Success() {
		r.Hint = "已找到但 --version 执行失败：" + res.Attribution
		return r
	}
	r.Version = firstLine(res.StdoutTail)
	if r.Version == "" {
		r.Version = firstLine(res.StderrTail) // 部分工具把版本打到 stderr（如 ffmpeg 老版本）
	}
	if len(r.Version) > versionLineMax {
		r.Version = r.Version[:versionLineMax]
	}
	r.OK = true
	return r
}

func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}
