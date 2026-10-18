package mme

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// VariantSetUsecase 变体簇编排（FR-MIX-01/03/04/05/06、契约 §4 组装与 Dispatch）。
type VariantSetUsecase struct {
	cfg      Config
	tasks    TaskRepo
	sets     VariantSetRepo
	variants VariantRepo
	jobs     RenderJobRepo
	assets   AssetRepo
	planner  *RenderPlanner
	copier   *CopyGenerator
	diff     *DiffChecker
	pub      EventPublisher
	tmpl     TemplateRepo
	log      *log.Helper
}

// NewVariantSetUsecase 构造。
func NewVariantSetUsecase(
	cfg Config,
	tasks TaskRepo,
	sets VariantSetRepo,
	variants VariantRepo,
	jobs RenderJobRepo,
	assets AssetRepo,
	planner *RenderPlanner,
	copier *CopyGenerator,
	diff *DiffChecker,
	pub EventPublisher,
	tmpl TemplateRepo,
	logger log.Logger,
) *VariantSetUsecase {
	return &VariantSetUsecase{cfg: cfg, tasks: tasks, sets: sets, variants: variants, jobs: jobs, assets: assets, planner: planner, copier: copier, diff: diff, pub: pub, tmpl: tmpl, log: log.NewHelper(log.With(logger, "module", "biz/mme/variantset"))}
}

// ExpandInput 展开入参。
type ExpandInput struct {
	Aspects []string
	// ContentVersionIDs 非空时只展开这些高光子集（筛选后的内容版本列为输入），
	// 与任务高光无交集时按 ErrInvalidState 拒绝（不沉默忽略用户筛选）。
	ContentVersionIDs []string
	HookIDs           []string // 预留：固定 hook 选择（本轮默认全量 DefaultHooks）
	HookTexts         []HookSpec
	TemplateID        string
}

