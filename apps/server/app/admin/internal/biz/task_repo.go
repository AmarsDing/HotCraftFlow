package biz

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/hibiken/asynq"
	"github.com/robfig/cron/v3"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"github.com/tx7do/kratos-transport/broker"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	adminV1 "framework/api/gen/go/admin/v1"
)

// TaskTypeNameUserTokenCleanup 内置任务：每日清理过期用户令牌（处理器在 server 层注册）。
const TaskTypeNameUserTokenCleanup = "user_token_cleanup"

// TaskTypeNameScheduleAutoGenerate 内置任务：每日滚动补足未来 N 天排班空缺（处理器在 server 层注册）。
const TaskTypeNameScheduleAutoGenerate = "schedule_auto_generate"

// IC 任务类型名（处理器在 worker/ic 调度器注册；全局任务的周期登记由调度器自管，
// 不走任务中心 seed，避免双通道重复调度。
const TaskTypeNameIcCollect = "ic.collect"
const TaskTypeNameIcRepublish = "ic.republish"
const TaskTypeNameIcClusterIdleClose = "ic.cluster_idle_close"
const TaskTypeNameIcRetentionSweep = "ic.retention_sweep"

// validTaskTypeNames 已注册处理器的任务类型名白名单。
// 仅白名单内的 typeName 允许创建/启用：asynq 按 typeName 路由到处理器，
// 未注册的 typeName 入队后只会反复重试直至归档（永远无法执行）。
var validTaskTypeNames = map[string]struct{}{
	TaskTypeNameUserTokenCleanup:     {},
	TaskTypeNameScheduleAutoGenerate: {},

	TaskTypeNameIcCollect:          {},
	TaskTypeNameIcRepublish:        {},
	TaskTypeNameIcClusterIdleClose: {},
	TaskTypeNameIcRetentionSweep:   {},
}

// IsValidTaskTypeName 校验任务类型名是否已注册处理器。
func IsValidTaskTypeName(typeName string) bool {
	_, ok := validTaskTypeNames[typeName]
	return ok
}

// taskHandlerMeta 已注册处理器的元信息（ListTaskHandlers 返回，供前端下拉与载荷引导展示）。
// 与白名单同文件维护，新增处理器时必须同步补充元信息。
type taskHandlerMeta struct {
	// Description 处理器用途说明（选中后展示）
	Description string
	// PayloadDescription 载荷参数填写规则说明（无参数时说明"无需参数"）
	PayloadDescription string
	// PayloadExample 载荷示例（JSON，前端"填入示例"一键回填；无参数为 {}）
	PayloadExample string
}

var taskHandlerMetas = map[string]taskHandlerMeta{
	TaskTypeNameUserTokenCleanup: {
		Description:        "每日清理过期超过 7 天的用户令牌（access/refresh/mfa），防止 user_tokens 表无限增长",
		PayloadDescription: "无需参数，留空即可",
		PayloadExample:     "{}",
	},
	TaskTypeNameScheduleAutoGenerate: {
		Description: "每日滚动检查全部启用中的排班规则，为明天起未来 N 天（days，默认 14）缺失的排班按 skip_published 模式自动补足；已发布/人工调整过的排班保持不变",
		PayloadDescription: `JSON 对象，全部字段可选：
days - 滚动窗口天数（默认 14，1-365）；
mode - 生效模式（默认 skip_published，可选 draft_only/regenerate_untouched/regenerate_all）；
includeHoliday - 是否包含节假日班次（默认 true）`,
		PayloadExample: `{"days":14,"mode":"skip_published","includeHoliday":true}`,
	},

	// ---------- ① IC 情报收集 ----------
	TaskTypeNameIcCollect: {
		Description:        "触发一路信源的一轮采集（周期任务由 IC 调度器按信源 interval_minutes 自动维护，此处可手动重跑）",
		PayloadDescription: "JSON 对象：source_id - 信源 ID（必填）",
		PayloadExample:     `{"source_id":"src_01J..."}`,
	},
	TaskTypeNameIcRepublish: {
		Description:        "重发 publish_state=pending 的情报（B-25 投递失败兜底；周期 30s，由 IC 调度器自动登记）",
		PayloadDescription: "无需参数，留空即可",
		PayloadExample:     "{}",
	},
	TaskTypeNameIcClusterIdleClose: {
		Description:        "关闭 last_seen_at 超过阈值（默认 48h，S-2）的 open 簇；闭簇不再吸纳新成员",
		PayloadDescription: "无需参数，留空即可",
		PayloadExample:     "{}",
	},
	TaskTypeNameIcRetentionSweep: {
		Description:        "保留期清扫：evidence（默认 90d）先删文件再删行，intel（默认 365d）删行；删除打审计日志",
		PayloadDescription: "无需参数，留空即可",
		PayloadExample:     "{}",
	},
}

