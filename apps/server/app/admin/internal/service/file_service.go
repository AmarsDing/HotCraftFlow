package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/emptypb"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
	fileV1 "framework/api/gen/go/file/v1"

	"framework/pkg/middleware/auth"
)

// FileService 文件服务层。
// 持有 *biz.FileUsecase（不直接持有 data repo），负责 HTTP 入口鉴权、operator 注入；
// 修复 B5：跨层访问 data.FileRepo 改为经由 biz.FileUsecase。
type FileService struct {
	adminV1.FileServiceHTTPServer

	log *log.Helper

	uc *biz.FileUsecase
}

func NewFileService(logger log.Logger, uc *biz.FileUsecase) *FileService {
	l := log.NewHelper(log.With(logger, "module", "file/service/admin-service"))
	return &FileService{
		log: l,
		uc:  uc,
	}
}

func (s *FileService) List(ctx context.Context, req *pagination.PagingRequest) (*fileV1.ListFileResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *FileService) Get(ctx context.Context, req *fileV1.GetFileRequest) (*fileV1.File, error) {
	return s.uc.Get(ctx, req)
}

func (s *FileService) Create(ctx context.Context, req *fileV1.CreateFileRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息并注入审计字段
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

func (s *FileService) Update(ctx context.Context, req *fileV1.UpdateFileRequest) (*emptypb.Empty, error) {
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

func (s *FileService) Delete(ctx context.Context, req *fileV1.DeleteFileRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
