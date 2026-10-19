// Package mapworker ⑤ MAP 媒体自动化发布矩阵的异步运行时
// （M4：本机到点发 + 重试/死信；M6：表现回流周期拉取，见 analytics_worker.go）。
//
// 设计依据：详细设计 §4 发布执行流程 + §7 排期双模式 + ADR-MAP-11（到点代发身份裁决）。
// 🔴 ADR-MAP-11：worker 到点发布的 operator = 审批人（human actor）——审批本身即对
// 「内容 + 可见性 + 目标 + 时间窗」的完整授权，worker 只做时间触发器，不改任一人决
// 参数；门永远只认 human operator，禁自动化发布的红线不破。真相可查：publish 审计
// 由 worker 追加一条 reason="via=scheduler" 的伴随记录，且 IP/UA 为空。
//
// 结构（复用共享 asynq server——任务短平快、量级低，不像 MME 长渲染需要独立队列）：
//   - 周期扫描 map.publish.due-scan（30s）：local_worker + approved + pending/failed +
//     scheduled_for ≤ now+jitter 的单元 → 入队单发任务（TaskID=unit 去重）；
//   - 单发执行 map.publish.due：状态复核（Cancel/改态天然吊销）→ PublishUsecase.Publish；
//     可重试错误交给 asynq 重投（MaxRetry=conf.retry.max_attempts，指数退避用 asynq
//     默认 RetryDelayFunc——精确 base_delay 语义见 ADR-MAP-11 注4），耗尽写 deadletter
//     审计 + 日志告警 + 草稿保留（失败态不回退）。
//
// 🔴 红线继承：worker 不触碰凭据（解密/用完清零全在 PublishUsecase 内）；approval 缺失
// 即数据异常——跳过 + 告警，绝不自造审批人。local_worker_enabled=false 为运维级总闸
// （扫描注册与其内部双层把关）。
package mapworker

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/hibiken/asynq"
	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	algopublisher "framework/app/admin/internal/algo/publisher"
	bizpub "framework/app/admin/internal/biz/publish"
	"framework/pkg/mediacontract"
)

// 任务类型词表（入队侧与消费侧唯一真相在此）。
const (
	// TaskDueScan 周期扫描任务（30s，登记于 Start）。
	TaskDueScan = "map.publish.due-scan"
	// TaskPublishDue 单发任务：到点执行一个分发单元。
	TaskPublishDue = "map.publish.due"

	// jobDueScan 周期调度器内的登记 ID（幂等重建锚点）。
	jobDueScan = "map:due-scan"
	// scanCron 到点扫描节奏（错过时间点会延迟一个周期——概要设计 §7 已声明可接受）。
	scanCron = "@every 30s"
	// scanBatch 单轮扫描上限（行政后台量级充裕）。
	scanBatch = 64
	// taskTimeout 单次执行上限（适配器 HTTP 调用 + DB 重试余量）。
	taskTimeout = 2 * time.Minute
	// taskRetention 完成后任务在 Redis 的留存（排错线索，asynq 面板可查）。
	taskRetention = 24 * time.Hour
)

// dueScanPayload / duePayload 载荷（json codec 序列化；空构体仅保 codec 可序列化）。
type dueScanPayload struct{}

type duePayload struct {
	UnitID string `json:"unit_id"`
}

// PubRunner 发布执行端口（*bizpub.PublishUsecase 满足；单测注入替身）。
type PubRunner interface {
	Publish(ctx context.Context, unitID string, operator bizpub.Actor, confirmPublic bool, meta bizpub.AuditMeta) (*bizpub.PublishOutcome, error)
}

// ScheduleWorker 排期 worker（transport.Server 生命周期；asynq 未配置 / worker
// 总闸关闭时全面降级——扫描不注册、执行器虽注册但永远收不到任务）。
type ScheduleWorker struct {
	asynq     *asynqServer.Server
	conf      *bizpub.Conf
	dispatch  bizpub.DispatchRepo
	records   bizpub.ApprovalRepo
	publisher PubRunner
	log       *log.Helper
	clock     func() time.Time
	// newTask 入队函数（默认包装 asynq server.NewTask；单测注入捕获器——
	// broker.Any 与 any 同为空接口，互相可赋）。
	newTask func(typeName string, msg any, opts ...asynq.Option) error
}

