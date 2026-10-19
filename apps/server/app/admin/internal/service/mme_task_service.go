package service

import (
	"context"
	"encoding/json"

	"github.com/go-kratos/kratos/v2/log"
	"google.golang.org/protobuf/types/known/emptypb"

	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
	mmeV1 "framework/api/gen/go/media/v1/mme"
	bizmme "framework/app/admin/internal/biz/mme"
)

// MmeTaskService 混剪任务 REST（M1/M2/M3/M4：建任务→下载→分析→手工 EDL）。
// 领域哨兵错误（MME_*）原样透传——错误串本身可读，前端按消息弹出。
type MmeTaskService struct {
	uc  *bizmme.TaskUsecase
	log *log.Helper
}

// NewMmeTaskService 构造。
func NewMmeTaskService(uc *bizmme.TaskUsecase, logger log.Logger) *MmeTaskService {
	return &MmeTaskService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "mme/task/service")),
	}
}

func (s *MmeTaskService) List(ctx context.Context, req *pagination.PagingRequest) (*mmeV1.ListMontageTasksResponse, error) {
	items, total, err := s.uc.List(ctx, bizmme.TaskQuery{
		State:    mmeQueryFilter(req.GetQuery(), "state"),
		Page:     normalizePage(req.GetPage()),
		PageSize: normalizePageSize(req.GetPageSize()),
	})
	if err != nil {
		return nil, mmeBizErr(err)
	}
	out := make([]*mmeV1.MontageTask, 0, len(items))
	for _, t := range items {
		out = append(out, toProtoMontageTask(t, nil))
	}
	return &mmeV1.ListMontageTasksResponse{Items: out, Total: uint32(total)}, nil
}

