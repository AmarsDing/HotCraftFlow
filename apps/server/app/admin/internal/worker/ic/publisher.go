package icworker

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"
	natsgo "github.com/nats-io/nats.go"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/core"
	contract "framework/pkg/mediacontract"
)

// errNatsUnavailable NATS 未连接哨兵错（上游按可重试处理：发布失败保持 pending，B-25 重发兜底）。
var errNatsUnavailable = errors.New("ic: nats connection unavailable")

// NatsIntelPublisher biz.IntelPublisherPort 实现。
//
// 与 tt/mia 的 core.Publish 路径刻意不同：B-25 要求 Nats-Msg-Id = intel_id 供
// JetStream 投递去重（dedup window 内重复消息直接 ACK 不入库），core.Publish 的核心
// NATS JSON 发布无此头，故走 RawConn().JetStream().PublishMsg。
// PublishAck 同步回执：超时即视为失败（上游保持 pending，不伪造「已发布」）。
type NatsIntelPublisher struct {
	core *core.CoreServer
	log  *log.Helper
}

func NewNatsIntelPublisher(coreSrv *core.CoreServer, logger log.Logger) *NatsIntelPublisher {
	return &NatsIntelPublisher{
		core: coreSrv,
		log:  log.NewHelper(log.With(logger, "module", "worker/ic-publisher")),
	}
}

// PublishIntel 发布一条契约 subject 消息。subject 由 uesecase 决定
// （mm.intel.raw / mm.intel.normalized.v1 / mm.intel.blocked.v1），此处不做subject 词表判断。
func (p *NatsIntelPublisher) PublishIntel(ctx context.Context, subject string, env *contract.EventEnvelope, msgID string) error {
	if p.core == nil || p.core.Nats() == nil || p.core.Nats().RawConn() == nil {
		return errNatsUnavailable
	}
	if msgID == "" {
		// B-25：无幂等键视同契约违例（宁可失败也不放丢消息）。
		return errors.New("ic: publish msg_id (intel_id) 不能为空")
	}
	data, err := env.Encode()
	if err != nil {
		return err
	}
	js, err := p.core.Nats().RawConn().JetStream()
	if err != nil {
		return err
	}
	msg := &natsgo.Msg{
		Subject: subject,
		Data:    data,
		Header:  natsgo.Header{},
	}
	msg.Header.Set("Nats-Msg-Id", msgID)
	ack, err := js.PublishMsg(msg, natsgo.ExpectStream(contract.StreamIntel), natsgo.Context(ctx))
	if err != nil {
		return err
	}
	if ack.Duplicate {
		// 重复投递被 Stream 吸收：视为成功（幂等语义），仅观测一条。
		p.log.Debugf("duplicate message deduped by stream: subject=%s msg_id=%s", subject, msgID)
	}
	return nil
}

var _ biz.IntelPublisherPort = (*NatsIntelPublisher)(nil)
