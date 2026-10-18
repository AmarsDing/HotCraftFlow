package mmeworker

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
	"framework/app/admin/internal/core"
	"framework/pkg/mediacontract"
)

// NatsVariantPublisher bizmme.EventPublisher 实现：把审批通过的 VariantSet 契约
// 包装为 EventEnvelope（mm.mme.variant.ready.v1, producer=mme）经 core NATS 发布，
// 杀给 ⑤ MAP 消费（契约 §8 dispatch——“送出去就行，不代表已发布”）。
//
// core 发布无回执但消息会被 MM_MEDIA stream 按 subject 捕获：
// 持久化交给 JetStream，此处仅需连接存活（口径同 miaworker.NatsImpactPublisher）。
type NatsVariantPublisher struct {
	core *core.CoreServer
	log  *log.Helper
}

// NewNatsVariantPublisher 构造。
func NewNatsVariantPublisher(coreSrv *core.CoreServer, logger log.Logger) *NatsVariantPublisher {
	return &NatsVariantPublisher{
		core: coreSrv,
		log:  log.NewHelper(log.With(logger, "module", "worker/mme-variant-pub")),
	}
}

// errNatsUnavailable NATS 未连接的哨兵错（Dispatch 调用方如实报错，事件可手工补发）。
var errNatsUnavailable = errors.New("mme: nats connection unavailable")

// PublishVariantReady 发布变体就绪事件。event_id 取每次 dispatch 新建的 evt_ ULID
// （同一簇重发分发视为新事件，消费侧按 variant_set_id + 时间窗去重）。
func (p *NatsVariantPublisher) PublishVariantReady(ctx context.Context, set *mediacontract.VariantSet, traceID string) error {
	if p.core == nil || p.core.Nats() == nil || p.core.Nats().RawConn() == nil {
		return errNatsUnavailable
	}
	// 🔴 契约硬校验最后一道岗：违约 Payload 不出本系统。
	if set == nil {
		return errors.New("mme: nil variant set")
	}
	if err := set.Validate(); err != nil {
		return err
	}

	env, err := mediacontract.NewEnvelope("mme", mediacontract.SubjectMMEVariantReady, traceID, set)
	if err != nil {
		return err
	}

	return p.core.Publish(mediacontract.SubjectMMEVariantReady, env)
}

var _ bizmme.EventPublisher = (*NatsVariantPublisher)(nil)
