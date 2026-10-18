// Package jobrun 任务执行器：lane 分槽的进程内队列 + 类型注册处理器 +
// 内存事件扇出（SSE/WS 的上游）。
//
// 权威状态在 jobstore（PG）；本包只提供执行语义：
//   - Submit：幂等入队（idempotency_key 命中直接返回旧 job）；
//   - lane 信号量：infer/render/io 并发槽由配置给出（infer=1 为 AI 串行纪律）；
//   - Cancel：ctx 取消（exectree 类 handler 随之杀树）+ 终态落库；
//   - Retry：failed/cancelled 以同 payload 重入队（attempt+1）；
//   - 启动恢复：孤儿 running 归因 failed（service_restart）+ queued 重投递；
//   - 进度事件节流：≥ProgressMinGap 间隔，首次与 1.0 必发。
//
// 事件镜像纪律：每次迁移 = 状态 UPDATE + 事件 INSERT 同事务（jobstore 保
// 证），内存扇出只作"刷新信号"，断线补齐走 SSE after_seq 回放。
package jobrun

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"framework/app/media/internal/jobstore"
)

// Handler 任务处理器：ctx 取消必须尽快返回；emit 由执行器实现
// （节流 + 落库 + 扇出）。返回的 payload 落 result_json。
type Handler func(ctx context.Context, job jobstore.Job, payload json.RawMessage, emit func(progress float64)) (json.RawMessage, error)

// Config 执行器配置（来自 media.yaml jobs 段）。
type Config struct {
	Lanes            map[string]int32 // lane → 并发槽数（空走默认）
	EventCapPerJob   int              // 每 job 事件上限（默认 500）
	ProgressMinGapMs int              // 进度事件最小间隔（默认 100ms）
}

// Op 内存扇出操作类型。
type Op string

const (
	OpEvent    Op = "job.event"    // 详情级实时推送（SSE 订阅；不进 WS 总线）
	OpCreated  Op = "job.created"  // 列表级失效信号（WS）
	OpUpdated  Op = "job.updated"  // 列表级失效信号（WS）
	OpFinished Op = "job.finished" // 列表级失效信号（WS）
)

// Notice 内存扇出消息（轻量引用；详情由消费侧回查 DB）。
type Notice struct {
	Op        Op
	JobID     string
	Lane      string
	State     jobstore.State
	Progress  float64
	EventKind jobstore.EventKind
	At        time.Time
}

// ErrNotCancellable 重复取消终态 job（幂等语义：调用方拿到当前 job 即可）。
var ErrNotCancellable = errors.New("job is already terminal")

// ErrUnknownJobType 提交的任务类型未注册 handler。
var ErrUnknownJobType = errors.New("unknown job type")

// RetryRejected 非终态任务不可重试。
var RetryRejected = errors.New("only failed/cancelled jobs can be retried")

// Executor 任务执行器。
type Executor struct {
	store    *jobstore.Store
	cfg      Config
	log      *slog.Logger
	workerID string

	laneCh   map[string]chan struct{} // lane 信号量
	handlers map[string]Handler

	mu      sync.Mutex
	running map[string]context.CancelFunc // jobID → cancel（运行中的任务）

	subsMu sync.RWMutex
	subs   map[string]map[uint64]chan Notice // jobID → SSE 实时段订阅者

	bus     chan Notice // WS 失效信号总线
	closing atomic.Bool
	wg      sync.WaitGroup
}

// New 构造执行器；workerID 用于运行归属与孤儿归因判别。
func New(store *jobstore.Store, cfg Config, workerID string, log *slog.Logger) *Executor {
	if cfg.ProgressMinGapMs <= 0 {
		cfg.ProgressMinGapMs = 100
	}
	if len(cfg.Lanes) == 0 {
		cfg.Lanes = map[string]int32{"infer": 1, "render": 2, "io": 4}
	}
	if workerID == "" {
		workerID = "media-1"
	}
	if log == nil {
		log = slog.Default()
	}
	e := &Executor{
		store:    store,
		cfg:      cfg,
		log:      log,
		workerID: workerID,
		laneCh:   make(map[string]chan struct{}, len(cfg.Lanes)),
		handlers: make(map[string]Handler),
		running:  make(map[string]context.CancelFunc),
		subs:     make(map[string]map[uint64]chan Notice),
		bus:      make(chan Notice, 256),
	}
	for lane, n := range cfg.Lanes {
		if n < 1 {
			n = 1
		}
		e.laneCh[lane] = make(chan struct{}, n)
	}
	return e
}

