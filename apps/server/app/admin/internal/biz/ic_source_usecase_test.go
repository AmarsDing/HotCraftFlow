package biz

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// =============================================================================
// 信源用例（S-20 组合矩阵 / S-18 档位下限 / S-8 豁免审计 / PATCH 合并 / Runnable）
// =============================================================================

// srcRepo 内存信源仓储（create/update/delete/list 有界面行为）。
type srcRepo struct {
	rows    map[string]*IntelSourceRow
	created []*IntelSourceRow
	updates int
	deleted []string
}

func newSrcRepo(rows ...*IntelSourceRow) *srcRepo {
	r := &srcRepo{rows: map[string]*IntelSourceRow{}}
	for _, row := range rows {
		r.rows[row.ID] = row
	}
	return r
}

func (s *srcRepo) Create(_ context.Context, in *IntelSourceCreate) (*IntelSourceRow, error) {
	row := &IntelSourceRow{ID: "src_new", Kind: in.Kind, Method: in.Method,
		IntervalMinutes: in.IntervalMinutes, RobotsExempt: in.RobotsExempt,
		RobotsExemptReason: in.RobotsExemptReason, Enabled: in.Enabled, Status: "active"}
	s.rows[row.ID] = row
	s.created = append(s.created, row)
	return row, nil
}
func (s *srcRepo) GetByID(_ context.Context, id string) (*IntelSourceRow, error) {
	return s.rows[id], nil
}
func (s *srcRepo) Update(_ context.Context, id string, patch *IntelSourcePatch) (*IntelSourceRow, error) {
	row := s.rows[id]
	if row == nil {
		return nil, ErrIcSourceNotFound
	}
	s.updates++
	if patch.Enabled != nil {
		row.Enabled = *patch.Enabled
	}
	if patch.IntervalMinutes != nil {
		row.IntervalMinutes = *patch.IntervalMinutes
	}
	return row, nil
}
func (s *srcRepo) Delete(_ context.Context, id string) (bool, error) {
	s.deleted = append(s.deleted, id)
	delete(s.rows, id)
	return true, nil
}
func (s *srcRepo) List(_ context.Context, _ *IntelSourceListQuery) ([]*IntelSourceRow, int, error) {
	out := make([]*IntelSourceRow, 0, len(s.rows))
	for _, r := range s.rows {
		out = append(out, r)
	}
	return out, len(out), nil
}
func (s *srcRepo) ListAllEnabled(context.Context) ([]*IntelSourceRow, error) { panic("src stub: not used") }
func (s *srcRepo) SetStatus(context.Context, string, string, *time.Time) error {
	panic("src stub: not used")
}
func (s *srcRepo) RecordRunOutcome(context.Context, string, bool, string, time.Time, int32, time.Duration) error {
	panic("src stub: not used")
}
func (s *srcRepo) Recompute7dCounts(context.Context, string, int32, int32) error {
	panic("src stub: not used")
}

func newSourceUC(repo IntelSourceRepo) *IcSourceUsecase {
	return NewIcSourceUsecase(repo, log.NewStdLogger(io.Discard))
}

// TestSourceValidateMatrix S-20 矩阵 × S-18 下限 × S-8 原因的笛卡尔要点。
func TestSourceValidateMatrix(t *testing.T) {
	uc := newSourceUC(newSrcRepo())
	cs := []struct {
		name                                      string
		kind, method                              string
		interval                                  int32
		exempt                                    bool
		reason                                    string
		wantErr                                   error
	}{
		{"hotlist+hotlist 合法且 ≥30 分", "hotlist", "hotlist", 30, false, "", nil},
		{"hotlist+api 回退合法", "hotlist", "api", 10, false, "", nil},
		{"rss 只能配 rss 法", "rss", "api", 10, false, "", ErrIcKindMethod},
		{"kind 不在词表", "wiki", "api", 10, false, "", ErrIcKindMethod},
		{"creator+sidecar 组合合法但须豁免", "creator", "sidecar", 1, false, "", ErrIcSidecarNeedsAuth},
		{"creator+sidecar 带授权豁免合法", "creator", "sidecar", 1, true, "权利人书面授权", nil},
		{"interval 低于 crawl 下限", "keyword", "crawl", 30, false, "", ErrIcIntervalFloor},
		{"interval 恰贴下限", "keyword", "crawl", 60, false, "", nil},
		{"hotlist 29 分低于 newsnow 节拍", "hotlist", "hotlist", 29, false, "", ErrIcIntervalFloor},
		{"豁免必填原因（S-8 审计）", "creator", "api", 10, true, "", ErrIcRobotsExemptWhy},
		{"豁免+原因 合法", "creator", "api", 10, true, "已获转载授权", nil},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			err := uc.ValidateSourceSpec(c.kind, c.method, c.interval, c.exempt, c.reason)
			if !errors.Is(err, c.wantErr) {
				t.Fatalf("err=%v want %v", err, c.wantErr)
			}
		})
	}
}

