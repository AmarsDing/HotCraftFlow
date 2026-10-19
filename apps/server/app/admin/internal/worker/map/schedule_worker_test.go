package mapworker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/hibiken/asynq"

	algopublisher "framework/app/admin/internal/algo/publisher"
	bizpub "framework/app/admin/internal/biz/publish"
	"framework/pkg/mediacontract"
)

// ============================================================================
// 测试替身：无 DB/无 Redis/无 asynq 进程（入队/执行全捕获）。
// 死信用例约定：conf.Retry.MaxAttempts=0 → 普通 ctx 下 GetRetryCount 缺席即 0，
// 「当前即为最后一投」=任何可重试失败立即落死信——免造 asynq ctx。
// ============================================================================

type dispatchFake struct {
	mu    sync.Mutex
	units map[string]*bizpub.UnitRow
}

func newDispatchFake(rows ...*bizpub.UnitRow) *dispatchFake {
	f := &dispatchFake{units: map[string]*bizpub.UnitRow{}}
	for _, r := range rows {
		f.units[r.UnitID] = r
	}
	return f
}

func (f *dispatchFake) CreateUnit(_ context.Context, row *bizpub.UnitRow) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.units[row.UnitID] = row
	return nil
}
func (f *dispatchFake) UpdateStates(_ context.Context, unitID string, patch bizpub.UnitPatch) (*bizpub.UnitRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.units[unitID]
	if !ok {
		return nil, bizpub.ErrNotFound
	}
	if patch.PublishState != nil {
		r.PublishState = *patch.PublishState
	}
	return r, nil
}
func (f *dispatchFake) GetUnit(_ context.Context, unitID string) (*bizpub.UnitRow, error) {
	if r, ok := f.units[unitID]; ok {
		return r, nil
	}
	return nil, bizpub.ErrNotFound
}
func (f *dispatchFake) ListUnits(context.Context, int, int) ([]*bizpub.UnitRow, int, error) {
	return nil, 0, nil
}
func (f *dispatchFake) ListUnitsByApproval(context.Context, string, int, int) ([]*bizpub.UnitRow, int, error) {
	return nil, 0, nil
}
func (f *dispatchFake) ListScheduledBetween(context.Context, time.Time, time.Time) ([]*bizpub.UnitRow, error) {
	return nil, nil
}

