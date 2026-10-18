package service

import (
	"context"
	"strconv"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	ttV1 "framework/api/gen/go/media/v1/tt"
	biztt "framework/app/admin/internal/biz/tt"
)

// TtWeightService heat 权重配置 REST（M2 权重配置页后端，详设 §1.2 + 评审 A-15）。
type TtWeightService struct {
	uc  *biztt.WeightUsecase
	log *log.Helper
}

// NewTtWeightService 构造。
func NewTtWeightService(uc *biztt.WeightUsecase, logger log.Logger) *TtWeightService {
	return &TtWeightService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "tt/weight/service")),
	}
}

// GetHeatWeights 当前生效权重 + 变更历史（权重配置页数据源）。
func (s *TtWeightService) GetHeatWeights(ctx context.Context, req *ttV1.GetHeatWeightsRequest) (*ttV1.GetHeatWeightsResponse, error) {
	cur, err := s.uc.CurrentHeat(ctx)
	if err != nil {
		return nil, err
	}
	history, err := s.uc.HistoryHeat(ctx, int(req.GetHistoryLimit()))
	if err != nil {
		return nil, err
	}
	out := &ttV1.GetHeatWeightsResponse{Current: weightViewToProto(cur)}
	for _, h := range history {
		out.History = append(out.History, weightViewToProto(h))
	}
	return out, nil
}

// UpdateHeatWeights 变更 heat 权重（Σ=1 服务端强制校验；A-15 只追加，
// effective_from = 变更时刻；activated_by 取登录态，不接受客户端传入）。
func (s *TtWeightService) UpdateHeatWeights(ctx context.Context, req *ttV1.UpdateHeatWeightsRequest) (*ttV1.WeightView, error) {
	userID, err := currentUserID(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.uc.UpdateHeat(ctx, biztt.HeatWeights{
		WRank: req.GetWRank(), WEng: req.GetWEng(),
		WFresh: req.GetWFresh(), WBreadth: req.GetWBreadth(),
	}, strconv.FormatUint(uint64(userID), 10), req.GetNote())
	if err != nil {
		// Σ≠1 / 分量越界 → 400 语义由 errors middleware 按 Error 类型映射（详设 §5）。
		s.log.Warnf("reject heat weights update (user=%d): %v", userID, err)
		return nil, err
	}
	return weightViewToProto(v), nil
}

// weightViewToProto biz → proto 视图映射。
func weightViewToProto(v biztt.WeightView) *ttV1.WeightView {
	return &ttV1.WeightView{
		ConfigId:      v.ConfigID,
		WRank:         v.Weights.WRank,
		WEng:          v.Weights.WEng,
		WFresh:        v.Weights.WFresh,
		WBreadth:      v.Weights.WBreadth,
		EffectiveFrom: timestamppb.New(v.EffectiveFrom.UTC()),
		Epoch:         v.Epoch,
		ActivatedBy:   v.ActivatedBy,
		Note:          v.Note,
		Effective:     v.Effective,
	}
}
