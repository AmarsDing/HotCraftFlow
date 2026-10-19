package pubdata

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mapperformancesnapshot"
)

// EntPerformanceRepo bizpub.PerformanceRepo 的 Ent 实现（FR-PUB-08 读写两侧：
// worker/map 分析管道追加时序快照，REST/dispatch 用例读最新一条）。
type EntPerformanceRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntPerformanceRepo 构造。
func NewEntPerformanceRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntPerformanceRepo {
	return &EntPerformanceRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/map-performance-repo")),
	}
}

// SaveSnapshot 追加一条时序快照（append-only：表现数随时变化，历史轮次保留供趋势观察；
// 快照间无去重约束——同回执每轮扫描一行即一次时间片样本）。
func (r *EntPerformanceRepo) SaveSnapshot(ctx context.Context, row *bizpub.PerformanceSnapshotRow) error {
	builder := r.cli.Client().MapPerformanceSnapshot.Create().
		SetSnapshotID(row.SnapshotID).
		SetPlatformKey(row.PlatformKey).
		SetViews(row.Views).
		SetLikes(row.Likes).
		SetComments(row.Comments).
		SetShares(row.Shares).
		SetFavorites(row.Favorites).
		SetCompletionRate(row.CompletionRate).
		SetFetchedAt(row.FetchedAt.UTC())
	if row.ReceiptID != "" {
		builder.SetReceiptID(row.ReceiptID)
	}
	if row.DispatchUnitID != nil {
		builder.SetNillableDispatchUnitID(row.DispatchUnitID)
	}
	if row.VariantID != nil {
		builder.SetNillableVariantID(row.VariantID)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		r.log.Errorf("save performance snapshot failed snapshot=%s receipt=%s: %v", row.SnapshotID, row.ReceiptID, err)
		return fmt.Errorf("%w: 表现快照写入失败", bizpub.ErrInternal)
	}
	row.ID = uint32(e.ID)
	return nil
}

// LatestByReceipt 取回执最新一条快照；无数据返回 (nil, nil)——调用方据此输出诚实 note，不伪造。
func (r *EntPerformanceRepo) LatestByReceipt(ctx context.Context, receiptID string) (*bizpub.PerformanceSnapshotRow, error) {
	e, err := r.cli.Client().MapPerformanceSnapshot.Query().
		Where(mapperformancesnapshot.ReceiptIDEQ(receiptID)).
		Order(ent.Desc(mapperformancesnapshot.FieldFetchedAt), ent.Desc(mapperformancesnapshot.FieldID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query latest performance snapshot failed receipt=%s: %v", receiptID, err)
		return nil, fmt.Errorf("%w: 查询表现快照失败", bizpub.ErrInternal)
	}
	return snapshotToRow(e), nil
}

// snapshotToRow 实体 → 业务视图（dispatch_unit_id/variant_id 为闭环回指字段，nillable 透传）。
func snapshotToRow(e *ent.MapPerformanceSnapshot) *bizpub.PerformanceSnapshotRow {
	return &bizpub.PerformanceSnapshotRow{
		ID:             uint32(e.ID),
		SnapshotID:     e.SnapshotID,
		ReceiptID:      enumPtrToStr(e.ReceiptID),
		PlatformKey:    e.PlatformKey,
		DispatchUnitID: e.DispatchUnitID,
		VariantID:      e.VariantID,
		Views:          e.Views,
		Likes:          e.Likes,
		Comments:       e.Comments,
		Shares:         e.Shares,
		Favorites:      e.Favorites,
		CompletionRate: e.CompletionRate,
		FetchedAt:      e.FetchedAt,
	}
}
