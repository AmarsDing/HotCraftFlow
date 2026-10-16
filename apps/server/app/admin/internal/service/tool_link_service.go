package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
	adminV1 "framework/api/gen/go/admin/v1"

	"framework/pkg/middleware/auth"
)

// toolLinkAdminRoles 工具链接写操作放行角色（系统管理员/运维管理员）。
// 与 ops/virtman requireAdmin 同模式；平台 Casbin 资源组绑定落地前由本门禁兜底。
var toolLinkAdminRoles = map[string]bool{"super": true, "gly": true}

type ToolLinkService struct {
	adminV1.ToolLinkServiceHTTPServer

	log *log.Helper

	uc *biz.ToolLinkUsecase
}

func NewToolLinkService(logger log.Logger, uc *biz.ToolLinkUsecase) *ToolLinkService {
	l := log.NewHelper(log.With(logger, "module", "tool-link/service/admin-service"))
	return &ToolLinkService{
		log: l,
		uc:  uc,
	}
}

// List 查询列表；登录用户注入收藏/点击标记，非管理员按 visible_roles 过滤。
func (s *ToolLinkService) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListToolLinkResponse, error) {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.uc.List(ctx, req, operator.UserId, operator.GetRoles(), hasToolLinkAdminRole(operator.GetRoles()))
}

func (s *ToolLinkService) Get(ctx context.Context, req *adminV1.GetToolLinkRequest) (*adminV1.ToolLink, error) {
	return s.uc.Get(ctx, req)
}

func (s *ToolLinkService) Create(ctx context.Context, req *adminV1.CreateToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = requireToolLinkAdmin(operator.GetRoles()); err != nil {
		return nil, err
	}

	req.Data.CreateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Create(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ToolLinkService) Update(ctx context.Context, req *adminV1.UpdateToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil || req.Data == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	// 获取操作人信息
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = requireToolLinkAdmin(operator.GetRoles()); err != nil {
		return nil, err
	}

	req.Data.UpdateBy = trans.Ptr(operator.UserId)

	if err = s.uc.Update(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ToolLinkService) Delete(ctx context.Context, req *adminV1.DeleteToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	if err := s.uc.Delete(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

func (s *ToolLinkService) Move(ctx context.Context, req *adminV1.MoveToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}

	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}

	if err := s.uc.Move(ctx, req); err != nil {
		return nil, err
	}

	return &emptypb.Empty{}, nil
}

// requireAdmin 校验当前操作人具备工具链接写权限（super/gly）。
func (s *ToolLinkService) requireAdmin(ctx context.Context) error {
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return err
	}
	return requireToolLinkAdmin(operator.GetRoles())
}

// requireToolLinkAdmin 角色码命中放行集合时返回 nil，否则 403。
func requireToolLinkAdmin(roles []string) error {
	if hasToolLinkAdminRole(roles) {
		return nil
	}
	return adminV1.ErrorForbidden("tool link write operation requires admin role (super/gly)")
}

// hasToolLinkAdminRole 是否具备工具链接管理角色。
func hasToolLinkAdminRole(roles []string) bool {
	for _, role := range roles {
		if toolLinkAdminRoles[role] {
			return true
		}
	}
	return false
}

// Reorder 批量重排（拖拽排序）；管理员操作。
func (s *ToolLinkService) Reorder(ctx context.Context, req *adminV1.ReorderToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	if err := s.uc.Reorder(ctx, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Click 记录一次点击；任意登录用户。
func (s *ToolLinkService) Click(ctx context.Context, req *adminV1.ClickToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.uc.Click(ctx, operator.UserId, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// Favorite 收藏/取消收藏；任意登录用户。
func (s *ToolLinkService) Favorite(ctx context.Context, req *adminV1.FavoriteToolLinkRequest) (*emptypb.Empty, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.uc.Favorite(ctx, operator.UserId, req); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

// CheckHealth 手动触发健康检测；管理员操作。
func (s *ToolLinkService) CheckHealth(ctx context.Context, req *adminV1.CheckToolLinkHealthRequest) (*adminV1.ToolLink, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.uc.CheckHealth(ctx, req)
}

// FetchIcon 抓取目标站点 favicon；管理员操作。
func (s *ToolLinkService) FetchIcon(ctx context.Context, req *adminV1.FetchToolLinkIconRequest) (*adminV1.FetchToolLinkIconResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.uc.FetchIcon(ctx, req)
}

// Export 导出工具链接（JSON/YAML）；管理员操作。
func (s *ToolLinkService) Export(ctx context.Context, req *adminV1.ExportToolLinksRequest) (*adminV1.ExportToolLinksResponse, error) {
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.uc.Export(ctx, req)
}

// Import 导入工具链接（JSON/YAML/书签 HTML）；管理员操作。
func (s *ToolLinkService) Import(ctx context.Context, req *adminV1.ImportToolLinksRequest) (*adminV1.ImportToolLinksResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	if err := s.requireAdmin(ctx); err != nil {
		return nil, err
	}
	return s.uc.Import(ctx, req)
}

// IssueSsoTicket 签发 SSO 跳转票据；任意登录用户（不可见链接拒签）。
func (s *ToolLinkService) IssueSsoTicket(ctx context.Context, req *adminV1.IssueToolLinkSsoTicketRequest) (*adminV1.IssueToolLinkSsoTicketResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	operator, err := auth.FromContext(ctx)
	if err != nil {
		return nil, err
	}
	return s.uc.IssueSsoTicket(ctx, operator.UserId, operator.GetRoles(), hasToolLinkAdminRole(operator.GetRoles()), req)
}

// VerifySsoTicket 校验 SSO 票据（目标系统/网关回调核验）。
func (s *ToolLinkService) VerifySsoTicket(ctx context.Context, req *adminV1.VerifyToolLinkSsoTicketRequest) (*adminV1.VerifyToolLinkSsoTicketResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid request")
	}
	return s.uc.VerifySsoTicket(ctx, req)
}
