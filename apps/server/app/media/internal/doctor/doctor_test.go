package doctor_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"framework/app/media/internal/doctor"
)

// toolsFFmpegDir 仓内本地下载的 ffmpeg bin 目录（gitignore；缺则 skip）。
func toolsFFmpegDir(t *testing.T) string {
	t.Helper()
	dir := `E:\project\HotCraftFlow\apps\server\tools\ffmpeg\bin`
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		t.Skip("tools/ffmpeg 未就位，跳过探测集成测试")
	}
	return dir
}

// TestProbeFFmpegOffConsoleExitQuirk 钉住 gyan/gcc16 构建的怪癖：
// ffmpeg 版探在被 Go 子进程拉起时间歇性崩/挂（退出码乱码或超时），doctor 必须
// 以"拿到非空版本串"判绿而非"退 0"（见 doctor.go probe 注释）。
func TestProbeFFmpegOffConsoleExitQuirk(t *testing.T) {
	dir := toolsFFmpegDir(t)
	t.Setenv("PATH", dir+`;`+os.Getenv("PATH"))

	rep := doctor.Report(t.Context(), []string{"ffmpeg", "ffprobe"}, 10*time.Second)
	if len(rep) != 2 {
		t.Fatalf("期望 2 项探测: %d", len(rep))
	}
	for _, p := range rep {
		if !p.OK {
			t.Fatalf("%s 探测应为绿: hint=%s", p.Name, p.Hint)
		}
		// 版本串非空即可（不再苛求含 "version" 字样——崩/挂路径下首行可能是
		// banner 任意行；判据是探针拿到了可解析的版本输出）。
		if strings.TrimSpace(p.Version) == "" {
			t.Fatalf("%s 版本串为空", p.Name)
		}
	}
}

// TestProbeMissing 不存在的二进制：未找到 + 诚实指引。
func TestProbeMissing(t *testing.T) {
	rep := doctor.Report(t.Context(), []string{"definitely-not-exists-bin-xyz"}, time.Second)
	if len(rep) != 1 || rep[0].OK || rep[0].Hint == "" {
		t.Fatalf("缺失探测应红且有指引: %+v", rep)
	}
}
