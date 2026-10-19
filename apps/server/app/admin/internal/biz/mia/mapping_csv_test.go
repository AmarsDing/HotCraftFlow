package mia

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/go-kratos/kratos/v2/log"
)

// =============================================================================
// CSV 导入测试（FR-MIA-01/02：映射表是非技术可维护的长期运营资产）
// + 出厂种子金标：种子文件必须 100% 通过导入器（≥200/≥30 行）。
// =============================================================================

func TestImportEntityCSV_HappyAndRowErrors(t *testing.T) {
	repo := newEntityRepoStub()
	uc := NewMappingUsecase(repo, newNarrativeRepoStub(), log.DefaultLogger)

	csvText := "entity_name,entity_alias,entity_type,asset_class,symbol,direction_bias,confidence,source,notes,enabled\n" +
		"美联储,Fed;FOMC,org,index,SPY,neutral,0.70,seed,货币政策决定流动性,true\n" +
		"英伟达,NVIDIA;NVDA,org,equity,NVDA,neutral,0.90,seed,公司本体直接定价,true\n" +
		"缺notes的,X,org,equity,AAPL,neutral,0.90,seed,,true\n" + // notes 空 → 拒
		"坏枚举,X,alien,equity,MSFT,neutral,0.90,seed,有理由,true\n" + // 非法 entity_type → 拒
		"缺尾的,X,org,equity,TSLA,neutral\n" // 列数不足 → 拒
	created, updated, skipped, errs, err := uc.ImportEntityCSV(context.Background(), strings.NewReader(csvText))
	if err != nil {
		t.Fatalf("导入不应整体失败，got %v", err)
	}
	if created != 2 || updated != 0 || skipped != 3 {
		t.Fatalf("期望 created=2 skipped=3，got created=%d updated=%d skipped=%d", created, updated, skipped)
	}
	if len(errs) != 3 {
		t.Fatalf("行级错误应累计进 errs（含行号），got %v", errs)
	}
	// 别名按 ';' 拆分落表。
	rows, _ := repo.ListEnabled(context.Background())
	var fed *MappingItem
	for _, r := range rows {
		if r.EntityName == "美联储" {
			fed = r
		}
	}
	if fed == nil || len(fed.EntityAlias) != 2 || fed.EntityAlias[1] != "FOMC" {
		t.Fatalf("别名应按 ';' 拆分入库，got %+v", fed)
	}

	// 幂等：同唯一键再导入 → updated 而非重复行。
	created, updated, _, _, err = uc.ImportEntityCSV(context.Background(), strings.NewReader(csvText))
	if err != nil || created != 0 || updated != 2 {
		t.Fatalf("重复导入应幂等 update，got created=%d updated=%d err=%v", created, updated, err)
	}
	if n, _ := repo.Count(context.Background()); n != 2 {
		t.Fatalf("幂等更新不应新增行，got %d", n)
	}
}

func TestImportNarrativeCSV_Validation(t *testing.T) {
	nrepo := newNarrativeRepoStub()
	uc := NewMappingUsecase(newEntityRepoStub(), nrepo, log.DefaultLogger)

	csvText := "narrative,keywords,coins,default_sentiment_hint,ttl_hours,confidence,enabled\n" +
		"比特币减半,减半;halving,BTC,bullish,24,0.65,true\n" +
		"超长TTL,x,BTC,bullish,25,0.65,true\n" + // ttl>24 → 拒（衰减纪律）
		"零TTL,x,BTC,bullish,0,0.65,true\n" + // ttl<1 → 拒
		"坏提示,x,BTC,moonish,12,0.65,true\n" + // 非法 hint → 拒
		"空币种,x,,bullish,12,0.65,true\n" // coins 空 → 拒
	created, _, skipped, errs, err := uc.ImportNarrativeCSV(context.Background(), strings.NewReader(csvText))
	if err != nil {
		t.Fatalf("导入不应整体失败，got %v", err)
	}
	if created != 1 || skipped != 4 {
		t.Fatalf("期望 created=1 skipped=4，got created=%d skipped=%d errs=%v", created, skipped, errs)
	}
	rows, _ := nrepo.ListEnabled(context.Background())
	if len(rows) != 1 || rows[0].Coins[0] != "BTC" || len(rows[0].Keywords) != 2 {
		t.Fatalf("合法行应入库且关键词/币种拆列，got %+v", rows)
	}
}

