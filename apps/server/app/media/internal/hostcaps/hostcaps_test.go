package hostcaps

import (
	"runtime"
	"testing"
)

// TestDetectEnvOverride 环境变量整体覆盖探测结果（容器/特殊硬件的逃生口）。
func TestDetectEnvOverride(t *testing.T) {
	cases := []struct {
		env  string
		want string
	}{
		{"cuda", CUDA},
		{"rocm", ROCm},
		{"metal", Metal},
		{"none", None},
		{" CUDA ", CUDA},   // trim + lower
		{"garbage", None},  // 非法值降级 none（不误报有卡）
	}
	for _, c := range cases {
		t.Run(c.env, func(t *testing.T) {
			t.Setenv(EnvOverride, c.env)
			got := Detect()
			if got.Accel != c.want {
				t.Fatalf("env=%q → %s, want %s", c.env, got.Accel, c.want)
			}
			if got.OS != runtime.GOOS {
				t.Fatalf("OS 字段应为 %s: %s", runtime.GOOS, got.OS)
			}
		})
	}
}

// TestDetectWithoutOverride 无覆盖时：结果必须落在词表内且与平台自洽。
// （darwin/arm64 必 metal；其余看宿主 binary——本测试只断言词表合法性，
// 不假设 CI 有无 nvidia-smi。）
func TestDetectWithoutOverride(t *testing.T) {
	t.Setenv(EnvOverride, "")
	got := Detect()
	switch got.Accel {
	case CUDA, ROCm, Metal, None:
	default:
		t.Fatalf("非法 accel: %q", got.Accel)
	}
	if runtime.GOOS == "darwin" && runtime.GOARCH == "arm64" && got.Accel != Metal {
		t.Fatalf("Apple Silicon 必须 metal: %s", got.Accel)
	}
}
