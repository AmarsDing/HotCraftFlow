// Package httpx 提供 kratos HTTP 传输层的统一错误语义编码器。
//
// 背景：部分服务的 biz/data 层将裸错误（ent not found、fmt.Errorf 参数校验等）
// 直接返回，kratos 默认错误编码统一映射为 500，导致 not-found/参数错误语义丢失。
// 本编码器在委托默认编码前先将裸错误归一化为带状态码的 kratos 业务错误：
//
//   - 已带 reason 的 kratos 业务错误（proto 生成）  → 原样透传
//   - ent not found / sql.ErrNoRows / wrapped "not found" → 404 NOT_FOUND
//   - ent validator / "xxx is required" / "invalid xxx" / 状态机拒绝 → 400 BAD_REQUEST
//   - 其余未知错误 → 保持 500
//
// 匹配基于消息文本以兼容多 ent 生成包（ent 主库与 ent_log 日志库的
// NotFoundError 为各自独立的类型，无法用统一的 typed 检查覆盖）。
package httpx

import (
	"database/sql"
	stderrors "errors"
	stdhttp "net/http"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
	"github.com/go-kratos/kratos/v2/transport/http"
)

// ErrorEncoder 为 http.ErrorEncoder ServerOption 提供统一错误语义映射。
func ErrorEncoder(w stdhttp.ResponseWriter, r *stdhttp.Request, err error) {
	http.DefaultErrorEncoder(w, r, normalize(err))
}

func normalize(err error) error {
	if err == nil {
		return nil
	}
	// 已带 reason 的 kratos 业务错误（proto 生成，状态码语义明确）直接透传
	var se *errors.Error
	if stderrors.As(err, &se) && se.Reason != "" {
		return err
	}

	msg := err.Error()
	switch {
	// 记录不存在 → 404
	case stderrors.Is(err, sql.ErrNoRows),
		strings.Contains(msg, " not found"),
		strings.Contains(msg, "no rows in result set"):
		return errors.NotFound("NOT_FOUND", msg)
	// 参数/业务校验 → 400
	case strings.Contains(msg, "validator failed"),
		strings.HasSuffix(msg, " is required"),
		strings.HasPrefix(msg, "invalid "),
		strings.Contains(msg, "cannot be marked"):
		return errors.BadRequest("BAD_REQUEST", msg)
	}
	return err
}
