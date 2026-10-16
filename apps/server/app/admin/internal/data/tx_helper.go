package data

import (
	"context"

	"framework/app/admin/internal/data/ent"
)

// txKey 是 ctx 中携带 *ent.Tx 的 key 类型（私有避免冲突）。
type txKey struct{}

// RunInTx 在事务中执行 fn：fn 返回 error 时自动 Rollback，否则 Commit。
// 嵌套调用（fn 内部再调 RunInTx）复用外层事务，避免双开。
//
// 用法：
//
//	err := data.RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
//	    // 在 tx 内执行多个 repo 操作，任一失败全部回滚
//	    return r.repoA.DoSomethingTx(ctx, tx, ...)
//	})
func RunInTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	// 若 ctx 已携带 tx（外层 RunInTx 注入），直接复用，不再开新事务
	if tx, ok := ctx.Value(txKey{}).(*ent.Tx); ok {
		return fn(tx)
	}
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	// 将 tx 注入 ctx，供 fn 内部调用的 repo 方法检测复用
	ctx = WithTx(ctx, tx)
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// TxFromContext 从 ctx 取出当前事务（无则返回 nil）。
// data 层 repo 方法可用此函数检测是否处于外层事务中，决定复用 tx 还是新开。
func TxFromContext(ctx context.Context) *ent.Tx {
	if tx, ok := ctx.Value(txKey{}).(*ent.Tx); ok {
		return tx
	}
	return nil
}

// WithTx 将 tx 注入 ctx，返回新的 ctx。
// 由 RunInTx 内部调用；外部调用方一般用 RunInTx 而非直接用 WithTx。
func WithTx(ctx context.Context, tx *ent.Tx) context.Context {
	return context.WithValue(ctx, txKey{}, tx)
}
