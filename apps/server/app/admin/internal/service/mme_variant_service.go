package service

import (
	"context"
	"encoding/json"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"

	adminV1 "framework/api/gen/go/admin/v1"
	mmeV1 "framework/api/gen/go/media/v1/mme"
	bizmme "framework/app/admin/internal/biz/mme"
	contract "framework/pkg/mediacontract"
)

// MmeVariantService 变体簇 REST（FR-MIX-01 展开 / FR-MIX-05 差异化 / FR-MIX-06 审核分发）。
type MmeVariantService struct {
	uc    *bizmme.VariantSetUsecase
	tplUC *bizmme.TemplateUsecase
	log   *log.Helper
}

// NewMmeVariantService 构造。
func NewMmeVariantService(uc *bizmme.VariantSetUsecase, tplUC *bizmme.TemplateUsecase, logger log.Logger) *MmeVariantService {
	return &MmeVariantService{
		uc:    uc,
		tplUC: tplUC,
		log:   log.NewHelper(log.With(logger, "module", "mme/variant/service")),
	}
}

// Expand ★ 矩阵展开：内容版本 × 画幅 × Hook 笛卡尔积（纯元数据，秒级），全部 selected=true。
func (s *MmeVariantService) Expand(ctx context.Context, req *mmeV1.ExpandRequest) (*mmeV1.VariantSetView, error) {
	if req.GetTaskId() == "" {
		return nil, adminV1.ErrorBadRequest("task_id 必填")
	}
	hooks := make([]bizmme.HookSpec, 0, len(req.GetHooks()))
	for _, h := range req.GetHooks() {
		hooks = append(hooks, bizmme.HookSpec{ID: h.GetId(), Label: h.GetLabel(), Text: h.GetText()})
	}
	set, variants, err := s.uc.Expand(ctx, req.GetTaskId(), bizmme.ExpandInput{
		Aspects:           req.GetAspects(),
		ContentVersionIDs: req.GetContentVersionIds(),
		HookTexts:         hooks,
		TemplateID:        req.GetTemplateId(),
	})
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoVariantSetView(set, variants), nil
}

