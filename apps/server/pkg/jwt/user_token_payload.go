package jwt

import (
	"errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/tx7do/go-utils/trans"

	authn "github.com/tx7do/kratos-authn/engine"

	authenticationV1 "framework/api/gen/go/authentication/v1"
	userV1 "framework/api/gen/go/user/v1"
)

const (
	ClaimFieldUserName  = authn.ClaimFieldSubject // 用户名
	ClaimFieldUserID    = "uid"                   // 用户ID
	ClaimFieldTenantID  = "tid"                   // 租户ID
	ClaimFieldClientID  = "cid"                   // 客户端ID
	ClaimFieldDeviceID  = "did"                   // 设备ID
	ClaimFieldAuthority = "aut"                   // 用户权限
	ClaimFieldRoleID    = "rid"                   // 角色ID
	ClaimFieldRoleCodes = "roc"                   // 角色码列表
	ClaimFieldJwtID     = authn.ClaimFieldJwtID   // JWT 唯一标识，用于区分每次登录签发的 token
)

// NewUserTokenPayload 创建用户令牌
func NewUserTokenPayload(user *userV1.User, clientId string) *authenticationV1.UserTokenPayload {
	return &authenticationV1.UserTokenPayload{
		UserId:    user.GetId(),
		TenantId:  user.TenantId,
		Username:  user.Username,
		ClientId:  trans.Ptr(clientId),
		Authority: user.GetAuthority(),
		Roles:     user.Roles,
	}
}

// NewUserTokenAuthClaims 构造访问令牌的 JWT claims。
// 必须包含 jti（每次签发生成唯一 UUID），否则同一用户多次登录会产出完全相同的 JWT，
// 与 user_tokens 表上既有的（可能已撤销的）记录冲突，storeToken 幂等忽略后导致鉴权中间件找不到有效令牌。
func NewUserTokenAuthClaims(user *userV1.User, clientId string) *authn.AuthClaims {
	return &authn.AuthClaims{
		ClaimFieldUserName:  user.GetUsername(),
		ClaimFieldUserID:    user.GetId(),
		ClaimFieldTenantID:  user.GetTenantId(),
		ClaimFieldClientID:  clientId,
		ClaimFieldAuthority: user.Authority.String(),
		ClaimFieldRoleCodes: user.Roles,
		ClaimFieldJwtID:     uuid.New().String(),
	}
}

func NewUserTokenPayloadWithClaims(claims *authn.AuthClaims) (*authenticationV1.UserTokenPayload, error) {
	payload := &authenticationV1.UserTokenPayload{}

	sub, _ := claims.GetSubject()
	if sub != "" {
		payload.Username = trans.Ptr(sub)
	}

	userId, _ := claims.GetUint32(ClaimFieldUserID)
	if userId != 0 {
		payload.UserId = userId
	}

	tenantId, _ := claims.GetUint32(ClaimFieldTenantID)
	if tenantId != 0 {
		payload.TenantId = trans.Ptr(tenantId)
	}

	clientId, _ := claims.GetString(ClaimFieldClientID)
	if clientId != "" {
		payload.ClientId = trans.Ptr(clientId)
	}

	authority, _ := claims.GetString(ClaimFieldAuthority)
	if authority != "" {
		v, ok := userV1.UserAuthority_value[authority]
		if ok {
			payload.Authority = userV1.UserAuthority(v)
		}
	}

	roles, _ := claims.GetStrings(ClaimFieldRoleCodes)
	if roles != nil {
		payload.Roles = roles
	}

	return payload, nil
}

func NewUserTokenPayloadWithJwtMapClaims(claims jwt.MapClaims) (*authenticationV1.UserTokenPayload, error) {
	payload := &authenticationV1.UserTokenPayload{}

	sub, _ := claims.GetSubject()
	if sub != "" {
		payload.Username = trans.Ptr(sub)
	}

	userId, _ := claims[ClaimFieldUserID]
	if userId != nil {
		payload.UserId = uint32(userId.(float64))
	}

	tenantId, _ := claims[ClaimFieldTenantID]
	if tenantId != nil {
		payload.TenantId = trans.Ptr(uint32(tenantId.(float64)))
	}

	clientId, _ := claims[ClaimFieldClientID]
	if clientId != nil {
		payload.ClientId = trans.Ptr(clientId.(string))
	}

	authority, _ := claims[ClaimFieldAuthority]
	if authority != nil {
		v, ok := userV1.UserAuthority_value[authority.(string)]
		if ok {
			payload.Authority = userV1.UserAuthority(v)
		}
	}

	roles, _ := claims[ClaimFieldRoleCodes]
	if roles != nil {
		switch itf := roles.(type) {
		case []interface{}:
			payload.Roles = make([]string, 0, len(itf))
			for _, v := range itf {
				if str, ok := v.(string); ok {
					payload.Roles = append(payload.Roles, str)
				}
			}

		case []string:
			payload.Roles = itf

		default:
			return nil, errors.New("invalid roles type")
		}
	}

	return payload, nil
}
