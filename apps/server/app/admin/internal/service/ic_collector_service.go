package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// ① IC · IntelCollectorService —— 单 URL 补采 / 健康度 / 运行记录 / 死信重放。
//  - FetchURL（FR-INT-04）：同主链路、跳过调度，治理不旁路（实现见 biz ic_fetch.go）。
//  - 死信重放（S-13）：operator 必填 + 状态机 pending→replayed；task_type 词表
//    collect|normalize 对应可执行动作，其余显式报错（不做「假成功」重放）。
// =============================================================================

// IcCollectorService 采集器管理与健康度。
type IcCollectorService struct {
	icV1.UnimplementedIntelCollectorServiceServer

	collectUC *biz.IcCollectUsecase
	normUC    *biz.IcNormalizeUsecase
	sources   biz.IntelSourceRepo
	runs      biz.CollectRunRepo
	dead      biz.DeadLetterRepo
	log       *log.Helper
}

// NewIcCollectorService 构造。
func NewIcCollectorService(
	collectUC *biz.IcCollectUsecase,
	normUC *biz.IcNormalizeUsecase,
	sources biz.IntelSourceRepo,
	runs biz.CollectRunRepo,
	dead biz.DeadLetterRepo,
	logger log.Logger,
) *IcCollectorService {
	return &IcCollectorService{
		collectUC: collectUC, normUC: normUC,
		sources: sources, runs: runs, dead: dead,
		log: log.NewHelper(log.With(logger, "module", "ic/collector/service")),
	}
}

// FetchURL 单 URL 补采（FR-INT-04 手贴续跑入口之一）。
// 幂等重放（dedup_key 命中）返回既有条目——天然安全重试。
func (s *IcCollectorService) FetchURL(ctx context.Context, req *icV1.FetchURLRequest) (*icV1.IntelItem, error) {
	if req.GetUrl() == "" {
		return nil, icV1.ErrorIcConfigInvalid("url 必填")
	}
	row, err := s.collectUC.FetchURL(ctx, req.GetUrl(), req.GetMethod())
	if err != nil {
		s.log.Warnf("fetch url failed (url=%s, method=%s): %v", req.GetUrl(), req.GetMethod(), err)
		return nil, mapIcError(err)
	}
	return intelItemToProto(&row.IntelItem), nil
}

// GetCollectorHealth 各信源健康度与熔断状态（NFR-IC-05）。
func (s *IcCollectorService) GetCollectorHealth(ctx context.Context, req *icV1.GetCollectorHealthRequest) (*icV1.GetCollectorHealthResponse, error) {
	rows, _, err := s.sources.List(ctx, &biz.IntelSourceListQuery{
		Page: 1, PageSize: 200, Platform: req.GetPlatform(),
	})
	if err != nil {
		return nil, err
	}
	out := &icV1.GetCollectorHealthResponse{}
	for _, r := range rows {
		out.Items = append(out.Items, &icV1.SourceHealthEntry{
			SourceId: r.ID, Platform: r.Platform, Method: r.Method,
			Health:        icHealthView(r),
			CircuitStatus: r.Status,
			LastRunAt:     icPBTime(r.LastRunAt),
		})
	}
	return out, nil
}

// ListCollectRuns 采集运行记录（审计流水）。
func (s *IcCollectorService) ListCollectRuns(ctx context.Context, req *icV1.ListCollectRunsRequest) (*icV1.ListCollectRunsResponse, error) {
	page, pageSize := icPageClamp(req.GetPage(), req.GetPageSize())
	rows, total, err := s.runs.List(ctx, req.GetSourceId(), page, pageSize)
	if err != nil {
		return nil, err
	}
	out := &icV1.ListCollectRunsResponse{Total: int32(total)}
	for _, r := range rows {
		out.Items = append(out.Items, &icV1.CollectRun{
			Id: r.ID, SourceId: r.SourceID, Trigger: r.Trigger,
			StartedAt:  timestamppb.New(r.StartedAt.UTC()),
			FinishedAt: icPBTime(r.FinishedAt),
			Fetched:    r.Fetched, Normalized: r.Normalized, Merged: r.Merged,
			Blocked: r.Blocked, Skipped: r.Skipped,
			Status: r.Status, ErrorCode: r.ErrorCode,
		})
	}
	return out, nil
}

