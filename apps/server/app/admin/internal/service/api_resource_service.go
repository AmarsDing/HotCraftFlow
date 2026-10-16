package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

type ApiResourceService struct {
	adminV1.ApiResourceServiceHTTPServer

	log *log.Helper

	uc         *biz.ApiResourceUsecase
	authorizer biz.AuthorizerPort
}

func NewApiResourceService(logger log.Logger, uc *biz.ApiResourceUsecase, authorizer biz.AuthorizerPort) *ApiResourceService {
	l := log.NewHelper(log.With(logger, "module", "api-resource/service/admin-service"))
	return &ApiResourceService{
		log:        l,
		uc:         uc,
		authorizer: authorizer,
	}
}

// resetPolicies 资源/绑定变更后全量重建授权策略；失败不阻断主流程（下次变更或重启自愈）。
func (s *ApiResourceService) resetPolicies(ctx context.Context, reason string) {
	if resetErr := s.authorizer.ResetPolicies(ctx); resetErr != nil {
		s.log.Errorf("reset policies after %s failed (non-fatal): %v", reason, resetErr)
	}
}

func (s *ApiResourceService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListApiResourceResponse, error) {
	return s.uc.List(ctx, req)
}

func (s *ApiResourceService) Get(ctx context.Context, req *adminV1.GetApiResourceRequest) (*adminV1.ApiResource, error) {
	return s.uc.Get(ctx, req)
}

func (s *ApiResourceService) Create(ctx context.Context, req *adminV1.CreateApiResourceRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ApiResourceService) Update(ctx context.Context, req *adminV1.UpdateApiResourceRequest) (*emptypb.Empty, error) {
	if req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	// path/method/status 可能变化，影响策略展开（启停切换、端点改址）
	s.resetPolicies(ctx, "api resource update")

	return &emptypb.Empty{}, nil
}

func (s *ApiResourceService) Delete(ctx context.Context, req *adminV1.DeleteApiResourceRequest) (*emptypb.Empty, error) {
	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}

	// 删除已事务级联清理角色绑定，必须重建策略使失效端点立即不可达
	s.resetPolicies(ctx, "api resource delete")

	return &emptypb.Empty{}, nil
}

// SyncApiResources 幂等 upsert 同步并返回差异报告；有鉴权面变化时重建策略。
func (s *ApiResourceService) SyncApiResources(ctx context.Context, req *emptypb.Empty) (*adminV1.SyncApiResourcesResponse, error) {
	report, err := s.uc.SyncApiResources(ctx, req)
	if err != nil {
		return nil, err
	}

	// 下线/复活改变 ListAllBriefs 过滤结果，直接影响策略展开，需即时重建；
	// created/updated 不碰 path/method/绑定，无鉴权面变化。
	if report.GetDelisted() > 0 || report.GetRelisted() > 0 {
		s.resetPolicies(ctx, "api resource sync")
	}

	return report, nil
}