// Expand FR-MIX-01：三维矩阵展开为 VariantSet + variants（幂等：每任务一簇，重复展开=重建）。
// 🔴 先展开后渲染：全部 selected=true。
func (u *VariantSetUsecase) Expand(ctx context.Context, taskID string, in ExpandInput) (*VariantSet, []*Variant, error) {
	t, err := u.tasks.Get(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if t == nil {
		return nil, nil, ErrTaskNotFound
	}
	if t.AssetID == "" {
		return nil, nil, ErrAssetMissing
	}
	if len(t.Highlights) == 0 {
		return nil, nil, ErrHighlightsEmpty
	}
	if t.State != TaskDownloaded && t.State != TaskAnalyzed && t.State != TaskExpanded {
		return nil, nil, fmt.Errorf("%w: expand requires downloaded/analyzed (state=%s)", ErrInvalidState, t.State)
	}

	hooks := in.HookTexts
	if len(hooks) == 0 {
		hooks = DefaultHooks(u.cfg)
	}

	// 高光子集筛选（content_version_ids 与任务高光取交集；
	// 前端勾选少量行时只展开这些行——矩阵维度 A=B 子集语义）。
	highlights := t.Highlights
	if len(in.ContentVersionIDs) > 0 {
		want := make(map[string]bool, len(in.ContentVersionIDs))
		for _, id := range in.ContentVersionIDs {
			want[id] = true
		}
		sub := make([]Highlight, 0, len(highlights))
		for _, h := range highlights {
			if want[h.ID] {
				sub = append(sub, h)
			}
		}
		if len(sub) == 0 {
			return nil, nil, fmt.Errorf("%w: content_version_ids 与任务高光无交集", ErrInvalidState)
		}
		highlights = sub
	}

	// 幂等重建：已有簇 → 连同未渲染变体一起换掉（已有产物的变体保留不清除，只过滤重复）
	set, err := u.sets.GetByTask(ctx, taskID)
	if err != nil {
		return nil, nil, err
	}
	if set == nil {
		set = &VariantSet{
			ID:          contract.NewVariantSetID(),
			TaskID:      t.ID,
			CandidateID: t.CandidateID,
			AssetID:     t.AssetID,
			State:       SetPlanned,
			TemplateID:  in.TemplateID,
			CreatedAt:   time.Now().UTC(),
		}
	} else {
		// 已渲染过的簇禁止重建（防把已审核产物连带抹掉）
		existing, lerr := u.variants.ListBySet(ctx, set.ID)
		if lerr != nil {
			return nil, nil, lerr
		}
		for _, ev := range existing {
			if ev.RenderState == RenderDone || ev.RenderState == RenderRunning {
				return nil, nil, fmt.Errorf("%w: set %s 已有渲染产物，禁止重建（用 Rerender 单体重渲）", ErrInvalidState, set.ID)
			}
		}
		set.State = SetPlanned
		set.TemplateID = in.TemplateID
	}

	variants, err := ExpandMatrix(u.cfg, set.ID, highlights, in.Aspects, hooks)
	if err != nil {
		return nil, nil, err
	}
	set.VariantCount = len(variants)

	// FR-MIX-04 文案：每个变体一份（内容版本作为输入差量）
	asset, _ := u.assets.Get(ctx, t.AssetID)
	hookTextMap := map[string]string{}
	for _, h := range hooks {
		hookTextMap[h.ID] = h.Text
	}
	highlightNote := map[string]string{}
	for _, h := range highlights {
		highlightNote[h.ID] = h.Note
	}
	for _, v := range variants {
		title := t.Title
		if asset != nil && asset.MetaTitle != "" {
			title = asset.MetaTitle
		}
		co, cerr := u.copier.Generate(ctx, CopyInput{
			AssetTitle:  title,
			UserIntent:  t.UserIntent,
			HookText:    hookTextMap[v.HookID],
			ContentNote: highlightNote[v.ContentVersion],
		})
		if cerr != nil {
			// 生成器内部已做模板降级；到这里报错=不可恢复（如 ctx 取消），向上冒泡
			return nil, nil, cerr
		}
		v.Copy = co.Copy
		v.CopyDegraded = co.Degraded
	}

	// 落库：先清旧未渲染行（幂等重建），再插新展开结果。
	// ⚠️ 顺序约束：Create(set) 须先于 CreateBatch(variants)（外键引用）。
	// 中途失败会留下"簇已建/变体缺失"的中间态——状态仍 SetPlanned，重试 Expand 可自愈。
	probe, gerr := u.sets.Get(ctx, set.ID)
	if gerr != nil {
		return nil, nil, gerr
	}
	if probe == nil {
		set.CreatedAt = time.Now().UTC()
		if err = u.sets.Create(ctx, set); err != nil {
			return nil, nil, err
		}
	} else {
		if err = u.sets.Update(ctx, set); err != nil {
			return nil, nil, err
		}
	}
	if err = u.variants.DeleteUnrenderedBySet(ctx, set.ID); err != nil {
		return nil, nil, err
	}
	if err = u.variants.CreateBatch(ctx, variants); err != nil {
		return nil, nil, err
	}

	// FR-MIX-05 差异化检查（强提示，不阻断）
	if err = u.recalcDiff(ctx, set.ID, hookTextMap); err != nil {
		u.log.Warnf("diff check failed (non-blocking): %v", err)
	}

	// 任务状态推进
	if t.State != TaskExpanded {
		t.State = TaskExpanded
		_ = u.tasks.Update(ctx, t)
	}
	u.log.Infof("expanded set %s: %d variants (degraded=%v)", set.ID, len(variants), variants[0].CopyDegraded)
	return set, variants, nil
}

// GetSet 簇详情 + 变体列表 + 关联查询用视图。
func (u *VariantSetUsecase) GetSet(ctx context.Context, setID string) (*VariantSet, []*Variant, error) {
	set, err := u.sets.Get(ctx, setID)
	if err != nil {
		return nil, nil, err
	}
	if set == nil {
		return nil, nil, ErrSetNotFound
	}
	vs, err := u.variants.ListBySet(ctx, setID)
	if err != nil {
		return nil, nil, err
	}
	return set, vs, nil
}

// UpdateSelection 删减语义（selected=false 的变体不会被规划渲染）。
func (u *VariantSetUsecase) UpdateSelection(ctx context.Context, setID string, selectedIDs, deselectedIDs []string) error {
	set, err := u.sets.Get(ctx, setID)
	if err != nil {
		return err
	}
	if set == nil {
		return ErrSetNotFound
	}
	// 已渲染/渲染中变体的选中态不可变更（产物已在路上）
	if len(selectedIDs) > 0 {
		if err = u.variants.UpdateSelection(ctx, selectedIDs, true); err != nil {
			return err
		}
	}
	if len(deselectedIDs) > 0 {
		if err = u.variants.UpdateSelection(ctx, deselectedIDs, false); err != nil {
			return err
		}
	}
	return nil
}

// Render 触发批量渲染（FR-MIX-03，只渲染 selected=true）。
func (u *VariantSetUsecase) Render(ctx context.Context, setID string) (*PlanResult, error) {
	set, err := u.sets.Get(ctx, setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, ErrSetNotFound
	}
	return u.planner.Plan(ctx, set, nil)
}

// RerenderVariant 单变体重渲（FR-MIX-08 模板变更场景）。
func (u *VariantSetUsecase) RerenderVariant(ctx context.Context, setID, variantID string) (*PlanResult, error) {
	set, err := u.sets.Get(ctx, setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, ErrSetNotFound
	}
	v, err := u.variants.Get(ctx, variantID)
	if err != nil {
		return nil, err
	}
	if v == nil || v.SetID != setID {
		return nil, ErrVariantNotFound
	}
	return u.planner.Plan(ctx, set, []string{variantID})
}

// RerenderByVariant 按变体 ID 反查所属簇后单体重渲（service 单 id 入参适配）。
func (u *VariantSetUsecase) RerenderByVariant(ctx context.Context, variantID string) (*PlanResult, error) {
	v, err := u.variants.Get(ctx, variantID)
	if err != nil {
		return nil, err
	}
	if v == nil {
		return nil, ErrVariantNotFound
	}
	return u.RerenderVariant(ctx, v.SetID, variantID)
}

// ReviewInput 批量审核入参（FR-MIX-06）。
type ReviewInput struct {
	ApprovedIDs []string
	Rejections  []RejectionInput
}

// RejectionInput 驳回项。
type RejectionInput struct {
	VariantID string
	Reason    string
}

// ReviewResult 审核结果。
type ReviewResult struct {
	Approved    int32
	Rejected    int32
	BlockedQC   []string // 被硬门禁拦截的变体 ID（前端红标）
}

// Review 批量审核（🔴 QC 硬门禁在此执行；驳回必填原因）。
func (u *VariantSetUsecase) Review(ctx context.Context, setID string, in ReviewInput) (*ReviewResult, error) {
	set, err := u.sets.Get(ctx, setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, ErrSetNotFound
	}
	res := &ReviewResult{}
	for _, r := range in.Rejections {
		if r.Reason == "" {
			return nil, fmt.Errorf("%w: variant %s", ErrRejectReasonEmpty, r.VariantID)
		}
	}
	for _, id := range in.ApprovedIDs {
		v, verr := u.variants.Get(ctx, id)
		if verr != nil {
			return nil, verr
		}
		if v == nil || v.SetID != setID {
			return nil, ErrVariantNotFound
		}
		// 🔴 硬门禁：boundary_ok/subtitle_ok 任一 false → 拒绝进入 approved，绝不静默放行
		if !(v.QCBoundaryOK && v.QCSubtitleOK) {
			res.BlockedQC = append(res.BlockedQC, id)
			u.log.Warnf("QC gate blocked approve: %s (boundary=%v subtitle=%v)", id, v.QCBoundaryOK, v.QCSubtitleOK)
			continue
		}
		v.ReviewState = ReviewApproved
		v.ReviewReason = ""
		if verr = u.variants.Update(ctx, v); verr != nil {
			return nil, verr
		}
		res.Approved++
	}
	for _, r := range in.Rejections {
		v, verr := u.variants.Get(ctx, r.VariantID)
		if verr != nil {
			return nil, verr
		}
		if v == nil || v.SetID != setID {
			return nil, ErrVariantNotFound
		}
		v.ReviewState = ReviewRejected
		v.ReviewReason = r.Reason
		if verr = u.variants.Update(ctx, v); verr != nil {
			return nil, verr
		}
		res.Rejected++
	}
	// 簇状态：全部终态 → approved/rejected
	allDone := true
	anyBlocked := len(res.BlockedQC) > 0
	vs, _ := u.variants.ListBySet(ctx, setID)
	for _, v := range vs {
		if v.ReviewState == ReviewPending {
			allDone = false
			break
		}
	}
	if allDone && !anyBlocked {
		if res.Approved > 0 {
			set.State = SetApproved
		} else {
			set.State = SetRejected
		}
		if err = u.sets.Update(ctx, set); err != nil {
			return nil, err
		}
	}
	return res, nil
}

// GetDiffCheck 读取缓存的差异化结果。
func (u *VariantSetUsecase) GetDiffCheck(ctx context.Context, setID string) (*DiffOutcome, error) {
	set, err := u.sets.Get(ctx, setID)
	if err != nil {
		return nil, err
	}
	if set == nil {
		return nil, ErrSetNotFound
	}
	return &DiffOutcome{
		MinSimilarity: set.DiffMinSimilarity,
		Warn:          set.DiffWarn,
		Pairs:         set.DiffPairs,
	}, nil
}

// RecalcDiffCheck 手动重算（模板/文案修正后）。
func (u *VariantSetUsecase) RecalcDiffCheck(ctx context.Context, setID string) (*DiffOutcome, error) {
	if err := u.recalcDiff(ctx, setID, nil); err != nil {
		return nil, err
	}
	return u.GetDiffCheck(ctx, setID)
}

// recalcDiff 计算并落库差异化结果。
func (u *VariantSetUsecase) recalcDiff(ctx context.Context, setID string, hookText map[string]string) error {
	set, err := u.sets.Get(ctx, setID)
	if err != nil || set == nil {
		return err
	}
	vs, err := u.variants.ListBySet(ctx, setID)
	if err != nil {
		return err
	}
	resolve := func(id string) string { return hookText[id] }
	out := u.diff.Check(ctx, vs, resolve, "douyin", nil)
	set.DiffMinSimilarity = out.MinSimilarity
	set.DiffWarn = out.Warn
	set.DiffPairs = out.Pairs
	return u.sets.Update(ctx, set)
}

// Dispatch 组装契约 VariantSet 并发 NATS mm.mme.variant.ready.v1（M13 下游 = MAP）。
// 只发 approved 且 QC 通过的变体；发完不标"已发布"——🔴 发布不属于本系统。
func (u *VariantSetUsecase) Dispatch(ctx context.Context, setID, traceID string) (*contract.VariantSet, error) {
	set, vs, err := u.GetSet(ctx, setID)
	if err != nil {
		return nil, err
	}
	payload := &contract.VariantSet{
		APIVersion:   "1.0",
		VariantSetID: set.ID,
		Origin: contract.VariantOrigin{
			AssetID:       set.AssetID,
			TranscriptRef: set.TranscriptRef,
		},
		DiffCheck: contract.DiffCheck{
			MinPairSimilarity: set.DiffMinSimilarity,
			Warn:              set.DiffWarn,
		},
	}
	if set.CandidateID != "" {
		cid := set.CandidateID
		payload.Origin.CandidateID = &cid
	}
	for _, v := range vs {
		// 契约 §4：boundary_ok/subtitle_ok 任一 false 的变体不得进入审批门 → 不下发
		if v.ReviewState != ReviewApproved || !(v.QCBoundaryOK && v.QCSubtitleOK) {
			continue
		}
		style := contract.CaptionStyle(v.CaptionStyle)
		if !style.Valid() {
			style = contract.CaptionSimple
		}
		cvv := contract.Variant{
			VariantID:      v.ID,
			Aspect:         v.Aspect,
			ContentVersion: v.ContentVersion,
			HookID:         v.HookID,
			DurationSec:    v.DurationSec,
			VideoPath:      v.VideoPath,
			CoverPath:      v.CoverPath,
			EDLRef:         v.EDLRef,
			Captions: contract.Captions{
				Style:   style,
				SRTPath: v.SrtPath,
			},
			Copy:         map[string]contract.PlatformCopy{},
			ManifestPath: v.ManifestPath,
			QC: contract.QC{
				BoundaryOK: v.QCBoundaryOK,
				SubtitleOK: v.QCSubtitleOK,
				Score:      v.QCScore,
			},
		}
		for k, pc := range v.Copy {
			cvv.Copy[k] = contract.PlatformCopy{Title: pc.Title, Desc: pc.Desc, Tags: pc.Tags}
		}
		payload.Variants = append(payload.Variants, cvv)
	}
	if len(payload.Variants) == 0 {
		return nil, fmt.Errorf("%w: 没有 approved 且 QC 通过的变体可下发", ErrInvalidState)
	}
	if err = payload.Validate(); err != nil {
		return nil, fmt.Errorf("contract validate: %w", err)
	}
	if err = u.pub.PublishVariantReady(ctx, payload, traceID); err != nil {
		return nil, err
	}
	u.log.Infof("dispatched variant set %s: %d approved variants -> mm.mme.variant.ready.v1", setID, len(payload.Variants))
	return payload, nil
}

// FillTaskAsset 前端任务详情要展示素材信息时的装配助手。
func (u *VariantSetUsecase) assetOf(ctx context.Context, assetID string) *Asset {
	a, err := u.assets.Get(ctx, assetID)
	if err != nil || a == nil {
		return nil
	}
	return a
}
