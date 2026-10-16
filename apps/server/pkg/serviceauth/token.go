package serviceauth

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	authnEngine "github.com/tx7do/kratos-authn/engine"

	userV1 "framework/api/gen/go/user/v1"
	pkgjwt "framework/pkg/jwt"
)

const (
	// ClaimFieldAudience 目标服务标识（如 "admin" / "linemonitor"）。
	ClaimFieldAudience = "aud"
	// ClaimFieldScope 权限范围列表（如 ["alarm:read","asset:read"]）。
	ClaimFieldScope = "scp"
)

// DefaultTTL 服务令牌默认有效期。
const DefaultTTL = 5 * time.Minute

// Token 从 JWT claims 解析出的服务令牌信息。
type Token struct {
	Subject   string
	Audience  string
	Scopes    []string
	UserID    uint32
	Authority string
	Roles     []string
}

// Signer 统一服务令牌签发器（基于 kratos-authn Authenticator）。
type Signer struct {
	auth    authnEngine.Authenticator
	subject string
}

// NewSigner 创建签发器。subject 为本服务标识（如 "voice-service"）。
func NewSigner(auth authnEngine.Authenticator, subject string) *Signer {
	return &Signer{auth: auth, subject: subject}
}

// Sign 签发服务令牌。
//   audience: 目标服务标识（如 "admin", "linemonitor"）。
//   scopes:   权限范围（如 []string{"alarm:read", "asset:read"}）。
func (s *Signer) Sign(audience string, scopes []string) (string, error) {
	if s.auth == nil {
		return "", fmt.Errorf("authenticator not configured")
	}
	claims := authnEngine.AuthClaims{
		pkgjwt.ClaimFieldUserName:  s.subject,
		pkgjwt.ClaimFieldUserID:    uint32(0),
		pkgjwt.ClaimFieldAuthority: userV1.UserAuthority_SYS_ADMIN.String(),
		pkgjwt.ClaimFieldRoleCodes: []string{"super"}, // 保持 casbin 兼容
		pkgjwt.ClaimFieldJwtID:     uuid.NewString(),
		ClaimFieldAudience:         audience,
		ClaimFieldScope:            scopes,
		"exp":                      time.Now().Add(DefaultTTL).Unix(),
	}
	return s.auth.CreateIdentity(claims)
}

// Parse 从 authn claims 解析服务令牌。
func Parse(claims *authnEngine.AuthClaims) (*Token, error) {
	t := &Token{}
	sub, _ := claims.GetSubject()
	t.Subject = sub
	t.UserID, _ = claims.GetUint32(pkgjwt.ClaimFieldUserID)
	t.Audience, _ = claims.GetString(ClaimFieldAudience)
	t.Scopes, _ = claims.GetStrings(ClaimFieldScope)
	t.Authority, _ = claims.GetString(pkgjwt.ClaimFieldAuthority)
	t.Roles, _ = claims.GetStrings(pkgjwt.ClaimFieldRoleCodes)
	return t, nil
}

// IsServiceToken 判断 claims 是否为服务令牌（uid == 0）。
func IsServiceToken(claims *authnEngine.AuthClaims) bool {
	uid, _ := claims.GetUint32(pkgjwt.ClaimFieldUserID)
	return uid == 0
}

// HasScope 判断 scopes 是否包含指定 scope。
func HasScope(scopes []string, scope string) bool {
	for _, s := range scopes {
		if s == scope {
			return true
		}
	}
	return false
}
