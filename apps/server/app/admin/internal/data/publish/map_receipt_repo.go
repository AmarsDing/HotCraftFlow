package pubdata

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mappublishreceipt"
)

// EntReceiptRepo bizpub.ReceiptRepo 的 Ent 实现。
// 🔴 (dispatch_unit_id, platform_key) 唯一索引 = 发布幂等 DB 兜底（NFR-MAP-05）：
// 冲突转 bizpub.ErrReceiptConflict，由发布用例按幂等重放处理。
type EntReceiptRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntReceiptRepo 构造。
func NewEntReceiptRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntReceiptRepo {
	return &EntReceiptRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/map-receipt-repo")),
	}
}

// CreateReceipt 新增回执；唯一索引冲突 → ErrReceiptConflict。
func (r *EntReceiptRepo) CreateReceipt(ctx context.Context, row *bizpub.ReceiptRow) error {
	builder := r.cli.Client().MapPublishReceipt.Create().
		SetReceiptID(row.ReceiptID).
		SetDispatchUnitID(row.DispatchUnitID).
		SetPlatformKey(row.PlatformKey).
		SetVisibility(mappublishreceipt.Visibility(row.Visibility)).
		SetPublishedAt(row.PublishedAt.UTC()).
		SetOperator(row.Operator)
	if row.AccountID != nil {
		builder.SetNillableAccountID(row.AccountID)
	}
	if row.PostID != nil {
		builder.SetNillablePostID(row.PostID)
	}
	if row.PostURL != nil {
		builder.SetNillablePostURL(row.PostURL)
	}
	if row.RequestPayloadHash != nil {
		builder.SetNillableRequestPayloadHash(row.RequestPayloadHash)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			// 🔴 幂等兜底命中：正常信号（重复发布被 DB 拦下），非错误路径。
			return fmt.Errorf("%w: 单元 %s 在平台 %s 已有回执", bizpub.ErrReceiptConflict, row.DispatchUnitID, row.PlatformKey)
		}
		r.log.Errorf("create map receipt failed unit=%s platform=%s: %v", row.DispatchUnitID, row.PlatformKey, err)
		return fmt.Errorf("%w: 回执写入失败", bizpub.ErrInternal)
	}
	row.ID = uint32(e.ID)
	return nil
}

// ListReceiptsByUnit 按 unit 列出回执（published_at 升序）。
func (r *EntReceiptRepo) ListReceiptsByUnit(ctx context.Context, unitID string) ([]*bizpub.ReceiptRow, error) {
	entities, err := r.cli.Client().MapPublishReceipt.Query().
		Where(mappublishreceipt.DispatchUnitIDEQ(unitID)).
		Order(ent.Asc(mappublishreceipt.FieldPublishedAt), ent.Asc(mappublishreceipt.FieldID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map receipts by unit failed unit=%s: %v", unitID, err)
		return nil, fmt.Errorf("%w: 查询单元回执失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.ReceiptRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, receiptToRow(e))
	}
	return rows, nil
}

// ListReceipts 分页列出（published_at 倒序）。
func (r *EntReceiptRepo) ListReceipts(ctx context.Context, pageNo, pageSize int) ([]*bizpub.ReceiptRow, int, error) {
	pageNo, pageSize = clampPage(pageNo, pageSize)
	q := r.cli.Client().MapPublishReceipt.Query()
	total, err := q.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count map receipts failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询回执总数失败", bizpub.ErrInternal)
	}
	entities, err := q.
		Order(ent.Desc(mappublishreceipt.FieldPublishedAt), ent.Desc(mappublishreceipt.FieldID)).
		Offset((pageNo - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map receipts failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询回执列表失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.ReceiptRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, receiptToRow(e))
	}
	return rows, total, nil
}

// GetReceipt 按 receipt_id（pr_ 前缀）查询。
func (r *EntReceiptRepo) GetReceipt(ctx context.Context, receiptID string) (*bizpub.ReceiptRow, error) {
	e, err := r.cli.Client().MapPublishReceipt.Query().
		Where(mappublishreceipt.ReceiptIDEQ(receiptID)).
		Only(ctx)
	if err != nil {
		return nil, mapQueryErr(err, "回执")
	}
	return receiptToRow(e), nil
}

// CountReceiptsSince 统计 since 后的回执总数（24h 总量配额）。
func (r *EntReceiptRepo) CountReceiptsSince(ctx context.Context, since time.Time) (int, error) {
	n, err := r.cli.Client().MapPublishReceipt.Query().
		Where(mappublishreceipt.PublishedAtGTE(since.UTC())).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count map receipts since failed: %v", err)
		return 0, fmt.Errorf("%w: 统计回执失败", bizpub.ErrInternal)
	}
	return n, nil
}

