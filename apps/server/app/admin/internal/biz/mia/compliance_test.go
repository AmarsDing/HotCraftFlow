package mia

import (
	"context"
	"errors"
	"testing"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// =============================================================================
// 🔴 合规测试（详细设计 §7：合规校验路径覆盖率 100%）
// ① disclaimer 为空 → 拒绝落库；② path 为空 → 拒绝落库；
// ③ confidence 越界 → 拒绝落库；④ 传导项 confidence >0.6 → 拒绝落库。
// =============================================================================

// validCardForStore 构造一张应通过硬校验的卡（直接命中 + 传导项）。
func validCardForStore() *ImpactCardRow {
	return &ImpactCardRow{
		ImpactID:   "im_test01",
		EventID:    "ec_test01",
		Title:      "美联储降息 50bp",
		Disclaimer: mediacontract.ImpactDisclaimer,
		Affected: []mediacontract.AffectedItem{
			{
				Symbol:     "SPY",
				AssetClass: mediacontract.AssetIndex,
				Direction:  mediacontract.DirectionBullish,
				Confidence: 0.8,
				Path:       []string{"央行降息", "流动性宽松预期抬升权益估值"},
			},
			{
				Symbol:         "XAUUSD",
				AssetClass:     mediacontract.AssetCommodity,
				Direction:      mediacontract.DirectionBullish,
				Confidence:     0.55,
				Path:           []string{"央行降息", "实际利率下行假设"},
				IsTransmission: true,
				RuleID:         "rule_test",
			},
		},
	}
}

func TestValidateForStore_DisclaimerEmpty(t *testing.T) {
	card := validCardForStore()
	card.Disclaimer = "   "
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrDisclaimerEmpty) {
		t.Fatalf("空 disclaimer 必须拒绝落库（MIA_DISCLAIMER_EMPTY），got %v", err)
	}
}

func TestValidateForStore_PathEmpty(t *testing.T) {
	// path 整个为空。
	card := validCardForStore()
	card.Affected[0].Path = nil
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrPathEmpty) {
		t.Fatalf("affected.path 为空必须拒绝落库（MIA_PATH_EMPTY），got %v", err)
	}
	// path 元素为空白串同样不允许（防空壳路径）。
	card = validCardForStore()
	card.Affected[0].Path = []string{"央行降息", "  "}
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrPathEmpty) {
		t.Fatalf("path 元素空白必须拒绝落库，got %v", err)
	}
	// 传导项 path 空同样拒绝。
	card = validCardForStore()
	card.Affected[1].Path = nil
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrPathEmpty) {
		t.Fatalf("传导项 path 为空必须拒绝落库，got %v", err)
	}
}

func TestValidateForStore_ConfidenceOutOfRange(t *testing.T) {
	for _, bad := range []float64{-0.01, 1.01, -1, 2} {
		card := validCardForStore()
		card.Affected[0].Confidence = bad
		if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrConfidenceInvalid) {
			t.Fatalf("confidence=%v 越界必须拒绝落库（MIA_CONFIDENCE_INVALID），got %v", bad, err)
		}
	}
}

func TestValidateForStore_TransmissionConfidenceCapped(t *testing.T) {
	// 🔴 假设非因果：传导项 confidence >0.6 拒绝落库（标 0.9 是自欺欺人）。
	card := validCardForStore()
	card.Affected[1].Confidence = 0.61
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrConfidenceInvalid) {
		t.Fatalf("传导项 confidence=0.61 必须拒绝落库，got %v", err)
	}
	// 恰好 0.6 是上限边界，允许。
	card = validCardForStore()
	card.Affected[1].Confidence = 0.6
	if err := ValidateImpactCardForStore(card); err != nil {
		t.Fatalf("传导项 confidence=0.60 在边界内应通过，got %v", err)
	}
	// 直接命中 0.9 合法（封顶只针对传导项）。
	card = validCardForStore()
	card.Affected[0].Confidence = 0.9
	if err := ValidateImpactCardForStore(card); err != nil {
		t.Fatalf("直接命中 confidence=0.9 应通过，got %v", err)
	}
}

func TestValidateForStore_InvalidEnums(t *testing.T) {
	card := validCardForStore()
	card.Affected[0].Direction = "sideways"
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrConfidenceInvalid) {
		t.Fatalf("非法 direction 必须拒绝落库，got %v", err)
	}
	card = validCardForStore()
	card.Affected[0].AssetClass = "bond"
	if err := ValidateImpactCardForStore(card); !errors.Is(err, ErrConfidenceInvalid) {
		t.Fatalf("非法 asset_class 必须拒绝落库，got %v", err)
	}
}

func TestValidateForStore_ValidCardPasses(t *testing.T) {
	if err := ValidateImpactCardForStore(validCardForStore()); err != nil {
		t.Fatalf("合规卡应通过三项硬校验，got %v", err)
	}
}

// =============================================================================
// 🔴 传导规则诚意底线（详细设计 §2.5）：note 必填 + confidence ∈(0,0.6]
// =============================================================================

