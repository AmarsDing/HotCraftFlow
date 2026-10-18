package engines

import (
	"context"
	"testing"

	"framework/app/media/internal/hostcaps"
)

func TestResolveFullMatrix(t *testing.T) {
	host := func(accel string) hostcaps.Caps { return hostcaps.Caps{Accel: accel} }

	cases := []struct {
		name   string
		compat []string
		host   hostcaps.Caps
		want   Routing
	}{
		{"空 compat 恒 cpu_only", nil, host(hostcaps.CUDA), RoutingCPUOnly},
		{"cuda 宿主命中 cuda", []string{"cuda", "cpu"}, host(hostcaps.CUDA), RoutingAccelerated},
		{"cuda 宿主未声明 cuda 但有 cpu → 显式降级", []string{"cpu"}, host(hostcaps.CUDA), RoutingCPUFallback},
		{"cuda 宿主无 cpu 声明 → 拒绝", []string{"rocm"}, host(hostcaps.CUDA), RoutingUnavailable},
		{"无加速器 + cpu 声明", []string{"cpu"}, host(hostcaps.None), RoutingCPUOnly},
		{"无加速器 + 纯 GPU 声明 → 拒绝", []string{"cuda"}, host(hostcaps.None), RoutingUnavailable},
		{"大小写不敏感", []string{"CUDA"}, host(hostcaps.CUDA), RoutingAccelerated},
		// 注：Resolve 仅看声明与宿主，VRAM 闸留在下期（hostcaps 不探显存）。
		{"metal 宿主 + metal 声明", []string{"metal"}, host(hostcaps.Metal), RoutingAccelerated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(Caps{GPUCompat: c.compat}, c.host); got != c.want {
				t.Fatalf("Resolve = %v, want %v", got, c.want)
			}
		})
	}
}

func TestGateErrorAndNotice(t *testing.T) {
	if err := GateError(RoutingUnavailable); err == nil {
		t.Fatal("unavailable 必须为硬错误")
	}
	for _, r := range []Routing{RoutingAccelerated, RoutingCPUFallback, RoutingCPUOnly} {
		if err := GateError(r); err != nil {
			t.Fatalf("%v 不应门禁报错: %v", r, err)
		}
	}
	// cpu_fallback 必有用户提示；accelerated/cpu_only 无提示。
	n := Notice(RoutingCPUFallback, Caps{}, hostcaps.Caps{Accel: hostcaps.CUDA})
	if n == "" {
		t.Fatal("cpu_fallback 必须透出提示")
	}
	if Notice(RoutingCPUOnly, Caps{}, hostcaps.Caps{}) != "" {
		t.Fatal("cpu_only 不应有提示")
	}
}

func TestSelectRejectsUnavailable(t *testing.T) {
	reg := NewRegistry(hostcaps.Caps{Accel: hostcaps.None}, nil)
	reg.Register(&fakeEngine{id: "gpu-only", caps: Caps{GPUCompat: []string{"cuda"}}})
	if _, err := reg.Select(t.Context(), "gpu-only"); err == nil {
		t.Fatal("unavailable 引擎必须被 Select 拒绝（不静默降级）")
	}
	if _, err := reg.Select(t.Context(), "missing"); err != ErrEngineNotFound {
		t.Fatalf("未注册引擎应报 ErrEngineNotFound, got %v", err)
	}
}

// fakeEngine 仅实现接口的最小假引擎。
type fakeEngine struct {
	id   string
	caps Caps
}

func (f *fakeEngine) ID() string          { return f.id }
func (f *fakeEngine) DisplayName() string { return f.id }
func (f *fakeEngine) Kind() Kind          { return KindTTS }
func (f *fakeEngine) Caps() Caps          { return f.caps }
func (f *fakeEngine) Available(context.Context) (bool, string) {
	return true, ""
}
func (f *fakeEngine) Synthesize(context.Context, SynthesisRequest, ChunkSink) (SynthesisResult, error) {
	return SynthesisResult{}, nil
}
