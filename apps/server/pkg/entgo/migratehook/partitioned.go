// Package migratehook 提供 ent migrate 钩子：跳过已被 PG 原生分区接管（relkind='p'）的表。
//
// 背景（T4/T5 分区改造）：flight_track_points / metar_reports / taf_reports /
// metar_reports_hourly 经 0004 切换（hourly 随 0001 直接建立）为分区父表后，
// 主键变为 (id, 分区键) 复合键；ent diff 期望单 id 主键，会尝试 DROP 分区键列，
// PG 拒绝（42P17/2BP01）→ ent_client Fatalf 崩溃循环。
// 分区表 DDL 所有权在 migrate/*.sql，ent 仅作读写映射，
// 故 diff 阶段将分区表从 current/desired 双侧剔除（双侧剔除避免误生成 DropTable）。
package migratehook

import (
	"context"

	aschema "ariga.io/atlas/sql/schema"
	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
	entschema "entgo.io/ent/dialect/sql/schema"
)

// PartitionedTables 返回 public schema 下所有分区父表名集合（relkind='p'）。
// 查询失败返回错误——宁可启动期暴露，不静默放行 diff（否则可能误改分区表）。
func PartitionedTables(ctx context.Context, drv dialect.Driver) (map[string]bool, error) {
	// ent dialect.Driver.Query 的 v 必须是 *sql.Rows（ent 生成代码同款模式），
	// 再经 ScanSlice 扫入切片；直接传 *[]string 会被方言层拒绝（invalid type）。
	// args 必须传 []any（nil 接口过不了方言层 args.([]any) 断言）。
	rows := &entsql.Rows{}
	if err := drv.Query(ctx,
		`SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		 WHERE n.nspname = 'public' AND c.relkind = 'p'`,
		[]any{}, rows); err != nil {
		return nil, err
	}
	defer rows.Close()
	var names []string
	if err := entsql.ScanSlice(rows, &names); err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(names))
	for _, n := range names {
		set[n] = true
	}
	return set, nil
}

// SkipPartitioned 返回 DiffHook：将 skip 集合中的表从 diff 双侧剔除。
// skip 为空时零开销直通。current/desired 均被复制后修改，不改写调用方对象。
func SkipPartitioned(skip map[string]bool) entschema.DiffHook {
	return func(next entschema.Differ) entschema.Differ {
		return entschema.DiffFunc(func(current, desired *aschema.Schema) ([]aschema.Change, error) {
			if len(skip) == 0 {
				return next.Diff(current, desired)
			}
			cur := *current
			cur.Tables = filterTables(cur.Tables, skip)
			des := *desired
			des.Tables = filterTables(des.Tables, skip)
			return next.Diff(&cur, &des)
		})
	}
}

func filterTables(tables []*aschema.Table, skip map[string]bool) []*aschema.Table {
	out := make([]*aschema.Table, 0, len(tables))
	for _, t := range tables {
		if !skip[t.Name] {
			out = append(out, t)
		}
	}
	return out
}
