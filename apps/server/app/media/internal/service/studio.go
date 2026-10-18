// Package service 媒体工作室的 kratos 服务实现（proto 路由的业务面）。
//
// 错误语义纪律：对外一律 kratos 业务错误（稳定 reason），内部错误细节
// 只进日志；与 pkg/httpx.ErrorEncoder 的归一化配合出统一错误体。
package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"google.golang.org/protobuf/types/known/emptypb"

	studiov1 "framework/api/gen/go/media/v1/studio"
	"framework/app/media/internal/doctor"
	"framework/app/media/internal/engines"
	"framework/app/media/internal/hostcaps"
	"framework/app/media/internal/jobrun"
	"framework/app/media/internal/jobstore"
)

// 类型 → 默认 lane（请求 lane 为空时的兜底映射）。
var defaultLaneByType = map[string]string{
	"echo.synthesize": "infer",
	"exec.probe":      "io",
}

// timeFormat 对外时间戳格式（稳定契约）。
func ts(t time.Time) string { return t.UTC().Format(time.RFC3339) }

func tsPtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return ts(*t)
}

// ---------------------------------------------------------------- jobs

// JobsService MediaJobsService 实现。
type JobsService struct {
	studiov1.UnimplementedMediaJobsServiceServer
	ex *jobrun.Executor
}

// NewJobsService 构造。
func NewJobsService(ex *jobrun.Executor) *JobsService { return &JobsService{ex: ex} }

// SubmitJob 提交任务（idempotency_key 幂等）。
func (s *JobsService) SubmitJob(ctx context.Context, req *studiov1.SubmitJobRequest) (*studiov1.MediaJob, error) {
	if req.GetType() == "" {
		return nil, kerrors.BadRequest("MEDIA_BAD_REQUEST", "type is required")
	}
	payload := req.GetPayloadJson()
	if payload == "" {
		payload = "{}"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(payload), &obj); err != nil {
		return nil, kerrors.BadRequest("MEDIA_BAD_REQUEST", "payload_json must be a JSON object: "+err.Error())
	}

	lane := req.GetLane()
	if lane == "" {
		lane = defaultLaneByType[req.GetType()]
	}
	j, _, err := s.ex.Submit(ctx, lane, req.GetType(), json.RawMessage(payload), req.GetIdempotencyKey())
	switch {
	case errors.Is(err, jobrun.ErrUnknownJobType):
		return nil, kerrors.BadRequest("MEDIA_UNKNOWN_JOB_TYPE", err.Error())
	case err != nil:
		return nil, kerrors.InternalServer("MEDIA_INTERNAL", "submit failed")
	}
	return s.jobView(ctx, j), nil
}

// ListJobs 任务列表。
func (s *JobsService) ListJobs(ctx context.Context, req *studiov1.ListJobsRequest) (*studiov1.ListJobsResponse, error) {
	jobs, total, err := s.ex.List(ctx, jobstore.ListFilter{
		State: req.GetState(), Lane: req.GetLane(), Type: req.GetType(),
		Page: req.GetPage(), PageSize: req.GetPageSize(),
	})
	if err != nil {
		return nil, kerrors.InternalServer("MEDIA_INTERNAL", "list failed")
	}
	out := &studiov1.ListJobsResponse{Total: total}
	for i := range jobs {
		out.Items = append(out.Items, s.jobView(ctx, jobs[i]))
	}
	return out, nil
}

// GetJob 任务详情。
func (s *JobsService) GetJob(ctx context.Context, req *studiov1.JobIdRequest) (*studiov1.MediaJob, error) {
	j, err := s.ex.Get(ctx, req.GetId())
	if errors.Is(err, jobstore.ErrNotFound) {
		return nil, kerrors.NotFound("MEDIA_JOB_NOT_FOUND", "job not found")
	}
	if err != nil {
		return nil, kerrors.InternalServer("MEDIA_INTERNAL", "get failed")
	}
	return s.jobView(ctx, j), nil
}

// CancelJob 幂等取消：终态重复调用直接返回当前 job（200）。
func (s *JobsService) CancelJob(ctx context.Context, req *studiov1.JobIdRequest) (*studiov1.MediaJob, error) {
	j, err := s.ex.Cancel(ctx, req.GetId())
	if errors.Is(err, jobstore.ErrNotFound) {
		return nil, kerrors.NotFound("MEDIA_JOB_NOT_FOUND", "job not found")
	}
	if err != nil && !errors.Is(err, jobrun.ErrNotCancellable) {
		return nil, kerrors.InternalServer("MEDIA_INTERNAL", "cancel failed")
	}
	return s.jobView(ctx, j), nil
}

// RetryJob 终态重试。
func (s *JobsService) RetryJob(ctx context.Context, req *studiov1.JobIdRequest) (*studiov1.MediaJob, error) {
	j, err := s.ex.Retry(ctx, req.GetId())
	switch {
	case errors.Is(err, jobstore.ErrNotFound):
		return nil, kerrors.NotFound("MEDIA_JOB_NOT_FOUND", "job not found")
	case errors.Is(err, jobrun.RetryRejected):
		return nil, kerrors.BadRequest("MEDIA_JOB_NOT_RETRYABLE", err.Error())
	case err != nil:
		return nil, kerrors.InternalServer("MEDIA_INTERNAL", "retry failed")
	}
	return s.jobView(ctx, j), nil
}

