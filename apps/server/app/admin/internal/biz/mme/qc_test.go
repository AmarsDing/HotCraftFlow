package mme

import (
	"strings"
	"testing"
	"time"
)

// ---------- 硬门禁判定 ----------

func TestGateOKBlocksBoundaryFail(t *testing.T) {
	cfg := testConfig()
	rep := &QCReport{BoundaryOK: false, SubtitleOK: true}
	if rep.GateOK(cfg) {
		t.Fatal("🔴 硬门禁：boundary_ok=false 不得过门")
	}
}

func TestGateOKBlocksSubtitleFail(t *testing.T) {
	cfg := testConfig()
	rep := &QCReport{BoundaryOK: true, SubtitleOK: false}
	if rep.GateOK(cfg) {
		t.Fatal("🔴 硬门禁：subtitle_ok=false 不得过门")
	}
}

func TestGateOKPass(t *testing.T) {
	rep := &QCReport{BoundaryOK: true, SubtitleOK: true}
	if !rep.GateOK(testConfig()) {
		t.Fatal("both ok should pass gate")
	}
}

// ---------- 安全区映射 ----------

func TestSafetyMarginMapping(t *testing.T) {
	cfg := testConfig()
	if m := cfg.MarginOf("9:16"); m.Bottom != 380 || m.Top != 130 {
		t.Fatalf("9:16 margin want 380/130, got %d/%d", m.Bottom, m.Top)
	}
	if m := cfg.MarginOf("1:1"); m.Bottom != 180 {
		t.Fatalf("1:1 margin bottom want 180, got %d", m.Bottom)
	}
	// 未知画幅退最保守值
	if m := cfg.MarginOf("3:4"); m.Bottom != 380 {
		t.Fatalf("unknown aspect should fall back to most conservative, got %d", m.Bottom)
	}
}

func TestForceStyleContainsMarginV(t *testing.T) {
	got := ForceStyleFor("9:16", testConfig())
	if !strings.Contains(got, "MarginV=380") {
		t.Fatalf("force_style must carry MarginV=380 for 9:16, got %q", got)
	}
}

// ---------- SRT 处理 ----------

func TestParseAndWriteSRT(t *testing.T) {
	raw := "1\r\n00:00:01,000 --> 00:00:03,500\r\n你好世界\r\n\r\n2\n00:00:04,000 --> 00:00:05,000\n第二句。\n"
	entries, err := ParseSRT(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("want 2 entries, got %d", len(entries))
	}
	if entries[0].Start != time.Second || entries[0].End != 3500*time.Millisecond {
		t.Fatalf("entry0 ts wrong: %v %v", entries[0].Start, entries[0].End)
	}
	out := WriteSRT(entries)
	again, err := ParseSRT(out)
	if err != nil || len(again) != 2 || again[1].Text != "第二句。" {
		t.Fatalf("roundtrip broken: %v %v", err, again)
	}
}

func TestShiftSRTClipsAndAligns(t *testing.T) {
	entries := []SRTEntry{
		{Start: 0, End: 2 * time.Second, Text: "裁切窗口外"},
		{Start: 5 * time.Second, End: 8 * time.Second, Text: "窗口内"},
		{Start: 9 * time.Second, End: 12 * time.Second, Text: "部分越界"},
	}
	// 裁 [5s,10s) → 平移 -5s
	got := ShiftSRT(entries, -5*time.Second, 5*time.Second, 10*time.Second)
	if len(got) != 2 {
		t.Fatalf("want 2 entries after clip, got %d", len(got))
	}
	if got[0].Start != 0 || got[0].End != 3*time.Second {
		t.Fatalf("entry0 not aligned: %v %v", got[0].Start, got[0].End)
	}
	if got[1].End != 5*time.Second { // 12s 越界截断到 10s，平移后 = 5s
		t.Fatalf("entry1 should be truncated to clip end, got %v", got[1].End)
	}
}

func TestSubtitleHardCutDetection(t *testing.T) {
	okEntries := []SRTEntry{
		{Start: 0, End: 2 * time.Second, Text: "这是一句完整的话。"},
		{Start: 2*time.Second + 100*time.Millisecond, End: 4 * time.Second, Text: "下一句。"},
	}
	if SubtitleHasHardCut(okEntries, 120*time.Millisecond) {
		t.Fatal("complete sentence followed by next should not be hard-cut")
	}
	badEntries := []SRTEntry{
		{Start: 0, End: 2 * time.Second, Text: "这句话没有说完然后"},
		{Start: 2*time.Second + 50*time.Millisecond, End: 3 * time.Second, Text: "硬切进来"},
	}
	if !SubtitleHasHardCut(badEntries, 120*time.Millisecond) {
		t.Fatal("mid-sentence narrow-gap transition must be detected as hard cut")
	}
}
