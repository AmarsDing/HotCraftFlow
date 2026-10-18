package publish

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// DispatchUsecase 查询组合用例：列表 / 详情 / 审批队列 / 日历 / 回执 / 表现数据。
// 全部只读；状态变更在 ApprovalGateUsecase / PublishUsecase。
type DispatchUsecase struct {
	dispatch    DispatchRepo
	records     ApprovalRepo
	receipts    ReceiptRepo
	performance PerformanceRepo
	log         *log.Helper
}

// NewDispatchUsecase 构造查询用例。
func NewDispatchUsecase(
	dispatch DispatchRepo,
	records ApprovalRepo,
	receipts ReceiptRepo,
	performance PerformanceRepo,
	logger log.Logger,
) *DispatchUsecase {
	return &DispatchUsecase{
		dispatch:    dispatch,
		records:     records,
		receipts:    receipts,
		performance: performance,
		log:         log.NewHelper(log.With(logger, "module", "biz/publish-query")),
	}
}

// UnitDetail 详情视图：单元 + 审批流水 + 回执。
type UnitDetail struct {
	Unit     *UnitRow
	Records  []*RecordEntry
	Receipts []*ReceiptRow
}

// ListUnits 分发单元分页列表。
func (uc *DispatchUsecase) ListUnits(ctx context.Context, pageNo, pageSize int) ([]*UnitRow, int, error) {
	return uc.dispatch.ListUnits(ctx, pageNo, pageSize)
}

// ListApprovalQueue 审批队列（approval_state=draft，FR-PUB-04 核心页面数据源）。
func (uc *DispatchUsecase) ListApprovalQueue(ctx context.Context, pageNo, pageSize int) ([]*UnitRow, int, error) {
	return uc.dispatch.ListUnitsByApproval(ctx, string(mediacontract.ApprovalDraft), pageNo, pageSize)
}

// GetUnitDetail 详情（含审计流水与回执）。
func (uc *DispatchUsecase) GetUnitDetail(ctx context.Context, unitID string) (*UnitDetail, error) {
	unit, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	records, err := uc.records.ListRecordsByUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	receipts, err := uc.receipts.ListReceiptsByUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	return &UnitDetail{Unit: unit, Records: records, Receipts: receipts}, nil
}

// ListCalendar 日历视图（scheduled_for ∈ [from, to]，FR-PUB-09 只读）。
func (uc *DispatchUsecase) ListCalendar(ctx context.Context, from, to time.Time) ([]*UnitRow, error) {
	return uc.dispatch.ListScheduledBetween(ctx, from, to)
}

// ListReceipts 回执分页（FR-PUB-07；前端 CSV 导出）。
func (uc *DispatchUsecase) ListReceipts(ctx context.Context, pageNo, pageSize int) ([]*ReceiptRow, int, error) {
	return uc.receipts.ListReceipts(ctx, pageNo, pageSize)
}

// GetAnalytics 表现数据（FR-PUB-08）。无快照时返回 (nil, receipt, nil)——
// service 层据此输出诚实 note（「表现数据尚未回流」），绝不伪造数字。
func (uc *DispatchUsecase) GetAnalytics(ctx context.Context, receiptID string) (*PerformanceSnapshotRow, *ReceiptRow, error) {
	receipt, err := uc.receipts.GetReceipt(ctx, receiptID)
	if err != nil {
		return nil, nil, err
	}
	snap, err := uc.performance.LatestByReceipt(ctx, receiptID)
	if err != nil {
		return nil, nil, err
	}
	return snap, receipt, nil
}
