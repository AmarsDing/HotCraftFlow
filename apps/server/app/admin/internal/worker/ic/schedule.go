package icworker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	"framework/app/admin/internal/biz"
)

// =============================================================================
// IC 调度器（S-13：asynq 周期调度；B-11：interval_minutes 为唯一调度真值，cron 只是载体）。
//
// 结构：
//   - 信源采集任务：taskID = "ic:collect:"{sourceID}，cron = "@every {interval}m"，
//     启停/增删源经 reconcile 差分同步（评审 gap#19 变更路径）：60s 轮询 ListAllEnabled，
//     interval 变更即重建周期任务；
//   - 内建任务（固定 cadence，B-25/S-2/NFR-IC-08 钉死）：重发 / 闭簇 / 保留期清扫。
//
// 处理器注册必须在构造期完成（asynq handleFunc 禁止 Start 后注册，与 TaskWorker 同纪律）。
// asynq 未配置时：处理器不注册、Start 直接降级（任务 CRUD 与 API 不受影响）。
// =============================================================================

// icCollectPayload 信源采集任务载荷。
type icCollectPayload struct {
	SourceID string `json:"source_id"`
}

// icJobPayload 内建任务载荷（占位结构，保证 codec 可序列化）。
type icJobPayload struct{}

// registeredCron 已登记的采集任务（interval 变化判定重建用）。
type registeredCron struct {
	intervalMinutes int32
}

type Scheduler struct {
	asynq     *asynqServer.Server
	cfg       *biz.IcConfig
	sources   biz.IntelSourceRepo
	intel     biz.IntelItemRepo
	clusters  biz.IntelClusterRepo
	evidence  biz.EvidenceRepo
	store     biz.EvidenceStorePort
	collect   *biz.IcCollectUsecase
	normalize *biz.IcNormalizeUsecase
	log       *log.Helper

	mu         sync.Mutex
	registered map[string]registeredCron
	stopCh     chan struct{}
	doneCh     chan struct{}
}

