package publish

import "fmt"

// SpecCheckResult 媒体规格适配结论（FR-PUB-02）。
//
// V1 诚实态度：平台不支持的规格**记警告不阻断**（由平台侧最终裁决或人工处理），
// 真实 ffmpeg 转码为 TODO 桩——不伪造「已转码」语义（FR-SAFE：禁止假装完成）。
type SpecCheckResult struct {
	Warnings []string
}

// CheckMediaSpec 按平台能力声明校验媒体规格；durationSec/sizeBytes 为 0 表示未知（跳过校验）。
func CheckMediaSpec(cap Capability, durationSec int, sizeBytes int64, aspect string) SpecCheckResult {
	var r SpecCheckResult
	if durationSec > 0 && cap.MaxDurationSec > 0 && durationSec > cap.MaxDurationSec {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("媒体时长 %ds 超出平台上限 %ds，未自动转码（TODO：ffmpeg 转码桩 V1 未接），请人工处理或接受平台侧截断",
				durationSec, cap.MaxDurationSec))
	}
	if sizeBytes > 0 && cap.MaxSizeBytes > 0 && sizeBytes > cap.MaxSizeBytes {
		r.Warnings = append(r.Warnings,
			fmt.Sprintf("媒体体积 %dB 超出平台上限 %dB，未自动压缩（TODO 桩），请人工压缩后重发",
				sizeBytes, cap.MaxSizeBytes))
	}
	if aspect != "" && len(cap.AspectRatios) > 0 {
		ok := false
		for _, a := range cap.AspectRatios {
			if a == aspect {
				ok = true
				break
			}
		}
		if !ok {
			r.Warnings = append(r.Warnings,
				fmt.Sprintf("画幅 %s 不在平台支持列表 %v，可能发布为平台默认裁切", aspect, cap.AspectRatios))
		}
	}
	return r
}