// RegisterHandler 注册任务类型处理器（启动期调用；空类型/重复为配置错误）。
func (e *Executor) RegisterHandler(jobType string, h Handler) {
	if jobType == "" || h == nil {
		panic("jobrun: invalid handler registration")
	}
	if _, dup := e.handlers[jobType]; dup {
		panic("jobrun: duplicate handler for type " + jobType)
	}
	e.handlers[jobType] = h
}

// Bus 返回 WS 失效信号通道（只读视图）。
func (e *Executor) Bus() <-chan Notice { return e.bus }

// Start 启动恢复纪律：孤儿 running → failed；遗留 queued 重投递。
func (e *Executor) Start(ctx context.Context) error {
	orphans, err := e.store.FailOrphaned(ctx, e.workerID)
	if err != nil {
		return fmt.Errorf("fail orphaned: %w", err)
	}
	if orphans > 0 {
		e.log.Warn("recovered orphaned running jobs", "count", orphans)
	}
	queued, err := e.store.RecoverQueued(ctx)
	if err != nil {
		return fmt.Errorf("recover queued: %w", err)
	}
	for _, j := range queued {
		e.dispatch(j.ID)
	}
	if len(queued) > 0 {
		e.log.Info("re-dispatched queued jobs", "count", len(queued))
	}
	return nil
}

// Stop 等待全部运行中任务结束；超时则强制取消（杀树经各 handler 的 ctx）。
func (e *Executor) Stop(ctx context.Context) error {
	e.closing.Store(true)
	done := make(chan struct{})
	go func() { e.wg.Wait(); close(done) }()
	select {
	case <-ctx.Done():
		e.mu.Lock()
		for _, cancel := range e.running {
			cancel()
		}
		e.mu.Unlock()
		return ctx.Err()
	case <-done:
		return nil
	}
}

// Submit 幂等提交；lane 未配置时按 io 槽兜底。
func (e *Executor) Submit(ctx context.Context, lane, jobType string, payload json.RawMessage, idemKey string) (jobstore.Job, bool, error) {
	if e.closing.Load() {
		return jobstore.Job{}, false, errors.New("executor is shutting down")
	}
	if _, ok := e.handlers[jobType]; !ok {
		return jobstore.Job{}, false, fmt.Errorf("%w: %s", ErrUnknownJobType, jobType)
	}
	if lane == "" {
		lane = "io"
	}
	j, created, err := e.store.Submit(ctx, jobstore.Job{
		Lane:           lane,
		Type:           jobType,
		PayloadJSON:    string(payload),
		IdempotencyKey: idemKey,
	}, jobstore.PayloadFor(map[string]any{"lane": lane, "type": jobType}))
	if err != nil {
		return jobstore.Job{}, false, err
	}
	if created {
		e.dispatch(j.ID)
		e.fanout(Notice{Op: OpCreated, JobID: j.ID, Lane: j.Lane, State: j.State, At: time.Now()})
	}
	return j, created, nil
}

// Get 读单 job。
func (e *Executor) Get(ctx context.Context, id string) (jobstore.Job, error) {
	return e.store.Get(ctx, id)
}

// List 分页列表（直通 jobstore）。
func (e *Executor) List(ctx context.Context, f jobstore.ListFilter) ([]jobstore.Job, uint32, error) {
	return e.store.List(ctx, f)
}

// Cancel 幂等取消：终态重复调用返回 ErrNotCancellable + 当前 job。
// queued → 直接落 cancelled；running → 取消 ctx，由 runOne 统一落终态。
func (e *Executor) Cancel(ctx context.Context, id string) (jobstore.Job, error) {
	j, err := e.store.Get(ctx, id)
	if err != nil {
		return j, err
	}
	switch {
	case j.State.Terminal():
		return j, ErrNotCancellable
	case j.State == jobstore.StateQueued:
		nj, ok, terr := e.store.Transition(ctx, id, jobstore.Patch{
			From:      []jobstore.State{jobstore.StateQueued},
			To:        jobstore.StateCancelled,
			ErrorCode: strPtr("MEDIA_JOB_CANCELLED"),
			ErrorMsg:  strPtr("cancelled while queued"),
		}, jobstore.EvCancelled, jobstore.PayloadFor(map[string]any{"phase": "queued"}))
		if terr != nil {
			return j, terr
		}
		if ok {
			e.publishTransition(nj, jobstore.EvCancelled)
			return nj, nil
		}
		// 并发下已被摘走 → 回落为读当前态。
		return e.store.Get(ctx, id)
	default: // running
		e.mu.Lock()
		cancel := e.running[j.ID]
		e.mu.Unlock()
		if cancel != nil {
			cancel()
			return e.store.Get(ctx, id)
		}
		// running 但句柄缺失（重启竞态）→ 兜底直接落 cancelled。
		nj, ok, terr := e.store.Transition(ctx, id, jobstore.Patch{
			From:      []jobstore.State{jobstore.StateRunning},
			To:        jobstore.StateCancelled,
			ErrorCode: strPtr("MEDIA_JOB_CANCELLED"),
			ErrorMsg:  strPtr("cancelled without live handle"),
		}, jobstore.EvCancelled, jobstore.PayloadFor(map[string]any{"phase": "orphan"}))
		if terr == nil && ok {
			e.publishTransition(nj, jobstore.EvCancelled)
		}
		return nj, terr
	}
}

