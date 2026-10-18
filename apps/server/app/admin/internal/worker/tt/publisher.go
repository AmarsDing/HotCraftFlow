package ttworker

import (
	"context"
	"errors"

	"github.com/go-kratos/kratos/v2/log"

	biztt "framework/app/admin/internal/biz/tt"
	"framework/app/admin/internal/core"
	"framework/pkg/mediacontract"
)

// NatsSnapshotPublisher biz/tt.SnapshotPublisher 实现：把快照点包装为
// EventEnvelope（mm.trend.snapshot.v1, producer=tt）经 core NATS 发布。
//
// core 发布无回执但会被 MM_TREND stream 按 subject 捕获（pkg/events/streams.go
// 注释）：持久化交给 JetStream，此处仅需连接存活。
type NatsSnapshotPublisher struct {
	core *core.CoreServer
	log  *log.Helper
}

// NewNatsSnapshotPublisher 构造。
func NewNatsSnapshotPublisher(coreSrv *core.CoreServer, logger log.Logger) *NatsSnapshotPublisher {
	return &NatsSnapshotPublisher{
		core: coreSrv,
		log:  log.NewHelper(log.With(logger, "module", "worker/tt-snapshot-pub")),
	}
}

// errNatsUnavailable NATS 未连接的哨兵错（上游映射为 ResultError → NAK 重投）。
var errNatsUnavailable = errors.New("tt: nats connection unavailable")

// PublishSnapshot 发布快照事件。消息体 SnapshotPublished（内部载荷，消费侧同构解码）。
func (p *NatsSnapshotPublisher) PublishSnapshot(ctx context.Context, point biztt.SnapshotPoint, traceID string) error {
	if p.core == nil || p.core.Nats() == nil || p.core.Nats().RawConn() == nil {
		return errNatsUnavailable
	}

	msg := biztt.SnapshotPublished{
		EventID: mediacontract.NewID("evt"),
		TraceID: traceID,
		Point:   point,
	}
	env, err := mediacontract.NewEnvelope("tt", mediacontract.SubjectTrendSnapshot, msg.TraceID, msg)
	if err != nil {
		return err
	}
	// 消费侧幂等键复用事件 ID：包身与载荷一致，避免双层 ID 漂移。
	env.EventID = msg.EventID

	// occurred_at 取桶时间（事件发生时间）而非投递时间；
	// Publisher 走 core 编码（JSON），payload 内嵌为原生 JSON 对象。
	env.OccurredAt = point.TBucket.UTC()
	return p.core.Publish(mediacontract.SubjectTrendSnapshot, env)
}

// 编译期接口校验。
var _ biztt.SnapshotPublisher = (*NatsSnapshotPublisher)(nil)
