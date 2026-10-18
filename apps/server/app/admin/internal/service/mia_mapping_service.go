package service

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	miaV1 "framework/api/gen/go/media/v1/mia"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/pkg/mediacontract"
)

// MiaMappingService 映射表 A/B 管理 REST（FR-MIA-01/02）。
type MiaMappingService struct {
	uc  *bizmia.MappingUsecase
	log *log.Helper
}

// NewMiaMappingService 构造。
func NewMiaMappingService(uc *bizmia.MappingUsecase, logger log.Logger) *MiaMappingService {
	return &MiaMappingService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "mia/mapping/service")),
	}
}

// ---------- 映射表 A：实体 → 标的 ----------

func (s *MiaMappingService) ListEntityMappings(ctx context.Context, req *miaV1.ListEntityMappingsRequest) (*miaV1.ListEntityMappingsResponse, error) {
	items, total, err := s.uc.ListEntityMappings(ctx, bizmia.MappingQuery{
		Page:        normalizePage(req.GetPage()),
		PageSize:    normalizePageSize(req.GetPageSize()),
		Keyword:     req.GetKeyword(),
		EntityType:  req.GetEntityType(),
		AssetClass:  req.GetAssetClass(),
		EnabledOnly: req.GetEnabledOnly(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*miaV1.EntityAssetMappingItem, 0, len(items))
	for _, it := range items {
		out = append(out, toProtoEntityMapping(it))
	}
	return &miaV1.ListEntityMappingsResponse{Items: out, Total: int32(total)}, nil
}

func (s *MiaMappingService) CreateEntityMapping(ctx context.Context, req *miaV1.CreateEntityMappingRequest) (*miaV1.EntityAssetMappingItem, error) {
	item, err := toBizEntityMappingItem(req.GetItem())
	if err != nil {
		return nil, err
	}
	created, err := s.uc.CreateEntityMapping(ctx, item)
	if err != nil {
		return nil, err
	}
	return toProtoEntityMapping(created), nil
}

func (s *MiaMappingService) UpdateEntityMapping(ctx context.Context, req *miaV1.UpdateEntityMappingRequest) (*miaV1.EntityAssetMappingItem, error) {
	item, err := toBizEntityMappingItem(req.GetItem())
	if err != nil {
		return nil, err
	}
	updated, err := s.uc.UpdateEntityMapping(ctx, req.GetId(), item)
	if err != nil {
		return nil, err
	}
	return toProtoEntityMapping(updated), nil
}

func (s *MiaMappingService) DeleteEntityMapping(ctx context.Context, req *miaV1.DeleteEntityMappingRequest) (*miaV1.EmptyResponse, error) {
	if err := s.uc.DeleteEntityMapping(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &miaV1.EmptyResponse{}, nil
}

func (s *MiaMappingService) ImportEntityMappings(ctx context.Context, req *miaV1.ImportEntityMappingsRequest) (*miaV1.ImportEntityMappingsResponse, error) {
	if strings.TrimSpace(req.GetCsv()) == "" {
		return nil, adminV1.ErrorBadRequest("csv 内容为空")
	}
	created, updated, skipped, errs, err := s.uc.ImportEntityCSV(ctx, strings.NewReader(req.GetCsv()))
	if err != nil {
		return nil, err
	}
	return &miaV1.ImportEntityMappingsResponse{
		Created: int32(created),
		Updated: int32(updated),
		Skipped: int32(skipped),
		Errors:  errs,
	}, nil
}

func (s *MiaMappingService) ExportEntityMappings(ctx context.Context, _ *miaV1.ExportEntityMappingsRequest) (*miaV1.ExportEntityMappingsResponse, error) {
	var sb strings.Builder
	if err := s.uc.ExportEntityCSV(ctx, &sb); err != nil {
		return nil, err
	}
	return &miaV1.ExportEntityMappingsResponse{Csv: sb.String()}, nil
}

// ---------- 映射表 B：叙事 → 币种 ----------

func (s *MiaMappingService) ListNarrativeMappings(ctx context.Context, req *miaV1.ListNarrativeMappingsRequest) (*miaV1.ListNarrativeMappingsResponse, error) {
	items, total, err := s.uc.ListNarrativeMappings(ctx, bizmia.NarrativeQuery{
		Page:        normalizePage(req.GetPage()),
		PageSize:    normalizePageSize(req.GetPageSize()),
		Keyword:     req.GetKeyword(),
		EnabledOnly: req.GetEnabledOnly(),
	})
	if err != nil {
		return nil, err
	}
	out := make([]*miaV1.NarrativeCoinMappingItem, 0, len(items))
	for _, it := range items {
		out = append(out, toProtoNarrativeMapping(it))
	}
	return &miaV1.ListNarrativeMappingsResponse{Items: out, Total: int32(total)}, nil
}

func (s *MiaMappingService) CreateNarrativeMapping(ctx context.Context, req *miaV1.CreateNarrativeMappingRequest) (*miaV1.NarrativeCoinMappingItem, error) {
	item, err := toBizNarrativeMappingItem(req.GetItem())
	if err != nil {
		return nil, err
	}
	created, err := s.uc.CreateNarrativeMapping(ctx, item)
	if err != nil {
		return nil, err
	}
	return toProtoNarrativeMapping(created), nil
}

func (s *MiaMappingService) UpdateNarrativeMapping(ctx context.Context, req *miaV1.UpdateNarrativeMappingRequest) (*miaV1.NarrativeCoinMappingItem, error) {
	item, err := toBizNarrativeMappingItem(req.GetItem())
	if err != nil {
		return nil, err
	}
	updated, err := s.uc.UpdateNarrativeMapping(ctx, req.GetId(), item)
	if err != nil {
		return nil, err
	}
	return toProtoNarrativeMapping(updated), nil
}

func (s *MiaMappingService) DeleteNarrativeMapping(ctx context.Context, req *miaV1.DeleteNarrativeMappingRequest) (*miaV1.EmptyResponse, error) {
	if err := s.uc.DeleteNarrativeMapping(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &miaV1.EmptyResponse{}, nil
}

func (s *MiaMappingService) ImportNarrativeMappings(ctx context.Context, req *miaV1.ImportNarrativeMappingsRequest) (*miaV1.ImportEntityMappingsResponse, error) {
	if strings.TrimSpace(req.GetCsv()) == "" {
		return nil, adminV1.ErrorBadRequest("csv 内容为空")
	}
	created, updated, skipped, errs, err := s.uc.ImportNarrativeCSV(ctx, strings.NewReader(req.GetCsv()))
	if err != nil {
		return nil, err
	}
	return &miaV1.ImportEntityMappingsResponse{
		Created: int32(created),
		Updated: int32(updated),
		Skipped: int32(skipped),
		Errors:  errs,
	}, nil
}

func (s *MiaMappingService) ExportNarrativeMappings(ctx context.Context, _ *miaV1.ExportNarrativeMappingsRequest) (*miaV1.ExportNarrativeMappingsResponse, error) {
	var sb strings.Builder
	if err := s.uc.ExportNarrativeCSV(ctx, &sb); err != nil {
		return nil, err
	}
	return &miaV1.ExportNarrativeMappingsResponse{Csv: sb.String()}, nil
}

// ---------- 转换 ----------

// toBizEntityMappingItem proto → biz（基本合法性在此拦截，完整校验在 biz.Validate）。
func toBizEntityMappingItem(p *miaV1.EntityAssetMappingItem) (*bizmia.MappingItem, error) {
	if p == nil {
		return nil, adminV1.ErrorBadRequest("item 必填")
	}
	item := &bizmia.MappingItem{
		EntityName:    strings.TrimSpace(p.GetEntityName()),
		EntityAlias:   p.GetEntityAlias(),
		EntityType:    mediacontract.EntityType(p.GetEntityType()),
		AssetClass:    mediacontract.AssetClass(p.GetAssetClass()),
		Symbol:        strings.TrimSpace(p.GetSymbol()),
		DirectionBias: mediacontract.Direction(p.GetDirectionBias()),
		Confidence:    p.GetConfidence(),
		Source:        p.GetSource(),
		Notes:         p.GetNotes(),
		Enabled:       p.GetEnabled(),
	}
	if item.EntityName == "" || item.Symbol == "" {
		return nil, adminV1.ErrorBadRequest("entity_name 与 symbol 必填")
	}
	if !item.EntityType.Valid() {
		return nil, adminV1.ErrorBadRequest("entity_type 非法：%s", p.GetEntityType())
	}
	if !item.AssetClass.Valid() {
		return nil, adminV1.ErrorBadRequest("asset_class 非法：%s", p.GetAssetClass())
	}
	if !item.DirectionBias.Valid() {
		return nil, adminV1.ErrorBadRequest("direction_bias 非法：%s", p.GetDirectionBias())
	}
	return item, nil
}

// toBizNarrativeMappingItem proto → biz。
func toBizNarrativeMappingItem(p *miaV1.NarrativeCoinMappingItem) (*bizmia.NarrativeItem, error) {
	if p == nil {
		return nil, adminV1.ErrorBadRequest("item 必填")
	}
	item := &bizmia.NarrativeItem{
		Narrative:            strings.TrimSpace(p.GetNarrative()),
		Keywords:             p.GetKeywords(),
		Coins:                p.GetCoins(),
		DefaultSentimentHint: mediacontract.Direction(p.GetDefaultSentimentHint()),
		TTLHours:             int(p.GetTtlHours()),
		Confidence:           p.GetConfidence(),
		Enabled:              p.GetEnabled(),
	}
	if item.Narrative == "" {
		return nil, adminV1.ErrorBadRequest("narrative 必填")
	}
	if len(item.Coins) == 0 {
		return nil, adminV1.ErrorBadRequest("coins 至少 1 个")
	}
	if !item.DefaultSentimentHint.Valid() {
		return nil, adminV1.ErrorBadRequest("default_sentiment_hint 非法：%s", p.GetDefaultSentimentHint())
	}
	return item, nil
}

func toProtoEntityMapping(m *bizmia.MappingItem) *miaV1.EntityAssetMappingItem {
	return &miaV1.EntityAssetMappingItem{
		Id:            m.ID,
		EntityName:    m.EntityName,
		EntityAlias:   m.EntityAlias,
		EntityType:    string(m.EntityType),
		AssetClass:    string(m.AssetClass),
		Symbol:        m.Symbol,
		DirectionBias: string(m.DirectionBias),
		Confidence:    m.Confidence,
		Source:        m.Source,
		Notes:         m.Notes,
		Enabled:       m.Enabled,
		UpdatedAt:     formatRFC3339OrEmpty(m.UpdatedAt),
	}
}

func toProtoNarrativeMapping(n *bizmia.NarrativeItem) *miaV1.NarrativeCoinMappingItem {
	return &miaV1.NarrativeCoinMappingItem{
		Id:                   n.ID,
		Narrative:            n.Narrative,
		Keywords:             n.Keywords,
		Coins:                n.Coins,
		DefaultSentimentHint: string(n.DefaultSentimentHint),
		TtlHours:             int32(n.TTLHours),
		Confidence:           n.Confidence,
		Enabled:              n.Enabled,
		UpdatedAt:            formatRFC3339OrEmpty(n.UpdatedAt),
	}
}

// normalizePage / normalizePageSize 分页归一化（service 层公共）。
func normalizePage(p int32) int {
	if p < 1 {
		return 1
	}
	return int(p)
}

func normalizePageSize(p int32) int {
	if p < 1 || p > 500 {
		return 50
	}
	return int(p)
}

func formatRFC3339OrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}
