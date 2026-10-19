package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
	mmeV1 "framework/api/gen/go/media/v1/mme"
	bizmme "framework/app/admin/internal/biz/mme"
)

// MmeRenderService 渲染队列 REST（FR-MIX-03 规划/进度/重试）。
// 进度与 ETA 全部由真实 job 聚合；无完成样本时 eta=-1（前端显示「估算中」），不编数。
type MmeRenderService struct {
	planner *bizmme.RenderPlanner
	log     *log.Helper
}

// NewMmeRenderService 构造。
func NewMmeRenderService(planner *bizmme.RenderPlanner, logger log.Logger) *MmeRenderService {
	return &MmeRenderService{
		planner: planner,
		log:     log.NewHelper(log.With(logger, "module", "mme/render/service")),
	}
}

// ListJobs 渲染任务列表（query={"state":"failed"} / {"variant_id":"v_..."} 过滤走数据层）。
func (s *MmeRenderService) ListJobs(ctx context.Context, req *pagination.PagingRequest) (*mmeV1.ListRenderJobsResponse, error) {
	items, total, err := s.planner.ListJobs(ctx, bizmme.JobQuery{
		State:    mmeQueryFilter(req.GetQuery(), "state"),
		Page:     normalizePage(req.GetPage()),
		PageSize: normalizePageSize(req.GetPageSize()),
	})
	if err != nil {
		return nil, mmeBizErr(err)
	}
	out := make([]*mmeV1.RenderJob, 0, len(items))
	for _, j := range items {
		out = append(out, s.toProtoRenderJob(ctx, j, -1))
	}
	return &mmeV1.ListRenderJobsResponse{Items: out, Total: uint32(total)}, nil
}

// GetSetProgress 某变体簇的渲染进度（含 ETA）。
func (s *MmeRenderService) GetSetProgress(ctx context.Context, req *mmeV1.GetVariantSetRequest) (*mmeV1.SetRenderProgress, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	v, err := s.planner.SetProgress(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	// 每 job 剩余时间不可精算（单 job 级 out_time 进度在 worker 侧）；
	// 本端点 job 行仍然如实回填 eta=-1，簇级 ETA 用 SetEta。
	out := &mmeV1.SetRenderProgress{
		SetId:           v.SetID,
		Total:           v.Total,
		Queued:          v.Queued,
		Running:         v.Running,
		Done:            v.Done,
		Failed:          v.Failed,
		Cancelled:       v.Cancelled,
		OverallProgress: v.Overall,
		EtaSec:          int32(v.EtaSec),
	}
	out.Jobs = make([]*mmeV1.RenderJob, 0, len(v.Jobs))
	for _, j := range v.Jobs {
		out.Jobs = append(out.Jobs, s.toProtoRenderJob(ctx, j, -1))
	}
	return out, nil
}

// RetryJob 失败任务重试（attempt+1，重新入队；保留上次中间产物供排查）。
func (s *MmeRenderService) RetryJob(ctx context.Context, req *mmeV1.RetryJobRequest) (*emptypb.Empty, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	if err := s.planner.RetryJob(ctx, req.GetId()); err != nil {
		return nil, mmeBizErr(err)
	}
	return &emptypb.Empty{}, nil
}

// CancelJob 取消排队中的任务（仅 pending 可取消；running 由 worker 收尾）。
func (s *MmeRenderService) CancelJob(ctx context.Context, req *mmeV1.RetryJobRequest) (*emptypb.Empty, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	if err := s.planner.CancelJob(ctx, req.GetId()); err != nil {
		return nil, mmeBizErr(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 映射 ----

// toProtoRenderJob job→proto。展示字段（aspect/content_version/set_id/duration）从变体回填；
// 变体行缺失时留空（产物可能被清理），不阻断列表。
func (s *MmeRenderService) toProtoRenderJob(ctx context.Context, j *bizmme.RenderJob, etaSec int) *mmeV1.RenderJob {
	out := &mmeV1.RenderJob{
		Id:               j.ID,
		VariantId:        j.VariantID,
		State:            j.State,
		Progress:         j.Progress,
		StartedAt:        formatRFC3339OrEmpty(j.StartedAt),
		FinishedAt:       formatRFC3339OrEmpty(j.FinishedAt),
		WorkerId:         j.WorkerID,
		ErrorCode:        j.ErrorCode,
		ErrorMsg:         j.ErrorMsg,
		IntermediatePath: j.IntermediatePath,
		Attempt:          int32(j.Attempt),
		EtaSec:           int32(etaSec),
	}
	if v, err := s.planner.GetVariant(ctx, j.VariantID); err == nil && v != nil {
		out.Aspect = v.Aspect
		out.ContentVersion = v.ContentVersion
		out.DurationSec = v.DurationSec
		out.SetId = v.SetID
	}
	return out
}
