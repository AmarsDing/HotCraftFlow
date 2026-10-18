package mme

import (
	"fmt"

	contract "framework/pkg/mediacontract"
)

// ExpandMatrix 三维矩阵展开（FR-MIX-01）：B 内容版本 × A 画幅 × C Hook 笛卡尔积。
//
// 纯函数：秒级出结果，只产元数据不落盘。
//   - 组合数 > cfg.MaxVariantsPerSet → ErrExpandTooLarge（不截断不缩水，要求用户减维度）
//   - 全部 selected=true（🔴 先展开后渲染、允许删减，ADR-MME-01）
//   - 画幅按 cfg.DefaultAspects 过滤请求里的 aspect；非法 key 报可读错误
func ExpandMatrix(cfg Config, setID string, highlights []Highlight, aspects []string, hooks []HookSpec) ([]*Variant, error) {
	if len(highlights) == 0 {
		return nil, ErrHighlightsEmpty
	}
	if len(hooks) == 0 {
		hooks = DefaultHooks(cfg)
	}
	// 解析画幅：空 → 默认；非法 → 报错
	if len(aspects) == 0 {
		aspects = append([]string(nil), cfg.DefaultAspects...)
	}
	specs := make([]AspectSpec, 0, len(aspects))
	seen := map[string]bool{}
	for _, key := range aspects {
		spec, ok := LookupAspect(key)
		if !ok {
			return nil, fmt.Errorf("%w: unknown aspect %q (want 9:16|1:1|16:9)", ErrInvalidState, key)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		specs = append(specs, spec)
	}
	total := len(highlights) * len(specs) * len(hooks)
	if total > cfg.MaxVariantsPerSet {
		return nil, fmt.Errorf("%w: %d content x %d aspects x %d hooks = %d > max %d",
			ErrExpandTooLarge, len(highlights), len(specs), len(hooks), total, cfg.MaxVariantsPerSet)
	}
	out := make([]*Variant, 0, total)
	for _, h := range highlights {
		for _, spec := range specs {
			for _, hook := range hooks {
				out = append(out, &Variant{
					ID:             contract.NewVariantID(),
					SetID:          setID,
					ContentVersion: h.ID,
					Aspect:         spec.Key,
					HookID:         hook.ID,
					EDLRef:         h.EDLRef(),
					DurationSec:    h.Duration(),
					CaptionStyle:   "simple",
					Copy:           map[string]PlatformCopy{},
					RenderState:    RenderPending,
					ReviewState:    ReviewPending,
					Selected:       true, // 🔴 默认全选，删减后只渲染 selected=true
				})
			}
		}
	}
	return out, nil
}
