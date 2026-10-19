package auth

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"unicode"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/transport"
	"github.com/go-kratos/kratos/v2/transport/http"

	authnEngine "github.com/tx7do/kratos-authn/engine"
	authn "github.com/tx7do/kratos-authn/middleware"

	authzEngine "github.com/tx7do/kratos-authz/engine"
	authz "github.com/tx7do/kratos-authz/middleware"

	"github.com/tx7do/go-utils/stringutil"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	authenticationV1 "framework/api/gen/go/authentication/v1"

	"framework/pkg/entgo/viewer"
	"framework/pkg/jwt"
	"framework/pkg/metadata"
)

var defaultAction = authzEngine.Action("ANY")

// extractRawToken 从 Authorization 头提取裸 token（去除 Bearer 前缀）。
func extractRawToken(ctx context.Context) string {
	tr, ok := transport.FromServerContext(ctx)
	if !ok {
		return ""
	}
	authHeader := tr.RequestHeader().Get("Authorization")
	if authHeader == "" {
		return ""
	}
	const prefix = "Bearer "
	if len(authHeader) > len(prefix) && authHeader[:len(prefix)] == prefix {
		return authHeader[len(prefix):]
	}
	return authHeader
}

// Server 衔接认证和鉴权
func Server(opts ...Option) middleware.Middleware {
	op := options{
		log: log.NewHelper(log.With(log.DefaultLogger, "module", "auth.middleware")),

		injectOperatorId: false,
		injectTenantId:   false,
		enableAuthz:      true,
		injectEnt:        true,
		injectMetadata:   true,
	}
	for _, o := range opts {
		o(&op)
	}

	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			tr, ok := transport.FromServerContext(ctx)
			if !ok {
				op.log.Errorf("auth middleware: missing transport in context")
				return nil, ErrWrongContext
			}

			authnClaims, ok := authn.FromContext(ctx)
			if !ok {
				op.log.Errorf("auth middleware: missing transport in context")
				return nil, ErrWrongContext
			}

			tokenPayload, err := jwt.NewUserTokenPayloadWithClaims(authnClaims)
			if err != nil {
				op.log.Errorf("auth middleware: invalid token payload in context [%s]", err.Error())
				return nil, ErrExtractUserInfoFailed
			}

			// 校验访问令牌是否存在（含 per-token 黑名单校验）
			if op.isExistAccessToken != nil {
				rawToken := extractRawToken(ctx)
				if !op.isExistAccessToken(ctx, tokenPayload.UserId, rawToken) {
					op.log.Errorf("auth middleware: access token revoked or not found for user %d", tokenPayload.UserId)
					return nil, ErrAccessTokenExpired
				}
			}

			if op.injectOperatorId {
				if err = setRequestOperationId(req, tokenPayload); err != nil {
					op.log.Errorf("auth middleware: invalid token payload in context [%s]", err.Error())
					return nil, err
				}
			}
			if op.injectTenantId {
				if err = setRequestTenantId(req, tokenPayload); err != nil {
					op.log.Errorf("auth middleware: invalid token payload in context [%s]", err.Error())
					return nil, err
				}

				if err = ensurePagingRequestTenantId(req, tokenPayload); err != nil {
					op.log.Errorf("auth middleware: invalid token payload in context [%s]", err.Error())
					return nil, err
				}
			}

			if op.injectEnt {
				ctx = viewer.NewContext(ctx, viewer.UserViewer{
					UserID:    trans.Ptr(tokenPayload.UserId),
					Authority: tokenPayload.GetAuthority(),
					TenantId:  tokenPayload.TenantId,
				})
			}

			if op.injectMetadata {
				ctx = metadata.NewOperatorMetadataContext(ctx,
					trans.Ptr(tokenPayload.UserId),
					tokenPayload.TenantId,
					trans.Ptr(tokenPayload.GetAuthority()),
				)
			}

			if op.enableAuthz {
				ctx, err = processAuthz(ctx, tr, tokenPayload)
				if err != nil {
					op.log.Errorf("auth middleware: invalid token payload in context [%s]", err.Error())
					return nil, err
				}
			}

			return handler(ctx, req)
		}
	}
}

