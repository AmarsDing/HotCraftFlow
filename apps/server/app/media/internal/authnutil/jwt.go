// Package authnutil 裸端点（SSE/WS/artifacts）的手工 JWT 验签。
//
// 背景：kratos middleware 链不覆盖裸挂 handler，而 EventSource 又无法
// 携带 Authorization header——故鉴权在 handler 内手动做：
// 支持 Authorization: Bearer <jwt> 与 ?token=<jwt> 两种携带方式。
// 密钥与 admin 服务同钥（server.yaml auth.jwt.key），令牌互通。
package authnutil

import (
	"errors"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// ErrUnauthorized 验签/取词失败。
var ErrUnauthorized = errors.New("unauthorized")

// Verifier HS256 验签器（与 admin 同一密钥即令牌互通）。
type Verifier struct {
	key []byte
}

// NewVerifier 构造验签器；key 为空字符串视为致命配置错误（上层启动期炸停）。
func NewVerifier(key string) *Verifier {
	if key == "" {
		panic("authnutil: jwt key is empty")
	}
	return &Verifier{key: []byte(key)}
}

// ExtractToken 取令牌：Authorization: Bearer 优先，其次 ?token=。
func ExtractToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if t, ok := strings.CutPrefix(h, "Bearer "); ok && t != "" {
		return t
	}
	return r.URL.Query().Get("token")
}

// Verify 验签（仅校验签名与过期；v1 不做 sub/aud 断言）。
// 返回 claims（调用方想要 sub/roles 时可读）。
func (v *Verifier) Verify(r *http.Request) (jwt.MapClaims, error) {
	tokenStr := ExtractToken(r)
	if tokenStr == "" {
		return nil, ErrUnauthorized
	}
	claims := jwt.MapClaims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(*jwt.Token) (any, error) {
		return v.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}))
	if err != nil || !tok.Valid {
		return nil, ErrUnauthorized
	}
	return claims, nil
}
