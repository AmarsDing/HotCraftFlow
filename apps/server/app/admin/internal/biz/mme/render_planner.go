package mme

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// RenderPlanner FR-MIX-03 渲染规划器：
// 把 selected=true 且未完成的变体逐个建 RenderJob 并入 asynq 队列。
// 幂等：已有非 failed 运行态 job 的变体跳过（重复触发不产生重复任务）。
type RenderPlanner struct {
	cfg      Config
	variants VariantRepo
	jobs     RenderJobRepo
	sets     VariantSetRepo
	enq      TaskEnqueuer
	log      *log.Helper
}

// NewRenderPlanner 构造。
func NewRenderPlanner(cfg Config, variants VariantRepo, jobs RenderJobRepo, sets VariantSetRepo, enq TaskEnqueuer, logger log.Logger) *RenderPlanner {
	return &RenderPlanner{cfg: cfg, variants: variants, jobs: jobs, sets: sets, enq: enq, log: log.NewHelper(log.With(logger, "module", "biz/mme/planner"))}
}

// PlanResult 规划结果（供 HTTP 响应：queued/skipped/already_done）。
type PlanResult struct {
	Queued      int32
	Skipped     int32
	AlreadyDone int32
}

// Plan 为一个 VariantSet 规划渲染。onlyIDs 非空时只处理这些变体（单体重渲复用）。
func (p *RenderPlanner) Plan(ctx context.Context, set *VariantSet, onlyIDs []string) (*PlanResult, error) {
	vs, err := p.variants.ListBySet(ctx, set.ID)
	if err != nil {
		return nil, err
	}
	filter := map[string]bool{}
	for _, id := range onlyIDs {
		filter[id] = true
	}
	res := &PlanResult{}
	setChanged := false
	for _, v := range vs {
		if len(filter) > 0 && !filter[v.ID] {
			continue
		}
		if !v.Selected {
			res.Skipped++
			continue
		}
		switch v.RenderState {
		case RenderDone:
			res.AlreadyDone++
			continue
		case RenderRunning:
			res.AlreadyDone++
			continue
		}
		// reused job or create
		job, err := p.jobs.GetByVariant(ctx, v.ID)
		if err != nil {
			return nil, err
		}
		if job == nil {
			job = &RenderJob{
				VariantID: v.ID,
				State:     RenderPending,
				Progress:  0,
				CreatedAt: time.Now().UTC(),
			}
			if err = p.jobs.Create(ctx, job); err != nil {
				return nil, err
			}
		} else {
			job.State = RenderPending
			job.Progress = 0
			job.Attempt++
			if err = p.jobs.Update(ctx, job); err != nil {
				return nil, err
			}
		}
		v.RenderState = RenderPending
		v.RenderError = ""
		if err = p.variants.Update(ctx, v); err != nil {
			return nil, err
		}
		if err = p.enq.EnqueueRender(ctx, job.ID); err != nil {
			return nil, err
		}
		res.Queued++
		setChanged = true
	}
	if setChanged && set.State != SetRendering {
		set.State = SetRendering
		if err = p.sets.Update(ctx, set); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// RetryJob 失败 job 重试（attempt++，状态回 pending，重新入队）。
func (p *RenderPlanner) RetryJob(ctx context.Context, jobID string) error {
	job, err := p.jobs.Get(ctx, jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return ErrJobNotFound
	}
	if job.State != RenderFailed {
		return ErrInvalidState
	}
	job.State = RenderPending
	job.Progress = 0
	job.ErrorCode, job.ErrorMsg = "", ""
	job.Attempt++
	if err = p.jobs.Update(ctx, job); err != nil {
		return err
	}
	return p.enq.EnqueueRender(ctx, job.ID)
}

// CancelJob 取消 job（仅 pending 可取消；running 由 worker 自行收尾）。
func (p *RenderPlanner) CancelJob(ctx context.Context, jobID string) error {
	job, err := p.jobs.Get(ctx, jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return ErrJobNotFound
	}
	if job.State != RenderPending {
		return ErrInvalidState
	}
	job.State = RenderFailed
	job.ErrorCode = ErrInvalidState.Error()
	job.ErrorMsg = "cancelled by user"
	job.FinishedAt = time.Now().UTC()
	if err = p.jobs.Update(ctx, job); err != nil {
		return err
	}
	if v, verr := p.variants.Get(ctx, job.VariantID); verr == nil && v != nil {
		v.RenderState = RenderFailed
		v.RenderError = "cancelled by user"
		return p.variants.Update(ctx, v)
	}
	return nil
}

// ListJobs 全局队列视图（service 分页透传）。
func (p *RenderPlanner) ListJobs(ctx context.Context, q JobQuery) ([]*RenderJob, int, error) {
	return p.jobs.List(ctx, q)
}

// GetVariant 按 ID 取变体（service 组装 job 展示字段 aspect/content_version/set_id）。
func (p *RenderPlanner) GetVariant(ctx context.Context, id string) (*Variant, error) {
	return p.variants.Get(ctx, id)
}

// SetProgressView 簇级渲染进度聚合（渲染队列 / 详情页 ETA）。
type SetProgressView struct {
	SetID     string
	Total     int32
	Queued    int32
	Running   int32
	Done      int32
	Failed    int32
	Cancelled int32
	Overall   float64
	// EtaSec 无完成样本时为 -1（不编数，前端显示「估算中」）。
	EtaSec int
	Jobs   []*RenderJob
}

// SetProgress 聚合簇内全部 job。超时取消的 job 以 error_msg=cancelled by user 区分计数
// （状态机无独立 cancelled 态，文档约定取消即 failed+标记）。
func (p *RenderPlanner) SetProgress(ctx context.Context, setID string) (*SetProgressView, error) {
	jobs, err := p.jobs.ListBySet(ctx, setID)
	if err != nil {
		return nil, err
	}
	out := &SetProgressView{SetID: setID, Jobs: jobs}
	for _, j := range jobs {
		out.Total++
		switch j.State {
		case RenderPending:
			out.Queued++
		case RenderRunning:
			out.Running++
		case RenderDone:
			out.Done++
		case RenderFailed:
			if j.ErrorMsg == "cancelled by user" {
				out.Cancelled++
			} else {
				out.Failed++
			}
		}
	}
	out.Overall, out.EtaSec = EstimateETASec(jobs, p.cfg.RenderConcurrency)
	return out, nil
}

// EstimateETASec 估算剩余秒数：平均单变体时长 × 未完成数 / 并发数。
// 进度不可算时返回 -1（前端展示「估算中」），不编数。avgSec 由 cluster 已完成 job 统计。
func EstimateETASec(jobs []*RenderJob, concurrency int) (overall float64, eta int) {
	if len(jobs) == 0 || concurrency <= 0 {
		return 0, -1
	}
	var done float64
	var finishedDur float64
	var finished int
	for _, j := range jobs {
		done += clamp01(j.Progress)
		if j.State == RenderDone {
			done = done - j.Progress + 1 // 完成的按 1.0 计
			if !j.StartedAt.IsZero() && !j.FinishedAt.IsZero() {
				finishedDur += j.FinishedAt.Sub(j.StartedAt).Seconds()
				finished++
			}
		}
	}
	overall = done / float64(len(jobs))
	remaining := len(jobs) - int(done) // 近似
	if finished == 0 {
		return overall, -1 // 无完成样本，ETA 不可估——诚实返回 -1
	}
	avg := finishedDur / float64(finished)
	eta = int(avg * float64(remaining) / float64(concurrency))
	return overall, eta
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
