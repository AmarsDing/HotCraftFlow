package middleware

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"

	userV1 "framework/api/gen/go/user/v1"
	"framework/app/admin/internal/biz"
	"framework/pkg/entgo/viewer"
	"framework/pkg/middleware/auth"
)

// DataPermission 数据权限中间件。
//
// 在 auth.Server 之后运行，根据用户角色查询 data_permissions 规则，
// 构建 viewer.DeptIDs 并覆盖 auth 中间件注入的基础 viewer。
//
// 管理员（SYS_ADMIN/TENANT_ADMIN）跳过（保持 auth 注入的无限制 viewer）。
// 白名单接口（login/verifyMFA）跳过（由外层 selector.Match 控制）。
//
// 注意：本中间件依赖 biz.DataPermissionUsecase，每次请求会查 DB（user_role + data_permissions + ap_org_role_dept）。
// 性能敏感场景可考虑加缓存（按 userID 缓存 viewer，角色变更时清缓存）。
func DataPermission(uc *biz.DataPermissionUsecase, logger log.Logger) middleware.Middleware {
	helper := log.NewHelper(log.With(logger, "module", "data-permission.middleware"))
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			// 从 auth 中间件注入的 ctx 中获取 token payload
			tokenPayload, err := auth.FromContext(ctx)
			if err != nil {
				// 无 token（白名单接口或未认证），跳过数据权限处理
				return handler(ctx, req)
			}

			userID := tokenPayload.UserId
			authority := tokenPayload.GetAuthority()

			// 管理员跳过（保持 auth 注入的无限制 viewer）
			if authority == userV1.UserAuthority_SYS_ADMIN || authority == userV1.UserAuthority_TENANT_ADMIN {
				return handler(ctx, req)
			}

			if userID == 0 {
				helper.Warn("data permission middleware: user_id is 0 in token payload")
				return handler(ctx, req)
			}

			// 构建 viewer（查 DB：user_role + data_permissions + ap_org_role_dept）
			v, err := uc.BuildViewer(ctx, userID, authority, tokenPayload.TenantId)
			if err != nil {
				helper.Errorf("data permission middleware: build viewer failed for user %d: %v", userID, err)
				// 构建失败时，降级为最严格权限（仅 SELF），避免权限泄露
				v = viewer.UserViewer{
					Authority: authority,
					TenantId:  tokenPayload.TenantId,
					DeptIDs:   []uint32{}, // 空=仅 SELF
				}
			}

			// 覆盖 auth 中间件注入的 viewer
			ctx = viewer.NewContext(ctx, v)

			return handler(ctx, req)
		}
	}
}