// TestSeedCSVsPassImport 🔒 出厂种子金标：种子文件必须 100% 通过导入器。
// 约束源自落地清单：表 A ≥200 条（notes 必填、枚举合法）、表 B ≥30 条（ttl 1–24）。
func TestSeedCSVsPassImport(t *testing.T) {
	const (
		entitySeedPath    = "../../../config/seed/mia_entity_mapping_seed.csv"
		narrativeSeedPath = "../../../config/seed/mia_narrative_mapping_seed.csv"
	)
	fa, errA := os.Open(entitySeedPath)
	fb, errB := os.Open(narrativeSeedPath)
	if errA != nil || errB != nil {
		t.Skipf("种子文件不在预期位置（跳出金标）：%v / %v", errA, errB)
	}
	defer func() { _ = fa.Close(); _ = fb.Close() }()

	erepo := newEntityRepoStub()
	nrepo := newNarrativeRepoStub()
	uc := NewMappingUsecase(erepo, nrepo, log.DefaultLogger)

	createdA, _, skippedA, errsA, err := uc.ImportEntityCSV(context.Background(), fa)
	if err != nil {
		t.Fatalf("表 A 种子导入整体失败：%v", err)
	}
	if skippedA != 0 || len(errsA) != 0 {
		t.Fatalf("表 A 种子必须 100%% 导入成功：skipped=%d errs=%v", skippedA, errsA[:min(5, len(errsA))])
	}
	if createdA < 200 {
		t.Fatalf("表 A 种子应 ≥200 条（落地清单），got %d", createdA)
	}
	n, _ := erepo.Count(context.Background())
	if n != createdA {
		t.Fatalf("唯一键存在重复行：created=%d 实存=%d", createdA, n)
	}

	createdB, _, skippedB, errsB, err := uc.ImportNarrativeCSV(context.Background(), fb)
	if err != nil {
		t.Fatalf("表 B 种子导入整体失败：%v", err)
	}
	if skippedB != 0 || len(errsB) != 0 {
		t.Fatalf("表 B 种子必须 100%% 导入成功：skipped=%d errs=%v", skippedB, errsB[:min(5, len(errsB))])
	}
	if createdB < 30 {
		t.Fatalf("表 B 种子应 ≥30 条（落地清单），got %d", createdB)
	}

	t.Logf("种子金标通过：表 A %d 行，表 B %d 行，全部零拒绝", createdA, createdB)
}

// ---------- 测试替身 ----------

// entityRepoStub 内存版 EntityMappingRepo（唯一键 entity_name|asset_class|symbol）。
type entityRepoStub struct {
	rows   map[string]*MappingItem
	nextID int64
}

func newEntityRepoStub() *entityRepoStub {
	return &entityRepoStub{rows: make(map[string]*MappingItem), nextID: 1}
}

func entityKeyOf(it *MappingItem) string {
	return it.EntityName + "|" + string(it.AssetClass) + "|" + it.Symbol
}

func (s *entityRepoStub) Create(_ context.Context, item *MappingItem) (*MappingItem, error) {
	k := entityKeyOf(item)
	if _, dup := s.rows[k]; dup {
		return nil, errors.New("duplicate key")
	}
	cp := *item
	cp.ID = s.nextID
	s.nextID++
	s.rows[k] = &cp
	return &cp, nil
}

func (s *entityRepoStub) Update(_ context.Context, id int64, item *MappingItem) (*MappingItem, error) {
	for k, r := range s.rows {
		if r.ID == id {
			cp := *item
			cp.ID = id
			delete(s.rows, k)
			s.rows[entityKeyOf(item)] = &cp
			return &cp, nil
		}
	}
	return nil, errors.New("not found")
}

