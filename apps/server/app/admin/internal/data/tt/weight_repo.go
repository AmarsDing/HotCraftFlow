package ttdata

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/weightconfig"
	biztt "framework/app/admin/internal/biz/tt"
)

// EntWeightConfigRepo biz/tt.WeightConfigRepo 的 Ent 实现（A-15：只追加不改写）。
type EntWeightConfigRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntWeightConfigRepo 构造。
func NewEntWeightConfigRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntWeightConfigRepo {
	return &EntWeightConfigRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/tt-weight-repo")),
	}
}

const weightScopeHeat = "heat"

// ActiveHeatWeights 取 at 时刻生效的 heat 配置（effective_from ≤ at 最新一行）。
func (r *EntWeightConfigRepo) ActiveHeatWeights(ctx context.Context, at time.Time) (biztt.WeightConfigRow, bool, error) {
	row, err := r.cli.Client().WeightConfig.Query().
		Where(
			weightconfig.ScopeEQ(weightconfig.ScopeHeat),
			weightconfig.EffectiveFromLTE(at.UTC()),
		).
		Order(ent.Desc(weightconfig.FieldEffectiveFrom)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return biztt.WeightConfigRow{}, false, nil
		}
		return biztt.WeightConfigRow{}, false, err
	}
	return toRow(row), true, nil
}

// ListHeatWeights 历史列表（effective_from 倒序）。
func (r *EntWeightConfigRepo) ListHeatWeights(ctx context.Context, limit int) ([]biztt.WeightConfigRow, error) {
	if limit <= 0 {
		limit = 20
	}
	rows, err := r.cli.Client().WeightConfig.Query().
		Where(weightconfig.ScopeEQ(weightconfig.ScopeHeat)).
		Order(ent.Desc(weightconfig.FieldEffectiveFrom)).
		Limit(limit).
		All(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]biztt.WeightConfigRow, 0, len(rows))
	for _, row := range rows {
		out = append(out, toRow(row))
	}
	return out, nil
}

// AppendHeatWeights 插入新行（effective_from 由 biz 取变更时刻，不得回溯）。
func (r *EntWeightConfigRepo) AppendHeatWeights(ctx context.Context, row biztt.WeightConfigRow) error {
	create := r.cli.Client().WeightConfig.Create().
		SetConfigID(row.ConfigID).
		SetScope(weightconfig.ScopeHeat).
		SetWRank(row.Weights.WRank).
		SetWEng(row.Weights.WEng).
		SetWFresh(row.Weights.WFresh).
		SetWBreadth(row.Weights.WBreadth).
		SetEffectiveFrom(row.EffectiveFrom.UTC())
	if row.ActivatedBy != "" {
		create.SetActivatedBy(row.ActivatedBy)
	}
	if row.Note != "" {
		create.SetNote(row.Note)
	}
	_, err := create.Save(ctx)
	return err
}

// toRow Ent → biz 行映射。
func toRow(e *ent.WeightConfig) biztt.WeightConfigRow {
	out := biztt.WeightConfigRow{
		ConfigID: e.ConfigID,
		Scope:    string(e.Scope),
		Weights: biztt.HeatWeights{
			WRank: e.WRank, WEng: e.WEng, WFresh: e.WFresh, WBreadth: e.WBreadth,
		},
		EffectiveFrom: e.EffectiveFrom.UTC(),
	}
	if e.ActivatedBy != nil {
		out.ActivatedBy = *e.ActivatedBy
	}
	if e.Note != nil {
		out.Note = *e.Note
	}
	return out
}

// 编译期接口校验。
var _ biztt.WeightConfigRepo = (*EntWeightConfigRepo)(nil)