// jobView 内部任务 → proto 视图（含队列位次与事件水位）。
func (s *JobsService) jobView(ctx context.Context, j jobstore.Job) *studiov1.MediaJob {
	return &studiov1.MediaJob{
		Id:            j.ID,
		Lane:          j.Lane,
		Type:          j.Type,
		State:         string(j.State),
		Progress:      j.Progress,
		PayloadJson:   j.PayloadJSON,
		ResultJson:    j.ResultJSON,
		ErrorCode:     j.ErrorCode,
		ErrorMsg:      j.ErrorMsg,
		Attempt:       j.Attempt,
		WorkerId:      j.WorkerID,
		QueuedAt:      ts(j.QueuedAt),
		StartedAt:     tsPtr(j.StartedAt),
		FinishedAt:    tsPtr(j.FinishedAt),
		QueuePosition: s.ex.QueuePosition(ctx, j),
		LastSeq:       j.LastSeq,
	}
}

// ---------------------------------------------------------------- engines

// EnginesService MediaEnginesService 实现。
type EnginesService struct {
	studiov1.UnimplementedMediaEnginesServiceServer
	reg  *engines.Registry
	host hostcaps.Caps
}

// NewEnginesService 构造。
func NewEnginesService(reg *engines.Registry, host hostcaps.Caps) *EnginesService {
	return &EnginesService{reg: reg, host: host}
}

// ListEngines 引擎清单。
func (s *EnginesService) ListEngines(ctx context.Context, _ *emptypb.Empty) (*studiov1.ListEnginesResponse, error) {
	views := s.reg.List(ctx)
	out := &studiov1.ListEnginesResponse{
		ActiveId:  s.reg.ActiveID(),
		HostAccel: s.host.Accel,
	}
	for _, v := range views {
		out.Items = append(out.Items, engineViewProto(v))
	}
	return out, nil
}

// SelectEngine 激活引擎（unavailable 直接 400）。
func (s *EnginesService) SelectEngine(ctx context.Context, req *studiov1.SelectEngineRequest) (*studiov1.EngineView, error) {
	if req.GetId() == "" {
		return nil, kerrors.BadRequest("MEDIA_BAD_REQUEST", "id is required")
	}
	v, err := s.reg.Select(ctx, req.GetId())
	switch {
	case errors.Is(err, engines.ErrEngineNotFound):
		return nil, kerrors.NotFound("MEDIA_ENGINE_NOT_FOUND", "engine not found")
	case errors.Is(err, engines.ErrEngineNoCPUFallbk),
		errors.Is(err, engines.ErrEngineUnavailable):
		// 不静默降级：把稳定公开原因返回给客户。
		msg := v.Reason
		if msg == "" {
			msg = v.RoutingNotice
		}
		if msg == "" {
			msg = "engine unavailable on this host"
		}
		return nil, kerrors.BadRequest("MEDIA_ENGINE_UNAVAILABLE", msg)
	case err != nil:
		return nil, kerrors.InternalServer("MEDIA_INTERNAL", "select failed")
	}
	return engineViewProto(v), nil
}

// GetEngineHealth 实时可用性探针（计时）。
func (s *EnginesService) GetEngineHealth(ctx context.Context, req *studiov1.JobIdRequest) (*studiov1.EngineHealth, error) {
	v, found := lookupView(s.reg, ctx, req.GetId())
	if !found {
		return nil, kerrors.NotFound("MEDIA_ENGINE_NOT_FOUND", "engine not found")
	}
	start := time.Now()
	ok := v.Available
	cost := time.Since(start).Milliseconds()
	return &studiov1.EngineHealth{
		Id:        v.ID,
		Available: ok,
		Reason:    v.Reason,
		Routing:   string(v.Routing),
		LatencyMs: cost,
	}, nil
}

// lookupView 从清单中找指定 id 的视图（List 已含可用性实检）。
func lookupView(reg *engines.Registry, ctx context.Context, id string) (engines.View, bool) {
	for _, v := range reg.List(ctx) {
		if v.ID == id {
			return v, true
		}
	}
	return engines.View{}, false
}

// engineViewProto 引擎视图 → proto。
func engineViewProto(v engines.View) *studiov1.EngineView {
	return &studiov1.EngineView{
		Id:          v.ID,
		Kind:        string(v.Kind),
		DisplayName: v.DisplayName,
		Caps: &studiov1.EngineCaps{
			GpuCompat:       v.Caps.GPUCompat,
			MinVramGb:       v.Caps.MinVRAMGB,
			SupportsCloning: v.Caps.SupportsCloning,
			SupportsEmotion: v.Caps.SupportsEmotion,
		},
		Available:     v.Available,
		Reason:        v.Reason,
		Routing:       string(v.Routing),
		RoutingNotice: v.RoutingNotice,
		Selected:      v.Selected,
	}
}

// ---------------------------------------------------------------- doctor

// DoctorService MediaDoctorService 实现。
type DoctorService struct {
	studiov1.UnimplementedMediaDoctorServiceServer
	probes  []string
	timeout time.Duration
	host    hostcaps.Caps
}

// NewDoctorService 构造。
func NewDoctorService(probes []string, timeout time.Duration, host hostcaps.Caps) *DoctorService {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	return &DoctorService{probes: probes, timeout: timeout, host: host}
}

// GetDoctorReport 环境体检。
func (s *DoctorService) GetDoctorReport(ctx context.Context, _ *emptypb.Empty) (*studiov1.DoctorReport, error) {
	rep := doctor.Report(ctx, s.probes, s.timeout)
	out := &studiov1.DoctorReport{
		HostAccel: s.host.Accel,
		CheckedAt: ts(time.Now()),
	}
	for _, p := range rep {
		out.Probes = append(out.Probes, &studiov1.ProbeResult{
			Name:    p.Name,
			Ok:      p.OK,
			Path:    p.Path,
			Version: p.Version,
			Hint:    p.Hint,
		})
	}
	return out, nil
}
