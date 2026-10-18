package mme

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// TemplateUsecase 混剪模板（FR-MIX-08）。
// 启动时若无模板则种入 3 套默认（cut / fade / 极简无转场），
// intro/outro 路径为空 = 不装片头片尾（模板仅是可选增强）。
type TemplateUsecase struct {
	cfg  Config
	repo TemplateRepo
	log  *log.Helper
}

// NewTemplateUsecase 构造。
func NewTemplateUsecase(cfg Config, repo TemplateRepo, logger log.Logger) *TemplateUsecase {
	return &TemplateUsecase{cfg: cfg, repo: repo, log: log.NewHelper(log.With(logger, "module", "biz/mme/template"))}
}

// defaultTemplates 默认模板种子。
func defaultTemplates() []*Template {
	mk := func(name, transition string, skin map[string]any) *Template {
		return &Template{
			ID:          newTemplateID(),
			Name:        name,
			Transition:  transition,
			CaptionSkin: skin,
			Enabled:     true,
		}
	}
	return []*Template{
		mk("极简直切", "cut", map[string]any{"style": "simple", "font_size": 14}),
		mk("淡入淡出", "fade", map[string]any{"style": "simple", "font_size": 14, "fade_ms": 240}),
		mk("滑动转场", "slide", map[string]any{"style": "simple", "font_size": 16}),
	}
}

// SeedIfEmpty 启动种子（幂等；仅在 0 模板时写入）。
func (u *TemplateUsecase) SeedIfEmpty(ctx context.Context) error {
	n, err := u.repo.Count(ctx)
	if err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	for _, t := range defaultTemplates() {
		if err = u.repo.Create(ctx, t); err != nil {
			return err
		}
	}
	u.log.Infof("seeded %d default mme templates", len(defaultTemplates()))
	return nil
}

// List 模板列表（enabledOnly=true 给前端下拉）。
func (u *TemplateUsecase) List(ctx context.Context, enabledOnly bool) ([]*Template, error) {
	return u.repo.List(ctx, enabledOnly)
}

// Get 详情。
func (u *TemplateUsecase) Get(ctx context.Context, id string) (*Template, error) {
	return u.repo.Get(ctx, id)
}
