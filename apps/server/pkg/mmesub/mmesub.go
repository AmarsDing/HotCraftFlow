// Package mmesub 是 MME 字幕烧录样式的唯一真相（M5 修复）。
//
// 此前 admin 侧（biz/mme/qc.go ForceStyleFor）与 media 委托渲染侧
// （jobrun/mme_render.go subtitleForceStyle/escapeSubFilterPath）各持一份手抄
// 副本，注释自认是债——QC 改字号/颜色后，委托路径仍按旧值烧字，两侧可独立漂移。
// 抽到此共享包后双执行面 import 同一实现，漂移在编译期即暴露（改一处编译错另一处）。
package mmesub

import (
	"fmt"
	"path/filepath"
	"strings"
)

// forceStyleTemplate 烧录样式模板（MarginV 由安全区底部留白注入）。
// 与 FFmpeg subtitles 滤镜 force_style 语法对齐。
const forceStyleTemplate = "FontName=Microsoft YaHei,FontSize=14,PrimaryColour=&H00FFFFFF," +
	"OutlineColour=&H80000000,BorderStyle=1,Outline=2,Shadow=0,Alignment=2,MarginV=%d"

// ForceStyle 生成字幕烧录 force_style 字符串。
// marginVBottom 为安全区底部留白像素（由调用方按画幅档位算出）。
func ForceStyle(marginVBottom int) string {
	return sprintf(forceStyleTemplate, marginVBottom)
}

// EscapeFilterPath subtitles 滤镜路径转义（Windows 盘符冒号必须处理，
// 否则 ffmpeg 把 C: 的冒号当滤镜参数分隔符）。
func EscapeFilterPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, ":", `\:`)
	return p
}

// sprintf 即 fmt.Sprintf 的薄封装（单独成函数便于未来扩展占位符；行为与其等价）。
func sprintf(tmpl string, v int) string {
	return fmt.Sprintf(tmpl, v)
}
