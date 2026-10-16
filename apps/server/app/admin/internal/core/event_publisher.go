package core

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
)

type natsEventPublisher struct {
	cs  *CoreServer
	log *log.Helper
}

func NewEventPublisher(cs *CoreServer, logger log.Logger) biz.EventPublisher {
	return &natsEventPublisher{
		cs:  cs,
		log: log.NewHelper(log.With(logger, "module", "event-publisher")),
	}
}

func (p *natsEventPublisher) PublishAssetEvent(_ context.Context, event *biz.AssetEvent) error {
	return p.cs.Publish(event.EventType, event)
}