func (s *entityRepoStub) Delete(_ context.Context, id int64) error {
	for k, r := range s.rows {
		if r.ID == id {
			delete(s.rows, k)
			return nil
		}
	}
	return errors.New("not found")
}

func (s *entityRepoStub) Get(_ context.Context, id int64) (*MappingItem, error) {
	for _, r := range s.rows {
		if r.ID == id {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *entityRepoStub) List(_ context.Context, q MappingQuery) ([]*MappingItem, int, error) {
	out := make([]*MappingItem, 0, len(s.rows))
	for _, r := range s.rows {
		if q.EnabledOnly && !r.Enabled {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	return out, len(out), nil
}

func (s *entityRepoStub) ListEnabled(ctx context.Context) ([]*MappingItem, error) {
	rows, _, err := s.List(ctx, MappingQuery{EnabledOnly: true})
	return rows, err
}

func (s *entityRepoStub) UpsertByKey(_ context.Context, item *MappingItem) (bool, error) {
	k := entityKeyOf(item)
	if old, ok := s.rows[k]; ok {
		cp := *item
		cp.ID = old.ID
		s.rows[k] = &cp
		return false, nil
	}
	cp := *item
	cp.ID = s.nextID
	s.nextID++
	s.rows[k] = &cp
	return true, nil
}

func (s *entityRepoStub) Count(_ context.Context) (int, error) { return len(s.rows), nil }

// narrativeRepoStub 内存版 NarrativeMappingRepo（唯一键 narrative）。
type narrativeRepoStub struct {
	rows   map[string]*NarrativeItem
	nextID int64
}

func newNarrativeRepoStub() *narrativeRepoStub {
	return &narrativeRepoStub{rows: make(map[string]*NarrativeItem), nextID: 1}
}

func (s *narrativeRepoStub) Create(_ context.Context, item *NarrativeItem) (*NarrativeItem, error) {
	if _, dup := s.rows[item.Narrative]; dup {
		return nil, errors.New("duplicate key")
	}
	cp := *item
	cp.ID = s.nextID
	s.nextID++
	s.rows[item.Narrative] = &cp
	return &cp, nil
}

func (s *narrativeRepoStub) Update(_ context.Context, id int64, item *NarrativeItem) (*NarrativeItem, error) {
	for k, r := range s.rows {
		if r.ID == id {
			cp := *item
			cp.ID = id
			delete(s.rows, k)
			s.rows[item.Narrative] = &cp
			return &cp, nil
		}
	}
	return nil, errors.New("not found")
}

func (s *narrativeRepoStub) Delete(_ context.Context, id int64) error {
	for k, r := range s.rows {
		if r.ID == id {
			delete(s.rows, k)
			return nil
		}
	}
	return errors.New("not found")
}

func (s *narrativeRepoStub) Get(_ context.Context, id int64) (*NarrativeItem, error) {
	for _, r := range s.rows {
		if r.ID == id {
			cp := *r
			return &cp, nil
		}
	}
	return nil, nil
}

func (s *narrativeRepoStub) List(_ context.Context, q NarrativeQuery) ([]*NarrativeItem, int, error) {
	out := make([]*NarrativeItem, 0, len(s.rows))
	for _, r := range s.rows {
		if q.EnabledOnly && !r.Enabled {
			continue
		}
		cp := *r
		out = append(out, &cp)
	}
	return out, len(out), nil
}

func (s *narrativeRepoStub) ListEnabled(ctx context.Context) ([]*NarrativeItem, error) {
	rows, _, err := s.List(ctx, NarrativeQuery{EnabledOnly: true})
	return rows, err
}

func (s *narrativeRepoStub) UpsertByKey(_ context.Context, item *NarrativeItem) (bool, error) {
	if old, ok := s.rows[item.Narrative]; ok {
		cp := *item
		cp.ID = old.ID
		s.rows[item.Narrative] = &cp
		return false, nil
	}
	cp := *item
	cp.ID = s.nextID
	s.nextID++
	s.rows[item.Narrative] = &cp
	return true, nil
}

func (s *narrativeRepoStub) Count(_ context.Context) (int, error) { return len(s.rows), nil }

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
