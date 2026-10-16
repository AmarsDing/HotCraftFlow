package logging

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
)

type WriteOperationLogFunc func(ctx context.Context, data *adminV1.AdminOperationLog) error
type WriteLoginLogFunc func(ctx context.Context, data *adminV1.AdminLoginLog) error

type options struct {
	logger                log.Logger
	writeOperationLogFunc WriteOperationLogFunc
	writeLoginLogFunc     WriteLoginLogFunc
}

type Option func(*options)

// WithLogger 设置日志器（异步落库的丢弃/失败告警用）。
func WithLogger(logger log.Logger) Option {
	return func(opts *options) {
		opts.logger = logger
	}
}

func WithWriteOperationLogFunc(fnc WriteOperationLogFunc) Option {
	return func(opts *options) {
		opts.writeOperationLogFunc = fnc
	}
}

func WithWriteLoginLogFunc(fnc WriteLoginLogFunc) Option {
	return func(opts *options) {
		opts.writeLoginLogFunc = fnc
	}
}
