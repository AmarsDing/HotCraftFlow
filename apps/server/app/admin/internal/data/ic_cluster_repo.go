package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/dedupsample"
	"framework/app/admin/internal/data/ent/intelcluster"
	"framework/app/admin/internal/data/ent/intelitem"
	contract "framework/pkg/mediacontract"
)

// IntelClusterRepo 簇仓储（S-2 空闲关闭 / S-3 合簇同事务 / S-4 质心冻结由 biz 控制）。
type IntelClusterRepo struct {
	data *Data
	log  *log.Helper
}

func NewIntelClusterRepo(data *Data, logger log.Logger) biz.IntelClusterRepo {
	return &IntelClusterRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/cluster/repo/admin-service")),
		data: data,
	}
}

func (r *IntelClusterRepo) Create(ctx context.Context, row *biz.IntelClusterRow) (*biz.IntelClusterRow, error) {
	id := row.ID
	if id == "" {
		id = contract.NewClusterID()
	}
	now := time.Now().UTC()
	builder := r.data.db.Client().IntelCluster.Create().
		SetID(id).
		SetLabel(row.Label).
		SetMemberCount(row.MemberCount).
		SetPlatformCount(row.PlatformCount).
		SetRegion(row.Region).
		SetFirstSeenAt(row.FirstSeenAt.UTC()).
		SetLastSeenAt(row.LastSeenAt.UTC()).
		SetMergedBy(row.MergedBy).
		SetStatus("open").
		SetCreateTime(now).
		SetUpdateTime(now)
	if row.RepresentativeID != "" {
		builder.SetRepresentativeID(row.RepresentativeID)
	}
	if row.Category != "" {
		builder.SetCategory(row.Category)
	}
	if row.CentroidRef != "" {
		builder.SetCentroidRef(row.CentroidRef)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("create cluster failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("create cluster failed")
	}
	return toBizIntelCluster(e), nil
}

func (r *IntelClusterRepo) GetByID(ctx context.Context, id string) (*biz.IntelClusterRow, error) {
	e, err := r.data.db.Client().IntelCluster.Query().
		Where(intelcluster.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query cluster failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("query cluster failed")
	}
	return toBizIntelCluster(e), nil
}

func (r *IntelClusterRepo) List(ctx context.Context, q *biz.IntelClusterListQuery) ([]*biz.IntelClusterRow, int, error) {
	p, s := 1, 20
	var opts biz.IntelClusterListQuery
	if q != nil {
		opts = *q
		p, s = icPageClamp(opts.Page, opts.PageSize)
	}
	query := r.data.db.Client().IntelCluster.Query()
	if opts.Region != "" {
		query = query.Where(intelcluster.RegionEQ(opts.Region))
	}
	if opts.Status != "" {
		query = query.Where(intelcluster.StatusEQ(opts.Status))
	}
	total, err := query.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count clusters failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("count clusters failed")
	}
	entities, err := query.
		Order(ent.Desc(intelcluster.FieldLastSeenAt)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list clusters failed: %s", err.Error())
		return nil, 0, adminV1.ErrorInternalServerError("list clusters failed")
	}
	out := make([]*biz.IntelClusterRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizIntelCluster(e))
	}
	return out, total, nil
}

// TouchSeen 刷新簇活跃时间。只前进不回拨（GREATEST 语义）：乱序/迟到并入
// （NATS at-least-once 重投、队列积压）携带的是旧 collected_at，若无条件覆盖
// 会把 LastSeenAt 拨回过去，触发 S-2 idle-close 误关仍活跃收新成员的簇。
func (r *IntelClusterRepo) TouchSeen(ctx context.Context, id string, at time.Time) error {
	if _, err := r.data.db.Client().IntelCluster.UpdateOneID(id).
		Where(intelcluster.LastSeenAtLT(at.UTC())).
		SetLastSeenAt(at.UTC()).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("touch cluster seen failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("touch cluster failed")
	}
	return nil
}

