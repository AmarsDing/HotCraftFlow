// Package hostcaps 宿主加速能力探测：路由引擎时判定 accelerated /
// cpu_fallback / cpu_only / unavailable 的输入。
//
// v1 探测策略刻意保守：只探"加速器是否存在"（可执行文件/OS），
// 不探 VRAM 余量、不引 cgo/nvml——引擎 min_vram_gb 精确闸下期再接。
// MEDIA_HOST_ACCEL 环境变量可整体覆盖（容器/特殊硬件场景）。
package hostcaps

import (
	"os"
	"runtime"
	"strings"

	"framework/pkg/exectree"
)

// Accel 加速器词表。
const (
	CUDA  = "cuda"
	ROCm  = "rocm"
	Metal = "metal"
	None  = "none"
)

// Caps 宿主探测结果。
type Caps struct {
	Accel string
	OS    string
}

// EnvOverride 环境变量名（显式覆盖探测结果）。
const EnvOverride = "MEDIA_HOST_ACCEL"

// Detect 探测宿主加速能力。顺序：环境覆盖 → Apple Silicon(metal) →
// nvidia-smi(cuda) → rocm-smi(rocm) → none。
func Detect() Caps {
	if v := strings.TrimSpace(strings.ToLower(os.Getenv(EnvOverride))); v != "" {
		switch v {
		case CUDA, ROCm, Metal, None:
			return Caps{Accel: v, OS: runtime.GOOS}
		}
		return Caps{Accel: None, OS: runtime.GOOS}
	}

	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" {
		return Caps{Accel: Metal, OS: runtime.GOOS}
	}
	if _, err := exectree.LookPath("nvidia-smi"); err == nil {
		return Caps{Accel: CUDA, OS: runtime.GOOS}
	}
	if _, err := exectree.LookPath("rocm-smi"); err == nil {
		return Caps{Accel: ROCm, OS: runtime.GOOS}
	}
	return Caps{Accel: None, OS: runtime.GOOS}
}
