package middleware

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/go-kratos/kratos/v2/middleware"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"
)

// SanitizePagingQuery 分页查询清洗中间件。
//
// go-utils/entgo 的 BuildQuerySelector 对「空 JSON 对象/数组」的 query/or 字符串（"{}" / "[]"）
// 会生成零谓词的 WHERE 选择器，渲染出 "WHERE  ORDER BY ..." 残缺 SQL（pq: 语法错误 在 "ORDER" 或附近, 42601）。
// 调用方传入空过滤对象等价于不过滤，此处统一归一化（置 nil），
// 避免 admin 全部 repo（100+ 个 BuildQuerySelector 调用点）各自防御。
func SanitizePagingQuery() middleware.Middleware {
	return func(handler middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req interface{}) (interface{}, error) {
			if pr, ok := req.(*pagination.PagingRequest); ok && pr != nil {
				pr.Query = nilIfEmptyFilterJSON(pr.Query)
				pr.OrQuery = nilIfEmptyFilterJSON(pr.OrQuery)
			}
			return handler(ctx, req)
		}
	}
}

// nilIfEmptyFilterJSON 过滤 JSON 串解析后无任何条件时返回 nil，否则原样返回。
func nilIfEmptyFilterJSON(s *string) *string {
	if s == nil {
		return nil
	}
	t := strings.TrimSpace(*s)
	if t == "" {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal([]byte(t), &m); err == nil {
		if len(m) == 0 {
			return nil
		}
		return s
	}
	var arr []json.RawMessage
	if err := json.Unmarshal([]byte(t), &arr); err == nil && len(arr) == 0 {
		return nil
	}
	return s
}
