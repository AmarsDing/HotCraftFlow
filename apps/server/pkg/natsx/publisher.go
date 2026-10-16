// Package natsx 提供全站统一的 JetStream 事件发布器，收敛此前 12 处复制粘贴的
// publisher 实现（连接建立、统一信封封装、1s/2s/4s 指数退避重试、可选 gzip、stream ensure）。
//
// 用法（服务方保持自有 Publisher 类型与端口接口，仅内部委托本包）：
//
//	pub, cleanup, err := natsx.NewPublisher(cfg.Nats.GetUrl(), logger,
//	    natsx.WithModule("biz/metar-publisher"))
//	...
//	err = pub.Publish(ctx, events.SubjectMetarData, reportID, payload)
//
// 语义对齐历史实现：NATS 未配置/连接失败时降级为空发布器（仅 Warn，不阻塞服务启动）。
package natsx

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/pkg/events"
)

// 默认指数退避序列（全站历史统一口径：1s → 2s → 4s，共 4 次尝试）。
var defaultBackoffs = []time.Duration{1 * time.Second, 2 * time.Second, 4 * time.Second}

// Option 发布器选项。
type Option func(*Publisher)

// WithCompress 开启 gzip 压缩（消费端须用 events.Decode 解码，其自动嗅探解压）。
func WithCompress(compress bool) Option {
	return func(p *Publisher) { p.compress = compress }
}

// WithBestEffort 设置为尽力发布：单次尝试、失败仅 Warn 返回错误，不做退避重试。
// 适用于事件为辅助链路、不允许阻塞主流程的服务（aiops/video 历史口径）。
func WithBestEffort() Option {
	return func(p *Publisher) { p.bestEffort = true }
}

// WithRetryOnFailedConnect 连接建立策略：NATS 暂不可用时进入重连态而非直接失败
//（collector/video 历史口径；其余服务为启动期一次性连接，失败即降级）。
func WithRetryOnFailedConnect() Option {
	return func(p *Publisher) { p.retryConnect = true }
}

// WithModule 覆盖日志 module 名（默认 "natsx/publisher"）。
func WithModule(module string) Option {
	return func(p *Publisher) { p.module = module }
}

// Publisher JetStream 事件发布器。js 为 nil 时所有 Publish 为 no-op（降级态）。
type Publisher struct {
	nc           *natsgo.Conn
	js           natsgo.JetStreamContext
	compress     bool
	bestEffort   bool
	retryConnect bool
	module       string
	log          *log.Helper
}

// NewPublisher 创建发布器。natsURL 为空或连接失败时返回降级发布器（nil js），
// 与历史各服务 "NATS 不可用仅 Warn 不阻塞" 语义一致；仅 JetStream 上下文获取失败才返回 error。
func NewPublisher(natsURL string, logger log.Logger, opts ...Option) (*Publisher, func(), error) {
	p := &Publisher{module: "natsx/publisher"}
	for _, opt := range opts {
		opt(p)
	}
	p.log = log.NewHelper(log.With(logger, "module", p.module))

	noop := func() {}
	if natsURL == "" {
		p.log.Warn("nats not configured, event publishing will be skipped")
		return p, noop, nil
	}

	connOpts := []natsgo.Option{
		natsgo.MaxReconnects(-1),
		natsgo.ReconnectWait(3 * time.Second),
	}
	if p.retryConnect {
		connOpts = append(connOpts, natsgo.RetryOnFailedConnect(true), natsgo.ReconnectWait(2*time.Second))
	}
	nc, err := natsgo.Connect(natsURL, connOpts...)
	if err != nil {
		p.log.Warnf("connect nats failed (event publishing will be skipped): %v", err)
		return p, noop, nil
	}

	js, err := nc.JetStream()
	if err != nil {
		nc.Close()
		return nil, noop, fmt.Errorf("get jetstream context: %w", err)
	}

	p.nc, p.js = nc, js
	return p, func() { nc.Close() }, nil
}

// Enabled 报告发布器是否处于可用态（持有 JetStream 上下文）。
func (p *Publisher) Enabled() bool { return p.js != nil }

// JS 暴露底层 JetStreamContext（降级态为 nil），供启动期 EnsureStream 等场景使用。
func (p *Publisher) JS() natsgo.JetStreamContext { return p.js }

