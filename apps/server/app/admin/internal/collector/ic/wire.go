package iccollector

import (
	"github.com/google/wire"

	"framework/app/admin/internal/biz"
)

// ProviderSet collector/ic 端口适配器注入清单。
// 依赖倒置收口：biz 只认 ic_ports.go 声明的接口，本包是唯一业务端实现装配点；
// V1 显式降级实现（DisabledEmbedder/DisabledLLMEnhancer）经 Bind 暴露端口，
// 未来替换推理适配器（bge/ollama）只动这里，biz 零改动。
var ProviderSet = wire.NewSet(
	NewHotlistCollector,
	NewAPICollector,
	NewRSSCollector,
	NewCrawlCollector,
	NewSidecarCollector,
	NewRegistry,

	NewRedisRateLimiter,
	NewRobotsChecker,
	NewShortlinkExpander,
	NewFSEvidenceStore,
	NewEmbedder,
	NewLLMEnhancer,

	wire.Bind(new(biz.CollectorRegistry), new(*Registry)),
	wire.Bind(new(biz.RateLimiterPort), new(*RedisRateLimiter)),
	wire.Bind(new(biz.RobotsCheckerPort), new(*RobotsChecker)),
	wire.Bind(new(biz.ShortlinkExpanderPort), new(*ShortlinkExpander)),
	wire.Bind(new(biz.EvidenceStorePort), new(*FSEvidenceStore)),
	wire.Bind(new(biz.EmbedderPort), new(*DisabledEmbedder)),
	wire.Bind(new(biz.LLMEnhancePort), new(*DisabledLLMEnhancer)),
)
