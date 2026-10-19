package mmesub

import (
	"strings"
	"testing"
)

// TestForceStyle 与 admin/media 两侧原手抄常量逐字一致（抽包后行为不变性）。
func TestForceStyle(t *testing.T) {
	got := ForceStyle(220)
	want := "FontName=Microsoft YaHei,FontSize=14,PrimaryColour=&H00FFFFFF," +
		"OutlineColour=&H80000000,BorderStyle=1,Outline=2,Shadow=0,Alignment=2,MarginV=220"
	if got != want {
		t.Fatalf("ForceStyle(220) mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestForceStyleZeroMargin(t *testing.T) {
	got := ForceStyle(0)
	if !strings.Contains(got, "MarginV=0") {
		t.Fatalf("ForceStyle(0) should embed MarginV=0, got %s", got)
	}
	// 模板里不应残留 %d 占位符（sprintf 未替换干净即错）。
	if strings.Contains(got, "%d") {
		t.Fatalf("ForceStyle(0) left unsubstituted placeholder: %s", got)
	}
}

// TestEscapeFilterPath Windows 盘符冒号转义（ffmpeg subtitles 滤镜参数分隔）。
func TestEscapeFilterPath(t *testing.T) {
	got := EscapeFilterPath(`C:\tmp\字幕 文件\burn.srt`)
	// 反斜杠转正斜杠 + 盘符冒号转义；路径里其余字符原样保留。
	want := `C\:/tmp/字幕 文件/burn.srt`
	if got != want {
		t.Fatalf("EscapeFilterPath mismatch:\n got=%s\nwant=%s", got, want)
	}
}

func TestEscapeFilterPathPOSIX(t *testing.T) {
	// POSIX 路径无盘符冒号，只有反斜杠→正斜杠折叠（无反斜杠即原样）。
	got := EscapeFilterPath("/data/subs/burn.srt")
	if got != "/data/subs/burn.srt" {
		t.Fatalf("EscapeFilterPath(POSIX) should be identity, got %s", got)
	}
}

// TestEscapeFilterPathSpacePreserved 空格不被转义（ffmpeg subtitles 滤镜对空格
// 路径由调用方另行引号处理，此处只保证不引入额外变形）。
func TestEscapeFilterPathSpacePreserved(t *testing.T) {
	got := EscapeFilterPath("E:/my videos/burn.srt")
	if !strings.Contains(got, "my videos") {
		t.Fatalf("EscapeFilterPath should preserve spaces, got %s", got)
	}
	if !strings.HasPrefix(got, `E\:`) {
		t.Fatalf("EscapeFilterPath should escape drive colon, got %s", got)
	}
}