func processAuthz(
	ctx context.Context,
	tr transport.Transporter,
	tokenPayload *authenticationV1.UserTokenPayload,
) (context.Context, error) {
	path := authzEngine.Resource(tr.Operation())
	action := defaultAction

	var htr *http.Transport
	var ok bool
	if htr, ok = tr.(*http.Transport); ok {
		path = authzEngine.Resource(normalizePathTemplate(htr.PathTemplate()))
		action = authzEngine.Action(htr.Request().Method)
	}

	// 主体投影纪律：只投角色（Subjects），不投用户名（Subject）。
	// 上游 tx7do authz middleware@v1.1.13 是 else-if 语义——Subject 非 nil 时只查
	// Subject，Subjects 永远轮不到；而 generateAllPolicies 是「角色直绑」模型
	// （p, <roleCode>, path, act，无 g 规则），用户名主体在策略中不存在，投了必 403。
	authzClaims := authzEngine.AuthClaims{
		Subjects: trans.Ptr(tokenPayload.GetRoles()),
		Action:   trans.Ptr(action),
		Resource: trans.Ptr(path),
	}

	ctx = authz.NewContext(ctx, &authzClaims)

	return ctx, nil
}

// normalizePathTemplate 归一路由模板与 openapi（gnostic）的路径参数命名。
//
// 背景：api_resource 与 casbin 策略由 app/admin/assets/openapi.yaml 同步而来，
// 该文件由 protoc-gen-openapi(gnostic) 生成——它会把路径参数 camel 化
// （proto 注解中的 {task_id} 在文档里变成 {taskId}）；而 kratos
// http.Transport.PathTemplate() 保留 proto 注解开头的 snake 形态。
// 两侧参数名不一致时模板字符串失配 → casbin 拒访（403），受影响面是所有
// 多词参数路由（mme /tasks/{task_id}/expand、monitor roles/{role_id}/depts、
// sessions/{user_id} 等）。这里把路由模板参数名归一到 gnostic 形态：
// 仅重写 {} 内的参数名（下划线转小驼峰），静态段与自定义动词（:expand）不动。
func normalizePathTemplate(t string) string {
	if !strings.Contains(t, "_") {
		return t
	}
	var b strings.Builder
	b.Grow(len(t))
	inParam := false
	upperNext := false
	for _, r := range t {
		switch {
		case r == '{':
			inParam = true
			b.WriteRune(r)
		case r == '}':
			inParam = false
			b.WriteRune(r)
		case inParam && r == '_':
			upperNext = true
		case inParam && upperNext:
			b.WriteRune(unicode.ToUpper(r))
			upperNext = false
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func FromContext(ctx context.Context) (*authenticationV1.UserTokenPayload, error) {
	claims, ok := authnEngine.AuthClaimsFromContext(ctx)
	if !ok {
		return nil, ErrMissingJwtToken
	}

	return jwt.NewUserTokenPayloadWithClaims(claims)
}

func setRequestOperationId(req interface{}, payload *authenticationV1.UserTokenPayload) error {
	if req == nil {
		return ErrInvalidRequest
	}

	v := reflect.ValueOf(req).Elem()
	field := v.FieldByName("OperatorId")
	if field.IsValid() && field.Kind() == reflect.Ptr {
		field.Set(reflect.ValueOf(&payload.UserId))
	}

	return nil
}

func setRequestTenantId(req interface{}, payload *authenticationV1.UserTokenPayload) error {
	if req == nil {
		return ErrInvalidRequest
	}

	v := reflect.ValueOf(req).Elem()
	field := v.FieldByName("TenantId")
	if field.IsValid() && field.Kind() == reflect.Ptr {
		field.Set(reflect.ValueOf(&payload.TenantId))
	}

	return nil
}

func ensurePagingRequestTenantId(req interface{}, payload *authenticationV1.UserTokenPayload) error {
	if paging, ok := req.(*pagination.PagingRequest); ok && payload.GetTenantId() > 0 {
		if paging.Query != nil {
			newStr := stringutil.ReplaceJSONField("tenantId|tenant_id", strconv.Itoa(int(payload.GetTenantId())), paging.GetQuery())
			paging.Query = trans.Ptr(newStr)
		}
		if paging.OrQuery != nil {
			newStr := stringutil.ReplaceJSONField("tenantId|tenant_id", strconv.Itoa(int(payload.GetTenantId())), paging.GetOrQuery())
			paging.OrQuery = trans.Ptr(newStr)
		}
	}
	return nil
}