// ---------- 死信（S-13） ----------

func (s *IcCollectorService) ListDeadLetters(ctx context.Context, req *icV1.ListDeadLettersRequest) (*icV1.ListDeadLettersResponse, error) {
	page, pageSize := icPageClamp(req.GetPage(), req.GetPageSize())
	rows, total, err := s.dead.List(ctx, req.GetStatus(), page, pageSize)
	if err != nil {
		return nil, err
	}
	out := &icV1.ListDeadLettersResponse{Total: int32(total)}
	for _, r := range rows {
		out.Items = append(out.Items, deadLetterToProto(r))
	}
	return out, nil
}

// ReplayDeadLetter 死信重放（S-13：operator 审计必填；成功后才 MarkReplayed——
// 失败保持 pending 留给下一次诊断，不丢失现场）。
func (s *IcCollectorService) ReplayDeadLetter(ctx context.Context, req *icV1.ReplayDeadLetterRequest) (*icV1.ReplayDeadLetterResponse, error) {
	if req.GetOperator() == "" {
		return nil, icV1.ErrorIcConfigInvalid("operator 必填（S-13 审计）")
	}
	row, err := s.dead.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, icV1.ErrorIcSrcNotFound("dead letter %s 不存在", req.GetId())
	}
	if row.Status != "pending" {
		return nil, icV1.ErrorIcConfigInvalid("仅 pending 状态可重放（当前 %s）", row.Status)
	}

	switch row.TaskType {
	case "collect":
		sourceID, _ := row.Payload["source_id"].(string)
		if sourceID == "" {
			return nil, icV1.ErrorIcConfigInvalid("死信 payload 缺 source_id（task_type=collect）")
		}
		// trigger=retry + force：人工诊断后的重放意图明确，不被熔断窗二次吞掉
		if _, _, rerr := s.collectUC.RunWithStats(ctx, sourceID, "retry", true); rerr != nil {
			return nil, mapIcError(rerr)
		}
	case "normalize":
		intelID, _ := row.Payload["intel_id"].(string)
		if intelID == "" {
			return nil, icV1.ErrorIcConfigInvalid("死信 payload 缺 intel_id（task_type=normalize）")
		}
		if _, rerr := s.normUC.Process(ctx, &contract.RawIntelNotice{
			APIVersion: "1.0", IntelID: intelID, CollectedAt: time.Now().UTC(),
		}, ""); rerr != nil {
			return nil, rerr
		}
	default:
		return nil, icV1.ErrorIcConfigInvalid("task_type=%s 暂不支持自动重放（publish 路由由 B-25 周期打捞覆盖）", row.TaskType)
	}

	if err := s.dead.MarkReplayed(ctx, req.GetId(), req.GetOperator(), time.Now().UTC()); err != nil {
		return nil, err
	}
	s.log.Infof("[audit] dead letter replayed: id=%s type=%s operator=%s", req.GetId(), row.TaskType, req.GetOperator())
	return &icV1.ReplayDeadLetterResponse{Ok: true}, nil
}

func deadLetterToProto(r *biz.DeadLetterRow) *icV1.DeadLetter {
	return &icV1.DeadLetter{
		Id: r.ID, TaskType: r.TaskType, ErrorCode: r.ErrorCode, ErrorMsg: r.ErrorMsg,
		Attempt:       r.Attempt,
		FirstFailedAt: timestamppb.New(r.FirstFailedAt.UTC()),
		LastFailedAt:  timestamppb.New(r.LastFailedAt.UTC()),
		Status:        r.Status, ReplayedBy: r.ReplayedBy, ReplayedAt: icPBTime(r.ReplayedAt),
	}
}