// CountReceiptsByPlatformSince 统计指定平台 since 后的回执数（平台小时配额）。
func (r *EntReceiptRepo) CountReceiptsByPlatformSince(ctx context.Context, platform string, since time.Time) (int, error) {
	n, err := r.cli.Client().MapPublishReceipt.Query().
		Where(
			mappublishreceipt.PlatformKeyEQ(platform),
			mappublishreceipt.PublishedAtGTE(since.UTC()),
		).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count map receipts by platform failed platform=%s: %v", platform, err)
		return 0, fmt.Errorf("%w: 统计平台回执失败", bizpub.ErrInternal)
	}
	return n, nil
}

// ListReceiptsPublishedSince 回流扫描源（worker/map 分析拉取管道）：
// published_at ≥ since 且 post_id 非空（无 post_id = 平台无回执对象，无可拉取），
// published_at 倒序（新鲜数据优先——批量受限时优先保新鲜轮次），限 limit。
func (r *EntReceiptRepo) ListReceiptsPublishedSince(ctx context.Context, since time.Time, limit int) ([]*bizpub.ReceiptRow, error) {
	// limit <= 0 → 默认 64（接口语义，见 biz/publish/ports.go；fake 必须复制本默认）。
	if limit <= 0 {
		limit = 64
	}
	entities, err := r.cli.Client().MapPublishReceipt.Query().
		Where(
			mappublishreceipt.PublishedAtGTE(since.UTC()),
			mappublishreceipt.PostIDNotNil(),
			mappublishreceipt.PostIDNEQ(""),
		).
		Order(ent.Desc(mappublishreceipt.FieldPublishedAt), ent.Desc(mappublishreceipt.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list receipts for analytics pull failed since=%s: %v", since.Format(time.RFC3339), err)
		return nil, fmt.Errorf("%w: 查询回流回执失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.ReceiptRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, receiptToRow(e))
	}
	return rows, nil
}

// ListReceiptsPublishedSinceBefore 分页变体（M6 修复）：published_at ∈ [since, before)
// 且 post_id 非空，published_at 倒序，限 limit。worker/map 单轮内以「本页最旧
// published_at」为下一页 before 游标翻页，把 30 天窗口拉满——此前无分页，>256 条
// 时最老回执永远进不了任何一轮。before 为零值时退化为窗口上界 now（首页）。
func (r *EntReceiptRepo) ListReceiptsPublishedSinceBefore(ctx context.Context, since, before time.Time, limit int) ([]*bizpub.ReceiptRow, error) {
	if limit <= 0 {
		limit = 64
	}
	q := r.cli.Client().MapPublishReceipt.Query().
		Where(
			mappublishreceipt.PublishedAtGTE(since.UTC()),
			mappublishreceipt.PostIDNotNil(),
			mappublishreceipt.PostIDNEQ(""),
		)
	if !before.IsZero() {
		// 严格小于：上一页最旧行已在上一页处理，等值会重复拉同一行。
		q = q.Where(mappublishreceipt.PublishedAtLT(before.UTC()))
	}
	entities, err := q.
		Order(ent.Desc(mappublishreceipt.FieldPublishedAt), ent.Desc(mappublishreceipt.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list receipts page for analytics pull failed since=%s before=%s: %v",
			since.Format(time.RFC3339), before.Format(time.RFC3339), err)
		return nil, fmt.Errorf("%w: 查询回流回执分页失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.ReceiptRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, receiptToRow(e))
	}
	return rows, nil
}

func receiptToRow(e *ent.MapPublishReceipt) *bizpub.ReceiptRow {
	return &bizpub.ReceiptRow{
		ID:                 uint32(e.ID),
		ReceiptID:          e.ReceiptID,
		DispatchUnitID:     e.DispatchUnitID,
		AccountID:          e.AccountID,
		PlatformKey:        e.PlatformKey,
		PostID:             e.PostID,
		PostURL:            e.PostURL,
		Visibility:         string(e.Visibility),
		PublishedAt:        e.PublishedAt,
		Operator:           e.Operator,
		RequestPayloadHash: e.RequestPayloadHash,
	}
}
