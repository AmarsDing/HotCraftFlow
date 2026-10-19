// Package doctor 环境体检：按配置名单探测外部可执行文件（ffmpeg/yt-dlp/…），
// 解析路径与版本，给出一句修复指引。前端体检页与任务失败归因共用这份报告。
package doctor

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"framework/pkg/exectree"
)

// versionArgCandidates 版本探测参数候选（并发拉起，见 probe() 注释）。
// ffmpeg/ffprobe 对 "-V" 当输入文件→快速报错；python 只认 --version/-V
// （"-version" 快速报错退 2）；yt-dlp 只认 --version。
var versionArgCandidates = [][]string{{"-version"}, {"--version"}, {"-V"}}

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

// probe 单点探测：LookPath 找到路径 → 版本探测。
//
// 版本探测的 gyan 怪癖（钉死，勿回退为 Success() 门）：本机一台 gyan/gcc16 构建的
// ffmpeg 在版探被 Go 子进程拉起时（控制台已无、stdin 是 pipe）间歇性崩或挂死
// （同命令在已开控制台的手动 bash 复跑 ~15ms 正常退 0 | 卡到超时被杀 | 6s 崩溃乱码
// 退 0xABAFB008）——真实渲染命令路径稳定退 0（mme_render/jobrun 实测全绿），仅这个
// 构建的版探拆解有缺陷。故判定"绿"以"拿到非空版本串"为准而非"退 0"：探针意义是
// 确认二进制可执行且能吐出版本，退出码这类宿主噪音不应把它打红。
//
// M3 修复（二次判别）：原判定仅看"首行非空"——坏二进制/包装脚本退非零但把
// usage/help 打到 stdout 时照样绿，探针失去存在意义。现加版本特征门槛：
// 首行非空 AND （退出码==0 OR 首行含数字版本 pattern）。gyan 怪癖机那条"退非零但
// 吐真实版本串"的路径仍绿（版本串含 N.x 数字特征）；只吐 usage（无版本数字）的
// 坏二进制不再蒙混。ExitCode 一并留存供 Hint 归因。
//
// 候选并发（非串行）：间歇挂死是概率事件，串行最坏要扫满 N×timeout，
// 并发后总耗时≈单个 timeout，且先成功者立即返回（挂死的由 ctx 统一收）。
func probe(ctx context.Context, name string, timeout time.Duration) ProbeResult {
	r := ProbeResult{Name: name}

	path, err := exectree.LookPath(name)
	if err != nil {
		r.Hint = "未找到可执行文件：请安装并加入 PATH，或在配置中写入绝对路径"
		return r
	}
	r.Path = path

	type probeTry struct {
		version     string
		attribution string
		exitCode    int
	}
	tries := make([]probeTry, len(versionArgCandidates))
	var wg sync.WaitGroup
	for i, args := range versionArgCandidates {
		wg.Add(1)
		go func(i int, args []string) {
			defer wg.Done()
			res := exectree.Run(ctx, exectree.RunSpec{
				Bin:     path,
				Args:    args,
				Timeout: timeout,
			})
			v := firstLine(res.StdoutTail)
			if v == "" {
				v = firstLine(res.StderrTail) // 部分工具把版本打到 stderr（如 python）
			}
			tries[i] = probeTry{version: v, attribution: res.Attribution, exitCode: res.ExitCode}
		}(i, args)
	}
	wg.Wait()
	lastAttribution := ""
	for _, tr := range tries {
		if tr.version == "" {
			lastAttribution = tr.attribution
			continue
		}
		// 二次判别：退 0 直接采信；非 0 要求首行含版本数字特征（剔 usage/help）。
		if tr.exitCode != 0 && !versionNumRe.MatchString(tr.version) {
			lastAttribution = fmt.Sprintf("%s（exit=%d，首行无版本数字特征，疑 usage：%.60s）",
				tr.attribution, tr.exitCode, tr.version)
			continue
		}
		r.Version = tr.version
		if len(r.Version) > versionLineMax {
			r.Version = r.Version[:versionLineMax]
		}
		r.OK = true
		return r
	}
	r.Hint = "已找到但版本探测执行失败：" + lastAttribution
	return r
}

// versionNumRe 版本数字特征（"ffmpeg version 6.1"、"yt-dlp 2024.08"、"N-123-g…" 均含
// 一个后随数字的边界；usage/help 首行一般无此形态）。M3 探针二次判别用。
var versionNumRe = regexp.MustCompile(`\d`)

func firstLine(s string) string {
	for _, ln := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(ln); t != "" {
			return t
		}
	}
	return ""
}