// Conn 暴露底层 core NATS 连接（降级态为 nil），供非持久化广播（awos.config.changed 等
// core NATS 语义主题）直接 Publish 使用。
func (p *Publisher) Conn() *natsgo.Conn { return p.nc }

// PublishCore 以 core NATS 语义发布（不持久化、不流捕获）：用于配置变更等瞬时广播。
// 信封编码与 Publish 一致（v1.0，可选 gzip）；降级态为 no-op。
func (p *Publisher) PublishCore(ctx context.Context, subject string, assetID uint32, payload map[string]any) error {
	if p.nc == nil {
		return nil
	}
	msg, err := events.Encode(events.New(subject, assetID, payload), p.compress)
	if err != nil {
		return err
	}
	if err := p.nc.Publish(subject, msg); err != nil {
		p.log.WithContext(ctx).Warnf("nats core publish %s failed: %v", subject, err)
		return err
	}
	return nil
}

// Publish 以 v1.0 信封发布（EventType=subject，EventID/时间自动生成）。
func (p *Publisher) Publish(ctx context.Context, subject string, assetID uint32, payload map[string]any) error {
	return p.PublishEvent(ctx, events.New(subject, assetID, payload))
}

// PublishEvent 发布完整信封（可指定版本/时间/操作人，alarm.events 等 v2.0 事件走此入口）。
// 投递 subject 取 env.EventType；subject 与 EventType 不一致的场景用 PublishEventAs。
func (p *Publisher) PublishEvent(ctx context.Context, env *events.Envelope) error {
	if env == nil {
		return nil
	}
	return p.PublishEventAs(ctx, env.EventType, env)
}

// PublishEventAs 将信封投递到显式指定的 subject（信封 EventType 保持原值不改写）。
// 适用于 remediation.events 这类"传输 subject 与业务 event_type 分离"的事件。
func (p *Publisher) PublishEventAs(ctx context.Context, subject string, env *events.Envelope) error {
	if p.js == nil || env == nil {
		return nil
	}
	msg, err := events.Encode(env, p.compress)
	if err != nil {
		return err
	}
	return p.publishRaw(ctx, subject, msg)
}

// publishRaw 按策略发布已编码消息：bestEffort 单次尝试，否则 1/2/4s 退避重试。
func (p *Publisher) publishRaw(ctx context.Context, subject string, msg []byte) error {
	if p.bestEffort {
		if _, err := p.js.Publish(subject, msg); err != nil {
			p.log.WithContext(ctx).Warnf("nats publish %s failed: %v", subject, err)
			return err
		}
		return nil
	}

	for i := 0; i <= len(defaultBackoffs); i++ {
		if _, err := p.js.Publish(subject, msg); err == nil {
			return nil
		} else {
			p.log.Warnf("nats publish %s attempt %d failed: %v", subject, i+1, err)
		}
		if i < len(defaultBackoffs) {
			select {
			case <-time.After(defaultBackoffs[i]):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	p.log.Errorf("nats publish %s failed after retries, data lost", subject)
	return fmt.Errorf("nats publish %s failed after retries", subject)
}

// EnsureStream 幂等确保 stream：不存在则创建，存在且缺 subject 则增量补齐。
// 对齐历史 collector/video ensureStream 语义；stream 词表统一取 events.StreamDefs。
func EnsureStream(js natsgo.JetStreamContext, def events.StreamDef, l *log.Helper) error {
	stream, err := js.StreamInfo(def.Name)
	if err != nil {
		if err != natsgo.ErrStreamNotFound {
			return fmt.Errorf("query stream info: %w", err)
		}
		if _, err = js.AddStream(def.ToNatsConfig()); err != nil {
			return fmt.Errorf("add stream: %w", err)
		}
		l.Infof("jetstream stream created: %s (subjects=%v)", def.Name, def.Subjects)
		return nil
	}

	missing := make([]string, 0, len(def.Subjects))
	for _, want := range def.Subjects {
		found := false
		for _, have := range stream.Config.Subjects {
			if have == want {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, want)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	cfg := stream.Config
	cfg.Subjects = append(append([]string{}, cfg.Subjects...), missing...)
	if _, err = js.UpdateStream(&cfg); err != nil {
		return fmt.Errorf("update stream subjects (+%v): %w", missing, err)
	}
	l.Infof("jetstream stream updated: %s (added subjects=%v)", def.Name, missing)
	return nil
}
