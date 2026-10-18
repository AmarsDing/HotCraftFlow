// Package publishadapters 是 ⑤ MAP 平台适配器注册表与内置适配器（详细设计 §2）。
//
// 注册表 = 「支持哪些平台」的唯一真值（FR-PUB-01）：
//   - 未注册平台 Get 返回 miss，调用方包装 MAP_ADAPTER_NOT_FOUND 明确报错（ADR-MAP-05，
//     绝不静默降级为「假装已发」「转发其他平台」）。
//   - 真实平台适配器（YouTube/B 站/X/Reddit 等）需凭据/企业资质，V1 未实装——
//     它们自然落在 miss 分支，验收语义「明确报错」即由此满足。
//
// 注册走 init() 自注册（database/sql driver 模式）：全局单例 defaultRegistry，
// wire 经 ProvideRegistry 取同一实例注入 biz。
package publishadapters

import (
	"sort"
	"sync"

	"github.com/go-kratos/kratos/v2/log"

	bizpub "framework/app/admin/internal/biz/publish"
)

// Registry 平台适配器注册表（并发安全）。
type Registry struct {
	mu       sync.RWMutex
	adapters map[string]bizpub.Publisher
	log      *log.Helper
}

// defaultRegistry 全局单例（init() 自注册落点）。
var defaultRegistry = &Registry{adapters: make(map[string]bizpub.Publisher)}

// ProvideRegistry wire provider：返回全局单例（init 注册与 DI 注入的是同一张表）。
func ProvideRegistry(logger log.Logger) *Registry {
	defaultRegistry.mu.Lock()
	defaultRegistry.log = log.NewHelper(log.With(logger, "module", "map/publish-adapters"))
	defaultRegistry.mu.Unlock()
	return defaultRegistry
}

// Register 注册适配器（重复注册同键视为配置错误，打 Warn 覆盖——最后一次生效，便于测试替换）。
func (r *Registry) Register(p bizpub.Publisher) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if p == nil || p.PlatformKey() == "" {
		return
	}
	if _, dup := r.adapters[p.PlatformKey()]; dup && r.log != nil {
		r.log.Warnf("[map-adapters] 平台 %s 适配器重复注册，后者覆盖前者", p.PlatformKey())
	}
	r.adapters[p.PlatformKey()] = p
}

// Get 按平台键取适配器（miss = 未注册，调用方负责包装 MAP_ADAPTER_NOT_FOUND）。
func (r *Registry) Get(key string) (bizpub.Publisher, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.adapters[key]
	return p, ok
}

// Keys 已注册平台键（排序返回，便于诊断/测试断言）。
func (r *Registry) Keys() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.adapters))
	for k := range r.adapters {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