// builtinSeedTasks 内置任务种子：EnsureSeedTasks 启动期幂等播种；
// 同时也是 builtin 判定与删除保护的唯一口径（新增内置任务必须在此登记）。
var builtinSeedTasks = []*adminV1.Task{
	{
		Type:        adminV1.TaskType_PERIODIC.Enum(),
		TypeName:    trans.Ptr(TaskTypeNameUserTokenCleanup),
		Name:        trans.Ptr("每日清理过期令牌"),
		TaskPayload: trans.Ptr("{}"),
		CronSpec:    trans.Ptr("30 3 * * *"), // 每日 03:30，避开 02:00 license / 03:00 排班检测
		Enable:      trans.Ptr(true),
		Remark:      trans.Ptr("内置任务：每日清理过期超过 7 天的用户令牌（access/refresh/mfa），防止 user_tokens 表无限增长"),
	},
	{
		Type:        adminV1.TaskType_PERIODIC.Enum(),
		TypeName:    trans.Ptr(TaskTypeNameScheduleAutoGenerate),
		Name:        trans.Ptr("排班滚动自动生成"),
		TaskPayload: trans.Ptr(`{"days":14,"mode":"skip_published","includeHoliday":true}`),
		CronSpec:    trans.Ptr("0 4 * * *"), // 每日 04:00，避开 02:00 license / 03:30 令牌清理
		Enable:      trans.Ptr(true),
		Remark:      trans.Ptr("内置任务：遍历启用中的排班规则，按 skip_published 模式补足明天起未来 14 天的排班空缺"),
	},
}

// IsBuiltinTask 判定 typeName 是否内置任务（内置任务禁止删除，cron/启用态允许调整）。
func IsBuiltinTask(typeName string) bool {
	for _, s := range builtinSeedTasks {
		if s.GetTypeName() == typeName {
			return true
		}
	}
	return false
}

// TaskRepo 是任务仓储端口，由 data 层未导出的 taskRepo 实现。
// Create/Update 返回 *adminV1.Task，供 Usecase 调度编排使用。
type TaskRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListTaskResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, id uint32) (*adminV1.Task, error)
	GetByTypeName(ctx context.Context, typeName string) (*adminV1.Task, error)
	Create(ctx context.Context, req *adminV1.CreateTaskRequest) (*adminV1.Task, error)
	Update(ctx context.Context, req *adminV1.UpdateTaskRequest) (*adminV1.Task, error)
	Delete(ctx context.Context, req *adminV1.DeleteTaskRequest) error
}

// TaskExecutionRepo 是任务执行记录仓储端口，由 data 层实现。
// 记录为追加写：处理器每次执行落一条，任务删除后记录保留（type_name 冗余可辨识）。
type TaskExecutionRepo interface {
	Create(ctx context.Context, e *adminV1.TaskExecution) error
	// ListByTask 按任务查询最近执行记录（按 ID 倒序，limit 上限）。
	ListByTask(ctx context.Context, taskID uint32, limit int) ([]*adminV1.TaskExecution, error)
	// LatestByTask 查询任务最近一次执行记录；无记录返回 (nil, nil)。
	LatestByTask(ctx context.Context, taskID uint32) (*adminV1.TaskExecution, error)
}

// TaskScheduler 是任务调度端口，由 server 层 *asynqServer.Server 隐式实现（鸭子类型）。
// 当 asynq 未配置时，server 层 NewTaskScheduler 返回 noopTaskScheduler（方法均为 no-op），
// 保证 CRUD 不受影响。方法签名与 *asynqServer.Server 对齐。
type TaskScheduler interface {
	NewPeriodicTask(cronSpec string, taskID string, typeName string, payload broker.Any, opts ...asynq.Option) (string, error)
	NewTask(typeName string, payload broker.Any, opts ...asynq.Option) error
	NewWaitResultTask(typeName string, payload broker.Any, opts ...asynq.Option) error
	// RemovePeriodicTask 按 taskID 移除周期任务，taskID 必须与 NewPeriodicTask 注册时一致。
	RemovePeriodicTask(taskID string) error
	RemoveAllPeriodicTask()
}

