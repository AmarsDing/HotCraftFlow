package nats

import (
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/nats-io/nats.go"

	"framework/conf/pb"
)

type NatServer struct {
	nc      *nats.Conn
	enc     *nats.EncodedConn
	encoder string
	log     log.Logger
}

// NewNats 初始化 NATS 连接
func NewNats(conf *pb.Nats, encoder string, logger log.Logger) (*NatServer, error) {
	ns := &NatServer{log: logger}

	nc, err := nats.Connect(conf.GetUrl(),
		nats.DontRandomize(),
		// 重连次数
		nats.MaxReconnects(-1),
		// 重连缓冲区大小 10M = 10 * 1024 * 1024
		nats.ReconnectBufSize(10*1024*1024),
		// 重连等待时间
		nats.ReconnectWait(3*time.Second),
		nats.DisconnectErrHandler(ns.natsDisconnectErrHandler),
		nats.ReconnectHandler(ns.natsReconnectHandler),
		nats.ClosedHandler(ns.natsClosedHandler),
		nats.DiscoveredServersHandler(ns.natsDiscoveredServersHandler),
		nats.ErrorHandler(ns.natsErrorHandler),
	)
	if err != nil {
		return nil, err
	}

	// json = nats.JSON_ENCODER
	// protobuf = protobuf.PROTOBUF_ENCODER
	enc, err := nats.NewEncodedConn(nc, encoder)
	if err != nil {
		return nil, err
	}

	ns.nc = nc
	ns.enc = enc
	ns.encoder = encoder

	return ns, nil
}

// natsDisconnectErrHandler NATS 断开连接错误处理
func (ns *NatServer) natsDisconnectErrHandler(nc *nats.Conn, err error) {
	if err != nil {
		_ = ns.log.Log(log.LevelError, "msg", "nats disconnect error", "err", err.Error())
		return
	}
}

// natsReconnectHandler NATS 重连操作
func (ns *NatServer) natsReconnectHandler(nc *nats.Conn) {
	_ = ns.log.Log(log.LevelInfo, "msg", "nats reconnect", "url", nc.ConnectedUrl())
}

// natsClosedHandler NATS 关闭操作
func (ns *NatServer) natsClosedHandler(nc *nats.Conn) {
	if nc.IsClosed() {
		_ = ns.log.Log(log.LevelError, "msg", "nats disconnect", "url", nc.ConnectedUrl())
		return
	}
	nc.Close()
	_ = ns.log.Log(log.LevelError, "msg", "nats exiting", "err", nc.LastError())
}

// natsDiscoveredServersHandler NATS 发现服务操作
func (ns *NatServer) natsDiscoveredServersHandler(nc *nats.Conn) {
	_ = ns.log.Log(log.LevelInfo, "msg", "nats discovered servers", "url", nc.ConnectedUrl())
}

// natsErrorHandler NATS 操作过程中出现错误处理。
// 异步错误仅记录日志，连接生命周期交由 nats 客户端重连机制管理；
// 不得在此调用 nc.Close()，否则一次 slow consumer 等异步错误即永久关闭连接，
// 与 MaxReconnects(-1) + 10MB 重连缓冲的配置意图相悖。
func (ns *NatServer) natsErrorHandler(nc *nats.Conn, subscription *nats.Subscription, e error) {
	if errors.Is(e, nats.ErrSlowConsumer) {
		// 慢消费者告警：消息被丢弃，提示消费端处理能力不足
		subject := ""
		if subscription != nil {
			subject = subscription.Subject
		}
		_ = ns.log.Log(log.LevelWarn, "msg", "nats slow consumer, messages dropped", "subject", subject, "err", e.Error())
		return
	}
	_ = ns.log.Log(log.LevelError, "msg", "nats error", "err", e.Error())
}

func (ns *NatServer) RawEncodeConn() *nats.EncodedConn {
	return ns.enc
}

func (ns *NatServer) RawConn() *nats.Conn {
	return ns.nc
}

func (ns *NatServer) Publish(topic string, msg interface{}) error {
	if err := ns.enc.Publish(topic, msg); err != nil {
		return err
	}
	if err := ns.enc.Flush(); err != nil {
		return err
	}
	if err := ns.enc.LastError(); err != nil {
		return err
	}
	return nil
}

func (ns *NatServer) RegistSub(topic string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	subscribe, err := ns.enc.Subscribe(topic, handler)
	if err != nil {
		return nil, err
	}
	if err := ns.enc.Flush(); err != nil {
		return nil, err
	}
	return subscribe, nil
}

func (ns *NatServer) QueueSubscribe(topic, cluster string, handler func(*nats.Msg)) (*nats.Subscription, error) {
	subscribe, err := ns.enc.QueueSubscribe(topic, cluster, handler)
	if err != nil {
		return nil, err
	}
	return subscribe, nil
}

func (ns *NatServer) PubReply() {
}

func (ns *NatServer) Close() {
	ns.enc.Close()
	ns.nc.Close()
}