// Retry 终态重试：failed/cancelled → attempt+1 重入队。
func (e *Executor) Retry(ctx context.Context, id string) (jobstore.Job, error) {
	nj, err := e.store.Requeue(ctx, id)
	if errors.Is(err, jobstore.ErrNotFound) {
		return nj, err
	}
	if errors.Is(err, jobstore.ErrNotTerminal) {
		j, gerr := e.store.Get(ctx, id)
		if gerr != nil {
			return nj, gerr
		}
		return j, RetryRejected
	}
	if err != nil {
		return nj, err
	}
	e.dispatch(nj.ID)
	e.fanout(Notice{Op: OpUpdated, JobID: nj.ID, Lane: nj.Lane, State: nj.State, At: time.Now()})
	return nj, nil
}

// dispatch 投入 lane 信号量队列（goroutine 化；满槽阻塞是刻意的背压）。
func (e *Executor) dispatch(id string) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		ctx := context.Background()
		j, err := e.store.Get(ctx, id)
		if err != nil || j.State != jobstore.StateQueued {
			return
		}
		sem, ok := e.laneCh[j.Lane]
		if !ok {
			sem = e.laneCh["io"] // 未配置 lane 的宽容兜底
		}
		sem <- struct{}{}
		defer func() { <-sem }()
		// 摘走前二次确认（Cancel 可能已先落 cancelled）。
		fresh, err := e.store.Get(ctx, id)
		if err != nil || fresh.State != jobstore.StateQueued {
			return
		}
		e.runOne(fresh)
	}()
}

// runOne 单个任务的完整生命周期。
func (e *Executor) runOne(j jobstore.Job) {
	bg := context.Background()

	// queued → running
	now := time.Now()
	j, ok, err := e.store.Transition(bg, j.ID, jobstore.Patch{
		From:      []jobstore.State{jobstore.StateQueued},
		To:        jobstore.StateRunning,
		WorkerID:  strPtr(e.workerID),
		StartedAt: &now,
	}, jobstore.EvRunning, jobstore.PayloadFor(map[string]any{"worker": e.workerID}))
	if err != nil || !ok {
		return
	}
	e.publishTransition(j, jobstore.EvRunning)

	ctx, cancel := context.WithCancel(bg)
	e.mu.Lock()
	e.running[j.ID] = cancel
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		delete(e.running, j.ID)
		e.mu.Unlock()
		cancel()
	}()

	h := e.handlers[j.Type]
	var (
		lastEmit time.Time
		lastProg = -1.0
		emitMu   sync.Mutex
	)
	emit := func(p float64) {
		emitMu.Lock()
		defer emitMu.Unlock()
		if p < 0 {
			p = 0
		}
		if p > 1 {
			p = 1
		}
		nowT := time.Now()
		// 节流：未满间隔且未达 1.0 略过（首帧必发）。
		if lastProg >= 0 && p < 1 && nowT.Sub(lastEmit) < time.Duration(e.cfg.ProgressMinGapMs)*time.Millisecond {
			return
		}
		if p == lastProg {
			return
		}
		lastProg = p
		lastEmit = nowT
		nj, okT, terr := e.store.Transition(bg, j.ID, jobstore.Patch{
			From:     []jobstore.State{jobstore.StateRunning},
			To:       jobstore.StateRunning,
			Progress: &p,
		}, jobstore.EvProgress, jobstore.PayloadFor(map[string]any{"progress": p}))
		if terr != nil || !okT {
			return // 任务已被取消等并发终局：进度写入竞争失败属正常
		}
		_ = e.store.TrimEvents(bg, j.ID)
		e.fanout(Notice{
			Op: OpEvent, JobID: nj.ID, Lane: nj.Lane, State: nj.State, Progress: p,
			EventKind: jobstore.EvProgress, At: nowT,
		})
	}

	result, runErr := h(ctx, j, json.RawMessage(j.PayloadJSON), emit)

	// 终局迁移。
	switch {
	case errors.Is(runErr, context.Canceled) || ctx.Err() == context.Canceled:
		nj, okT, _ := e.store.Transition(bg, j.ID, jobstore.Patch{
			From:      []jobstore.State{jobstore.StateRunning},
			To:        jobstore.StateCancelled,
			ErrorCode: strPtr("MEDIA_JOB_CANCELLED"),
			ErrorMsg:  strPtr("cancelled by user"),
		}, jobstore.EvCancelled, jobstore.PayloadFor(map[string]any{"progress": lastProg}))
		if okT {
			e.publishTransition(nj, jobstore.EvCancelled)
		}
	case runErr != nil:
		code, msg := classifyError(runErr)
		e.log.Warn("job failed", "job", j.ID, "type", j.Type, "code", code, "err", runErr)
		nj, okT, _ := e.store.Transition(bg, j.ID, jobstore.Patch{
			From:      []jobstore.State{jobstore.StateRunning},
			To:        jobstore.StateFailed,
			ErrorCode: strPtr(code),
			ErrorMsg:  strPtr(msg),
		}, jobstore.EvFailed, jobstore.PayloadFor(map[string]any{"code": code}))
		if okT {
			e.publishTransition(nj, jobstore.EvFailed)
		}
	default:
		res := string(result)
		nj, okT, _ := e.store.Transition(bg, j.ID, jobstore.Patch{
			From:       []jobstore.State{jobstore.StateRunning},
			To:         jobstore.StateDone,
			Progress:   floatPtr(1),
			ResultJSON: &res,
		}, jobstore.EvDone, jobstore.PayloadFor(map[string]any{"progress": 1.0}))
		if okT {
			e.publishTransition(nj, jobstore.EvDone)
		}
	}
	_ = e.store.TrimEvents(bg, j.ID)
}

