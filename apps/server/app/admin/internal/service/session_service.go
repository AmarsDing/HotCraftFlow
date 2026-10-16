package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"

	"framework/app/admin/internal/biz"

	adminV1 "framework/api/gen/go/admin/v1"
)

// SessionService 会话管理服务：在线用户查询、强制下线。
type SessionService struct {
	adminV1.SessionServiceHTTPServer

	uc *biz.SessionUsecase

	log *log.Helper
}

func NewSessionService(
	logger log.Logger,
	uc *biz.SessionUsecase,
) *SessionService {
	return &SessionService{
		log: log.NewHelper(log.With(logger, "module", "session/service/admin-service")),
		uc:  uc,
	}
}

// ListOnline 查询在线用户列表。
func (s *SessionService) ListOnline(ctx context.Context, _ *emptypb.Empty) (*adminV1.ListOnlineUserResponse, error) {
	return s.uc.ListOnline(ctx)
}

// ForceOffline 强制下线：撤销指定用户的所有 access token。
func (s *SessionService) ForceOffline(ctx context.Context, req *adminV1.ForceOfflineRequest) (*emptypb.Empty, error) {
	if err := s.uc.ForceOffline(ctx, req.GetUserId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}