func NewScheduler(
	asynqSrv *asynqServer.Server,
	cfg *biz.IcConfig,
	sources biz.IntelSourceRepo,
	intel biz.IntelItemRepo,
	clusters biz.IntelClusterRepo,
	evidence biz.EvidenceRepo,
	store biz.EvidenceStorePort,
	collect *biz.IcCollectUsecase,
	normalize *biz.IcNormalizeUsecase,
	logger log.Logger,
) *Scheduler {
	s := &Scheduler{
		asynq:      asynqSrv,
		cfg:        cfg,
		sources:    sources,
		intel:      intel,
		clusters:   clusters,
		evidence:   evidence,
		store:      store,
		collect:    collect,
		normalize:  normalize,
		log:        log.NewHelper(log.With(logger, "module", "worker/ic-scheduler")),
		registered: map[string]registeredCron{},
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
	if asynqSrv != nil {
		s.registerHandlers(asynqSrv)
	}
	return s
}

// registerHandlers 构造期全量注册处理器（payload 反序列化由 codec 保障）。
func (s *Scheduler) registerHandlers(srv *asynqServer.Server) {
	must := func(name string, err error) {
		if err != nil {
			panic("register ic subscriber " + name + " failed: " + err.Error())
		}
	}

	must(taskCollect, asynqServer.RegisterSubscriberWithCtx(srv, taskCollect,
		func(ctx context.Context, _ string, p *icCollectPayload) error {
			if p.SourceID == "" {
				return fmt.Errorf("ic.collect payload missing source_id")
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Minute) // 单轮采集上限
			defer cancel()
			if _, err := s.collect.RunSource(ctx, p.SourceID, "cron"); err != nil {
				// 熔断/限流/不可达：失败计数走 RecordRunOutcome，调度层如实上报供 asynq 重试判定。
				return err
			}
			return nil
		}))

	must(taskRepublishPending, asynqServer.RegisterSubscriberWithCtx(srv, taskRepublishPending,
		func(ctx context.Context, _ string, _ *icJobPayload) error {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			n, err := s.normalize.RepublishPending(ctx, 200)
			if n > 0 {
				s.log.Infof("republished %d pending intel item(s)", n)
			}
			return err
		}))

	must(taskClusterIdleClose, asynqServer.RegisterSubscriberWithCtx(srv, taskClusterIdleClose,
		func(ctx context.Context, _ string, _ *icJobPayload) error {
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			return s.closeIdleClusters(ctx)
		}))

	must(taskRetentionSweep, asynqServer.RegisterSubscriberWithCtx(srv, taskRetentionSweep,
		func(ctx context.Context, _ string, _ *icJobPayload) error {
			ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
			defer cancel()
			return s.retentionSweep(ctx)
		}))
}

// Start 登记全局任务 + 首跑差分 + 60s 热更新轮询。asynq 未配置或 IC 关闭时降级。
func (s *Scheduler) Start(context.Context) error {
	if s.cfg == nil || !s.cfg.Enabled {
		s.log.Info("ic disabled, scheduler not started")
		return nil
	}
	if s.asynq == nil {
		s.log.Warn("asynq not configured, ic scheduler disabled (source CRUD 不受影响)")
		return nil
	}
	if err := s.registerGlobalJobs(); err != nil {
		s.log.Errorf("register global jobs failed: %v", err)
		return err
	}
	if err := s.reconcileSources(); err != nil {
		s.log.Warnf("initial source reconcile failed (重试交给轮询): %v", err)
	}
	go s.reconcileLoop()
	return nil
}

// Stop 停掉差分轮询（周期任务体本身由 asynqSrv 生命周期管理，重投同理）。
func (s *Scheduler) Stop(context.Context) error {
	select {
	case <-s.doneCh:
	default:
		close(s.stopCh)
		<-s.doneCh
	}
	return nil
}

// registerGlobalJobs 内建周期任务（幂等：重复登记的 taskID 相同由 asynq 去重）。
func (s *Scheduler) registerGlobalJobs() error {
	jobs := []struct {
		taskID, cron, typeName string
	}{
		{jobRepublishPending, "@every 30s", taskRepublishPending},         // B-25：发布失败 30s 重试
		{jobClusterIdleClose, "@every 1h", taskClusterIdleClose},          // S-2：48h 闲时闭簇（小时级扫描足够）
		{jobRetentionSweep, s.cfg.RetentionSweepCron, taskRetentionSweep}, // 默认 0 3 * * *
	}
	for _, j := range jobs {
		if _, err := s.asynq.NewPeriodicTask(j.cron, j.taskID, j.typeName, &icJobPayload{}); err != nil {
			return fmt.Errorf("register %s: %w", j.taskID, err)
		}
	}
	return nil
}

// reconcileLoop 60s 差分信源调度（gap#19：新增/停用/改 interval 得即时生效，不重启进程）。
func (s *Scheduler) reconcileLoop() {
	defer close(s.doneCh)
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			if err := s.reconcileSources(); err != nil {
				s.log.Warnf("source reconcile failed: %v", err)
			}
		}
	}
}

// reconcileSources 以 ListAllEnabled 为真值做差分（新增注册 / interval 变更重建 / 消失移除）。
func (s *Scheduler) reconcileSources() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	rows, err := s.sources.ListAllEnabled(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	seen := map[string]bool{}
	for _, src := range rows {
		// 注：paused 信源的周期任务保留注册（熔断器自行恢复，30m 档）；
		// 运行期 RunSource Runnable 早退即可——避免熔断期间频繁增删调度器条目。
		seen[src.ID] = true
		if prev, ok := s.registered[src.ID]; ok && prev.intervalMinutes == src.IntervalMinutes {
			continue
		}
		if _, ok := s.registered[src.ID]; ok {
			_ = s.asynq.RemovePeriodicTask(srcCollectTaskID(src.ID)) // interval 变更重建
		}
		cron := fmt.Sprintf("@every %dm", src.IntervalMinutes)
		if _, err := s.asynq.NewPeriodicTask(cron, srcCollectTaskID(src.ID), taskCollect,
			&icCollectPayload{SourceID: src.ID}); err != nil {
			s.log.Warnf("register collect task failed: source=%s interval=%dm err=%v", src.ID, src.IntervalMinutes, err)
			continue
		}
		s.registered[src.ID] = registeredCron{intervalMinutes: src.IntervalMinutes}
	}
	for id := range s.registered {
		if seen[id] {
			continue
		}
		if err := s.asynq.RemovePeriodicTask(srcCollectTaskID(id)); err != nil {
			s.log.Warnf("remove collect task failed: source=%s err=%v", id, err)
			continue
		}
		delete(s.registered, id)
	}
	return nil
}

