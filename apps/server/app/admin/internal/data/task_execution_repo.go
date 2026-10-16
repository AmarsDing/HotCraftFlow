package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/timestamppb"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/taskexecution"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 编译期接口检查：确保 taskExecutionRepo 完整实现 biz.TaskExecutionRepo 端口。
var _ biz.TaskExecutionRepo = (*taskExecutionRepo)(nil)

// taskExecutionRepo 未导出，实现 biz.TaskExecutionRepo 接口（端口-适配器模式）。
// 字段少且含 proto/ent 枚举互转，显式手写映射（不引入 copier mapper）。
type taskExecutionRepo struct {
	data *Data
	log  *log.Helper
}

// NewTaskExecutionRepo 返回 biz.TaskExecutionRepo 接口，wire 据返回类型自动绑定。
func NewTaskExecutionRepo(data *Data, logger log.Logger) biz.TaskExecutionRepo {
	return &taskExecutionRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "task-execution/repo/admin-service")),
	}
}

func (r *taskExecutionRepo) toDTO(entity *ent.TaskExecution) *adminV1.TaskExecution {
	if entity == nil {
		return nil
	}
	dto := &adminV1.TaskExecution{
		Id:        trans.Ptr(uint32(entity.ID)),
		TaskId:    entity.TaskID,
		TypeName:  entity.TypeName,
		Error:     entity.Error,
		DurationMs: entity.DurationMs,
	}
	if entity.Status != nil {
		if v, ok := adminV1.TaskExecutionStatus_value[string(*entity.Status)]; ok {
			dto.Status = adminV1.TaskExecutionStatus(v).Enum()
		}
	}
	if entity.StartedAt != nil {
		dto.StartedAt = timestamppb.New(*entity.StartedAt)
	}
	if entity.FinishedAt != nil {
		dto.FinishedAt = timestamppb.New(*entity.FinishedAt)
	}
	if entity.CreateTime != nil {
		dto.CreateTime = timestamppb.New(*entity.CreateTime)
	}
	return dto
}

func (r *taskExecutionRepo) Create(ctx context.Context, e *adminV1.TaskExecution) error {
	if e == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	builder := r.data.db.Client().TaskExecution.Create().
		SetNillableTaskID(e.TaskId).
		SetNillableTypeName(e.TypeName).
		SetNillableError(e.Error).
		SetNillableDurationMs(e.DurationMs).
		SetCreateTime(time.Now())

	// 仅落库 Success/Failed 两种终态；Unknown 不是合法 ent 枚举值，避免 save 报错
	switch e.GetStatus() {
	case adminV1.TaskExecutionStatus_ExecutionStatus_Success, adminV1.TaskExecutionStatus_ExecutionStatus_Failed:
		s := taskexecution.Status(e.GetStatus().String())
		builder.SetNillableStatus(&s)
	}
	if e.StartedAt != nil {
		t := e.GetStartedAt().AsTime()
		builder.SetNillableStartedAt(&t)
	}
	if e.FinishedAt != nil {
		t := e.GetFinishedAt().AsTime()
		builder.SetNillableFinishedAt(&t)
	}

	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf("insert task execution failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert task execution failed")
	}
	return nil
}

func (r *taskExecutionRepo) ListByTask(ctx context.Context, taskID uint32, limit int) ([]*adminV1.TaskExecution, error) {
	if limit <= 0 {
		limit = 50
	}
	entities, err := r.data.db.Client().TaskExecution.Query().
		Where(taskexecution.TaskIDEQ(taskID)).
		Order(ent.Desc(taskexecution.FieldID)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("query task executions failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query task executions failed")
	}
	items := make([]*adminV1.TaskExecution, 0, len(entities))
	for _, entity := range entities {
		items = append(items, r.toDTO(entity))
	}
	return items, nil
}

func (r *taskExecutionRepo) LatestByTask(ctx context.Context, taskID uint32) (*adminV1.TaskExecution, error) {
	entity, err := r.data.db.Client().TaskExecution.Query().
		Where(taskexecution.TaskIDEQ(taskID)).
		Order(ent.Desc(taskexecution.FieldID)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil // 无记录属正常态（任务尚未执行过）
		}
		r.log.Errorf("query latest task execution failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query latest task execution failed")
	}
	return r.toDTO(entity), nil
}