// ListPublishDue 与 Ent 实现同谓词（local_worker + approved + pending/failed + ≤before）。
func (f *dispatchFake) ListPublishDue(_ context.Context, before time.Time, limit int) ([]*bizpub.UnitRow, error) {
	var out []*bizpub.UnitRow
	for _, r := range f.units {
		if r.ScheduledFor == nil || r.ScheduledFor.After(before) {
			continue
		}
		if r.ScheduleMode != "local_worker" || r.ApprovalState != string(mediacontract.ApprovalApproved) {
			continue
		}
		if r.PublishState != "pending" && r.PublishState != "failed" {
			continue
		}
		out = append(out, r)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

type recordsFake struct {
	mu      sync.Mutex
	entries []*bizpub.RecordEntry
}

func (f *recordsFake) Record(_ context.Context, e *bizpub.RecordEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}
func (f *recordsFake) ListRecordsByUnit(context.Context, string) ([]*bizpub.RecordEntry, error) {
	return f.entries, nil
}
func (f *recordsFake) byResult(result string) []*bizpub.RecordEntry {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []*bizpub.RecordEntry
	for _, e := range f.entries {
		if e.Result == result {
			out = append(out, e)
		}
	}
	return out
}

type capturedPublish struct {
	unitID    string
	operator  bizpub.Actor
	confirmed bool
}

type runnerFake struct {
	calls []capturedPublish
	err   error
}

func (f *runnerFake) Publish(_ context.Context, unitID string, operator bizpub.Actor, confirmPublic bool, _ bizpub.AuditMeta) (*bizpub.PublishOutcome, error) {
	f.calls = append(f.calls, capturedPublish{unitID: unitID, operator: operator, confirmed: confirmPublic})
	if f.err != nil {
		return nil, f.err
	}
	return &bizpub.PublishOutcome{Unit: &bizpub.UnitRow{UnitID: unitID, PublishState: "published"}}, nil
}

type enqueueCapture struct {
	items []string // 捕获的 unit_id
	err   error    // 注入的入队错误（去重冲突模拟）
}

func (c *enqueueCapture) NewTask(_ string, msg any, _ ...asynq.Option) error {
	if c.err != nil {
		return c.err
	}
	if p, ok := msg.(*duePayload); ok {
		c.items = append(c.items, p.UnitID)
	}
	return nil
}

func baseConf() *bizpub.Conf {
	c := bizpub.DefaultConf()
	c.Schedule.LocalWorkerEnabled = true
	c.Schedule.HumanizeJitter = 0 // 测试内关拟人化：点火判定可预期
	return &c
}

func sp(s string) *string { return &s }

func sp2t(t time.Time) *time.Time { return &t }

// ---------- dueScan ----------

// 扫描只入队「真正到点」的候选：非 local_worker / 未审批 / 已发完 / 未到期的一律不命中。
func TestDueScan_EnqueuesOnlyTrulyDue(t *testing.T) {
	now := time.Now()
	conf := baseConf()
	due := &bizpub.UnitRow{UnitID: "du_due", ScheduleMode: "local_worker",
		ApprovalState: string(mediacontract.ApprovalApproved), PublishState: "pending",
		ScheduledFor: sp2t(now.Add(-time.Minute)), Approver: sp("bob")}
	future := &bizpub.UnitRow{UnitID: "du_future", ScheduleMode: "local_worker",
		ApprovalState: string(mediacontract.ApprovalApproved), PublishState: "pending",
		ScheduledFor: sp2t(now.Add(time.Hour)), Approver: sp("bob")}
	native := &bizpub.UnitRow{UnitID: "du_native", ScheduleMode: "native",
		ApprovalState: string(mediacontract.ApprovalApproved), PublishState: "pending",
		ScheduledFor: sp2t(now.Add(-time.Minute)), Approver: sp("bob")}
	draft := &bizpub.UnitRow{UnitID: "du_draft", ScheduleMode: "local_worker",
		ApprovalState: string(mediacontract.ApprovalDraft), PublishState: "pending",
		ScheduledFor: sp2t(now.Add(-time.Minute))}
	published := &bizpub.UnitRow{UnitID: "du_pub", ScheduleMode: "local_worker",
		ApprovalState: string(mediacontract.ApprovalApproved), PublishState: "published",
		ScheduledFor: sp2t(now.Add(-time.Minute)), Approver: sp("bob")}

	dispatch := newDispatchFake(due, future, native, draft, published)
	cap := &enqueueCapture{}
	w := NewScheduleWorker(nil, conf, dispatch, &recordsFake{}, &runnerFake{}, log.DefaultLogger)
	w.clock = func() time.Time { return now }
	w.newTask = cap.NewTask

	if err := w.dueScan(context.Background()); err != nil {
		t.Fatalf("dueScan 失败: %v", err)
	}
	if len(cap.items) != 1 || cap.items[0] != "du_due" {
		t.Fatalf("只应入队 du_due，实际 %v", cap.items)
	}
}

// 总闸关闭：扫描不动一根手指（运维 kill-switch 可信）。
func TestDueScan_LocalWorkerDisabled_NoOp(t *testing.T) {
	conf := baseConf()
	conf.Schedule.LocalWorkerEnabled = false
	cap := &enqueueCapture{}
	w := NewScheduleWorker(nil, conf, newDispatchFake(), &recordsFake{}, &runnerFake{}, log.DefaultLogger)
	w.newTask = cap.NewTask
	if err := w.dueScan(context.Background()); err != nil {
		t.Fatalf("闸关闭应为 no-op: %v", err)
	}
	if len(cap.items) != 0 {
		t.Fatalf("闸关闭绝不得入队: %v", cap.items)
	}
}

// 拟人化偏移：确定性（同单元两次同值）+ 边界（|offset| ≤ jitter 全覆盖）。
func TestJitterOffset_DeterministicAndBounded(t *testing.T) {
	j := 3 * time.Minute
	for i := 0; i < 200; i++ {
		id := fmt.Sprintf("du_j%d", i)
		a, b := jitterOffset(id, j), jitterOffset(id, j)
		if a != b {
			t.Fatalf("同单元偏移必须确定: %s %v vs %v", id, a, b)
		}
		if a > j || a < -j {
			t.Fatalf("偏移越界: %s %v", id, a)
		}
	}
}

// ---------- executeDue ----------

func approvedLocalUnit(id string) *bizpub.UnitRow {
	return &bizpub.UnitRow{
		UnitID:            id,
		ScheduleMode:      "local_worker",
		ApprovalState:     string(mediacontract.ApprovalApproved),
		SubmittedBy:       "alice",
		Approver:          sp("bob"),
		PublishState:      "pending",
		VisibilityDefault: string(mediacontract.VisibilityPrivate),
	}
}

// 🔴 ADR-MAP-11：operator 归属审批人（human/bob 而非 submitter），confirm_public=true
// 是审批人对可见性的既确认；成功追加 via=scheduler 伴随审计。
func TestExecuteDue_AttributionAndCompanionAudit(t *testing.T) {
	unit := approvedLocalUnit("du_e1")
	runner := &runnerFake{}
	records := &recordsFake{}
	w := NewScheduleWorker(nil, baseConf(), newDispatchFake(unit), records, runner, log.DefaultLogger)

	if err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID}); err != nil {
		t.Fatalf("执行失败: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("应调用发布 1 次，实际 %d", len(runner.calls))
	}
	call := runner.calls[0]
	if call.operator.ID != "bob" || call.operator.Type != mediacontract.ActorHuman {
		t.Fatalf("🔴 operator 必须归属审批人 human/bob，实际 %+v", call.operator)
	}
	if !call.confirmed {
		t.Fatal("confirm_public 应为 true（审批既确认）")
	}
	rows := records.byResult("allow")
	if len(rows) != 1 {
		t.Fatalf("应有 1 条 via=scheduler allow 审计，实际 %d", len(rows))
	}
	rec := rows[0]
	if rec.Reason == nil || !strings.Contains(*rec.Reason, "via=scheduler") {
		t.Fatalf("🔴 真相标记缺失: %+v", rec)
	}
	if rec.Actor != "bob" || rec.ActorType != string(mediacontract.ActorHuman) {
		t.Fatalf("审计属主错误: %+v", rec)
	}
	if rec.ToState == nil || *rec.ToState != "published" {
		t.Fatalf("伴随审计终态: %+v", rec.ToState)
	}
}

// 等队期间被 Cancel（approval!=approved）→ no-op，绝不触碰适配器。
func TestExecuteDue_CancelledInFlight_NoOp(t *testing.T) {
	unit := approvedLocalUnit("du_e2")
	unit.ApprovalState = string(mediacontract.ApprovalDraft)
	runner := &runnerFake{}
	records := &recordsFake{}
	w := NewScheduleWorker(nil, baseConf(), newDispatchFake(unit), records, runner, log.DefaultLogger)

	if err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID}); err != nil {
		t.Fatalf("撤回单元应为 no-op: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("🔴 已撤回单元绝不得发布")
	}
	if len(records.entries) != 0 {
		t.Fatal("no-op 不得产审计")
	}
}

