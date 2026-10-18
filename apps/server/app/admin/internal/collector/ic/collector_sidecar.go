package iccollector

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/conf"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// sidecar 采集器（录播侧车，V1 实验位）
// 形态：侧车进程把录播产物整理为 JSON 批次暴露在本地 HTTP 端点，
// 采集器按 api 同型映射拉取（复用 jsonwalk；config keys 与 api 采集器一致，
// 另加 endpoint(必)）。授权前提由 S-20 强制（kind=creator & robots_exempt+reason）。
// =============================================================================

// SidecarCollector 侧车批次拉取。
type SidecarCollector struct {
	api *APICollector
	log *log.Helper
}

func NewSidecarCollector(bc *conf.Bootstrap, ic *biz.IcConfig, logger log.Logger) *SidecarCollector {
	return &SidecarCollector{
		api: NewAPICollector(bc, ic, logger),
		log: log.NewHelper(log.With(logger, "module", "ic/collector/sidecar")),
	}
}

func (c *SidecarCollector) Method() string { return string(contract.CollectSidecar) }

func (c *SidecarCollector) Collect(ctx context.Context, src *biz.IntelSourceRow) ([]*biz.RawCollectItem, error) {
	endpoint := asString(src.Config["endpoint"])
	if endpoint == "" {
		return nil, errors.New("sidecar source config.endpoint 必填（侧车批次服务地址）")
	}
	// 借用 api 采集器的映射引擎：把 endpoint 转写为 url 后委托执行。
	shadow := *src
	cfg := make(map[string]interface{}, len(src.Config)+1)
	for k, v := range src.Config {
		cfg[k] = v
	}
	cfg["url"] = endpoint
	shadow.Config = cfg
	return c.api.Collect(ctx, &shadow)
}