// classifyError 错误 → (稳定公开 code, msg)（内部细节进日志，不外泄）。
func classifyError(err error) (string, string) {
	code := "MEDIA_JOB_FAILED"
	var ce interface{ ErrorCode() string }
	if errors.As(err, &ce) && ce.ErrorCode() != "" {
		code = ce.ErrorCode()
	}
	return code, err.Error()
}

// publishTransition 态迁统一扇出：job.event（SSE）+ job.updated/finished（WS）。
func (e *Executor) publishTransition(j jobstore.Job, kind jobstore.EventKind) {
	at := time.Now()
	e.fanout(Notice{
		Op: OpEvent, JobID: j.ID, Lane: j.Lane, State: j.State, Progress: j.Progress,
		EventKind: kind, At: at,
	})
	op := OpUpdated
	if j.State.Terminal() {
		op = OpFinished
	}
	e.fanout(Notice{Op: op, JobID: j.ID, Lane: j.Lane, State: j.State, Progress: j.Progress, At: at})
}

// fanout 内存分发：OpEvent 只进 job 订阅（SSE）；created/updated/finished
// 再进 WS 总线。慢消费者直接丢（SSE 有 after_seq 回放、WS 只作失效信号）。
func (e *Executor) fanout(n Notice) {
	e.subsMu.RLock()
	sinks := e.subs[n.JobID]
	wildcard := e.subs["*"]
	e.subsMu.RUnlock()
	for _, ch := range sinks {
		select {
		case ch <- n:
		default:
		}
	}
	if n.Op == OpEvent {
		for _, ch := range wildcard {
			select {
			case ch <- n:
			default:
			}
		}
		return // 详情级事件不进 WS
	}
	select {
	case e.bus <- n:
	default:
	}
}

// Subscribe 订阅某 job 的实时段事件（SSE 用）；返回 channel 与退订函数。
// 语义：只推送订阅之后的 Notice；历史缺口由调用方先查 EventsSince 补齐。
// 注意：channel 永不主动关闭（防止 fanout 并发送 panic）；
// SSE 消费端靠自身 ctx 退出，channel 随其回收。
func (e *Executor) Subscribe(jobID string) (<-chan Notice, func()) {
	ch := make(chan Notice, 64)
	key := uint64(time.Now().UnixNano())
	e.subsMu.Lock()
	if e.subs[jobID] == nil {
		e.subs[jobID] = make(map[uint64]chan Notice)
	}
	e.subs[jobID][key] = ch
	e.subsMu.Unlock()
	return ch, func() {
		e.subsMu.Lock()
		delete(e.subs[jobID], key)
		e.subsMu.Unlock()
	}
}

// EventsSince 断线回放（直通 jobstore）。
func (e *Executor) EventsSince(ctx context.Context, jobID string, afterSeq int64, limit int) ([]jobstore.Event, error) {
	return e.store.EventsSince(ctx, jobID, afterSeq, limit)
}

// QueuePosition 队列位次（直通 jobstore；非 queued 返回 0）。
func (e *Executor) QueuePosition(ctx context.Context, j jobstore.Job) int32 {
	return e.store.QueuePosition(ctx, j)
}

func strPtr(s string) *string     { return &s }
func floatPtr(f float64) *float64 { return &f }
