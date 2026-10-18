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

// EntPerformanceRepo bizpub.PerformanceRepo 的 Ent 实现（V1 只读最新；
// 拉取 worker 与 mm.map.feedback.v1 回流管道属 Phase 3）。
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
	return &bizpub.PerformanceSnapshotRow{
		ID:             uint32(e.ID),
		SnapshotID:     e.SnapshotID,
		ReceiptID:      enumPtrToStr(e.ReceiptID),
		PlatformKey:    e.PlatformKey,
		Views:          e.Views,
		Likes:          e.Likes,
		Comments:       e.Comments,
		Shares:         e.Shares,
		Favorites:      e.Favorites,
		CompletionRate: e.CompletionRate,
		FetchedAt:      e.FetchedAt,
	}, nil
}