func (s *MmeTaskService) Get(ctx context.Context, req *mmeV1.GetMontageTaskRequest) (*mmeV1.MontageTask, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	t, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	asset, err := s.uc.GetAsset(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoMontageTask(t, asset), nil
}

func (s *MmeTaskService) Create(ctx context.Context, req *mmeV1.CreateMontageTaskRequest) (*mmeV1.MontageTask, error) {
	if req.GetTitle() == "" || req.GetSourceType() == "" || req.GetSourceRef() == "" {
		return nil, adminV1.ErrorBadRequest("title / source_type / source_ref 均必填")
	}
	t, err := s.uc.Create(ctx, req.GetTitle(), req.GetSourceType(), req.GetSourceRef(), req.GetUserIntent(), "", "")
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoMontageTask(t, nil), nil
}

func (s *MmeTaskService) Delete(ctx context.Context, req *mmeV1.GetMontageTaskRequest) (*emptypb.Empty, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	if err := s.uc.Delete(ctx, req.GetId()); err != nil {
		return nil, mmeBizErr(err)
	}
	return &emptypb.Empty{}, nil
}

// Analyze 触发分析（ASR → 章节/高光）；缺依赖时任务侧如实落 MME_ASR_FAIL / MME_LLM_UNAVAILABLE。
func (s *MmeTaskService) Analyze(ctx context.Context, req *mmeV1.GetMontageTaskRequest) (*mmeV1.MontageTask, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	if err := s.uc.RequestAnalyze(ctx, req.GetId()); err != nil {
		return nil, mmeBizErr(err)
	}
	t, err := s.uc.Get(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return toProtoMontageTask(t, nil), nil
}

// ListHighlights 高光候选列表（手工 EDL 来源）。
func (s *MmeTaskService) ListHighlights(ctx context.Context, req *mmeV1.GetMontageTaskRequest) (*mmeV1.ListHighlightsResponse, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	hs, err := s.uc.ListHighlights(ctx, req.GetId())
	if err != nil {
		return nil, mmeBizErr(err)
	}
	return &mmeV1.ListHighlightsResponse{Items: toProtoHighlights(hs)}, nil
}

// UpsertHighlights 手工 EDL 入口（无 AI 时内容版本的唯一来源；按任务全量覆盖）。
func (s *MmeTaskService) UpsertHighlights(ctx context.Context, req *mmeV1.UpsertHighlightsRequest) (*mmeV1.ListHighlightsResponse, error) {
	if req.GetId() == "" {
		return nil, adminV1.ErrorBadRequest("id 必填")
	}
	hs := make([]bizmme.Highlight, 0, len(req.GetItems()))
	for _, it := range req.GetItems() {
		hs = append(hs, bizmme.Highlight{
			ID:       it.GetId(),
			Label:    it.GetLabel(),
			StartSec: it.GetStartSec(),
			EndSec:   it.GetEndSec(),
			Note:     it.GetNote(),
			Origin:   it.GetOrigin(),
		})
	}
	t, err := s.uc.UpsertHighlights(ctx, req.GetId(), hs)
	if err != nil {
		return nil, mmeBizErr(err)
	}
	// 返回落库后的视图（ID/Origin 缺省值已补齐）
	return &mmeV1.ListHighlightsResponse{Items: toProtoHighlights(t.Highlights)}, nil
}

// Doctor 基础设施体检（W1 门禁：缺 ffmpeg/whisper 红标，不猜状态）。
func (s *MmeTaskService) Doctor(ctx context.Context, _ *emptypb.Empty) (*mmeV1.DoctorReport, error) {
	r, err := s.uc.Doctor(ctx)
	if err != nil {
		return nil, mmeBizErr(err)
	}
	items := make([]*mmeV1.DoctorItem, 0, len(r.Items))
	for _, it := range r.Items {
		items = append(items, &mmeV1.DoctorItem{
			Name:     it.Name,
			Ok:       it.OK,
			Required: it.Required,
			Version:  it.Version,
			Detail:   it.Detail,
		})
	}
	return &mmeV1.DoctorReport{Ready: r.OK, Items: items}, nil
}

// GetSettings 生效设置快照（W1 设置面板：matrix/render/storage 默认值）。
func (s *MmeTaskService) GetSettings(ctx context.Context, _ *emptypb.Empty) (*mmeV1.MmeSettings, error) {
	snap := s.uc.Settings(ctx)
	return &mmeV1.MmeSettings{
		DefaultAspects:    snap.DefaultAspects,
		DefaultHookCount:  int32(snap.DefaultHookCount),
		MaxVariantsPerSet: int32(snap.MaxVariantsPerSet),
		RenderConcurrency: int32(snap.RenderConcurrency),
		RenderPreset:      snap.Preset,
		RenderCrf:         int32(snap.CRF),
		MinDurationSec:    int32(snap.MinDurSec),
		MaxDurationSec:    int32(snap.MaxDurSec),
		MediaDir:          snap.MediaDir,
		RetentionDays:     int32(snap.RetentionDays),
		CacheEnabled:      snap.CacheEnabled,
	}, nil
}

// ---- 映射 ----

func toProtoMontageTask(t *bizmme.Task, asset *bizmme.Asset) *mmeV1.MontageTask {
	out := &mmeV1.MontageTask{
		Id:            t.ID,
		Title:         t.Title,
		SourceType:    t.SourceType,
		SourceRef:     t.SourceRef,
		CandidateId:   t.CandidateID,
		IntelId:       t.IntelID,
		AssetId:       t.AssetID,
		UserIntent:    t.UserIntent,
		State:         t.State,
		PromptVersion: t.PromptVersion,
		AsrEngine:     t.ASREngine,
		ErrorMsg:      t.ErrorMsg,
		CreatedAt:     formatRFC3339OrEmpty(t.CreatedAt),
		UpdatedAt:     formatRFC3339OrEmpty(t.UpdatedAt),
	}
	if asset != nil {
		out.Asset = toProtoAssetInfo(asset)
	}
	return out
}

func toProtoAssetInfo(a *bizmme.Asset) *mmeV1.AssetInfo {
	return &mmeV1.AssetInfo{
		AssetId:     a.ID,
		SourceUrl:   a.SourceURL,
		DurationSec: a.DurationSec,
		Width:       int32(a.Width),
		Height:      int32(a.Height),
		SizeBytes:   a.SizeBytes,
		RefCount:    int32(a.RefCount),
		Fingerprint: a.Fingerprint,
	}
}

func toProtoHighlights(hs []bizmme.Highlight) []*mmeV1.HighlightCandidate {
	out := make([]*mmeV1.HighlightCandidate, 0, len(hs))
	for _, h := range hs {
		out = append(out, &mmeV1.HighlightCandidate{
			Id:       h.ID,
			Label:    h.Label,
			StartSec: h.StartSec,
			EndSec:   h.EndSec,
			Note:     h.Note,
			Origin:   h.Origin,
		})
	}
	return out
}

// mmeQueryFilter 从 pagination.query（JSON map 字符串，如 {"state":"downloaded"}）
// 取一个字符串过滤字段；解析失败返回空串（不阻断列表）。
func mmeQueryFilter(query, key string) string {
	if query == "" {
		return ""
	}
	m := map[string]any{}
	if err := json.Unmarshal([]byte(query), &m); err != nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}