// 终态错误（配额）→ 不重投（err=nil 收尾）+ 显式置 publish_state=failed（H3 修复：
// 此前仅 Warn 即 return，单元停 pending 且 scheduled_for 在过去 → 下轮又入队，每 30s
// 全量静默重试）+ 终态 deny 审计留痕 + 不写死信。
func TestExecuteDue_TerminalError_MarkFailedAndAuditNoDeadletter(t *testing.T) {
	unit := approvedLocalUnit("du_e3")
	runner := &runnerFake{err: algopublisher.ErrQuotaExceeded}
	records := &recordsFake{}
	dispatch := newDispatchFake(unit)
	w := NewScheduleWorker(nil, baseConf(), dispatch, records, runner, log.DefaultLogger)

	if err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID}); err != nil {
		t.Fatalf("终态错误应 nil 收尾: %v", err)
	}
	if unit.PublishState != "failed" {
		t.Fatalf("终态错误应置 publish_state=failed（H3），实际 %s", unit.PublishState)
	}
	if len(records.byResult("deadletter")) != 0 {
		t.Fatal("终态错误不得产死信审计")
	}
	// H3：终态留痕（deny 裁决语义：被拒的发布尝试）。
	denies := records.byResult("deny")
	if len(denies) != 1 {
		t.Fatalf("终态错误应留 1 条 deny 审计，实际 %d", len(denies))
	}
}

