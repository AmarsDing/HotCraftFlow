package service

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"

	radarV1 "framework/api/gen/go/radar/v1"
	"framework/app/admin/internal/biz"
)

// RadarRepoService implements radar.service.v1.RepoService HTTP/gRPC.
// 仓库详情 / 文件树 / 文件内容 / 克隆 / AI 总结。
type RadarRepoService struct {
	uc  *biz.RadarRepoUsecase
	log *log.Helper
}

func NewRadarRepoService(uc *biz.RadarRepoUsecase, logger log.Logger) *RadarRepoService {
	return &RadarRepoService{
		uc:  uc,
		log: log.NewHelper(log.With(logger, "module", "radar/repo/service")),
	}
}

func (s *RadarRepoService) GetRepo(ctx context.Context, req *radarV1.GetRepoRequest) (*radarV1.GetRepoResponse, error) {
	if req == nil {
		req = &radarV1.GetRepoRequest{}
	}
	repo, readme, insight, stale, staleReason, err := s.uc.GetRepo(ctx, req.FullName, req.IncludeReadme, req.InsightLevel)
	if err != nil {
		return nil, err
	}
	resp := &radarV1.GetRepoResponse{
		Repo:        toRepoItem(repo),
		Readme:      readme,
		Stale:       stale,
		StaleReason: staleReason,
	}
	if insight != nil {
		resp.Insight = toAIInsight(insight)
	}
	return resp, nil
}

func (s *RadarRepoService) GetRepoTree(ctx context.Context, req *radarV1.GetRepoTreeRequest) (*radarV1.GetRepoTreeResponse, error) {
	if req == nil {
		req = &radarV1.GetRepoTreeRequest{}
	}
	nodes, defaultBranch, truncated, err := s.uc.GetRepoTree(ctx, req.FullName, req.Branch)
	if err != nil {
		return nil, err
	}
	out := make([]*radarV1.TreeNode, 0, len(nodes))
	for _, n := range nodes {
		out = append(out, &radarV1.TreeNode{Path: n.Path, Type: n.Type, Size: n.Size})
	}
	return &radarV1.GetRepoTreeResponse{
		Nodes: out, DefaultBranch: defaultBranch, Truncated: truncated,
	}, nil
}

func (s *RadarRepoService) GetRepoContent(ctx context.Context, req *radarV1.GetRepoContentRequest) (*radarV1.GetRepoContentResponse, error) {
	if req == nil {
		req = &radarV1.GetRepoContentRequest{}
	}
	ct, err := s.uc.GetRepoContent(ctx, req.FullName, req.Path, req.Ref)
	if err != nil {
		return nil, err
	}
	content := ct.Content
	encoding := ct.Encoding
	if ct.IsText {
		// 文本直接 utf-8 下发；二进制保持 base64 原文
		encoding = "utf-8"
	}
	return &radarV1.GetRepoContentResponse{
		Path: ct.Path, Name: ct.Name, Content: content, Encoding: encoding,
		Size: ct.Size, Truncated: ct.Truncated,
		HtmlUrl: ct.HTMLURL, DownloadUrl: ct.DownloadURL,
	}, nil
}

func (s *RadarRepoService) CloneRepo(ctx context.Context, req *radarV1.CloneRepoRequest) (*radarV1.CloneRepoResponse, error) {
	if req == nil {
		req = &radarV1.CloneRepoRequest{}
	}
	res, err := s.uc.CloneRepo(ctx, req.FullName, req.ToServer)
	if err != nil {
		return nil, err
	}
	return &radarV1.CloneRepoResponse{
		CloneUrl: res.CloneURL, SshUrl: res.SSHURL, ZipUrl: res.ZipURL,
		Command: res.Command, SshCommand: res.SSHCommand,
		LocalPath: res.LocalPath, Cloned: res.Cloned, Message: res.Message,
	}, nil
}

func (s *RadarRepoService) GenerateInsight(ctx context.Context, req *radarV1.GenerateInsightRequest) (*radarV1.GenerateInsightResponse, error) {
	if req == nil {
		req = &radarV1.GenerateInsightRequest{}
	}
	ins, err := s.uc.GenerateInsight(ctx, req.FullName, req.Level)
	if err != nil {
		return nil, err
	}
	return &radarV1.GenerateInsightResponse{Insight: toAIInsight(ins)}, nil
}

func toAIInsight(in *biz.RadarInsight) *radarV1.AIInsight {
	if in == nil {
		return nil
	}
	return &radarV1.AIInsight{
		FullName: in.FullName, Level: in.Level,
		ContentMarkdown: in.ContentMarkdown, Model: in.Model,
		GeneratedAt: in.GeneratedAt, Cached: in.Cached,
		DurationMs: in.DurationMs, Error: in.Error,
	}
}
