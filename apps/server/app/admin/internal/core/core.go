package core

import (
	"context"
	"slices"
	"sync"

	"framework/app/admin/internal/conf"
	"framework/pkg/events"
	pkgnats "framework/pkg/nats"
	"framework/pkg/natsx"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/google/wire"
	"github.com/nats-io/nats.go"
)

var ProviderSet = wire.NewSet(
	NewCoreServer,
	NewEventPublisher,
)

// 编译期校验 CoreServer 实现 kratos transport.Server 接口
var _ transport.Server = (*CoreServer)(nil)

// CoreServer 承载 admin 服务的 NATS 运行时。
//
// 设计说明（与 v1/demo 的差异）：
//   - admin 服务独立运行，不走 cfgcenter，NATS 配置来自本地 YAML
//   - 不实现 v1/demo 的心跳主备选举（admin 单实例部署）
//   - 预留订阅注册 API，供 service 层按需调用（业务消息预留）
//   - 实现 kratos.Server 接口，由 kratos.App 统一管理生命周期
type CoreServer struct {
	nats *pkgnats.NatServer
	log  *log.Helper
	mu   sync.Mutex
	subs []*nats.Subscription
}

// NewCoreServer 创建 CoreServer。
// 直接接收 *conf.Bootstrap，内部调用 bc.GetNats() 取公共配置，
// 避免 wire 中写 provideNats 这种 boilerplate provider。
func NewCoreServer(bc *conf.Bootstrap, logger log.Logger) (*CoreServer, func(), error) {
	helper := log.NewHelper(log.With(logger, "module", "core"))

	ns, err := pkgnats.NewNats(bc.GetNats(), nats.JSON_ENCODER, logger)
	if err != nil {
		return nil, nil, err
	}

	cs := &CoreServer{
		nats: ns,
		log:  helper,
		subs: make([]*nats.Subscription, 0),
	}

	return cs, func() {
		cs.Close()
	}, nil
}

// Start 启动 CoreServer（NATS 连接已在 NewNats 时建立，此处仅记录日志）。
func (cs *CoreServer) Start(ctx context.Context) error {
	cs.log.Infof("core server started, nats connected: %s", cs.nats.RawConn().ConnectedUrl())
	cs.ensureStreams("SECURITY", "SCHEDULE")
	return nil
}

// ensureStreams 启动期幂等确保指定 stream 存在。
// admin 经 core NATS 发布 security.login.abuse / schedule.changed：无 stream 捕获时消息静默丢失
// （core 发布无回执，不触发重试队列）。与订阅侧互为冗余，任一先生效链路即恢复。
// 失败仅告警不阻塞启动（与发布降级语义一致），NATS 恢复后由重连/重订阅兜底。
func (cs *CoreServer) ensureStreams(names ...string) {
	js, err := cs.nats.RawConn().JetStream()
	if err != nil {
		cs.log.Warnf("get jetstream context failed (skip stream ensure): %v", err)
		return
	}
	for _, def := range events.StreamDefs {
		if !slices.Contains(names, def.Name) {
			continue
		}
		if err := natsx.EnsureStream(js, def, cs.log); err != nil {
			cs.log.Warnf("ensure stream %s failed (events may drop until nats_setup reruns): %v", def.Name, err)
		}
	}
}

// Stop 停止 CoreServer，关闭 NATS 连接。
func (cs *CoreServer) Stop(ctx context.Context) error {
	cs.Close()
	return nil
}

// Close 释放资源（与 cleanup 函数复用）。
func (cs *CoreServer) Close() {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	for _, sub := range cs.subs {
		if sub != nil && sub.IsValid() {
			_ = sub.Unsubscribe()
		}
	}
	cs.subs = cs.subs[:0]

	if cs.nats != nil {
		cs.nats.Close()
	}
}

// Nats 返回底层 NATS 连接封装，供 service 层发布消息使用。
func (cs *CoreServer) Nats() *pkgnats.NatServer {
	return cs.nats
}

// Subscribe 注册订阅。返回订阅句柄，由 CoreServer 统一管理生命周期。
func (cs *CoreServer) Subscribe(topic string, handler func(*nats.Msg)) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	sub, err := cs.nats.RegistSub(topic, handler)
	if err != nil {
		return err
	}
	cs.subs = append(cs.subs, sub)
	return nil
}

// QueueSubscribe 注册队列订阅（多实例负载均衡）。
func (cs *CoreServer) QueueSubscribe(topic, queue string, handler func(*nats.Msg)) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	sub, err := cs.nats.QueueSubscribe(topic, queue, handler)
	if err != nil {
		return err
	}
	cs.subs = append(cs.subs, sub)
	return nil
}

// Publish 发布消息到指定 topic。
func (cs *CoreServer) Publish(topic string, msg interface{}) error {
	return cs.nats.Publish(topic, msg)
}