// TaskUsecase 任务业务用例。
// 承载 CRUD 编排 + 调度逻辑（startTask/stopTask/StartAllTasks 等）。
// service 层负责 HTTP 入口鉴权与 operator 注入，调度业务规则下沉到此。
type TaskUsecase struct {
	repo      TaskRepo
	execRepo  TaskExecutionRepo
	scheduler TaskScheduler
	log       *log.Helper
}

// NewTaskUsecase 创建任务 Usecase。
// repo/execRepo/scheduler 由 wire 注入：data.NewTaskRepo/NewTaskExecutionRepo 返回 biz 端口，
// server.NewTaskScheduler 返回 biz.TaskScheduler（asynq 未配置时为 noop 实现）。
func NewTaskUsecase(repo TaskRepo, execRepo TaskExecutionRepo, scheduler TaskScheduler, logger log.Logger) *TaskUsecase {
	return &TaskUsecase{
		repo:      repo,
		execRepo:  execRepo,
		scheduler: scheduler,
		log:       log.NewHelper(log.With(logger, "module", "task/usecase/admin-service")),
	}
}

// List 分页查询任务列表；逐项填充 builtin 标记与最近一次执行记录（均为计算字段，不落库）。
// 执行记录填充失败不阻断列表：降级为无 lastExecution 返回。
func (uc *TaskUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListTaskResponse, error) {
	resp, err := uc.repo.List(ctx, req)
	if err != nil {
		return nil, err
	}
	for _, t := range resp.GetItems() {
		t.Builtin = trans.Ptr(IsBuiltinTask(t.GetTypeName()))
		if uc.execRepo == nil {
			continue
		}
		last, err := uc.execRepo.LatestByTask(ctx, t.GetId())
		if err != nil {
			uc.log.Warnf("[%s] 查询最近执行记录失败: %s", t.GetTypeName(), err.Error())
			continue
		}
		t.LastExecution = last
	}
	return resp, nil
}

// ListTaskExecutions 查询任务最近执行记录（上限 50 条，按执行时间倒序）。
func (uc *TaskUsecase) ListTaskExecutions(ctx context.Context, taskID uint32) (*adminV1.ListTaskExecutionsResponse, error) {
	if uc.execRepo == nil {
		return &adminV1.ListTaskExecutionsResponse{}, nil
	}
	items, err := uc.execRepo.ListByTask(ctx, taskID, 50)
	if err != nil {
		return nil, err
	}
	return &adminV1.ListTaskExecutionsResponse{Items: items, Total: uint32(len(items))}, nil
}

// RecordExecution 记录一次任务执行（由 server 层处理器包装器调用）。
// typeName 为路由键；taskID 允许为 0（任务已删除时仅留 typeName 可辨识）。
// 记录失败仅记日志，不影响任务本身的执行结果。
func (uc *TaskUsecase) RecordExecution(ctx context.Context, typeName string, status adminV1.TaskExecutionStatus, execErr error, startedAt, finishedAt time.Time) {
	if uc.execRepo == nil {
		return
	}
	e := &adminV1.TaskExecution{
		TypeName:   trans.Ptr(typeName),
		Status:     status.Enum(),
		StartedAt:  timestamppb.New(startedAt),
		FinishedAt: timestamppb.New(finishedAt),
		DurationMs: trans.Ptr(uint64(finishedAt.Sub(startedAt).Milliseconds())),
	}
	if execErr != nil {
		e.Error = trans.Ptr(execErr.Error())
	}
	// 关联任务 ID：任务可能已被删除，查不到仅留 typeName
	if t, err := uc.repo.GetByTypeName(ctx, typeName); err == nil && t != nil {
		e.TaskId = trans.Ptr(t.GetId())
	}
	if err := uc.execRepo.Create(ctx, e); err != nil {
		uc.log.Warnf("[%s] 执行记录落库失败: %s", typeName, err.Error())
	}
}