// NewScheduleWorker 构造（处理器必须构造期注册——asynq 禁止 Start 后注册）。
func NewScheduleWorker(
	asynqSrv *asynqServer.Server,
	conf *bizpub.Conf,
	dispatch bizpub.DispatchRepo,
	records bizpub.ApprovalRepo,
	publisher PubRunner,
	logger log.Logger,
) *ScheduleWorker {
	w := &ScheduleWorker{
		asynq:     asynqSrv,
		conf:      conf,
		dispatch:  dispatch,
		records:   records,
		publisher: publisher,
		log:       log.NewHelper(log.With(logger, "module", "worker/map-schedule")),
		clock:     time.Now,
	}
	if asynqSrv != nil {
		w.registerHandlers(asynqSrv)
		w.newTask = func(typeName string, msg any, opts ...asynq.Option) error {
			return asynqSrv.NewTask(typeName, msg, opts...)
		}
	}
	return w
}

// registerHandlers 构造期全量注册处理器。
func (w *ScheduleWorker) registerHandlers(srv *asynqServer.Server) {
	must := func(name string, err error) {
		if err != nil {
			panic("register map subscriber " + name + " failed: " + err.Error())
		}
	}
	must(TaskDueScan, asynqServer.RegisterSubscriberWithCtx(srv, TaskDueScan,
		func(ctx context.Context, _ string, _ *dueScanPayload) error {
			ctx, cancel := context.WithTimeout(ctx, time.Minute)
			defer cancel()
			return w.dueScan(ctx)
		}))
	must(TaskPublishDue, asynqServer.RegisterSubscriberWithCtx(srv, TaskPublishDue,
		func(ctx context.Context, _ string, p *duePayload) error {
			return w.executeDue(ctx, p)
		}))
}

// Start 登记周期扫描（幂等；总闸关闭则不注册——运维级 kill-switch，重启生效）。
func (w *ScheduleWorker) Start(context.Context) error {
	if w.asynq == nil {
		w.log.Warn("asynq not configured, map schedule worker disabled（REST 立即发不受影响）")
		return nil
	}
	if w.conf == nil || !w.conf.Schedule.LocalWorkerEnabled {
		w.log.Info("map schedule.local_worker_enabled=false，本机到点发未启用（扫描不注册；平台原生排期不受影响）")
		return nil
	}
	if _, err := w.asynq.NewPeriodicTask(scanCron, jobDueScan, TaskDueScan, &dueScanPayload{}); err != nil {
		return fmt.Errorf("register %s: %w", jobDueScan, err)
	}
	w.log.Info("map schedule worker handlers registered (map.publish.due-scan / map.publish.due)")
	return nil
}

// Stop 周期任务体由共享 asynq server 生命周期管理，本层无自有 goroutine。
func (w *ScheduleWorker) Stop(context.Context) error { return nil }

// dueScan 到点扫描：取「理论点火窗内」的候选（before=now+jitter 上界），
// 逐单元套上确定性拟人化偏移后判定真正到点者入队。
func (w *ScheduleWorker) dueScan(ctx context.Context) error {
	if w.conf == nil || !w.conf.Schedule.LocalWorkerEnabled {
		return nil
	}
	if w.newTask == nil {
		w.log.Warn("asynq not configured, due scan no-op")
		return nil
	}
	jitter := w.conf.Schedule.HumanizeJitter
	now := w.clock()
	rows, err := w.dispatch.ListPublishDue(ctx, now.Add(jitter), scanBatch)
	if err != nil {
		return err // 扫描失败如实上报，asynq 按周期任务失败重投
	}
	for _, u := range rows {
		if u.ScheduledFor == nil {
			continue
		}
		fireAt := u.ScheduledFor.Add(jitterOffset(u.UnitID, jitter))
		if fireAt.After(now) {
			continue // 拟人化向右漂移，尚未到点
		}
		// H2 修复：TaskID 纳入 scheduled_for 时刻戳——同一单元上午失败、下午重排到
		// 新时间时生成不同 TaskID，规避 asynq 24h Retention 内 key 未过期导致的
		// ErrTaskIDConflict 静默吞掉重排（旧行为：单元永不在新时间触发且无告警）。
		taskID := fmt.Sprintf("map-due:%s:%d", u.UnitID, u.ScheduledFor.UTC().Unix())
		err := w.newTask(TaskPublishDue, &duePayload{UnitID: u.UnitID},
			asynq.TaskID(taskID),
			asynq.MaxRetry(w.conf.Retry.MaxAttempts),
			asynq.Timeout(taskTimeout),
			asynq.Retention(taskRetention),
		)
		switch {
		case err == nil:
			w.log.Infof("due unit enqueued: %s (fire_at=%s)", u.UnitID, fireAt.Format(time.RFC3339))
		case errors.Is(err, asynq.ErrTaskIDConflict):
			w.log.Debugf("due task already queued/running: %s", u.UnitID)
		default:
			w.log.Warnf("enqueue due task failed unit=%s err=%v", u.UnitID, err)
		}
	}
	return nil
}

