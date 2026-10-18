package tt

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// ============================================================================
// 权重配置管理（M2「权重配置页 API」业务层，详设 §1.2 WeightConfig + 评审 A-15）。
//
// 变更流程（A-15）：
//  ① 插入新行，effective_from = 变更时刻（不得回溯，方法内强制 now）；
//  ② weight_epoch = sha256(config_id)[:8]，写入侧后续点自动携带（HeatEnricher）
//     → 权重生效后该 topic 第一个点 α=1 重置动态链，防全站伪突发；
//  ⑥ 历史点不重算；HeatFactors 同时输出原始/生效权重（见 normalize.go）。
// ============================================================================

// WeightView 对外展示的权重视图（service 层转 proto）。
type WeightView struct {
	ConfigID      string
	Weights       HeatWeights
	EffectiveFrom time.Time
	Epoch         string
	ActivatedBy   string
	Note          string
	Effective     bool // 是否为当前生效行
}

// WeightUsecase heat 权重的查看与变更。
type WeightUsecase struct {
	repo  WeightConfigRepo
	clock func() time.Time
	log   *log.Helper
}

// NewWeightUsecase 构造。
func NewWeightUsecase(repo WeightConfigRepo, logger log.Logger) *WeightUsecase {
	return &WeightUsecase{
		repo:  repo,
		clock: time.Now,
		log:   log.NewHelper(log.With(logger, "module", "biz/tt-weight")),
	}
}

// ErrWeightRepoUnavailable 权重仓未接线（service 层映 503 降级，NFR-TT-08 不静默返空）。
var ErrWeightRepoUnavailable = errors.New("tt weight repo unavailable")

// CurrentHeat 当前生效权重；表内无行时回 Defaults（effective=true, epoch=default）。
func (uc *WeightUsecase) CurrentHeat(ctx context.Context) (WeightView, error) {
	if uc.repo == nil {
		return WeightView{}, ErrWeightRepoUnavailable
	}
	row, found, err := uc.repo.ActiveHeatWeights(ctx, uc.clock())
	if err != nil {
		return WeightView{}, err
	}
	if !found {
		now := uc.clock()
		return WeightView{
			Weights:       DefaultHeatWeights(),
			EffectiveFrom: now,
			Epoch:         DefaultWeightEpoch,
			Effective:     true,
		}, nil
	}
	return rowToView(row, true), nil
}

// HistoryHeat 权重变更历史（effective_from 倒序）。
func (uc *WeightUsecase) HistoryHeat(ctx context.Context, limit int) ([]WeightView, error) {
	if uc.repo == nil {
		return nil, ErrWeightRepoUnavailable
	}
	rows, err := uc.repo.ListHeatWeights(ctx, limit)
	if err != nil {
		return nil, err
	}
	now := uc.clock()
	out := make([]WeightView, 0, len(rows))
	for i, r := range rows {
		// 第一行（latest）且已到期 = 当前生效行。
		out = append(out, rowToView(r, i == 0 && !r.EffectiveFrom.After(now)))
	}
	return out, nil
}

// UpdateHeat 变更 heat 权重（校验 Σ=1 → 插行 → 缓存散热由 HeatEnricher TTL 承担）。
func (uc *WeightUsecase) UpdateHeat(ctx context.Context, w HeatWeights, activatedBy, note string) (WeightView, error) {
	if uc.repo == nil {
		return WeightView{}, ErrWeightRepoUnavailable
	}
	if err := w.Validate(); err != nil {
		return WeightView{}, err
	}
	row := WeightConfigRow{
		ConfigID:      mediacontract.NewID("wc"),
		Scope:         "heat",
		Weights:       w,
		EffectiveFrom: uc.clock().UTC(), // A-15 ①：恒为变更时刻
		ActivatedBy:   activatedBy,
		Note:          note,
	}
	if err := uc.repo.AppendHeatWeights(ctx, row); err != nil {
		return WeightView{}, err
	}
	WeightChangedTotal.Inc()
	uc.log.Infof("heat weights updated by %s: %+v (epoch=%s)", activatedBy, w, WeightEpochOf(row.ConfigID))
	return rowToView(row, true), nil
}

func rowToView(r WeightConfigRow, effective bool) WeightView {
	return WeightView{
		ConfigID:      r.ConfigID,
		Weights:       r.Weights,
		EffectiveFrom: r.EffectiveFrom,
		Epoch:         WeightEpochOf(r.ConfigID),
		ActivatedBy:   r.ActivatedBy,
		Note:          r.Note,
		Effective:     effective,
	}
}
