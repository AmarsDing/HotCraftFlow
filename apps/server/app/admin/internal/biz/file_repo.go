package biz

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	fileV1 "framework/api/gen/go/file/v1"
)

// FileRepo 是文件仓储端口，由 data 层未导出的 fileRepo 实现。
// 文件为简单 CRUD（无调度编排），Create/Update 返回 error。
type FileRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*fileV1.ListFileResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *fileV1.GetFileRequest) (*fileV1.File, error)
	Create(ctx context.Context, req *fileV1.CreateFileRequest) error
	Update(ctx context.Context, req *fileV1.UpdateFileRequest) error
	Delete(ctx context.Context, req *fileV1.DeleteFileRequest) error
}

// FileUsecase 文件业务用例。
// 文件无独立业务规则，仅做参数校验后透传 Repo；service 层负责 HTTP 入口鉴权与 operator 注入。
type FileUsecase struct {
	repo FileRepo
	log  *log.Helper
}

// NewFileUsecase 创建文件 Usecase。
// repo 由 wire 注入：data.NewFileRepo 返回 biz.FileRepo interface，wire 据返回类型自动绑定。
func NewFileUsecase(repo FileRepo, logger log.Logger) *FileUsecase {
	return &FileUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "file/usecase/admin-service")),
	}
}

// List 分页查询文件列表
func (uc *FileUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*fileV1.ListFileResponse, error) {
	return uc.repo.List(ctx, req)
}

// Get 查询单个文件
func (uc *FileUsecase) Get(ctx context.Context, req *fileV1.GetFileRequest) (*fileV1.File, error) {
	return uc.repo.Get(ctx, req)
}

// Create 创建文件
func (uc *FileUsecase) Create(ctx context.Context, req *fileV1.CreateFileRequest) error {
	if req.Data == nil {
		return fileV1.ErrorBadRequest("invalid parameter")
	}
	return uc.repo.Create(ctx, req)
}

// Update 更新文件
func (uc *FileUsecase) Update(ctx context.Context, req *fileV1.UpdateFileRequest) error {
	if req.Data == nil {
		return fileV1.ErrorBadRequest("invalid parameter")
	}
	return uc.repo.Update(ctx, req)
}

// Delete 删除文件
func (uc *FileUsecase) Delete(ctx context.Context, req *fileV1.DeleteFileRequest) error {
	return uc.repo.Delete(ctx, req)
}
