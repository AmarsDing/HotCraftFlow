// Package iccollector 是 ① IC 采集器与外部端口的适配器集合。
// biz 只认端口（biz/ic_ports.go）；本包提供五路采集器与限流/robots/短链/证据落盘实现。
package iccollector

import (
	"sync"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
)

// =============================================================================
// 采集器注册表（评审 S-21：RWMutex 包裹，init 装配后运行期只读；M-5：配置可禁一路）
// =============================================================================

// Registry 实现 biz.CollectorRegistry。
type Registry struct {
	mu         sync.RWMutex
	collectors map[string]biz.Collector
	enabled    map[string]bool
	log        *log.Helper
}

// NewRegistry 装配五路采集器（nil 采集器允许传入——对应外部依赖未部署的降级态，
// Get 依旧按配置启用位返回，避免半装配 panic）。
func NewRegistry(
	bc *conf.Bootstrap,
	hotlist *HotlistCollector,
	api *APICollector,
	rss *RSSCollector,
	crawl *CrawlCollector,
	sidecar *SidecarCollector,
	logger log.Logger,
) *Registry {
	r := &Registry{
		collectors: make(map[string]biz.Collector),
		enabled:    map[string]bool{},
		log:        log.NewHelper(log.With(logger, "module", "ic/collector-registry")),
	}
	r.register("hotlist", hotlist)
	r.register("api", api)
	r.register("rss", rss)
	r.register("crawl", crawl)
	r.register("sidecar", sidecar)

	// M-5：每路独立启停（ic.yaml collectors.*）；配置缺失保守全禁（不悄悄开采集）。
	flags := bc.GetIc().GetCollectors()
	r.enabled["hotlist"] = flags.GetHotlist()
	r.enabled["api"] = flags.GetApi()
	r.enabled["rss"] = flags.GetRss()
	r.enabled["crawl"] = flags.GetCrawl()
	r.enabled["sidecar"] = flags.GetSidecar()
	return r
}

func (r *Registry) register(method string, c biz.Collector) {
	if c == nil {
		r.log.Warnf("collector %s not constructed (adapter degraded)", method)
		return
	}
	r.collectors[method] = c
}

// Get 返回启用且已装配的采集器；未注册/被禁返回 (nil,false)。
func (r *Registry) Get(method string) (biz.Collector, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.enabled[method] {
		return nil, false
	}
	c, ok := r.collectors[method]
	return c, ok
}

// EnabledMethods 观测清单。
func (r *Registry) EnabledMethods() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.collectors))
	for m := range r.collectors {
		if r.enabled[m] {
			out = append(out, m)
		}
	}
	return out
}

var _ biz.CollectorRegistry = (*Registry)(nil)
