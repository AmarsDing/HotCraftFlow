package miaworker

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"

	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/core"
	"framework/pkg/mediacontract"
)

// NatsImpactPublisher biz/mia.ImpactPublisher 实现：把影响卡包装为
// EventEnvelope（mm.impact.card.v1, producer=mia）经 core NATS 发布。
// 🔴 发布体含 disclaimer（与落库同口径，NFR-MIA-01 贯通全链路）。
//
// core 发布无回执但消息会被 MM_IMPACT stream 按 subject 捕获：
// 持久化交给 JetStream，此处仅需连接存活（口径同 ttworker.NatsSnapshotPublisher）。
type NatsImpactPublisher struct {
	core *core.CoreServer
	log  *log.Helper
}

// NewNatsImpactPublisher 构造。
func NewNatsImpactPublisher(coreSrv *core.CoreServer, logger log.Logger) *NatsImpactPublisher {
	return &NatsImpactPublisher{
		core: coreSrv,
		log:  log.NewHelper(log.With(logger, "module", "worker/mia-impact-pub")),
	}
}

// errNatsUnavailable NATS 未连接的哨兵错（上游打 Warn、不阻断主链路——卡已落库可补发）。
var errNatsUnavailable = errors.New("mia: nats connection unavailable")

// PublishImpactCard 发布影响卡事件。卡内 impact_id 兼作幂等键复用：
// 消费侧按 impact_id 去重，包身 event_id 与之一致避免双层 ID 漂移。
func (p *NatsImpactPublisher) PublishImpactCard(ctx context.Context, card *mediacontract.ImpactCard, traceID string) error {
	if p.core == nil || p.core.Nats() == nil || p.core.Nats().RawConn() == nil {
		return errNatsUnavailable
	}
	// 🔴 契约硬校验最后一道岗：违约卡不得出本系统（空 disclaimer / 缺 path 拒发）。
	if card == nil {
		return errors.New("mia: nil impact card")
	}
	if err := card.Validate(); err != nil {
		return err
	}

	env, err := mediacontract.NewEnvelope("mia", mediacontract.SubjectImpactCard, traceID, card)
	if err != nil {
		return err
	}
	env.EventID = card.ImpactID
	env.OccurredAt = card.Event.OccurredAt.UTC()

	return p.core.Publish(mediacontract.SubjectImpactCard, env)
}

// EnsureImpactStream 启动期确保 MM_IMPACT stream 存在（订阅侧/发布侧任一先来均可）。
func (p *NatsImpactPublisher) EnsureImpactStream() {
	if p.core == nil || p.core.Nats() == nil || p.core.Nats().RawConn() == nil {
		return
	}
	js, err := p.core.Nats().RawConn().JetStream()
	if err != nil {
		return
	}
	ensureStreamsWith(js, p.log, mediacontract.StreamImpact)
}

var _ bizmia.ImpactPublisher = (*NatsImpactPublisher)(nil)
