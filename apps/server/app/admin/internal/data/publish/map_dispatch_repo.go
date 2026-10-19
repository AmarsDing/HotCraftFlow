package pubdata

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mapdispatchunit"
	"framework/app/admin/internal/data/ent/predicate"
)

// EntDispatchRepo bizpub.DispatchRepo 的 Ent 实现。
//
// 🔴 审批/发布状态只经 UnitPatch 流转（repo 不提供任意字段更新能力），
// DB CHECK 约束（map_dispatch_publish_requires_approval）对本 repo 的写入同样生效——
// 本层代码绝不允许把未 approved 的单元写成 published（补丁里没有这样的路径）。
type EntDispatchRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntDispatchRepo 构造。
func NewEntDispatchRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntDispatchRepo {
	return &EntDispatchRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/map-dispatch-repo")),
	}
}

// CreateUnit 新建分发单元。
func (r *EntDispatchRepo) CreateUnit(ctx context.Context, row *bizpub.UnitRow) error {
	builder := r.cli.Client().MapDispatchUnit.Create().
		SetUnitID(row.UnitID).
		SetTargets(row.Targets).
		SetScheduleMode(mapdispatchunit.ScheduleMode(row.ScheduleMode)).
		SetVisibilityDefault(mapdispatchunit.VisibilityDefault(row.VisibilityDefault)).
		SetApprovalState(mapdispatchunit.ApprovalState(row.ApprovalState)).
		SetSubmittedBy(row.SubmittedBy).
		SetPublishState(mapdispatchunit.PublishState(row.PublishState))
	if row.VariantSetID != nil {
		builder.SetNillableVariantSetID(row.VariantSetID)
	}
	if row.VariantID != nil {
		builder.SetNillableVariantID(row.VariantID)
	}
	if row.ScheduledFor != nil {
		builder.SetNillableScheduledFor(row.ScheduledFor)
	}
	if row.VisibilityOverrideReason != nil {
		builder.SetNillableVisibilityOverrideReason(row.VisibilityOverrideReason)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return fmt.Errorf("%w: 分发单元 %s 已存在", bizpub.ErrBadRequest, row.UnitID)
		}
		r.log.Errorf("create map dispatch unit failed: %v", err)
		return fmt.Errorf("%w: 创建分发单元失败", bizpub.ErrInternal)
	}
	row.ID = uint32(e.ID)
	row.CreateTime = derefTime(e.CreateTime)
	return nil
}

// UpdateStates 状态流转补丁（nil 字段不触碰；Clear* 显式清空）。
func (r *EntDispatchRepo) UpdateStates(ctx context.Context, unitID string, patch bizpub.UnitPatch) (*bizpub.UnitRow, error) {
	u := r.cli.Client().MapDispatchUnit.Update().Where(mapdispatchunit.UnitIDEQ(unitID))

	if patch.ApprovalState != nil {
		u = u.SetApprovalState(mapdispatchunit.ApprovalState(*patch.ApprovalState))
	}
	if patch.Approver != nil {
		u = u.SetNillableApprover(patch.Approver)
	}
	if patch.ApprovedAt != nil {
		u = u.SetNillableApprovedAt(patch.ApprovedAt)
	}
	if patch.ClearApprover {
		u = u.ClearApprover().ClearApprovedAt()
	}
	if patch.RejectReason != nil {
		u = u.SetNillableRejectReason(patch.RejectReason)
	}
	if patch.ClearRejectReason {
		u = u.ClearRejectReason()
	}

	if patch.PublishState != nil {
		u = u.SetPublishState(mapdispatchunit.PublishState(*patch.PublishState))
	}
	if patch.Operator != nil {
		u = u.SetNillableOperator(patch.Operator)
	}
	if patch.PublishedAt != nil {
		u = u.SetNillablePublishedAt(patch.PublishedAt)
	}
	if patch.PostID != nil {
		u = u.SetNillablePostID(patch.PostID)
	}
	if patch.PostURL != nil {
		u = u.SetNillablePostURL(patch.PostURL)
	}

	n, err := u.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// 🔴 CHECK 兜底被触发（企图绕过审批门写 published）——严重事件，必须可见。
			r.log.Errorf("[map-guard] DB CHECK 拦截非法发布状态写入 unit=%s: %v", unitID, err)
			return nil, fmt.Errorf("%w: 发布状态转移被数据库铁律约束拦截", bizpub.ErrInternal)
		}
		r.log.Errorf("update map dispatch unit states failed unit=%s: %v", unitID, err)
		return nil, fmt.Errorf("%w: 更新分发单元状态失败", bizpub.ErrInternal)
	}
	if n == 0 {
		return nil, fmt.Errorf("%w: 分发单元不存在", bizpub.ErrNotFound)
	}
	return r.GetUnit(ctx, unitID)
}