// RecomputeStats S-3：统计永远以成员表实际为准（不信任增量计数）。
func (r *IntelClusterRepo) RecomputeStats(ctx context.Context, id string) error {
	memberCount, err := r.data.db.Client().IntelItem.Query().
		Where(intelitem.ClusterIDEQ(id)).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count cluster members failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("count cluster members failed")
	}
	var platformRows []struct {
		Platform string `json:"platform"`
		Count    int    `json:"count"`
	}
	if err := r.data.db.Client().IntelItem.Query().
		Where(intelitem.ClusterIDEQ(id)).
		GroupBy(intelitem.FieldPlatform).
		Aggregate(ent.Count()).
		Scan(ctx, &platformRows); err != nil {
		r.log.Errorf("count cluster platforms failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("count cluster platforms failed")
	}
	if _, err := r.data.db.Client().IntelCluster.UpdateOneID(id).
		SetMemberCount(int32(memberCount)).
		SetPlatformCount(int32(len(platformRows))).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("recompute cluster stats failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("recompute cluster stats failed")
	}
	return nil
}

func (r *IntelClusterRepo) SetRepresentative(ctx context.Context, id, intelID string) error {
	if _, err := r.data.db.Client().IntelCluster.UpdateOneID(id).
		SetRepresentativeID(intelID).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("set cluster representative failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("set cluster representative failed")
	}
	return nil
}

func (r *IntelClusterRepo) SetCategory(ctx context.Context, id, category string) error {
	if _, err := r.data.db.Client().IntelCluster.UpdateOneID(id).
		SetCategory(category).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("set cluster category failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("set cluster category failed")
	}
	return nil
}

func (r *IntelClusterRepo) SetCentroidRef(ctx context.Context, id, ref string) error {
	if _, err := r.data.db.Client().IntelCluster.UpdateOneID(id).
		SetCentroidRef(ref).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("set cluster centroid failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("set cluster centroid failed")
	}
	return nil
}

// MergeTx S-3：成员迁移 + 计数重算 + 事件落库同事务。
// ent 模板无 FOR UPDATE：以「按 ID 升序的 UpdateOne 行锁」保证两簇锁顺序一致，避免死锁。
func (r *IntelClusterRepo) MergeTx(ctx context.Context, fromID, toID, reason, operator string) error {
	if fromID == "" || toID == "" || fromID == toID {
		return icV1.ErrorIcConfigInvalid("合簇 from/to 非法")
	}
	now := time.Now().UTC()
	err := RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
		// 1) 锁两簇（升序，死锁防护）
		first, second := fromID, toID
		if first > second {
			first, second = second, first
		}
		for _, lockID := range []string{first, second} {
			if _, err := tx.IntelCluster.UpdateOneID(lockID).SetUpdateTime(now).Save(ctx); err != nil {
				r.log.Errorf("lock cluster failed (id=%s): %s", lockID, err.Error())
				return adminV1.ErrorInternalServerError("merge cluster lock failed")
			}
		}
		// 2) 校验两簇存在且可合并
		from, err := tx.IntelCluster.Get(ctx, fromID)
		if err != nil {
			if ent.IsNotFound(err) {
				return icV1.ErrorIcConfigInvalid("源簇不存在")
			}
			return adminV1.ErrorInternalServerError("query cluster failed")
		}
		if from.Status != "open" {
			return icV1.ErrorIcConfigInvalid("源簇已关闭，不可再合并")
		}
		to, err := tx.IntelCluster.Get(ctx, toID)
		if err != nil {
			if ent.IsNotFound(err) {
				return icV1.ErrorIcConfigInvalid("目标簇不存在")
			}
			return adminV1.ErrorInternalServerError("query cluster failed")
		}
		if to.Status != "open" {
			return icV1.ErrorIcConfigInvalid("目标簇已关闭，不可并入")
		}
		// 3) 迁移成员
		moved, err := tx.IntelItem.Update().
			Where(intelitem.ClusterIDEQ(fromID)).
			SetClusterID(toID).
			SetUpdateTime(now).Save(ctx)
		if err != nil {
			r.log.Errorf("move members failed (from=%s to=%s): %s", fromID, toID, err.Error())
			return adminV1.ErrorInternalServerError("move cluster members failed")
		}
		// 4) from 关闭；to 刷新 seen
		if _, err := tx.IntelCluster.UpdateOneID(fromID).
			SetStatus("closed").
			SetClosedAt(now).
			SetMemberCount(0).
			Save(ctx); err != nil {
			return adminV1.ErrorInternalServerError("close source cluster failed")
		}
		if _, err := tx.IntelCluster.UpdateOneID(toID).
			SetLastSeenAt(now).
			Save(ctx); err != nil {
			return adminV1.ErrorInternalServerError("touch target cluster failed")
		}
		// 5) 合并事件（审计，不可裁剪）
		builder := tx.ClusterMergeEvent.Create().
			SetID(contract.ULID()).
			SetFromClusterID(fromID).
			SetToClusterID(toID).
			SetMemberCount(int32(moved)).
			SetCreateTime(now).
			SetUpdateTime(now)
		if reason != "" {
			builder.SetReason(reason + " (by " + operator + ")")
		} else if operator != "" {
			builder.SetReason("by " + operator)
		}
		if err := builder.Exec(ctx); err != nil {
			r.log.Errorf("write merge event failed: %s", err.Error())
			return adminV1.ErrorInternalServerError("write merge event failed")
		}
		return nil
	})
	if err != nil {
		return err
	}
	// 6) 事务后重算统计（S-3：以实际成员为准）
	if err := r.RecomputeStats(ctx, toID); err != nil {
		return err
	}
	return nil
}