// RunNow 立即执行一次任务：一次性入队 admin 队列，不影响既有周期调度。
// 手动触发不校验 enable（禁用态补跑是合法运维场景）。
func (uc *TaskUsecase) RunNow(ctx context.Context, id uint32) error {
	t, err := uc.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if !IsValidTaskTypeName(t.GetTypeName()) {
		return adminV1.ErrorBadRequest("任务类型名 %q 未注册处理器，无法执行", t.GetTypeName())
	}
	_, payload := uc.convertTaskOption(t)
	// 队列隔离由 server 层 queueTaskScheduler 统一注入，此处无需重复指定
	if err = uc.scheduler.NewTask(t.GetTypeName(), payload); err != nil {
		uc.log.Errorf("[%s] 手动触发入队失败: %s", t.GetTypeName(), err.Error())
		return adminV1.ErrorInternalServerError("任务入队失败: %s", err.Error())
	}
	uc.log.Infof("[%s] 手动触发已入队（taskID %d）", t.GetTypeName(), id)
	return nil
}

// ListTaskHandlers 返回已注册处理器清单（前端 typeName 下拉数据源，与创建/调度校验白名单同源）。
func (uc *TaskUsecase) ListTaskHandlers(_ context.Context) (*adminV1.ListTaskHandlersResponse, error) {
	names := make([]string, 0, len(validTaskTypeNames))
	for name := range validTaskTypeNames {
		names = append(names, name)
	}
	sort.Strings(names)
	items := make([]*adminV1.TaskHandlerInfo, 0, len(names))
	for _, name := range names {
		meta := taskHandlerMetas[name]
		items = append(items, &adminV1.TaskHandlerInfo{
			TypeName:           trans.Ptr(name),
			Description:        trans.Ptr(meta.Description),
			PayloadDescription: trans.Ptr(meta.PayloadDescription),
			PayloadExample:     trans.Ptr(meta.PayloadExample),
		})
	}
	return &adminV1.ListTaskHandlersResponse{Items: items}, nil
}

// Get 查询单个任务
func (uc *TaskUsecase) Get(ctx context.Context, id uint32) (*adminV1.Task, error) {
	return uc.repo.Get(ctx, id)
}

// GetByTypeName 按任务类型名查询
func (uc *TaskUsecase) GetByTypeName(ctx context.Context, typeName string) (*adminV1.Task, error) {
	return uc.repo.GetByTypeName(ctx, typeName)
}

// Create 创建任务；启用状态下注册调度。
// 管理接口仅允许创建周期任务：一次性任务（DELAY/WAIT_RESULT）是程序侧语义，
// 由代码直接 NewTask 入队（创建即执行、入队后不可撤销），UI 创建只会留下死记录。
func (uc *TaskUsecase) Create(ctx context.Context, req *adminV1.CreateTaskRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if req.Data.GetType() != adminV1.TaskType_PERIODIC {
		return adminV1.ErrorBadRequest("仅支持创建周期任务（PERIODIC）；一次性任务由程序侧发起，如需立即执行请使用 RunTask")
	}
	if err := uc.validateTask(req.Data); err != nil {
		return err
	}
	if err := uc.checkTypeNameUnique(ctx, req.Data.GetTypeName(), 0); err != nil {
		return err
	}
	t, err := uc.repo.Create(ctx, req)
	if err != nil {
		return err
	}
	if t.GetEnable() {
		if err = uc.startTask(t); err != nil {
			uc.log.Errorf("[%s] 创建后启动任务失败: %s", t.GetTypeName(), err.Error())
			return adminV1.ErrorInternalServerError("任务已保存，但注册调度失败: %s", err.Error())
		}
	}
	return nil
}

