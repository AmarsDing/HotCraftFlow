package service

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// ① IC · IntelService —— 情报检索 / 详情 / 手贴续跑 / 证据导出 / 簇治理。
//  - SupplementIntel 评审 C-12：UPDATE 既有条目 payload.text 后同步重跑归一化
//    （JetStream dedupe 窗过期后同 intel_id 消息可再投递——手贴发生在采集窗口外，
//    不会触发 B-25 幂等吞更新；DB 行即时一致兜底）。
//  - ExportEvidence M-19：operator 必填，审计字段进 zip meta.json。
// =============================================================================

// IcIntelService 情报查询与治理操作。
type IcIntelService struct {
	icV1.UnimplementedIntelServiceServer

	intel     biz.IntelItemRepo
	clusters  biz.IntelClusterRepo
	evidences biz.EvidenceRepo
	dedupUC   *biz.IcDedupUsecase
	normUC    *biz.IcNormalizeUsecase
	store     biz.EvidenceStorePort
	log       *log.Helper
}

// NewIcIntelService 构造。
func NewIcIntelService(
	intel biz.IntelItemRepo,
	clusters biz.IntelClusterRepo,
	evidences biz.EvidenceRepo,
	dedupUC *biz.IcDedupUsecase,
	normUC *biz.IcNormalizeUsecase,
	store biz.EvidenceStorePort,
	logger log.Logger,
) *IcIntelService {
	return &IcIntelService{
		intel: intel, clusters: clusters, evidences: evidences,
		dedupUC: dedupUC, normUC: normUC, store: store,
		log: log.NewHelper(log.With(logger, "module", "ic/intel/service")),
	}
}

// ListIntels 情报检索（FR-INT-14：V1 全文降级 ILIKE，B-19）。
func (s *IcIntelService) ListIntels(ctx context.Context, req *icV1.ListIntelsRequest) (*icV1.ListIntelsResponse, error) {
	page, pageSize := icPageClamp(req.GetPage(), req.GetPageSize())
	q := &biz.IntelListQuery{
		Page: page, PageSize: pageSize,
		Platform: req.GetPlatform(), Region: req.GetRegion(), Category: req.GetCategory(),
		ClusterID: req.GetClusterId(), CollectMethod: req.GetCollectMethod(), Gate: req.GetGate(),
		Keyword: req.GetKeyword(), Entity: req.GetEntity(),
	}
	if t := req.GetCollectedFrom(); t != nil {
		q.CollectedFrom = t.AsTime().UTC()
	}
	if t := req.GetCollectedTo(); t != nil {
		q.CollectedTo = t.AsTime().UTC()
	}
	rows, total, err := s.intel.List(ctx, q)
	if err != nil {
		return nil, err
	}
	out := &icV1.ListIntelsResponse{Total: int32(total)}
	for _, r := range rows {
		out.Items = append(out.Items, intelItemToProto(&r.IntelItem))
	}
	return out, nil
}

// GetIntel 单条详情。
func (s *IcIntelService) GetIntel(ctx context.Context, req *icV1.GetIntelRequest) (*icV1.IntelItem, error) {
	row, err := s.intel.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, icV1.ErrorIcSrcNotFound("intel %s 不存在", req.GetId())
	}
	return intelItemToProto(&row.IntelItem), nil
}

// SupplementIntel 手贴正文续跑（FR-INT-04 降级路径；C-12 UPDATE 不新建）。
func (s *IcIntelService) SupplementIntel(ctx context.Context, req *icV1.SupplementIntelRequest) (*icV1.SupplementIntelResponse, error) {
	if req.GetText() == "" {
		return nil, icV1.ErrorIcConfigInvalid("text 必填（手贴正文）")
	}
	row, err := s.intel.GetByID(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, icV1.ErrorIcSrcNotFound("intel %s 不存在", req.GetId())
	}
	if err := s.intel.SupplementText(ctx, req.GetId(), req.GetText()); err != nil {
		return nil, err
	}
	s.log.Infof("[audit] intel supplemented: id=%s text_len=%d", req.GetId(), len(req.GetText()))

	// 同步重跑归一化（提取/分类/闸门/去重全链以新正文为准；publish 失败由 B-25 打捞兜底）。
	if _, perr := s.normUC.Process(ctx, &contract.RawIntelNotice{
		APIVersion:  "1.0",
		IntelID:     req.GetId(),
		SourceID:    row.SourceID,
		CollectedAt: time.Now().UTC(),
	}, ""); perr != nil {
		return nil, perr
	}
	return &icV1.SupplementIntelResponse{Ok: true}, nil
}

// ExportEvidence 证据包导出（FR-INT-11/M-19：审计字段写入 zip meta.json）。
// download_url 为本机证据盘相对路径；字节流下载 handler 属 03 §6 独立开发项。
func (s *IcIntelService) ExportEvidence(ctx context.Context, req *icV1.ExportEvidenceRequest) (*icV1.EvidenceExport, error) {
	if req.GetOperator() == "" {
		return nil, icV1.ErrorIcConfigInvalid("operator 必填（导出审计 M-19）")
	}
	zipPath, _, err := s.store.Export(ctx, req.GetId(), map[string]string{
		"operator":    req.GetOperator(),
		"exported_at": time.Now().UTC().Format(time.RFC3339),
		"purpose":     "manual_export",
	})
	if err != nil {
		return nil, err
	}
	s.log.Infof("[audit] evidence exported: intel=%s operator=%s zip=%s", req.GetId(), req.GetOperator(), zipPath)

	out := &icV1.EvidenceExport{IntelId: req.GetId(), DownloadUrl: zipPath}
	// 链校验参数字段：取该条目最近一条快照的链三件套（复核入口直接可用）。
	if ev, err := s.evidences.GetLatestByIntelID(ctx, req.GetId()); err != nil {
		return nil, err
	} else if ev != nil {
		out.EvidenceId = ev.ID
		out.ChainHash = ev.ChainHash
		out.PrevHash = ev.PrevHash
		out.RawHash = ev.RawHash
	}
	return out, nil
}