func validRuleRow() *TransmissionRuleRow {
	return &TransmissionRuleRow{
		ToAsset:     "XAUUSD",
		ToDirection: mediacontract.DirectionBearish,
		Confidence:  0.55,
		Note:        "美元指数走强 → 黄金计价下压（计价效应假设）",
		Enabled:     true,
	}
}

func TestValidateTransmissionRule_NoteRequired(t *testing.T) {
	row := validRuleRow()
	row.Note = "  "
	if err := validateTransmissionRule(row); !errors.Is(err, ErrPathEmpty) {
		t.Fatalf("note 必填（无理由的规则不可信），got %v", err)
	}
}

func TestValidateTransmissionRule_ConfidenceRange(t *testing.T) {
	for _, bad := range []float64{0, -0.1, 0.6001, 0.9, 1} {
		row := validRuleRow()
		row.Confidence = bad
		if err := validateTransmissionRule(row); !errors.Is(err, ErrTransmissionConfidenceTooHigh) {
			t.Fatalf("confidence=%v 应拒绝（须在 (0,0.6]，假设非因果），got %v", bad, err)
		}
	}
	row := validRuleRow()
	row.Confidence = 0.6 // 上限边界允许
	if err := validateTransmissionRule(row); err != nil {
		t.Fatalf("confidence=0.6 边界合法，got %v", err)
	}
}

func TestRuleUsecaseCreate_EnforcesCompliance(t *testing.T) {
	repo := newRuleRepoStub()
	uc := NewRuleUsecase(repo, log.DefaultLogger)
	// 高置信传导规则在 Create 入口即被拒。
	bad := validRuleRow()
	bad.Confidence = 0.9
	if _, err := uc.Create(context.Background(), bad); !errors.Is(err, ErrTransmissionConfidenceTooHigh) {
		t.Fatalf("Create 必须在入口拒绝 confidence>0.6 的规则，got %v", err)
	}
	// 合法规则创建成功且分配 rule_ 前缀 ID。
	ok, err := uc.Create(context.Background(), validRuleRow())
	if err != nil {
		t.Fatalf("合法规则应创建成功，got %v", err)
	}
	if ok.RuleID == "" {
		t.Fatalf("应分配 rule_ 前缀 ID")
	}
}

// TestSeedDefaultRules_Idempotent 表空播种、非空跳过；6 条规则全部过诚意校验。
func TestSeedDefaultRules_Idempotent(t *testing.T) {
	repo := newRuleRepoStub()
	ctx := context.Background()
	if err := SeedDefaultRules(ctx, repo, log.DefaultLogger); err != nil {
		t.Fatalf("首次播种应成功，got %v", err)
	}
	if repo.CountMust() != 6 {
		t.Fatalf("应播种 6 条默认规则，got %d", repo.CountMust())
	}
	// 每条默认规则必须满足诚意校验（confidence ≤0.6、note 非空）。
	rows, err := repo.List(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if verr := validateTransmissionRule(r); verr != nil {
			t.Fatalf("默认规则 %s 未过诚意校验: %v", r.RuleID, verr)
		}
	}
	// 二次播种跳过（幂等）。
	if err := SeedDefaultRules(ctx, repo, log.DefaultLogger); err != nil {
		t.Fatalf("二次播种应幂等跳过，got %v", err)
	}
	if repo.CountMust() != 6 {
		t.Fatalf("二次播种不应新增，got %d", repo.CountMust())
	}
}

// ---------- 测试替身 ----------

// ruleRepoStub 内存版 TransmissionRuleRepo。
type ruleRepoStub struct {
	rows   []*TransmissionRuleRow
	nextID int64
}

func newRuleRepoStub() *ruleRepoStub { return &ruleRepoStub{nextID: 1} }

func (s *ruleRepoStub) Create(_ context.Context, row *TransmissionRuleRow) (*TransmissionRuleRow, error) {
	cp := *row
	cp.ID = s.nextID
	s.nextID++
	s.rows = append(s.rows, &cp)
	return &cp, nil
}

func (s *ruleRepoStub) Update(_ context.Context, id int64, row *TransmissionRuleRow) (*TransmissionRuleRow, error) {
	for i, r := range s.rows {
		if r.ID == id {
			cp := *row
			cp.ID = id
			s.rows[i] = &cp
			return &cp, nil
		}
	}
	return nil, errors.New("not found")
}

func (s *ruleRepoStub) Delete(_ context.Context, id int64) error {
	for i, r := range s.rows {
		if r.ID == id {
			s.rows = append(s.rows[:i], s.rows[i+1:]...)
			return nil
		}
	}
	return errors.New("not found")
}

func (s *ruleRepoStub) List(_ context.Context, enabledOnly bool) ([]*TransmissionRuleRow, error) {
	out := make([]*TransmissionRuleRow, 0, len(s.rows))
	for _, r := range s.rows {
		if enabledOnly && !r.Enabled {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}

func (s *ruleRepoStub) Count(_ context.Context) (int, error) { return len(s.rows), nil }

func (s *ruleRepoStub) CountMust() int { n, _ := s.Count(context.Background()); return n }