// Update 更新任务；根据 enable 状态分支处理调度（修复 S3：禁用时停止旧任务）。
// 调度操作失败如实返回错误（DB 已保存期望态，进程重启后 StartEnabledTasks 会按期望态恢复）。
func (uc *TaskUsecase) Update(ctx context.Context, req *adminV1.UpdateTaskRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	// 与 data 层 id 回退规则保持一致：优先顶层 id（URL 路径），回退 data.id
	id := req.GetId()
	if id == 0 {
		id = req.Data.GetId()
	}
	// 兼容 updateMask 部分更新：未提供的字段取库内现值，校验合并后的最终态。
	// allowMissing 且记录不存在时按新建处理，直接校验请求体。
	toValidate := req.Data
	existing, err := uc.repo.Get(ctx, id)
	switch {
	case err == nil:
		toValidate = mergeTaskForValidate(req.Data, existing)
	case req.GetAllowMissing() && adminV1.IsNotFound(err):
		// upsert 新建分支：按请求体校验
	default:
		return err
	}
	if err := uc.validateTask(toValidate); err != nil {
		return err
	}
	if err := uc.checkTypeNameUnique(ctx, toValidate.GetTypeName(), id); err != nil {
		return err
	}
	t, err := uc.repo.Update(ctx, req)
	if err != nil {
		return err
	}
	if t.GetEnable() {
		// 先停旧任务再启新任务，避免重复注册
		_ = uc.stopTask(t)
		if err = uc.startTask(t); err != nil {
			uc.log.Errorf("[%s] 更新后启动任务失败: %s", t.GetTypeName(), err.Error())
			return adminV1.ErrorInternalServerError("任务已保存，但注册调度失败: %s", err.Error())
		}
	} else {
		// enable 由 true 改为 false，停止已运行任务
		if err = uc.stopTask(t); err != nil {
			uc.log.Errorf("[%s] 停止任务失败: %s", t.GetTypeName(), err.Error())
			return adminV1.ErrorInternalServerError("任务已保存，但停止调度失败: %s", err.Error())
		}
	}
	return nil
}

// Delete 删除任务；先停调度再删记录。内置任务禁止删除（防误删导致关键维护任务静默停摆）。
func (uc *TaskUsecase) Delete(ctx context.Context, req *adminV1.DeleteTaskRequest) error {
	t, err := uc.repo.Get(ctx, req.GetId())
	if err != nil {
		uc.log.Errorf("删除前查询任务失败: %s", err.Error())
	}
	if t != nil && IsBuiltinTask(t.GetTypeName()) {
		return adminV1.ErrorBadRequest("内置任务 %q 禁止删除；如需暂停请禁用（enable=false）", t.GetTypeName())
	}
	if err = uc.repo.Delete(ctx, req); err != nil {
		return err
	}
	if t != nil {
		_ = uc.stopTask(t)
	}
	return nil
}

// ControlTask 控制单个任务（启动/停止/重启）
func (uc *TaskUsecase) ControlTask(ctx context.Context, req *adminV1.ControlTaskRequest) error {
	t, err := uc.repo.GetByTypeName(ctx, req.GetTypeName())
	if err != nil {
		return err
	}
	switch req.GetControlType() {
	case adminV1.TaskControlType_ControlType_Restart:
		if err = uc.stopTask(t); err != nil {
			return err
		}
		return uc.startTask(t)
	case adminV1.TaskControlType_ControlType_Stop:
		return uc.stopTask(t)
	case adminV1.TaskControlType_ControlType_Start:
		return uc.startTask(t)
	}
	return nil
}

// StopAllTask 停止所有调度任务
func (uc *TaskUsecase) StopAllTask() {
	uc.scheduler.RemoveAllPeriodicTask()
	uc.log.Info("已清除所有定时任务")
}

// RestartAllTask 重启所有调度任务，返回成功启动的数量
func (uc *TaskUsecase) RestartAllTask(ctx context.Context) (int32, error) {
	uc.StopAllTask()
	return uc.startAllTask(ctx)
}

// startAllTask 启动所有已启用任务
func (uc *TaskUsecase) startAllTask(ctx context.Context) (int32, error) {
	resp, err := uc.repo.List(ctx, &pagination.PagingRequest{
		NoPaging: trans.Ptr(true),
		Query:    trans.Ptr(""),
	})
	if err != nil {
		return 0, err
	}
	uc.log.Infof("开始启动定时任务，总计[%d]个", resp.GetTotal())
	var count int32
	for _, t := range resp.GetItems() {
		if uc.startTask(t) == nil {
			count++
		}
	}
	uc.log.Infof("总共成功启动定时任务[%d]个", count)
	return count, nil
}

// StartEnabledTasks 启动所有已启用任务（服务启动时调用一次，用于重启后恢复调度）。
// 与 RestartAllTask 的差异：不先清空调度器——进程刚启动时调度器本就为空。
func (uc *TaskUsecase) StartEnabledTasks(ctx context.Context) (int32, error) {
	return uc.startAllTask(ctx)
}

