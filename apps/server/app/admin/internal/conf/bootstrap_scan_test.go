package conf

import (
	"path/filepath"
	"testing"

	"github.com/go-kratos/kratos/v2/config"
	"github.com/go-kratos/kratos/v2/config/file"
)

// TestBootstrapScanAllConfigs 启动同款 c.Scan(&Bootstrap) 回归（配置格式红线守卫）。
// 历史事故：yaml 写 Go 时长字面量（15m/24h/30m），而 proto google.protobuf.Duration
// 只收 protojson 秒格式（900s/86400s/1800s）——c.Scan 逐个 panic，整栈起不来。
// 本测试以真实配置文件全量扫描复现启动路径；任何新加配置违反秒格式纪律即红。
// 注意：tt/map 子系统是 Go 结构体 yaml 解析（非 proto），允许 Go 时长字面量——
// 它们不经过 Bootstrap 扫描，故不在本测试约束面内。
func TestBootstrapScanAllConfigs(t *testing.T) {
	// 相对本测试文件解析到仓库真实配置目录：
	// internal/conf → admin → app → server（common 在 server/conf，private 在 admin/config）
	base, err := filepath.Abs("..") // app/admin/internal
	if err != nil {
		t.Fatal(err)
	}
	private := filepath.Join(base, "..", "config")          // app/admin/config
	common := filepath.Join(base, "..", "..", "..", "conf") // apps/server/conf

	c := config.New(
		config.WithSource(
			file.NewSource(common),
			file.NewSource(private),
		),
	)
	defer c.Close()
	if err := c.Load(); err != nil {
		t.Fatalf("config load: %v", err)
	}

	var bc Bootstrap
	if err := c.Scan(&bc); err != nil {
		t.Fatalf("Bootstrap scan（启动路径）失败——检查是否有 proto Duration 字段被写成 Go 时长字面量（如 30m/24h）；一律改秒格式（如 1800s/86400s）: %v", err)
	}

	// 关键字段抽验：确认 IC 段实际进入扫描结果（防配置键漂移静默脱钩）
	if bc.Ic == nil {
		t.Fatal("Bootstrap.Ic 为空——ic.yaml 未纳入扫描，回归失效")
	}
	if got := bc.Ic.GetRateLimit().GetBackoffMax().AsDuration().Seconds(); got != 900 {
		t.Fatalf("ic.rate_limit.backoff_max = %vs，期望 900s（秒格式纪律）", got)
	}
}