// TestSourceCRUDFlow create 预检 → get 404 → update PATCH 合并校验 → toggle → delete。
func TestSourceCRUDFlow(t *testing.T) {
	ctx := context.Background()
	repo := newSrcRepo()
	uc := newSourceUC(repo)

	// create：非法组合被预检挡下，repo 不可见
	if _, err := uc.Create(ctx, &IntelSourceCreate{Kind: "rss", Method: "crawl", IntervalMinutes: 60}); !errors.Is(err, ErrIcKindMethod) {
		t.Fatalf("非法 create 应被预检拒绝: %v", err)
	}
	if len(repo.created) != 0 {
		t.Fatal("预检失败的 create 不得触达仓储")
	}

	row, err := uc.Create(ctx, &IntelSourceCreate{Kind: "hotlist", Method: "hotlist", IntervalMinutes: 60, Enabled: true})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// get：存在/不存在
	got, err := uc.Get(ctx, row.ID)
	if err != nil || got != row {
		t.Fatalf("get: %v %v", got, err)
	}
	if _, err := uc.Get(ctx, "src_i-dont-exist"); !errors.Is(err, ErrIcSourceNotFound) {
		t.Fatalf("404 语义: %v", err)
	}

	// update：PATCH 合并后档位校验（60 → 10 < 30 下限，应拒）
	low := int32(10)
	if _, err := uc.Update(ctx, row.ID, &IntelSourcePatch{IntervalMinutes: &low}); !errors.Is(err, ErrIcIntervalFloor) {
		t.Fatalf("合并校验漏档位下限: %v", err)
	}
	if repo.updates != 0 {
		t.Fatal("校验失败的 update 不得触达仓储")
	}
	high := int32(45)
	if _, err := uc.Update(ctx, row.ID, &IntelSourcePatch{IntervalMinutes: &high}); err != nil {
		t.Fatalf("合法 update: %v", err)
	}
	if row.IntervalMinutes != 45 {
		t.Fatalf("patch 未生效: %d", row.IntervalMinutes)
	}
	// 豁免原因合并校验（长时间线上手工打开豁免但不给原因 → 拒）
	on, emptyReason := true, ""
	if _, err := uc.Update(ctx, row.ID, &IntelSourcePatch{RobotsExempt: &on, RobotsExemptReason: &emptyReason}); !errors.Is(err, ErrIcRobotsExemptWhy) {
		t.Fatalf("豁免开合须审原因: %v", err)
	}
	// 不存在的 id 走 404
	if _, err := uc.Update(ctx, "src_i-dont-exist", &IntelSourcePatch{}); !errors.Is(err, ErrIcSourceNotFound) {
		t.Fatalf("update 404: %v", err)
	}

	// toggle：存在性 + 转启停
	if err := uc.Toggle(ctx, "src_i-dont-exist", false); !errors.Is(err, ErrIcSourceNotFound) {
		t.Fatalf("toggle 404: %v", err)
	}
	if err := uc.Toggle(ctx, row.ID, false); err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if row.Enabled {
		t.Fatal("toggle 未写 enabled=false")
	}

	// list：透传仓储口径
	rows, total, err := uc.List(ctx, &IntelSourceListQuery{})
	if err != nil || total != len(rows) || total != 1 {
		t.Fatalf("list: total=%d err=%v", total, err)
	}

	// delete：存在性 + 透传
	if _, err := uc.Delete(ctx, "src_i-dont-exist"); !errors.Is(err, ErrIcSourceNotFound) {
		t.Fatalf("delete 404: %v", err)
	}
	if _, err := uc.Delete(ctx, row.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if repo.rows[row.ID] != nil {
		t.Fatal("仓储行未被删除")
	}
}

// TestSourceRunnable S-12 可调度判定（enabled + 非 paused + 熔断窗过期恢复）。
func TestSourceRunnable(t *testing.T) {
	now := time.Date(2026, 10, 19, 4, 0, 0, 0, time.UTC)
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	cs := []struct {
		name string
		src  *IntelSourceRow
		want bool
	}{
		{"nil 行", nil, false},
		{"未启用", &IntelSourceRow{Enabled: false, Status: "active"}, false},
		{"disabled 状态", &IntelSourceRow{Enabled: true, Status: "disabled"}, false},
		{"active 正常", &IntelSourceRow{Enabled: true, Status: "active"}, true},
		{"paused 且熔断窗未过", &IntelSourceRow{Enabled: true, Status: "paused", PausedUntil: &future}, false},
		{"paused 但熔断窗已过（恢复语义）", &IntelSourceRow{Enabled: true, Status: "paused", PausedUntil: &past}, true},
		{"paused 且无窗口时间（数据异常→不可行）", &IntelSourceRow{Enabled: true, Status: "paused"}, false},
	}
	for _, c := range cs {
		t.Run(c.name, func(t *testing.T) {
			if got := Runnable(c.src, now); got != c.want {
				t.Fatalf("Runnable=%v want %v", got, c.want)
			}
		})
	}
}