// SplitTx 劈出成员形成新簇（审计走 dedup_samples manual_split）。
func (r *IntelClusterRepo) SplitTx(ctx context.Context, fromID string, intelIDs []string, operator, label string) (string, error) {
	if len(intelIDs) == 0 || fromID == "" {
		return "", icV1.ErrorIcConfigInvalid("拆簇参数非法")
	}
	if operator == "" {
		operator = "system"
	}
	now := time.Now().UTC()
	newClusterID := contract.NewClusterID()
	err := RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
		src, err := tx.IntelCluster.Get(ctx, fromID)
		if err != nil {
			if ent.IsNotFound(err) {
				return icV1.ErrorIcConfigInvalid("源簇不存在")
			}
			return adminV1.ErrorInternalServerError("query cluster failed")
		}
		if label == "" {
			label = src.Label
		}
		// 1) 新簇
		builder := tx.IntelCluster.Create().
			SetID(newClusterID).
			SetLabel(label).
			SetMemberCount(0).
			SetPlatformCount(0).
			SetRegion(src.Region).
			SetFirstSeenAt(now).
			SetLastSeenAt(now).
			SetMergedBy("manual").
			SetStatus("open").
			SetCreateTime(now).
			SetUpdateTime(now)
		if src.Category != nil {
			builder.SetCategory(*src.Category)
		}
		if err := builder.Exec(ctx); err != nil {
			r.log.Errorf("create split cluster failed: %s", err.Error())
			return adminV1.ErrorInternalServerError("create cluster failed")
		}
		// 2) 迁移指定成员
		if _, err := tx.IntelItem.Update().
			Where(intelitem.IDIn(intelIDs...), intelitem.ClusterIDEQ(fromID)).
			SetClusterID(newClusterID).
			SetUpdateTime(now).Save(ctx); err != nil {
			r.log.Errorf("split move members failed: %s", err.Error())
			return adminV1.ErrorInternalServerError("split cluster members failed")
		}
		// 3) 审计样本（每条成员一行 manual_split，M-27 黄金集评审依据）
		now2 := now
		creates := make([]*ent.DedupSampleCreate, 0, len(intelIDs))
		for _, iid := range intelIDs {
			creates = append(creates, tx.DedupSample.Create().
				SetID(contract.ULID()).
				SetIntelID(iid).
				SetClusterID(newClusterID).
				SetAction("manual_split").
				SetSimScore(0).
				SetOperator(operator).
				SetCreateTime(now2).
				SetUpdateTime(now2))
		}
		if err := tx.DedupSample.CreateBulk(creates...).Exec(ctx); err != nil {
			r.log.Errorf("write split samples failed: %s", err.Error())
			return adminV1.ErrorInternalServerError("write split samples failed")
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	// 事务后重算两簇统计
	if err := r.RecomputeStats(ctx, newClusterID); err != nil {
		return "", err
	}
	if err := r.RecomputeStats(ctx, fromID); err != nil {
		return "", err
	}
	return newClusterID, nil
}

func (r *IntelClusterRepo) ListIdleOpen(ctx context.Context, idleBefore time.Time, limit int) ([]*biz.IntelClusterRow, error) {
	if limit <= 0 {
		limit = 200
	}
	entities, err := r.data.db.Client().IntelCluster.Query().
		Where(
			intelcluster.StatusEQ("open"),
			intelcluster.LastSeenAtLT(idleBefore.UTC()),
		).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list idle clusters failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("list idle clusters failed")
	}
	out := make([]*biz.IntelClusterRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizIntelCluster(e))
	}
	return out, nil
}

