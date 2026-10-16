package data

import (
	"context"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v4/stdlib"
	_ "github.com/lib/pq"

	"entgo.io/ent/dialect/sql/schema"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/migrate"

	conf "framework/app/admin/internal/conf"
)

// migrationTables 返回参与 ent auto-migrate 的表清单。
// warehouse_environment（库房环境监控，§1.25）为 PostgreSQL 声明式分区表，
// 主键必须包含分区键 (id, recorded_at)，ent migrate 无法表达，故从迁移清单排除，
// 由 ensureWarehouseEnvironmentPartitions 原生 SQL 在日志库幂等创建（主库不建该表）。
func migrationTables() []*schema.Table {
	tables := make([]*schema.Table, 0, len(migrate.Tables))
	for _, t := range migrate.Tables {
		if t.Name == "warehouse_environment" {
			continue
		}
		tables = append(tables, t)
	}
	return tables
}

// NewEntClient 创建主库 Ent ORM 客户端（framework 库）
func NewEntClient(cfg *conf.Bootstrap, logger log.Logger) *entgo.EntClient[*ent.Client] {
	l := log.NewHelper(log.With(logger, "module", "ent/data/admin-service"))

	drv, err := entgo.CreateDriver(
		cfg.Data.Database.GetDriver(),
		cfg.Data.Database.GetSource(),
		cfg.Data.Database.GetEnableTrace(),
		cfg.Data.Database.GetEnableMetrics(),
	)
	if err != nil {
		l.Fatalf("failed opening connection to db: %v", err)
		return nil
	}

	client := ent.NewClient(
		ent.Driver(drv),
		ent.Log(func(a ...any) {
			l.Debug(a...)
		}),
	)

	if cfg.Data.Database.GetMigrate() {
		if err = migrate.Create(context.Background(), client.Schema, migrationTables(), migrate.WithForeignKeys(true)); err != nil {
			l.Fatalf("failed creating schema resources: %v", err)
		}

		// FrameWork skeleton: skip TwinMonitor business post-migrate indexes (knowledge_base/inventory/assets/awos).

	}

	cli := entgo.NewEntClient(client, drv)

	cli.SetConnectionOption(
		int(cfg.Data.Database.GetMaxIdleConnections()),
		int(cfg.Data.Database.GetMaxOpenConnections()),
		cfg.Data.Database.GetConnectionMaxLifetime().AsDuration(),
	)

	return cli
}

// LogDB 包装日志库 ent client，使 wire 能按类型区分主库与日志库 provider。
type LogDB struct {
	*entgo.EntClient[*ent.Client]
}

// NewLogEntClient 创建日志库 Ent ORM 客户端（framework_log 库）
// 用于 admin_login_log / admin_operation_log 表，与主库物理隔离。
func NewLogEntClient(cfg *conf.Bootstrap, logger log.Logger) *LogDB {
	l := log.NewHelper(log.With(logger, "module", "ent/data/admin-service-log"))

	logDb := cfg.Data.GetLogDatabase()
	if logDb == nil {
		l.Info("log_database not configured, falling back to main database")
		return nil
	}

	drv, err := entgo.CreateDriver(
		logDb.GetDriver(),
		logDb.GetSource(),
		logDb.GetEnableTrace(),
		logDb.GetEnableMetrics(),
	)
	if err != nil {
		l.Fatalf("failed opening connection to log db: %v", err)
		return nil
	}

	client := ent.NewClient(
		ent.Driver(drv),
		ent.Log(func(a ...any) {
			l.Debug(a...)
		}),
	)

	if logDb.GetMigrate() {
		if err = migrate.Create(context.Background(), client.Schema, migrationTables(), migrate.WithForeignKeys(true)); err != nil {
			l.Fatalf("failed creating schema resources for log db: %v", err)
		}

		// 库房环境监控分区表（§1.25）：ent migrate 已排除该表，此处原生 SQL 幂等创建
		// 分区主表/序列/索引/近三月月度分区；90 天保留由 WarehouseEnvRetentionWorker 清理。
	}

	cli := entgo.NewEntClient(client, drv)

	cli.SetConnectionOption(
		int(logDb.GetMaxIdleConnections()),
		int(logDb.GetMaxOpenConnections()),
		logDb.GetConnectionMaxLifetime().AsDuration(),
	)

	return &LogDB{EntClient: cli}
}