// EnsureSeedTasks 启动期播种内置任务（幂等）：仅落库不注册调度，
// 调度统一由随后的 StartEnabledTasks 完成，避免重复注册。
func (uc *TaskUsecase) EnsureSeedTasks(ctx context.Context) error {
	for _, s := range builtinSeedTasks {
		existing, err := uc.repo.GetByTypeName(ctx, s.GetTypeName())
		if err == nil {
			// 已存在：cron/启用态以用户调整为准不覆盖；
			// 仅 name 为空时回填显示名（name 为新增字段，无历史用户值，回填不冲突）
			if existing.GetName() == "" && s.GetName() != "" {
				if _, uerr := uc.repo.Update(ctx, &adminV1.UpdateTaskRequest{
					Data:       &adminV1.Task{Id: existing.Id, Name: s.Name},
					UpdateMask: &fieldmaskpb.FieldMask{Paths: []string{"name"}},
				}); uerr != nil {
					uc.log.Warnf("回填内置任务显示名 [%s] 失败: %s", s.GetTypeName(), uerr.Error())
				}
			}
			continue
		}
		if !adminV1.IsNotFound(err) {
			return err
		}
		if _, err = uc.repo.Create(ctx, &adminV1.CreateTaskRequest{Data: s}); err != nil {
			return err
		}
		uc.log.Infof("seeded builtin task [%s]", s.GetTypeName())
	}
	return nil
}

// validateTask 校验任务最终态：
//   - typeName 必填且在已注册处理器白名单内（未注册的 typeName 入队后永远不会被执行）；
//   - PERIODIC 任务必须提供合法的五段式 cron 表达式（与 asynq Scheduler 的 robfig/cron v3 解析器一致）。
func (uc *TaskUsecase) validateTask(t *adminV1.Task) error {
	typeName := t.GetTypeName()
	if typeName == "" {
		return adminV1.ErrorBadRequest("任务类型名 typeName 不能为空")
	}
	if !IsValidTaskTypeName(typeName) {
		return adminV1.ErrorBadRequest("任务类型名 %q 未注册处理器，无法调度执行", typeName)
	}
	if t.GetType() == adminV1.TaskType_PERIODIC {
		spec := t.GetCronSpec()
		if spec == "" {
			return adminV1.ErrorBadRequest("周期任务必须提供 cron 表达式 cronSpec")
		}
		if _, err := cron.ParseStandard(spec); err != nil {
			return adminV1.ErrorBadRequest("cron 表达式 %q 不合法: %s", spec, err.Error())
		}
	}
	return nil
}

// checkTypeNameUnique 校验 typeName 唯一（excludeID 为更新时排除自身）。
// 唯一性约束源于 asynq 传输层以 typeName 为 key 跟踪周期任务 entry，
// 同 typeName 多条任务会互相覆盖 entry 导致无法停止。
func (uc *TaskUsecase) checkTypeNameUnique(ctx context.Context, typeName string, excludeID uint32) error {
	existing, err := uc.repo.GetByTypeName(ctx, typeName)
	if err != nil {
		if adminV1.IsNotFound(err) {
			return nil
		}
		return err
	}
	if existing.GetId() != excludeID {
		return adminV1.ErrorConflict("任务类型名 %q 已存在（任务ID %d），同一类型名只允许一条任务", typeName, existing.GetId())
	}
	return nil
}

// mergeTaskForValidate 合并更新请求与库内现值，得到更新后的最终态用于校验。
// 仅拷贝校验相关字段；请求中未提供（nil）的字段取现值。
func mergeTaskForValidate(in, base *adminV1.Task) *adminV1.Task {
	out := &adminV1.Task{
		Type:     in.Type,
		TypeName: in.TypeName,
		CronSpec: in.CronSpec,
		Enable:   in.Enable,
	}
	if out.Type == nil {
		out.Type = base.Type
	}
	if out.TypeName == nil {
		out.TypeName = base.TypeName
	}
	if out.CronSpec == nil {
		out.CronSpec = base.CronSpec
	}
	if out.Enable == nil {
		out.Enable = base.Enable
	}
	return out
}

