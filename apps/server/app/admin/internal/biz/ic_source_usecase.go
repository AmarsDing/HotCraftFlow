package biz

import (
	"context"
	"errors"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// =============================================================================
// 信源管理（FR-INT-01/02 + S-20 组合矩阵 + S-18 档位下限 + S-8 豁免审计）
// =============================================================================

// S-20 合法 kind×method 组合矩阵（评审原文，须与前端校验同源）。
// sidecar 仅 kind=creator 且须授权（RobotsExempt=true + 原因必填）。
var icKindMethodMatrix = map[string]map[string]bool{
	"hotlist":    {"hotlist": true, "api": true}, // hotlist(+api 回退)
	"keyword":    {"api": true, "crawl": true, "hotlist": true},
	"creator":    {"api": true, "rss": true, "crawl": true, "sidecar": true},
	"rss":        {"rss": true},
	"competitor": {"api": true, "rss": true, "crawl": true},
}

// S-18 按 method 分级的 interval_minutes 下限（分钟；sidecar 评审未指定，取 1）。
var icMethodIntervalFloor = map[string]int32{
	"hotlist": 30, // newsnow 缓存节拍下限
	"rss":     5,
	"api":     10,
	"crawl":   60,
	"sidecar": 1,
}

// IcSourceUsecase 信源 CRUD/启停与不变量校验。
type IcSourceUsecase struct {
	repo IntelSourceRepo
	log  *log.Helper
}

func NewIcSourceUsecase(repo IntelSourceRepo, logger log.Logger) *IcSourceUsecase {
	return &IcSourceUsecase{repo: repo, log: log.NewHelper(log.With(logger, "module", "ic/source/biz/admin-service"))}
}

var (
	ErrIcSourceNotFound   = errors.New("IC source not found")
	ErrIcKindMethod       = errors.New("IC kind×method 组合非法")
	ErrIcIntervalFloor    = errors.New("IC interval_minutes 低于 method 档位下限")
	ErrIcRobotsExemptWhy  = errors.New("IC robots_exempt 须填原因（S-8 授权审计）")
	ErrIcSidecarNeedsAuth = errors.New("IC sidecar 须授权豁免（S-20）")
)

// ValidateSourceSpec 集中校验（create/update 共用；update 对未改字段用原值合并后校验）。
func (u *IcSourceUsecase) ValidateSourceSpec(kind, method string, interval int32, robotsExempt bool, exemptReason string) error {
	methods, ok := icKindMethodMatrix[kind]
	if !ok || !methods[method] {
		return ErrIcKindMethod
	}
	if floor, ok := icMethodIntervalFloor[method]; ok && interval < floor {
		return ErrIcIntervalFloor
	}
	if robotsExempt && exemptReason == "" {
		return ErrIcRobotsExemptWhy
	}
	if method == "sidecar" && !robotsExempt {
		return ErrIcSidecarNeedsAuth
	}
	return nil
}

func (u *IcSourceUsecase) Create(ctx context.Context, in *IntelSourceCreate) (*IntelSourceRow, error) {
	if err := u.ValidateSourceSpec(in.Kind, in.Method, in.IntervalMinutes, in.RobotsExempt, in.RobotsExemptReason); err != nil {
		return nil, err
	}
	return u.repo.Create(ctx, in)
}

func (u *IcSourceUsecase) Get(ctx context.Context, id string) (*IntelSourceRow, error) {
	row, err := u.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrIcSourceNotFound
	}
	return row, nil
}

func (u *IcSourceUsecase) Update(ctx context.Context, id string, patch *IntelSourcePatch) (*IntelSourceRow, error) {
	cur, err := u.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// 合并后校验（PATCH 语义：nil = 不改）
	kind, method := cur.Kind, cur.Method
	interval := cur.IntervalMinutes
	if patch.IntervalMinutes != nil {
		interval = *patch.IntervalMinutes
	}
	exempt, reason := cur.RobotsExempt, cur.RobotsExemptReason
	if patch.RobotsExempt != nil {
		exempt = *patch.RobotsExempt
	}
	if patch.RobotsExemptReason != nil {
		reason = *patch.RobotsExemptReason
	}
	if err := u.ValidateSourceSpec(kind, method, interval, exempt, reason); err != nil {
		return nil, err
	}
	return u.repo.Update(ctx, id, patch)
}

func (u *IcSourceUsecase) Delete(ctx context.Context, id string) (bool, error) {
	if _, err := u.Get(ctx, id); err != nil {
		return false, err
	}
	return u.repo.Delete(ctx, id)
}

func (u *IcSourceUsecase) List(ctx context.Context, q *IntelSourceListQuery) ([]*IntelSourceRow, int, error) {
	return u.repo.List(ctx, q)
}

// Toggle 启停（FR-INT-02 人工干预入口；熔断 paused 状态由 RecordRunOutcome 驱动，不走此路）。
func (u *IcSourceUsecase) Toggle(ctx context.Context, id string, enabled bool) error {
	if _, err := u.Get(ctx, id); err != nil { // 存在性校验（404 语义）
		return err
	}
	one := enabled
	_, err := u.repo.Update(ctx, id, &IntelSourcePatch{Enabled: &one})
	return err
}

// Runnable 判定本轮是否可调度（enabled + 非 paused + 熔断窗已过）。
// paused_until 已过的 paused 信源由调用方顺手拨回 active（S-12 恢复语义）。
func Runnable(src *IntelSourceRow, now time.Time) bool {
	if src == nil || !src.Enabled || src.Status == "disabled" {
		return false
	}
	if src.Status == "paused" {
		return src.PausedUntil != nil && !now.Before(*src.PausedUntil)
	}
	return true
}