func (s *MmeVariantService) GetVariantSet(ctx context.Context, req *mmeV1.GetVariantSetRequest) (*mmeV1.VariantSetView, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	set, variants, err := s.uc.GetSet(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoVariantSetView(set, variants), nil
}

// UpdateSelection 删减/恢复：selected_variant_ids 为最终选中全集（其余置 deselected）。
func (s *MmeVariantService) UpdateSelection(ctx context.Context, req *mmeV1.UpdateSelectionRequest) (*mmeV1.VariantSetView, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	set, variants, err := s.uc.GetSet(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	want := make(map[string]bool, len(req.GetSelectedVariantIds()))
	for _, id := range req.GetSelectedVariantIds() {
		want[id] = true
	}
	var selected, deselected []string
	for _, v := range variants {
		if want[v.ID] {
			selected = append(selected, v.ID)
		} else {
			deselected = append(deselected, v.ID)
		}
	}
	if err = s.uc.UpdateSelection(ctx, set.ID, selected, deselected); err != nil {
		return nil, mmeBizErr(err)
	}
	set, variants, err = s.uc.GetSet(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoVariantSetView(set, variants), nil
}

// Render ★ 批量渲染：对 selected=true 且未完成的变体建 RenderJob 入队。
func (s *MmeVariantService) Render(ctx context.Context, req *mmeV1.RenderRequest) (*mmeV1.RenderResponse, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	res, err := s.uc.Render(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return &mmeV1.RenderResponse{Queued: res.Queued, Skipped: res.Skipped, AlreadyDone: res.AlreadyDone}, nil
}

// Review 批量审核。🔴 QC 硬门禁在 biz 层执行（boundary/subtitle=false 拒绝进入 approved）；
// 驳回必填原因。返回最新簇视图（审核台据 review_state/红标重渲染）。
func (s *MmeVariantService) Review(ctx context.Context, req *mmeV1.ReviewRequest) (*mmeV1.VariantSetView, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	in := bizmme.ReviewInput{ApprovedIDs: req.GetApprovedIds()}
	for _, r := range req.GetRejections() {
		in.Rejections = append(in.Rejections, bizmme.RejectionInput{VariantID: r.GetVariantId(), Reason: r.GetReason()})
	}
	res, err := s.uc.Review(ctx, req.GetId(), in)
	if err != nil {
		return nil, mmeBizErr(err)
	}
	if len(res.BlockedQC) > 0 {
		// 门禁拦截如实落日志（响应里无法多字段带出——前端通过 QC badge 可视化）
		s.log.Warnf("set %s review: %d variants blocked by QC gate: %v", req.GetId(), len(res.BlockedQC), res.BlockedQC)
	}
	set, variants, err := s.uc.GetSet(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoVariantSetView(set, variants), nil
}

// GetDiffCheck 差异化度检查结果（FR-MIX-05，强提示不硬阻断）。
func (s *MmeVariantService) GetDiffCheck(ctx context.Context, req *mmeV1.GetVariantSetRequest) (*mmeV1.DiffResult, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	d, err := s.uc.GetDiffCheck(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoDiffResult(d), nil
}

// RecalcDiffCheck 重算差异化检查。
func (s *MmeVariantService) RecalcDiffCheck(ctx context.Context, req *mmeV1.GetVariantSetRequest) (*mmeV1.DiffResult, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	d, err := s.uc.RecalcDiffCheck(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoDiffResult(d), nil
}

// Dispatch 送 ⑤ MAP：产出契约 VariantSet 并发布 mm.mme.variant.ready.v1。
func (s *MmeVariantService) Dispatch(ctx context.Context, req *mmeV1.GetVariantSetRequest) (*mmeV1.VariantSet, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	payload, err := s.uc.Dispatch(ctx, req.GetId(), "")
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoContractVariantSet(payload), nil
}

// ListTemplates 混剪模板列表（FR-MIX-08）。
func (s *MmeVariantService) ListTemplates(ctx context.Context, _ *emptypb.Empty) (*mmeV1.ListTemplatesResponse, error) {
	ts, err := s.tplUC.List(ctx, false)
	if err != nil {
		return nil, mmeBizErr(err)
	}
	items := make([]*mmeV1.MontageTemplate, 0, len(ts))
	for _, t := range ts {
		skin := ""
		if t.CaptionSkin != nil {
			if raw, err := json.Marshal(t.CaptionSkin); err == nil {
				skin = string(raw)
			}
		}
		items = append(items, &mmeV1.MontageTemplate{
			Id:          t.ID,
			Name:        t.Name,
			IntroPath:   t.IntroPath,
			OutroPath:   t.OutroPath,
			Transition:  t.Transition,
			CaptionSkin: skin,
			Enabled:     t.Enabled,
		})
	}
	return &mmeV1.ListTemplatesResponse{Items: items}, nil
}

// RerenderVariant 单变体重渲（入参 id=variant_id，biz 内部反查所属簇）。
func (s *MmeVariantService) RerenderVariant(ctx context.Context, req *mmeV1.RerenderVariantRequest) (*emptypb.Empty, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	if _, err := s.uc.RerenderByVariant(ctx, req.GetId()); err != nil {
		return nil, mmeBizErr(err)
	}
	return &emptypb.Empty{}, nil
}

// ---- 映射 ----

func toProtoVariantSetView(set *bizmme.VariantSet, variants []*bizmme.Variant) *mmeV1.VariantSetView {
	out := &mmeV1.VariantSetView{
		Id:           set.ID,
		TaskId:       set.TaskID,
		CandidateId:  set.CandidateID,
		AssetId:      set.AssetID,
		TranscriptRef: set.TranscriptRef,
		State:        set.State,
		VariantCount: int32(set.VariantCount),
		TemplateId:   set.TemplateID,
		CreatedAt:    formatRFC3339OrEmpty(set.CreatedAt),
		DiffCheck: &mmeV1.DiffResult{
			MinPairSimilarity: set.DiffMinSimilarity,
			Warn:              set.DiffWarn,
			Pairs:             toProtoPairSimilarities(set.DiffPairs),
		},
	}
	out.Variants = make([]*mmeV1.VariantItem, 0, len(variants))
	for _, v := range variants {
		out.Variants = append(out.Variants, toProtoVariantItem(v))
	}
	return out
}

func toProtoVariantItem(v *bizmme.Variant) *mmeV1.VariantItem {
	return &mmeV1.VariantItem{
		Id:             v.ID,
		SetId:          v.SetID,
		ContentVersion: v.ContentVersion,
		Aspect:         v.Aspect,
		HookId:         v.HookID,
		CoverId:        v.CoverID,
		EdlRef:         v.EDLRef,
		DurationSec:    v.DurationSec,
		VideoPath:      v.VideoPath,
		CoverPath:      v.CoverPath,
		CaptionStyle:   v.CaptionStyle,
		SrtPath:        v.SrtPath,
		Copy:           toProtoPlatformCopyMap(v.Copy),
		ManifestPath:   v.ManifestPath,
		Qc: &mmeV1.QC{
			BoundaryOk: v.QCBoundaryOK,
			SubtitleOk: v.QCSubtitleOK,
			Score:      v.QCScore,
		},
		RenderState:   v.RenderState,
		RenderError:   v.RenderError,
		Selected:      v.Selected,
		CopyDegraded:  v.CopyDegraded,
	}
}

func toProtoPlatformCopyMap(m map[string]bizmme.PlatformCopy) map[string]*mmeV1.PlatformCopy {
	if m == nil {
		return nil
	}
	out := make(map[string]*mmeV1.PlatformCopy, len(m))
	for k, c := range m {
		out[k] = &mmeV1.PlatformCopy{Title: c.Title, Desc: c.Desc, Tags: c.Tags}
	}
	return out
}

func toProtoPairSimilarities(pairs []bizmme.PairSimilarity) []*mmeV1.PairSimilarity {
	out := make([]*mmeV1.PairSimilarity, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, &mmeV1.PairSimilarity{
			VariantA: p.VariantA,
			VariantB: p.VariantB,
			Visual:   p.Visual,
			Hook:     p.Hook,
			Copy:     p.Copy,
			Overall:  p.Overall,
		})
	}
	return out
}

func toProtoDiffResult(d *bizmme.DiffOutcome) *mmeV1.DiffResult {
	return &mmeV1.DiffResult{
		MinPairSimilarity: d.MinSimilarity,
		Warn:              d.Warn,
		Pairs:             toProtoPairSimilarities(d.Pairs),
		Summary:           d.Summary,
	}
}

// toProtoContractVariantSet 契约 Payload → proto（仅结构映射，内容已由 biz Validate）。
func toProtoContractVariantSet(vs *contract.VariantSet) *mmeV1.VariantSet {
	out := &mmeV1.VariantSet{
		ApiVersion:  vs.APIVersion,
		VariantSetId: vs.VariantSetID,
		Origin: &mmeV1.VariantOrigin{
			AssetId:       vs.Origin.AssetID,
			TranscriptRef: vs.Origin.TranscriptRef,
		},
		DiffCheck: &mmeV1.DiffCheck{
			MinPairSimilarity: vs.DiffCheck.MinPairSimilarity,
			Warn:              vs.DiffCheck.Warn,
		},
	}
	if vs.Origin.CandidateID != nil {
		out.Origin.CandidateId = *vs.Origin.CandidateID
	}
	out.Variants = make([]*mmeV1.Variant, 0, len(vs.Variants))
	for _, v := range vs.Variants {
		pv := &mmeV1.Variant{
			VariantId:      v.VariantID,
			Aspect:         v.Aspect,
			ContentVersion: v.ContentVersion,
			HookId:         v.HookID,
			DurationSec:    v.DurationSec,
			VideoPath:      v.VideoPath,
			CoverPath:      v.CoverPath,
			EdlRef:         v.EDLRef,
			Captions: &mmeV1.Captions{
				Style:   string(v.Captions.Style),
				SrtPath: v.Captions.SRTPath,
			},
			ManifestPath: v.ManifestPath,
			Qc: &mmeV1.QC{
				BoundaryOk: v.QC.BoundaryOK,
				SubtitleOk: v.QC.SubtitleOK,
				Score:      v.QC.Score,
			},
		}
		if v.Copy != nil {
			pv.Copy = make(map[string]*mmeV1.PlatformCopy, len(v.Copy))
			for k, c := range v.Copy {
				pv.Copy[k] = &mmeV1.PlatformCopy{Title: c.Title, Desc: c.Desc, Tags: c.Tags}
			}
		}
		out.Variants = append(out.Variants, pv)
	}
	return out
}
