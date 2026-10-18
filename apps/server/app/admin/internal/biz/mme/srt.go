package mme

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SRTEntry 单条字幕。
type SRTEntry struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

// parseSRTTimestamp 解析 "00:00:01,234"（容忍句点毫秒）。
func parseSRTTimestamp(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, ".", ",")
	parts := strings.Split(s, ",")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad srt ts %q", s)
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 {
		return 0, fmt.Errorf("bad srt ts %q", s)
	}
	h, err := strconv.Atoi(hms[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(hms[1])
	if err != nil {
		return 0, err
	}
	sec, err := strconv.Atoi(hms[2])
	if err != nil {
		return 0, err
	}
	ms, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute +
		time.Duration(sec)*time.Second + time.Duration(ms)*time.Millisecond, nil
}

// ParseSRT 解析 SRT 文本（LF/CRLF 均容忍；序号行可缺失时按块处理）。
func ParseSRT(content string) ([]SRTEntry, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, nil
	}
	var out []SRTEntry
	for _, block := range strings.Split(content, "\n\n") {
		lines := strings.Split(strings.TrimSpace(block), "\n")
		if len(lines) == 0 {
			continue
		}
		// 跳过纯数字序号行
		if _, err := strconv.Atoi(strings.TrimSpace(lines[0])); err == nil {
			lines = lines[1:]
		}
		if len(lines) < 2 {
			continue
		}
		arrow := strings.Index(lines[0], "-->")
		if arrow < 0 {
			return nil, fmt.Errorf("bad srt block %q", lines[0])
		}
		start, err := parseSRTTimestamp(lines[0][:arrow])
		if err != nil {
			return nil, err
		}
		end, err := parseSRTTimestamp(lines[0][arrow+3:])
		if err != nil {
			return nil, err
		}
		out = append(out, SRTEntry{
			Start: start,
			End:   end,
			Text:  strings.TrimSpace(strings.Join(lines[1:], "\n")),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}

// FormatSRTTimestamp 输出 "00:00:01,234"。
func FormatSRTTimestamp(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h := ms / 3600000
	m := (ms % 3600000) / 60000
	s := (ms % 60000) / 1000
	msec := ms % 1000
	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, msec)
}

// WriteSRT 序列化为标准 SRT。
func WriteSRT(entries []SRTEntry) string {
	var b strings.Builder
	for i, e := range entries {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1,
			FormatSRTTimestamp(e.Start), FormatSRTTimestamp(e.End), e.Text)
	}
	return b.String()
}

// ShiftSRT 整体平移 offset（用于 EDL 裁切后把字幕对齐成片时间轴：
// 裁 [start,end) 片段 → offset = -start；越界条目被裁掉或截断）。
func ShiftSRT(entries []SRTEntry, offset time.Duration, clipStart, clipEnd time.Duration) []SRTEntry {
	out := make([]SRTEntry, 0, len(entries))
	for _, e := range entries {
		// 与裁切窗口无交集 → 丢弃
		if e.End <= clipStart || e.Start >= clipEnd {
			continue
		}
		if e.Start < clipStart {
			e.Start = clipStart
		}
		if e.End > clipEnd {
			e.End = clipEnd
		}
		e.Start += offset
		e.End += offset
		if e.End > e.Start {
			out = append(out, e)
		}
	}
	return out
}

// SubtitleHasHardCut 检测句中硬切：相邻条目间隙 < minGap 且前条不以标点收尾。
// QC 字幕自检的一项（详细设计 §2.7 subtitle 自检）。
func SubtitleHasHardCut(entries []SRTEntry, minGap time.Duration) bool {
	terminators := ".。!！?？…~"
	for i := 0; i+1 < len(entries); i++ {
		// 间隙 = 下条开始 - 本条结束（重叠为负，同样视为紧贴硬切）
		if entries[i+1].Start-entries[i].End >= minGap {
			continue
		}
		t := strings.TrimSpace(entries[i].Text)
		if t == "" {
			continue
		}
		last := string([]rune(t)[len([]rune(t))-1])
		if !strings.Contains(terminators, last) {
			return true
		}
	}
	return false
}
