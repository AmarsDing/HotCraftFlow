package server

import (
	"github.com/go-kratos/kratos/contrib/registry/discovery/v2"
	"github.com/go-kratos/kratos/v2/registry"

	"framework/app/admin/internal/conf"
)

// NewRegistrar 创建服务注册器（bilibili discovery 基座自动注册工具）。
// 若 Bootstrap 未配置 Discovery 或 nodes 为空，返回 nil（kratos 跳过注册，本地开发豁免）。
// 规范见 docs/开发文档/00-总体方案/04-服务注册与发现规范.md §4.1。
func NewRegistrar(cfg *conf.Bootstrap) registry.Registrar {
	if cfg == nil || cfg.Discovery == nil || len(cfg.Discovery.GetNodes()) == 0 {
		return nil
	}
	d := cfg.Discovery
	return discovery.New(&discovery.Config{
		Nodes:  d.GetNodes(),
		Env:    d.GetEnv(),
		Region: d.GetRegion(),
		Zone:   d.GetZone(),
		Host:   d.GetHost(),
	})
}
