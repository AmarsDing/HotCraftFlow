package data

import (
	"context"
	"fmt"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/casbinrule"
)

// CasbinRuleRepo 操作 casbin_rule 表，作为 Casbin 策略的持久化层。
// 启动时 ResetPolicies 全量重写；角色权限变更时 SyncPolicies 增量更新。
type CasbinRuleRepo struct {
	data *Data
	log  *log.Helper
}

func NewCasbinRuleRepo(data *Data, logger log.Logger) *CasbinRuleRepo {
	return &CasbinRuleRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "casbin-rule/repo/admin-service")),
	}
}

// CasbinRuleRow 对外暴露的行结构，字段与 ent.CasbinRule 对齐但解耦。
type CasbinRuleRow struct {
	PType string
	V0    string
	V1    string
	V2    string
	V3    string
	V4    string
	V5    string
}

// ListAll 读取全部策略。
func (r *CasbinRuleRepo) ListAll(ctx context.Context) ([]CasbinRuleRow, error) {
	entities, err := r.data.db.Client().CasbinRule.Query().All(ctx)
	if err != nil {
		r.log.Errorf("list casbin rules failed: %s", err.Error())
		return nil, fmt.Errorf("list casbin rules failed: %w", err)
	}

	rows := make([]CasbinRuleRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, toCasbinRuleRow(e))
	}
	return rows, nil
}

// ClearAll 清空全部策略（启动时全量重建前调用）。
func (r *CasbinRuleRepo) ClearAll(ctx context.Context) error {
	n, err := r.data.db.Client().CasbinRule.Delete().Exec(ctx)
	if err != nil {
		r.log.Errorf("clear casbin rules failed: %s", err.Error())
		return fmt.Errorf("clear casbin rules failed: %w", err)
	}
	r.log.Infof("cleared %d casbin rules", n)
	return nil
}

// DeleteBySubject 删除指定主体（v0 = subject）的所有策略，用于 SyncPolicies。
func (r *CasbinRuleRepo) DeleteBySubject(ctx context.Context, subject string) (int, error) {
	n, err := r.data.db.Client().CasbinRule.Delete().
		Where(casbinrule.V0EQ(subject)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete casbin rules by subject %q failed: %s", subject, err.Error())
		return 0, fmt.Errorf("delete casbin rules by subject failed: %w", err)
	}
	return n, nil
}

// BulkInsert 批量写入策略。
func (r *CasbinRuleRepo) BulkInsert(ctx context.Context, rows []CasbinRuleRow) error {
	if len(rows) == 0 {
		return nil
	}

	builders := make([]*ent.CasbinRuleCreate, 0, len(rows))
	for _, row := range rows {
		b := r.data.db.Client().CasbinRule.Create().
			SetPtype(row.PType).
			SetV0(row.V0).
			SetV1(row.V1).
			SetV2(row.V2)
		if row.V3 != "" {
			b.SetV3(row.V3)
		}
		if row.V4 != "" {
			b.SetV4(row.V4)
		}
		if row.V5 != "" {
			b.SetV5(row.V5)
		}
		builders = append(builders, b)
	}

	if err := r.data.db.Client().CasbinRule.CreateBulk(builders...).Exec(ctx); err != nil {
		r.log.Errorf("bulk insert casbin rules failed: %s", err.Error())
		return fmt.Errorf("bulk insert casbin rules failed: %w", err)
	}
	return nil
}

func toCasbinRuleRow(e *ent.CasbinRule) CasbinRuleRow {
	row := CasbinRuleRow{
		PType: e.Ptype,
		V0:    e.V0,
		V1:    e.V1,
		V2:    e.V2,
	}
	if e.V3 != nil {
		row.V3 = *e.V3
	}
	if e.V4 != nil {
		row.V4 = *e.V4
	}
	if e.V5 != nil {
		row.V5 = *e.V5
	}
	return row
}
