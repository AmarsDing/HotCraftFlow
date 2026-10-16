package biz

import (
	"context"
	"sync"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
	"framework/pkg/retention"
)

// 手动清理日志的保留天数合法区间（天）。
const (
	logCleanKeepDaysMin = 1
	logCleanKeepDaysMax = 3650
)

// AuditEntry 业务审计日志条目，供 biz/service 层写入操作日志时使用。
// 与 proto DTO 解耦，包含 proto 未暴露的 module/action/target/target_id 字段。
// 定义在 biz 层（端口出参），data 层以类型别名兼容旧引用。
type AuditEntry struct {
	Module    string // 操作模块（如 asset_management）
	Action    string // 操作类型（如 password_view）
	Target    string // 操作对象（如 device）
	TargetID  string // 操作对象 ID
	UserID    uint32 // 操作者用户 ID
	Username  string // 操作者用户名
	ClientIP  string // 客户端 IP
	UserAgent string // User-Agent
	Success   bool   // 是否成功
	Reason    string // 失败原因（成功时为空）
}

// AdminLoginLogUsecase 登录日志业务用例（薄透传）。
type AdminLoginLogUsecase struct {
	repo AdminLoginLogRepo
	log  *log.Helper
}

func NewAdminLoginLogUsecase(repo AdminLoginLogRepo, logger log.Logger) *AdminLoginLogUsecase {
	return &AdminLoginLogUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "admin-login-log/usecase/admin-service")),
	}
}

func (uc *AdminLoginLogUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminLoginLogResponse, error) {
	return uc.repo.List(ctx, req)
}

func (uc *AdminLoginLogUsecase) Get(ctx context.Context, req *adminV1.GetAdminLoginLogRequest) (*adminV1.AdminLoginLog, error) {
	return uc.repo.Get(ctx, req)
}

// Clean 手动清理登录日志：物理删除 create_time 早于 now-keepDays 的记录，返回删除行数。
func (uc *AdminLoginLogUsecase) Clean(ctx context.Context, req *adminV1.CleanAdminLoginLogRequest) (*adminV1.CleanAdminLoginLogResponse, error) {
	if req.GetKeepDays() < logCleanKeepDaysMin || req.GetKeepDays() > logCleanKeepDaysMax {
		return nil, adminV1.ErrorBadRequest("invalid parameter: keep_days must be in [%d, %d]", logCleanKeepDaysMin, logCleanKeepDaysMax)
	}
	deleted, err := uc.repo.DeleteOlderThan(ctx, retention.Cutoff(int(req.GetKeepDays())))
	if err != nil {
		return nil, err
	}
	uc.log.Infof("login logs cleaned manually: keepDays=%d deleted=%d", req.GetKeepDays(), deleted)
	return &adminV1.CleanAdminLoginLogResponse{Deleted: uint32(deleted)}, nil
}

// AdminOperationLogUsecase 操作日志业务用例。
// API 资源描述富化（api_description/api_module）与缓存逻辑自 service 层下移，行为不变。
type AdminOperationLogUsecase struct {
	operationLogRepo AdminOperationLogRepo
	apiResourceRepo  ApiResourceRepo

	apis     []*adminV1.ApiResource
	apiMutex sync.RWMutex

	log *log.Helper
}

func NewAdminOperationLogUsecase(
	operationLogRepo AdminOperationLogRepo,
	apiResourceRepo ApiResourceRepo,
	logger log.Logger,
) *AdminOperationLogUsecase {
	return &AdminOperationLogUsecase{
		operationLogRepo: operationLogRepo,
		apiResourceRepo:  apiResourceRepo,
		log:              log.NewHelper(log.With(logger, "module", "admin-operation-log/usecase/admin-service")),
	}
}

func (uc *AdminOperationLogUsecase) queryApiResources(ctx context.Context, path, method string) (*adminV1.ApiResource, error) {
	// 快路径：读锁检查缓存
	uc.apiMutex.RLock()
	apis := uc.apis
	uc.apiMutex.RUnlock()

	if len(apis) == 0 {
		// 慢路径：写锁加载，double-check 避免高并发重复查库
		uc.apiMutex.Lock()
		if len(uc.apis) == 0 {
			loaded, err := uc.apiResourceRepo.List(ctx, &pagination.PagingRequest{
				NoPaging: trans.Ptr(true),
			})
			if err != nil {
				uc.apiMutex.Unlock()
				return nil, err
			}
			uc.apis = loaded.Items
		}
		apis = uc.apis
		uc.apiMutex.Unlock()
	}

	if len(apis) == 0 {
		return nil, adminV1.ErrorNotFound("no API resources found")
	}

	for _, api := range apis {
		if api.GetPath() == path && api.GetMethod() == method {
			return api, nil
		}
	}

	return nil, nil
}

func (uc *AdminOperationLogUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListAdminOperationLogResponse, error) {
	resp, err := uc.operationLogRepo.List(ctx, req)
	if err != nil {
		return nil, err
	}

	for i := 0; i < len(resp.Items); i++ {
		l := resp.Items[i]
		if l == nil {
			continue
		}
		a, _ := uc.queryApiResources(ctx, l.GetPath(), l.GetMethod())
		if a != nil {
			l.ApiDescription = a.Description
			l.ApiModule = a.ModuleDescription
		}
	}

	return resp, nil
}

func (uc *AdminOperationLogUsecase) Get(ctx context.Context, req *adminV1.GetAdminOperationLogRequest) (*adminV1.AdminOperationLog, error) {
	resp, err := uc.operationLogRepo.Get(ctx, req)
	if err != nil {
		return nil, err
	}

	a, _ := uc.queryApiResources(ctx, resp.GetPath(), resp.GetMethod())
	if a != nil {
		resp.ApiDescription = a.Description
		resp.ApiModule = a.ModuleDescription
	}

	return resp, nil
}

// RecordAudit 写入业务审计日志（§2.6.6 等场景）。
func (uc *AdminOperationLogUsecase) RecordAudit(ctx context.Context, entry *AuditEntry) error {
	return uc.operationLogRepo.RecordAudit(ctx, entry)
}

// Clean 手动清理操作日志：物理删除 create_time 早于 now-keepDays 的记录，返回删除行数。
func (uc *AdminOperationLogUsecase) Clean(ctx context.Context, req *adminV1.CleanAdminOperationLogRequest) (*adminV1.CleanAdminOperationLogResponse, error) {
	if req.GetKeepDays() < logCleanKeepDaysMin || req.GetKeepDays() > logCleanKeepDaysMax {
		return nil, adminV1.ErrorBadRequest("invalid parameter: keep_days must be in [%d, %d]", logCleanKeepDaysMin, logCleanKeepDaysMax)
	}
	deleted, err := uc.operationLogRepo.DeleteOlderThan(ctx, retention.Cutoff(int(req.GetKeepDays())))
	if err != nil {
		return nil, err
	}
	uc.log.Infof("operation logs cleaned manually: keepDays=%d deleted=%d", req.GetKeepDays(), deleted)
	return &adminV1.CleanAdminOperationLogResponse{Deleted: uint32(deleted)}, nil
}
