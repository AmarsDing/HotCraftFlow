package observability

import (
	"context"
	"strings"

	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// InitTracerProvider 初始化 OTLP HTTP TracerProvider（Phase 5-5.3：全服务统一接入 Jaeger :4318 OTLP 接收端）。
//
// 用法（仿照 admin 模式）：
//   - 返回的 provider 通过 wire 显式注入到各 server 的 tracing.Server(tracing.WithTracerProvider(tp))，
//     不调用 otel.SetTracerProvider 设置全局 provider；
//   - endpoint 为空时返回 (nil, nil)，kratos tracing 中间件对 nil provider 会 fallback 到全局 noop tracer，
//     即未配置 trace 时服务零影响运行；
//   - endpoint 支持 "host:port" 或 "http(s)://host:port[/path]" 形式（路径忽略，OTLP HTTP 固定 /v1/traces）。
//
// 调用方负责 defer tp.Shutdown(ctx)（tp 非 nil 时）。
func InitTracerProvider(serviceName, endpoint string) (*tracesdk.TracerProvider, error) {
	if endpoint == "" {
		return nil, nil
	}

	ep := endpoint
	insecure := true
	if rest, ok := strings.CutPrefix(ep, "https://"); ok {
		insecure = false
		ep = rest
	} else if rest, ok := strings.CutPrefix(ep, "http://"); ok {
		ep = rest
	}
	if i := strings.IndexByte(ep, '/'); i >= 0 {
		ep = ep[:i]
	}

	opts := []otlptracehttp.Option{otlptracehttp.WithEndpoint(ep)}
	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	exp, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		return nil, err
	}

	return tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewSchemaless(
			semconv.ServiceNameKey.String(serviceName),
		)),
	), nil
}