// ---------- 簇管理（S-23） ----------

func (s *IcIntelService) ListClusters(ctx context.Context, req *icV1.ListClustersRequest) (*icV1.ListClustersResponse, error) {
	page, pageSize := icPageClamp(req.GetPage(), req.GetPageSize())
	rows, total, err := s.clusters.List(ctx, &biz.IntelClusterListQuery{
		Page: page, PageSize: pageSize, Region: req.GetRegion(), Status: req.GetStatus(),
	})
	if err != nil {
		return nil, err
	}
	out := &icV1.ListClustersResponse{Total: int32(total)}
	for _, r := range rows {
		out.Items = append(out.Items, &icV1.IntelCluster{
			Id: r.ID, Label: r.Label, MemberCount: r.MemberCount, PlatformCount: r.PlatformCount,
			RepresentativeId: r.RepresentativeID, Region: r.Region, MergedBy: r.MergedBy,
			Status:      r.Status,
			FirstSeenAt: timestamppb.New(r.FirstSeenAt.UTC()), LastSeenAt: timestamppb.New(r.LastSeenAt.UTC()),
		})
	}
	return out, nil
}

func (s *IcIntelService) SplitCluster(ctx context.Context, req *icV1.SplitClusterRequest) (*icV1.SplitClusterResponse, error) {
	if len(req.GetIntelIds()) == 0 {
		return nil, icV1.ErrorIcConfigInvalid("intel_ids 必填（劈出哪些条目）")
	}
	if req.GetOperator() == "" {
		return nil, icV1.ErrorIcConfigInvalid("operator 必填（S-23 审计）")
	}
	newID, err := s.dedupUC.SplitCluster(ctx, req.GetId(), req.GetIntelIds(), req.GetOperator(), "")
	if err != nil {
		return nil, err
	}
	return &icV1.SplitClusterResponse{NewClusterId: newID}, nil
}

func (s *IcIntelService) MergeCluster(ctx context.Context, req *icV1.MergeClusterRequest) (*icV1.MergeClusterResponse, error) {
	if len(req.GetFromClusterIds()) == 0 || req.GetToClusterId() == "" {
		return nil, icV1.ErrorIcConfigInvalid("from_cluster_ids / to_cluster_id 必填")
	}
	if req.GetOperator() == "" {
		return nil, icV1.ErrorIcConfigInvalid("operator 必填（S-23 审计）")
	}
	if req.GetReason() == "" {
		return nil, icV1.ErrorIcConfigInvalid("reason 必填（合并可复盘）")
	}
	for _, fromID := range req.GetFromClusterIds() {
		if fromID == req.GetToClusterId() {
			return nil, icV1.ErrorIcConfigInvalid("from 与 to 不能同簇: %s", fromID)
		}
		if err := s.dedupUC.MergeClusters(ctx, fromID, req.GetToClusterId(), req.GetOperator()); err != nil {
			return nil, err
		}
		s.log.Infof("[audit] cluster merged: %s → %s by %s reason=%s", fromID, req.GetToClusterId(), req.GetOperator(), req.GetReason())
	}
	return &icV1.MergeClusterResponse{Ok: true}, nil
}

// ---------- 映射 ----------

// intelItemToProto 契约结构 → proto（字段一一对应，枚举 string 直通——同一份词表）。
func intelItemToProto(x *contract.IntelItem) *icV1.IntelItem {
	out := &icV1.IntelItem{
		ApiVersion: x.APIVersion,
		IntelId:    x.IntelID,
		Source: &icV1.IntelItemSource{
			Platform:      x.Source.Platform,
			Region:        string(x.Source.Region),
			CollectMethod: string(x.Source.CollectMethod),
			Url:           x.Source.URL,
			Author:        x.Source.Author,
			CollectedAt:   timestamppb.New(x.Source.CollectedAt.UTC()),
			PublishedAt:   icPBTime(x.Source.PublishedAt),
			Evidence: &icV1.IntelEvidence{
				SnapshotPath: x.Source.Evidence.SnapshotPath,
				RawRef:       x.Source.Evidence.RawRef,
				HttpHeaders:  x.Source.Evidence.HTTPHeaders,
			},
		},
		Payload: &icV1.IntelPayload{
			Title: x.Payload.Title, Text: x.Payload.Text,
			MediaRefs: x.Payload.MediaRefs, Lang: x.Payload.Lang,
		},
		Category: string(x.Category),
		Metrics: &icV1.IntelMetrics{
			Rank: x.Metrics.Rank, RankTotal: x.Metrics.RankTotal,
			Views: x.Metrics.Views, Likes: x.Metrics.Likes, Comments: x.Metrics.Comments,
			AsOf: icPBTime(x.Metrics.AsOf),
		},
		Dedup: &icV1.IntelDedup{
			UrlKey: x.Dedup.URLKey, Fingerprint: x.Dedup.Fingerprint, ClusterId: x.Dedup.ClusterID,
		},
		Compliance: &icV1.IntelCompliance{
			Gate: string(x.Compliance.Gate), Reasons: x.Compliance.Reasons,
		},
		Workability: &icV1.IntelWorkability{
			Score: x.Workability.Score, Reason: x.Workability.Reason,
		},
	}
	for _, e := range x.Entities {
		out.Entities = append(out.Entities, &icV1.IntelEntity{
			Type: string(e.Type), Value: e.Value, Confidence: e.Confidence,
		})
	}
	return out
}
