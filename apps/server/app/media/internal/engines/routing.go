package engines

import (
	"fmt"
	"strings"

	"framework/app/media/internal/hostcaps"
)

// Routing 路由判定结果。
type Routing string

const (
	RoutingAccelerated Routing = "accelerated"
	RoutingCPUFallback Routing = "cpu_fallback"
	RoutingCPUOnly     Routing = "cpu_only"
	RoutingUnavailable Routing = "unavailable"
)

// Resolve 纯函数路由判定（全矩阵可单测）：
//
//	空 compat               → cpu_only
//	宿主加速器 ∈ compat     → accelerated
//	宿主无加速器 ∧ compat∋cpu → cpu_only
//	宿主加速器 ∉ compat ∧ compat∋cpu → cpu_fallback（显式降级，绝不静默）
//	宿主无加速器 ∧ compat 无 cpu → unavailable
//	宿主加速器 ∉ compat ∧ compat 无 cpu → unavailable
func Resolve(c Caps, host hostcaps.Caps) Routing {
	compat := c.GPUCompat
	if len(compat) == 0 {
		return RoutingCPUOnly
	}
	has := func(v string) bool {
		for _, x := range compat {
			if strings.EqualFold(x, v) {
				return true
			}
		}
		return false
	}

	if host.Accel != hostcaps.None && has(host.Accel) {
		return RoutingAccelerated
	}
	if has("cpu") {
		if host.Accel == hostcaps.None {
			return RoutingCPUOnly
		}
		return RoutingCPUFallback
	}
	return RoutingUnavailable
}

// Notice 面向用户的路由提示（稳定公开话术；cpu_fallback 必提示，其余空串）。
func Notice(r Routing, c Caps, host hostcaps.Caps) string {
	switch r {
	case RoutingCPUFallback:
		return fmt.Sprintf("引擎声明不支持本机 %s 加速，已按 CPU 路径执行，速度会显著下降", host.Accel)
	case RoutingUnavailable:
		if len(c.GPUCompat) > 0 {
			return fmt.Sprintf("引擎要求 %s 加速，本机探测不到，已拒绝执行", strings.Join(c.GPUCompat, "/"))
		}
		return "引擎在本机不可用"
	}
	return ""
}

// GateError 硬门禁：unavailable 且引擎无法本机执行时返回的错误。
// cpu_fallback 不报错（由 Notice 透出）；GPU 必需但宿主无 → ErrEngineNoCPUFallbk。
func GateError(r Routing) error {
	if r == RoutingUnavailable {
		return ErrEngineNoCPUFallbk
	}
	return nil
}