func (r *IntelClusterRepo) Close(ctx context.Context, id string, at time.Time) error {
	if _, err := r.data.db.Client().IntelCluster.UpdateOneID(id).
		SetStatus("closed").
		SetClosedAt(at.UTC()).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("close cluster failed (id=%s): %s", id, err.Error())
		return adminV1.ErrorInternalServerError("close cluster failed")
	}
	return nil
}

func toBizIntelCluster(e *ent.IntelCluster) *biz.IntelClusterRow {
	if e == nil {
		return nil
	}
	return &biz.IntelClusterRow{
		ID:               e.ID,
		Label:            e.Label,
		MemberCount:      e.MemberCount,
		PlatformCount:    e.PlatformCount,
		RepresentativeID: derefStr(e.RepresentativeID),
		Region:           e.Region,
		Category:         derefStr(e.Category),
		FirstSeenAt:      e.FirstSeenAt.UTC(),
		LastSeenAt:       e.LastSeenAt.UTC(),
		MergedBy:         e.MergedBy,
		CentroidRef:      derefStr(e.CentroidRef),
		Status:           e.Status,
		ClosedAt:         e.ClosedAt,
	}
}

// ---------- 去重抽样（M-27 黄金集回放评审依据） ----------

type DedupSampleRepo struct {
	data *Data
	log  *log.Helper
}

func NewDedupSampleRepo(data *Data, logger log.Logger) biz.DedupSampleRepo {
	return &DedupSampleRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/dedupsample/repo/admin-service")),
		data: data,
	}
}

func (r *DedupSampleRepo) Create(ctx context.Context, row *biz.DedupSampleRow) error {
	if row.Operator == "" {
		row.Operator = "system"
	}
	now := time.Now().UTC()
	builder := r.data.db.Client().DedupSample.Create().
		SetID(contract.ULID()).
		SetIntelID(row.IntelID).
		SetClusterID(row.ClusterID).
		SetAction(row.Action).
		SetSimScore(row.SimScore).
		SetOperator(row.Operator).
		SetCreateTime(now).
		SetUpdateTime(now)
	if row.Level != "" {
		builder.SetLevel(row.Level)
	}
	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("create dedup sample failed (intel=%s): %s", row.IntelID, err.Error())
		return adminV1.ErrorInternalServerError("create dedup sample failed")
	}
	return nil
}

func (r *DedupSampleRepo) ListByCluster(ctx context.Context, clusterID string, limit int) ([]*biz.DedupSampleRow, error) {
	if limit <= 0 {
		limit = 100
	}
	entities, err := r.data.db.Client().DedupSample.Query().
		Where(dedupsample.ClusterIDEQ(clusterID)).
		Order(ent.Desc(dedupsample.FieldCreateTime)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list dedup samples failed (cluster=%s): %s", clusterID, err.Error())
		return nil, adminV1.ErrorInternalServerError("list dedup samples failed")
	}
	out := make([]*biz.DedupSampleRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, &biz.DedupSampleRow{
			IntelID:   e.IntelID,
			ClusterID: e.ClusterID,
			Action:    e.Action,
			Level:     derefStr(e.Level),
			SimScore:  e.SimScore,
			Operator:  e.Operator,
		})
	}
	return out, nil
}