// 🔴 死信：可重试错误 + 重投耗尽（MaxAttempts=0 → 普通 ctx 即最后一投）→
// err 掷回（asynq 归档）+ deadletter 审计含「耗尽」。
func TestExecuteDue_DeadLetterAfterExhaustion(t *testing.T) {
	unit := approvedLocalUnit("du_e4")
	conf := baseConf()
	conf.Retry.MaxAttempts = 0
	runner := &runnerFake{err: bizpub.ErrUpstream}
	records := &recordsFake{}
	w := NewScheduleWorker(nil, conf, newDispatchFake(unit), records, runner, log.DefaultLogger)

	err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID})
	if !errors.Is(err, bizpub.ErrUpstream) {
		t.Fatalf("死信应掷回错误交 asynq 归档，实际 %v", err)
	}
	rows := records.byResult("deadletter")
	if len(rows) != 1 {
		t.Fatalf("应有 1 条死信审计，实际 %d", len(rows))
	}
	if rows[0].Reason == nil || !strings.Contains(*rows[0].Reason, "耗尽") {
		t.Fatalf("死信原因应记耗尽: %+v", rows[0])
	}
}

// 可重试错误未到尽头（MaxAttempts=5，普通 ctx retry=0）→ 掷回 + 无死信。
func TestExecuteDue_RetryableErrorNotExhausted(t *testing.T) {
	unit := approvedLocalUnit("du_e5")
	runner := &runnerFake{err: bizpub.ErrUpstream}
	records := &recordsFake{}
	w := NewScheduleWorker(nil, baseConf(), newDispatchFake(unit), records, runner, log.DefaultLogger)

	err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID})
	if !errors.Is(err, bizpub.ErrUpstream) {
		t.Fatalf("可重试错误应掷回 asynq，实际 %v", err)
	}
	if len(records.byResult("deadletter")) != 0 {
		t.Fatal("未到尽头不得产死信")
	}
}

// 审批人缺失 = 数据异常（DB CHECK 应拦住）：跳过 + 告警路（此测只断言不发布）。
func TestExecuteDue_MissingApprover_Skip(t *testing.T) {
	unit := approvedLocalUnit("du_e6")
	unit.Approver = nil
	runner := &runnerFake{}
	w := NewScheduleWorker(nil, baseConf(), newDispatchFake(unit), &recordsFake{}, runner, log.DefaultLogger)

	if err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID}); err != nil {
		t.Fatalf("数据异常单元应 no-op: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatal("🔴 无审批人绝不发布")
	}
}

// AlreadyPublishing（人工点发在途）→ nil 收尾，不占重试额度。
func TestExecuteDue_AlreadyPublishing_BenignSkip(t *testing.T) {
	unit := approvedLocalUnit("du_e7")
	runner := &runnerFake{err: bizpub.ErrAlreadyPublishing}
	w := NewScheduleWorker(nil, baseConf(), newDispatchFake(unit), &recordsFake{}, runner, log.DefaultLogger)

	if err := w.executeDue(context.Background(), &duePayload{UnitID: unit.UnitID}); err != nil {
		t.Fatalf("锁竞争应 benign 收尾: %v", err)
	}
}
