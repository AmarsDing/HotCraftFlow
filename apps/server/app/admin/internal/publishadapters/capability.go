package publishadapters

import (
	"github.com/go-kratos/kratos/v2/log"

	bizpub "framework/app/admin/internal/biz/publish"
)

// PlatformConf 单平台能力配置（对应 app/admin/config/platforms.yaml 的 `map_platforms.<key>` 段）。
//
// 与适配器注册表的关系：注册表回答「能不能发」（V1 仅 mock 可发），
// 本配置回答「平台长什么样」（能力约束/资质要求/默认可见性）——渠道建档时做快照。
// 🔴 安全：visibility_default 恒 private；requires_qualification=true 的国内平台
// （douyin/kuaishou/xiaohongshu）标注企业资质要求，未授权不发。
type PlatformConf struct {
	// Enabled 运营可用开关（false = 平台不支持/未开放，如 wechat_channels 无开放发布 API）。
	Enabled bool `json:"enabled"`
	// RequiresQualification 需企业资质/官方授权（发布前须线下取得）。
	RequiresQualification bool `json:"requires_qualification"`
	// Note 运营提示（为什么不支持/需要什么资质）。
	Note string `json:"note"`
	// VisibilityDefault 默认可见性（🔴 恒 private，NFR-MAP-01）。
	VisibilityDefault string `json:"visibility_default"`
	// Capability 平台能力约束（键名对齐 bizpub.Capability json tags）。
	Capability bizpub.Capability `json:"capability"`
}

// PlatformConfMap 平台能力配置表（yaml merge key `map_platforms`，main.go Scan 注入）。
type PlatformConfMap map[string]PlatformConf

// PlatformCatalog 能力目录：注册表（适配器声明）优先，platforms.yaml 配置兜底。
// 实现 bizpub.CapabilityProvider（渠道建档快照的数据源）。
type PlatformCatalog struct {
	registry *Registry
	confs    PlatformConfMap
	log      *log.Helper
}

// NewPlatformCatalog 构造。
func NewPlatformCatalog(registry *Registry, confs PlatformConfMap, logger log.Logger) *PlatformCatalog {
	if confs == nil {
		confs = PlatformConfMap{}
	}
	return &PlatformCatalog{
		registry: registry,
		confs:    confs,
		log:      log.NewHelper(log.With(logger, "module", "map/platform-catalog")),
	}
}

// CapabilityFor 平台能力视图（注册适配器声明优先；未注册但配置存在 → 配置值；全无 → miss）。
func (c *PlatformCatalog) CapabilityFor(platformKey string) (bizpub.Capability, bool) {
	if c.registry != nil {
		if p, ok := c.registry.Get(platformKey); ok {
			return p.Capability(), true
		}
	}
	if conf, ok := c.confs[platformKey]; ok && conf.Enabled {
		return conf.Capability, true
	}
	return bizpub.Capability{}, false
}

// ConfOf 取平台运营配置（requires_qualification/note/enabled），供渠道建档提示与测试。
func (c *PlatformCatalog) ConfOf(platformKey string) (PlatformConf, bool) {
	conf, ok := c.confs[platformKey]
	return conf, ok
}
