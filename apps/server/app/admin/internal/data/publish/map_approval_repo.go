package pubdata

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mapapprovalrecord"
)

// EntApprovalRepo bizpub.ApprovalRepo 的 Ent 实现。
// 🔴 append-only：刻意不提供 Update/Delete，审计流水不可篡改（FR-PUB-07）。
type EntApprovalRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntApprovalRepo 构造。
func NewEntApprovalRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntApprovalRepo {
	return &EntApprovalRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/map-approval-repo")),
	}
}

// Record 追加一条审计流水。
func (r *EntApprovalRepo) Record(ctx context.Context, e *bizpub.RecordEntry) error {
	builder := r.cli.Client().MapApprovalRecord.Create().
		SetDispatchUnitID(e.DispatchUnitID).
		SetAction(mapapprovalrecord.Action(e.Action)).
		SetActor(e.Actor).
		SetActorType(mapapprovalrecord.ActorType(e.ActorType)).
		SetResult(mapapprovalrecord.Result(e.Result))
	if e.FromState != nil {
		builder.SetNillableFromState(e.FromState)
	}
	if e.ToState != nil {
		builder.SetNillableToState(e.ToState)
	}
	if e.Reason != nil {
		builder.SetNillableReason(e.Reason)
	}
	if e.VisibilityAtAction != nil {
		builder.SetNillableVisibilityAtAction(e.VisibilityAtAction)
	}
	if e.IP != nil {
		builder.SetNillableIP(e.IP)
	}
	if e.UserAgent != nil {
		builder.SetNillableUserAgent(e.UserAgent)
	}
	saved, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("insert map approval record failed unit=%s action=%s: %v", e.DispatchUnitID, e.Action, err)
		return fmt.Errorf("%w: 审计流水写入失败", bizpub.ErrInternal)
	}
	e.ID = uint32(saved.ID)
	e.CreateTime = derefTime(saved.CreateTime)
	return nil
}

// ListRecordsByUnit 按 unit 列出全部流水（create_time 升序，形成时间线）。
func (r *EntApprovalRepo) ListRecordsByUnit(ctx context.Context, unitID string) ([]*bizpub.RecordEntry, error) {
	entities, err := r.cli.Client().MapApprovalRecord.Query().
		Where(mapapprovalrecord.DispatchUnitIDEQ(unitID)).
		Order(ent.Asc(mapapprovalrecord.FieldCreateTime), ent.Asc(mapapprovalrecord.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map approval records failed unit=%s: %v", unitID, err)
		return nil, fmt.Errorf("%w: 查询审计流水失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.RecordEntry, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, recordToEntry(e))
	}
	return rows, nil
}

func recordToEntry(e *ent.MapApprovalRecord) *bizpub.RecordEntry {
	return &bizpub.RecordEntry{
		ID:                 uint32(e.ID),
		DispatchUnitID:     e.DispatchUnitID,
		Action:             string(e.Action),
		Actor:              e.Actor,
		ActorType:          string(e.ActorType),
		FromState:          e.FromState,
		ToState:            e.ToState,
		Reason:             e.Reason,
		VisibilityAtAction: e.VisibilityAtAction,
		IP:                 e.IP,
		UserAgent:          e.UserAgent,
		Result:             string(e.Result),
		CreateTime:         derefTime(e.CreateTime),
	}
}