// executeDue 单发执行：
//  1. 状态复核（等队期间 Cancel/人工已发 → no-op；不存在/非 local_worker → no-op）；
//  2. ADR-MAP-11 代发：operator=审批人（human）；confirm_public=true 是审批人对可见性
//     的既确认（建单已有 override_reason 前置，审批时又经一次人工过目）；
//  3. 成功 → 追加 via=scheduler 审计；失败 → 可重试的掷回 asynq，终态的静默收尾；
//     重投耗尽 → deadletter 审计 + 告警（草稿保留，人工重发）。
func (w *ScheduleWorker) executeDue(ctx context.Context, p *duePayload) error {
	if p == nil || p.UnitID == "" {
		return nil // 空载荷无可救药，终态丢任务
	}
	unit, err := w.dispatch.GetUnit(ctx, p.UnitID)
	if err != nil {
		if errors.Is(err, bizpub.ErrNotFound) {
			return nil // 单元被删：终态
		}
		return err // DB 抖动：重投
	}
	// 🔴 状态复核：Cancel 吊销不再命中；published/publishing 重复命中由用例幂等吃掉，
	// 此处直接收尾免得空转审计。
	if unit.ApprovalState != string(mediacontract.ApprovalApproved) ||
		unit.PublishState == "published" || unit.PublishState == "publishing" ||
		unit.ScheduleMode != "local_worker" {
		w.log.Infof("due task stale, skip: unit=%s approval=%s publish=%s mode=%s",
			unit.UnitID, unit.ApprovalState, unit.PublishState, unit.ScheduleMode)
		return nil
	}
	// 🔴 ADR-MAP-11：operator 归属审批人；审批人缺失 = 数据异常（DB CHECK 应保证非空），
	// 跳过 + 告警，绝不自造审批人。
	if unit.Approver == nil || *unit.Approver == "" {
		w.log.Errorf("due unit missing approver (data anomaly), skip: %s", unit.UnitID)
		return nil
	}
	operator := bizpub.NewHumanActor(*unit.Approver)

	out, perr := w.publisher.Publish(ctx, unit.UnitID, operator, true, bizpub.AuditMeta{})
	if perr == nil {
		w.log.Infof("scheduled publish done: unit=%s state=%s receipts=%d",
			unit.UnitID, out.Unit.PublishState, len(out.Receipts))
		w.recordSchedulerAudit(ctx, unit, operator, "allow",
			"via=scheduler：ADR-MAP-11 代发（审批即授权时间窗，执行无人工点击）")
		return nil
	}

	// 失败分流：终态错误显式落 failed 并审计（H3 修复——此前仅 Warn 即 return，
	// 单元停在 pending/failed 且 scheduled_for 在过去，下轮 dueScan 又入队，每 30s
	// 全量静默重试；dashboard 永挂 pending、无审计、无告警）。
	retryCount, _ := asynq.GetRetryCount(ctx)
	if !retryable(perr) {
		w.log.Warnf("scheduled publish terminal error: unit=%s err=%v", unit.UnitID, perr)
		failed := "failed"
		if _, uerr := w.dispatch.UpdateStates(ctx, unit.UnitID, bizpub.UnitPatch{PublishState: &failed}); uerr != nil {
			w.log.Errorf("mark terminal-failed unit=%s failed: %v", unit.UnitID, uerr)
		}
		// 终态留痕（deny 裁决语义：被拒的发布尝试；deadletter 走下方重试耗尽分支）。
		w.recordSchedulerAudit(ctx, unit, operator, "deny",
			fmt.Sprintf("via=scheduler 终态失败（publish_state→failed，草稿保留，人工处理后重发）：%s", errBrief(perr)))
		return nil
	}
	maxRetry := w.conf.Retry.MaxAttempts
	if retryCount >= maxRetry {
		// 🔴 死信：重投耗尽——此返回值让 asynq 归档任务；审计 + 告警 + 草稿保留。
		w.log.Errorf("scheduled publish DEAD-LETTER: unit=%s retries=%d err=%v", unit.UnitID, retryCount, perr)
		w.recordSchedulerAudit(ctx, unit, operator, "deadletter",
			fmt.Sprintf("asynq 重试 %d 次耗尽（草稿保留，审批人可人工重发）：%s", retryCount, errBrief(perr)))
	}
	return perr
}

