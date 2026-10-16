package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/emptypb"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

// TaskService 任务服务层。
// 持有 *biz.TaskUsecase（不直接持有 data repo 或 asynq server），负责 HTTP 入口鉴权、
// operator 注入、DTO 组装；CRUD 与调度编排全部下沉到 Usecase。
// 修复 B5/S2/S5：删除对 *data.TaskRepo、*data.UserRepo、*asynqServer.Server 的直接依赖与
// AsyncBackup 等死代码，跨层访问统一经由 biz。
type TaskService struct {
	adminV1.TaskServiceHTTPServer

	log *log.Helper

	uc *biz.TaskUsecase
}

func NewTaskService(logger log.Logger, uc *biz.TaskUsecase) *TaskService {
	l := log.NewHelper(log.With(logger, "module", "task/service/admin-service"))
	return &TaskService{
		log: l,
		uc:  uc,
	}
}

func (s *TaskService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListTaskResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *TaskService) Get(ctx context.Context, req *adminV1.GetTaskRequest) (*adminV1.Task, error) {
	return s.uc.Get(ctx, req.GetId())
}

func (s *TaskService) Create(ctx context.Context, req *adminV1.CreateTaskRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息并注入审计字段（HTTP 入口鉴权延伸，业务规则在 Usecase）
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *TaskService) Update(ctx context.Context, req *adminV1.UpdateTaskRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息并注入审计字段
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return &emptypb.Empty{}, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *TaskService) Delete(ctx context.Context, req *adminV1.DeleteTaskRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// ControlTask 控制单个调度任务（启动/停止/重启）
func (s *TaskService) ControlTask(ctx context.Context, req *adminV1.ControlTaskRequest) (*emptypb.Empty, error) {
	if err := s.uc.ControlTask(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// StopAllTask 停止所有调度任务
func (s *TaskService) StopAllTask(context.Context, *emptypb.Empty) (*emptypb.Empty, error) {
	s.uc.StopAllTask()
	return &emptypb.Empty{}, nil
}

// RestartAllTask 重启所有调度任务
func (s *TaskService) RestartAllTask(ctx context.Context, _ *emptypb.Empty) (*adminV1.RestartAllTaskResponse, error) {
	count, err := s.uc.RestartAllTask(ctx)
	return &adminV1.RestartAllTaskResponse{Count: count}, err
}

// ListTaskHandlers 查询已注册任务处理器清单（前端 typeName 下拉数据源）
func (s *TaskService) ListTaskHandlers(ctx context.Context, _ *emptypb.Empty) (*adminV1.ListTaskHandlersResponse, error) {
	return s.uc.ListTaskHandlers(ctx)
}

// ListTaskExecutions 查询任务最近执行记录（最近 50 条，按执行时间倒序）
func (s *TaskService) ListTaskExecutions(ctx context.Context, req *adminV1.GetTaskRequest) (*adminV1.ListTaskExecutionsResponse, error) {
	return s.uc.ListTaskExecutions(ctx, req.GetId())
}

// RunTask 立即执行一次任务（一次性入队，不影响既有周期调度；禁用状态也可手动触发）
func (s *TaskService) RunTask(ctx context.Context, req *adminV1.RunTaskRequest) (*emptypb.Empty, error) {
	if err := s.uc.RunNow(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