// GetUnit 按 unit_id（du_ 前缀）查询。
func (r *EntDispatchRepo) GetUnit(ctx context.Context, unitID string) (*bizpub.UnitRow, error) {
	e, err := r.cli.Client().MapDispatchUnit.Query().
		Where(mapdispatchunit.UnitIDEQ(unitID)).
		Only(ctx)
	if err != nil {
		return nil, mapQueryErr(err, "分发单元")
	}
	return unitToRow(e), nil
}

// ListUnits 分页列出（create_time 倒序）。
func (r *EntDispatchRepo) ListUnits(ctx context.Context, pageNo, pageSize int) ([]*bizpub.UnitRow, int, error) {
	return r.listUnits(ctx, nil, pageNo, pageSize)
}

// ListUnitsByApproval 按审批状态分页列出（审批队列 draft 用）。
func (r *EntDispatchRepo) ListUnitsByApproval(ctx context.Context, state string, pageNo, pageSize int) ([]*bizpub.UnitRow, int, error) {
	return r.listUnits(ctx, mapdispatchunit.ApprovalStateEQ(mapdispatchunit.ApprovalState(state)), pageNo, pageSize)
}

func (r *EntDispatchRepo) listUnits(ctx context.Context, pred predicate.MapDispatchUnit, pageNo, pageSize int) ([]*bizpub.UnitRow, int, error) {
	pageNo, pageSize = clampPage(pageNo, pageSize)
	q := r.cli.Client().MapDispatchUnit.Query()
	if pred != nil {
		q = q.Where(pred)
	}
	total, err := q.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count map dispatch units failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询分发单元总数失败", bizpub.ErrInternal)
	}
	entities, err := q.
		Order(ent.Desc(mapdispatchunit.FieldCreateTime)).
		Offset((pageNo - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map dispatch units failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询分发单元列表失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.UnitRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, unitToRow(e))
	}
	return rows, total, nil
}

// ListScheduledBetween 日历视图：scheduled_for ∈ [from, to]（无 scheduled_for 不入日历）。
func (r *EntDispatchRepo) ListScheduledBetween(ctx context.Context, from, to time.Time) ([]*bizpub.UnitRow, error) {
	entities, err := r.cli.Client().MapDispatchUnit.Query().
		Where(
			mapdispatchunit.ScheduledForNotNil(),
			mapdispatchunit.ScheduledForGTE(from.UTC()),
			mapdispatchunit.ScheduledForLTE(to.UTC()),
		).
		Order(ent.Asc(mapdispatchunit.FieldScheduledFor)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map dispatch units calendar failed: %v", err)
		return nil, fmt.Errorf("%w: 查询日历失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.UnitRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, unitToRow(e))
	}
	return rows, nil
}

// ListPublishDue 到点扫描（worker/map 30s 轮询）：
// local_worker 模式 + 已审批 + pending/failed + scheduled_for ≤ before。
// 🔴 只读不改态——是否真正点火由 worker 侧拟人化偏移与执行器状态复核决定。
func (r *EntDispatchRepo) ListPublishDue(ctx context.Context, before time.Time, limit int) ([]*bizpub.UnitRow, error) {
	if limit <= 0 {
		limit = 64
	}
	entities, err := r.cli.Client().MapDispatchUnit.Query().
		Where(
			mapdispatchunit.ScheduledForNotNil(),
			mapdispatchunit.ScheduledForLTE(before.UTC()),
			mapdispatchunit.ScheduleModeEQ(mapdispatchunit.ScheduleModeLocalWorker),
			mapdispatchunit.ApprovalStateEQ(mapdispatchunit.ApprovalStateApproved),
			mapdispatchunit.PublishStateIn(mapdispatchunit.PublishStatePending, mapdispatchunit.PublishStateFailed),
		).
		Order(ent.Asc(mapdispatchunit.FieldScheduledFor)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map publish-due units failed: %v", err)
		return nil, fmt.Errorf("%w: 查询到点单元失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.UnitRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, unitToRow(e))
	}
	return rows, nil
}

func unitToRow(e *ent.MapDispatchUnit) *bizpub.UnitRow {
	return &bizpub.UnitRow{
		ID:                       uint32(e.ID),
		UnitID:                   e.UnitID,
		VariantSetID:             e.VariantSetID,
		VariantID:                e.VariantID,
		Targets:                  e.Targets,
		ScheduledFor:             e.ScheduledFor,
		ScheduleMode:             string(e.ScheduleMode),
		VisibilityDefault:        string(e.VisibilityDefault),
		VisibilityOverrideReason: e.VisibilityOverrideReason,
		ApprovalState:            string(e.ApprovalState),
		SubmittedBy:              e.SubmittedBy,
		Approver:                 e.Approver,
		ApprovedAt:               e.ApprovedAt,
		RejectReason:             e.RejectReason,
		PublishState:             string(e.PublishState),
		PostID:                   e.PostID,
		PostURL:                  e.PostURL,
		PublishedAt:              e.PublishedAt,
		Operator:                 e.Operator,
		FeedbackRef:              e.FeedbackRef,
		CreateTime:               derefTime(e.CreateTime),
	}
}