// retryable asynq 重投判定：会话态/建单态/配额/确认类错误重试无解（终态）；
// 上游抖动/熔断暂停/未知错误重试有解（MaxRetry 兜底）。
func retryable(err error) bool {
	switch {
	case errors.Is(err, bizpub.ErrAlreadyPublishing):
		return false // 另一执行在途，本任务无责收尾
	case errors.Is(err, bizpub.ErrNotFound),
		errors.Is(err, bizpub.ErrAdapterNotFound),
		errors.Is(err, bizpub.ErrBadRequest),
		errors.Is(err, bizpub.ErrPublicConfirmRequired),
		errors.Is(err, algopublisher.ErrNotApproved),
		errors.Is(err, algopublisher.ErrSelfApproval),
		errors.Is(err, algopublisher.ErrInvalidState),
		errors.Is(err, algopublisher.ErrVisibilityNoReason),
		errors.Is(err, algopublisher.ErrQuotaExceeded),
		errors.Is(err, algopublisher.ErrAutomationCannotPublish):
		return false
	default:
		return true
	}
}

// recordSchedulerAudit worker 侧伴随审计（publish-allow 由用例已记「执行动作」，
// 此条补记「触发来源=scheduler」；附录式追加，绝不回写既有行）。
func (w *ScheduleWorker) recordSchedulerAudit(ctx context.Context, unit *bizpub.UnitRow, operator bizpub.Actor, result, reason string) {
	toState := unit.PublishState
	if result == "allow" {
		toState = "published"
	}
	if err := w.records.Record(ctx, &bizpub.RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             bizpub.AuditActionPublish,
		Actor:              operator.ID,
		ActorType:          string(operator.Type),
		FromState:          strPtrOrNil(unit.PublishState),
		ToState:            strPtrOrNil(toState),
		Reason:             strPtrOrNil(reason),
		VisibilityAtAction: strPtrOrNil(unit.VisibilityDefault),
		Result:             result,
	}); err != nil {
		w.log.Warnf("scheduler companion audit failed unit=%s err=%v", unit.UnitID, err)
	}
}

// jitterOffset 确定性拟人化偏移：同一 unit 恒定落在 [-jitter, +jitter]，
// 既防整点批量可识别模式，又避免「每轮扫描重掷骰子」导致的点火时序抖晃。
func jitterOffset(unitID string, jitter time.Duration) time.Duration {
	if jitter <= 0 {
		return 0
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte("map-jitter:" + unitID))
	span := uint64(2*jitter) + 1
	return time.Duration(int64(h.Sum64()%span) - int64(jitter))
}

// errBrief 错误摘要进审计（上层消息须经脱敏的纪律不变，此处再截断防超长）。
func errBrief(err error) string {
	msg := err.Error()
	if len(msg) > 120 {
		msg = msg[:120] + "…"
	}
	return msg
}

func strPtrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
