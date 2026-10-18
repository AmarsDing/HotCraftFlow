package worker

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/transport"

	"framework/app/admin/internal/biz"
	"framework/pkg/retention"
)

// 盯盘事件保留策略：单用户最多 200 条（写入时裁剪）+ 全局 30 天物理清理，每日低峰执行。
const (
	radarEventRetentionDays    = 30
	radarEventRetentionCron    = "41 4 * * *"
	radarEventRetentionTimeout = 10 * time.Minute
)

var _ transport.Server = (*RadarEventRetentionWorker)(nil)

// RadarEventRetentionWorker 盯盘事件保留期清理 worker：
// 每日物理删除 create_time 早于 30 天的 radar_watch_events（单用户 200 条上限由写入路径保证）。
type RadarEventRetentionWorker struct {
	eventRepo biz.RadarWatchEventRepo
	inner     *retention.Worker
	log       *log.Helper
}

func NewRadarEventRetentionWorker(eventRepo biz.RadarWatchEventRepo, logger log.Logger) *RadarEventRetentionWorker {
	w := &RadarEventRetentionWorker{
		eventRepo: eventRepo,
		log:       log.NewHelper(log.With(logger, "module", "radar-event-retention-worker")),
	}
	w.inner = retention.New(retention.Config{
		Enabled:    true,
		Module:     "radar-event-retention-worker",
		Cron:       radarEventRetentionCron,
		Timeout:    radarEventRetentionTimeout,
		RunOnStart: true,
		Job:        w.cleanup,
	}, logger)
	return w
}

func (w *RadarEventRetentionWorker) Start(ctx context.Context) error { return w.inner.Start(ctx) }
func (w *RadarEventRetentionWorker) Stop(ctx context.Context) error  { return w.inner.Stop(ctx) }

func (w *RadarEventRetentionWorker) cleanup(ctx context.Context) {
	cutoff := retention.Cutoff(radarEventRetentionDays)
	deleted, err := w.eventRepo.DeleteOlderThan(ctx, cutoff)
	if err != nil {
		w.log.Errorf("delete expired radar watch events failed: %s", err.Error())
		return
	}
	if deleted > 0 {
		w.log.Infof("radar watch event retention done: deleted=%d cutoff=%s", deleted, cutoff.Format(time.RFC3339))
	}
}