// startTask 启动单个任务。
// taskID 由 typeName:id 组成，作为 asynq.TaskID 去重选项传入；
// 注意 asynq 传输层周期任务 entry 以 typeName 为 key 跟踪（见 stopTask）。
// 一次性任务（DELAY/WAIT_RESULT）不响应启动动作：创建即入队执行的语义由程序侧 NewTask 承担，
// 在此入队会导致 Update/启用时重复执行；需要再次执行请走 RunNow。
func (uc *TaskUsecase) startTask(t *adminV1.Task) error {
	if t == nil {
		return errors.New("task is nil")
	}
	if !t.GetEnable() {
		return errors.New("task is not enable")
	}
	// 启动期恢复（startAllTask）会遍历库内全部任务：未注册处理器的 typeName
	// 即使注册成功，触发后也只会反复重试直至归档，直接拒绝并告警。
	if !IsValidTaskTypeName(t.GetTypeName()) {
		uc.log.Warnf("[%s] 任务类型名未注册处理器，跳过调度", t.GetTypeName())
		return errors.New("task typeName not registered")
	}
	if t.GetType() != adminV1.TaskType_PERIODIC {
		uc.log.Warnf("[%s] 一次性任务不响应启动动作（避免重复入队），如需执行请调用 RunTask", t.GetTypeName())
		return nil
	}
	opts, payload := uc.convertTaskOption(t)
	taskID := buildTaskID(t)
	if _, err := uc.scheduler.NewPeriodicTask(t.GetCronSpec(), taskID, t.GetTypeName(), payload, opts...); err != nil {
		uc.log.Errorf("[%s] 创建定时任务失败: %s", t.GetTypeName(), err.Error())
		return err
	}
	return nil
}

// stopTask 停止单个任务。
// PERIODIC 任务按 typeName 从调度器移除：asynq 传输层注册 entry 时以 typeName 为 key
// （NewPeriodicTask 内 addPeriodicTaskEntryID(typeName, entryID)），
// 因此此处必须传 typeName 而非 taskID（typeName:id），否则永远查不到 entry 停止失败。
// DELAY/WAIT_RESULT 为一次性任务，入队后无法撤销，静默返回（修复 S4：明确语义而非空 case）。
func (uc *TaskUsecase) stopTask(t *adminV1.Task) error {
	if t == nil {
		return errors.New("task is nil")
	}
	switch t.GetType() {
	case adminV1.TaskType_PERIODIC:
		return uc.scheduler.RemovePeriodicTask(t.GetTypeName())
	case adminV1.TaskType_DELAY, adminV1.TaskType_WAIT_RESULT:
		// 一次性任务入队后由 asynq 执行完毕自动结束，无需也无法取消
		return nil
	}
	return nil
}

// buildTaskID 构造周期任务的唯一标识，供 NewPeriodicTask 注册与 RemovePeriodicTask 移除复用，
// 避免 start/stop 两侧 key 不一致导致任务无法移除。
func buildTaskID(t *adminV1.Task) string {
	return fmt.Sprintf("%s:%d", t.GetTypeName(), t.GetId())
}

// convertTaskOption 解析任务的 payload 与 asynq 选项
func (uc *TaskUsecase) convertTaskOption(t *adminV1.Task) (opts []asynq.Option, payload broker.Any) {
	if t == nil {
		return
	}
	if len(t.GetTaskPayload()) > 0 {
		_ = json.Unmarshal([]byte(t.GetTaskPayload()), &payload)
	}
	if t.TaskOptions != nil {
		if t.GetTaskOptions().GetRetryCount() > 0 {
			opts = append(opts, asynq.MaxRetry(int(t.GetTaskOptions().GetRetryCount())))
		}
		if t.GetTaskOptions().Timeout != nil {
			opts = append(opts, asynq.Timeout(t.GetTaskOptions().GetTimeout().AsDuration()))
		}
		if t.GetTaskOptions().Deadline != nil {
			opts = append(opts, asynq.Deadline(t.GetTaskOptions().GetDeadline().AsTime()))
		}
		if t.GetTaskOptions().ProcessIn != nil {
			opts = append(opts, asynq.ProcessIn(t.GetTaskOptions().GetProcessIn().AsDuration()))
		}
		if t.GetTaskOptions().ProcessAt != nil {
			opts = append(opts, asynq.ProcessAt(t.GetTaskOptions().GetProcessAt().AsTime()))
		}
	}
	return
}