func srcCollectTaskID(sourceID string) string { return "ic:collect:" + sourceID }

// closeIdleClusters S-2：last_seen_at 早于阈值的 open 簇关闭；闭簇不再吸成员（dedup 已只查 open）。
func (s *Scheduler) closeIdleClusters(ctx context.Context) error {
	cutoff := time.Now().UTC().Add(-time.Duration(s.cfg.RetentionClusterIdleHours) * time.Hour)
	rows, err := s.clusters.ListIdleOpen(ctx, cutoff, 200)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, c := range rows {
		if err := s.clusters.Close(ctx, c.ID, now); err != nil {
			s.log.Warnf("close idle cluster failed: cluster=%s err=%v", c.ID, err)
			continue
		}
	}
	if n := len(rows); n > 0 {
		s.log.Infof("closed %d idle cluster(s) (cutoff=%s)", n, cutoff.Format(time.RFC3339))
	}
	return nil
}

// retentionSweep NFR-IC-08：intel 与 evidence 双向清扫。
// 纪律：B-17 证据「禁覆盖删除，retention 例外且记审计」——每批删除打审计日志（数量+id 清单头尾）。
func (s *Scheduler) retentionSweep(ctx context.Context) error {
	now := time.Now().UTC()

	// 证据先行（文件+行，二者一致才放行 intel 行删——行删不影响证据引用的话顺序反之亦可；
	// 这里按「先清文件再清 intel」避免孤儿文件）。
	evCutoff := now.Add(-time.Duration(s.cfg.EvidenceRetainDays) * 24 * time.Hour)
	expired, err := s.evidence.ListExpired(ctx, evCutoff, 200)
	if err != nil {
		return err
	}
	var deletedIDs []string
	for _, ev := range expired {
		if err := s.store.Delete(ctx, ev.RawPath); err != nil {
			s.log.Warnf("evidence file delete failed (跳过行删保审计): evidence=%s raw=%s err=%v", ev.ID, ev.RawPath, err)
			continue // 文件删失败不动行，下轮重试
		}
		deletedIDs = append(deletedIDs, ev.ID)
	}
	if len(deletedIDs) > 0 {
		if _, err := s.evidence.DeleteIDs(ctx, deletedIDs); err != nil {
			return err
		}
		first, last := deletedIDs[0], deletedIDs[len(deletedIDs)-1]
		s.log.Infof("[audit] retention evidence deleted: count=%d window=[%s..%s] cutoff=%s job=%s",
			len(deletedIDs), first, last, evCutoff.Format("2006-01-02"), jobRetentionSweep)
	}

	// intel 行（关联 evidence 至此行可能失联——intel 365d > evidence 90d，行在文在；
	// 超 365d 的行已无证据意义，扫行不扫文件由 evidence 扫逻辑独立兜底）。
	inCutoff := now.Add(-time.Duration(s.cfg.RetentionIntelDays) * 24 * time.Hour)
	n, err := s.intel.DeleteBefore(ctx, inCutoff, 500)
	if err != nil {
		return err
	}
	if n > 0 {
		s.log.Infof("[audit] retention intel deleted: count=%d cutoff=%s job=%s",
			n, inCutoff.Format("2006-01-02"), jobRetentionSweep)
	}
	return nil
}
