package tt

import (
	"math"
	"testing"
)

var cc = DefaultClusterConf()

// score 便捷包装：同语言全通道打分。
func score(j, s, cos float64) (float64, ClusterDecision) {
	return ScoreTopicPair(SimParts{EntityJaccard: j, SimHashSim: s, Cosine: cos, HasEmbedding: true, SameLang: true}, cc)
}

// TestClusterSpecAnchors 权重与三档阈值即评审终稿（防漂移锚定）。
func TestClusterSpecAnchors(t *testing.T) {
	if math.Abs(cc.WJaccard+cc.WSimHash+cc.WCos-1.0) > 1e-9 {
		t.Fatalf("Σw 必须=1: %v", cc)
	}
	if cc.WJaccard != 0.45 || cc.WSimHash != 0.25 || cc.WCos != 0.30 {
		t.Fatalf("权重终稿 0.45/0.25/0.30: %v", cc)
	}
	if cc.ThetaMerge != 0.82 || cc.ThetaReview != 0.70 || cc.CrossLangCosMin != 0.72 {
		t.Fatalf("阈值终稿 0.82/0.70/0.72: %v", cc)
	}
}

// TestClusterScoreFormula 合成公式逐值验算：s = 0.45J + 0.25S + 0.30C。
func TestClusterScoreFormula(t *testing.T) {
	s, _ := score(0.6, 0.9, 0.8)
	want := 0.45*0.6 + 0.25*0.9 + 0.30*0.8
	if math.Abs(s-want) > 1e-12 {
		t.Fatalf("s=%v want %v", s, want)
	}
	// 满分三路 → 1.0
	if s, d := score(1, 1, 1); math.Abs(s-1) > 1e-12 || d != ClusterMerge {
		t.Fatalf("全 1 应 merge 且 s=1: s=%v d=%s", s, d)
	}
	// 全零 → new
	if s, d := score(0, 0, 0); s != 0 || d != ClusterNew {
		t.Fatalf("全 0 应 new 且 s=0: s=%v d=%s", s, d)
	}
}

// TestClusterDecisionBands 三段边界：0.82/0.70 含端点，0.6999 ∉ review。
// 用单通道配置（WJaccard=1）让合成 s ≡ 输入 Jaccard，浮点上精确命中阈值——
// 三档语义只与比较符有关，与加权求和的舍入无关（公式另有 TestClusterScoreFormula）。
func TestClusterDecisionBands(t *testing.T) {
	bandConf := ClusterConf{WJaccard: 1, WSimHash: 0, WCos: 0, ThetaMerge: 0.82, ThetaReview: 0.70}
	cases := []struct {
		s    float64
		want ClusterDecision
	}{
		{0.95, ClusterMerge},
		{0.82, ClusterMerge},   // 下含端点
		{0.819, ClusterReview}, // 紧邻阈值之下
		{0.70, ClusterReview},  // review 下含端点
		{0.6999, ClusterNew},
		{0.0, ClusterNew},
	}
	for _, tc := range cases {
		_, d := ScoreTopicPair(SimParts{EntityJaccard: tc.s, HasEmbedding: false, SameLang: true}, bandConf)
		if d != tc.want {
			t.Fatalf("s=%v: d=%s want %s", tc.s, d, tc.want)
		}
	}
}

// TestClusterCrossLangNaN 对抗裁决发现：NaN cosine 曾漏过 cos>=0.72 门
// （NaN < t 为 false）→ 误判 merge。修复后 !(cos>=t) fail-closed。
func TestClusterCrossLangNaN(t *testing.T) {
	nan := math.NaN()
	if _, d := ScoreTopicPair(SimParts{Cosine: nan, HasEmbedding: true, SameLang: false}, cc); d != ClusterNew {
		t.Fatalf("NaN cosine 跨语言应 new: d=%s", d)
	}
	// 同语言下 NaN cos 会污染加权分——fail-open 溢出到 NaN，决策段也 fail-closed。
	s, d := ScoreTopicPair(SimParts{EntityJaccard: 1, SimHashSim: 1, Cosine: nan, HasEmbedding: true, SameLang: true}, cc)
	if !math.IsNaN(s) {
		t.Fatalf("NaN 传播可观测: s=%v", s)
	}
	if d != ClusterNew {
		t.Fatalf("NaN 分不得归并: d=%s", d)
	}
}

// TestClusterConfValidate 零值配置静默灾变防线（对抗裁决发现：ThetaMerge=0 全量归入）。
func TestClusterConfValidate(t *testing.T) {
	if err := DefaultClusterConf().Validate(); err != nil {
		t.Fatalf("默认配置须过红线: %v", err)
	}
	if err := (ClusterConf{}).Validate(); err == nil {
		t.Fatal("零值 ClusterConf 必须拒绝（ThetaMerge=0 全量归入灾变）")
	}
	bad := DefaultClusterConf()
	bad.WJaccard = 0.5 // Σw=1.05
	if err := bad.Validate(); err == nil {
		t.Fatal("Σw≠1 必须拒绝（打分漂移）")
	}
	bad2 := DefaultClusterConf()
	bad2.ThetaMerge = bad2.ThetaReview // 0.82→0.70
	if err := bad2.Validate(); err == nil {
		t.Fatal("theta_merge<=theta_review 必须拒绝")
	}
}

// TestClusterCrossLang 跨语言：实体/SimHash 无效，只看 cos≥0.72；不设 review 档。
func TestClusterCrossLang(t *testing.T) {
	// cos 达标 → merge（即便实体/SimHash 全零也不影响）
	s, d := ScoreTopicPair(SimParts{Cosine: 0.72, HasEmbedding: true, SameLang: false}, cc)
	if d != ClusterMerge {
		t.Fatalf("cos=0.72 应 merge: s=%v d=%s", s, d)
	}
	if s != 0.72 {
		t.Fatalf("跨语言返回分应为 cos 本体: %v", s)
	}
	// cos 差一丝 → new（无 review 档）
	if _, d := ScoreTopicPair(SimParts{Cosine: 0.719, HasEmbedding: true, SameLang: false}, cc); d != ClusterNew {
		t.Fatalf("cos=0.719 应 new: d=%s", d)
	}
	// 高实体/高 SimHash 不能救跨语言低 cos
	if _, d := ScoreTopicPair(SimParts{EntityJaccard: 1, SimHashSim: 1, Cosine: 0.5, HasEmbedding: true, SameLang: false}, cc); d != ClusterNew {
		t.Fatalf("跨语言实体/SimHash 无效: d=%s", d)
	}
	// 无 embedding → 恒 new（宁可多建也不乱并）
	if _, d := ScoreTopicPair(SimParts{EntityJaccard: 1, SimHashSim: 1, HasEmbedding: false, SameLang: false}, cc); d != ClusterNew {
		t.Fatalf("跨语言无 embedding 应恒 new: d=%s", d)
	}
}

// TestClusterNoEmbeddingRenorm 同语言缺 embedding：0.45/0.25 重归一（÷0.70）。
func TestClusterNoEmbeddingRenorm(t *testing.T) {
	s, _ := ScoreTopicPair(SimParts{EntityJaccard: 0.5, SimHashSim: 1, HasEmbedding: false, SameLang: true}, cc)
	want := (0.45*0.5 + 0.25*1) / 0.70
	if math.Abs(s-want) > 1e-12 {
		t.Fatalf("重归一 s=%v want %v", s, want)
	}
	// 两路全满 → 仍能抵达 merge
	if s, d := ScoreTopicPair(SimParts{EntityJaccard: 1, SimHashSim: 1, HasEmbedding: false, SameLang: true}, cc); d != ClusterMerge || math.Abs(s-1) > 1e-12 {
		t.Fatalf("无 embedding 两路满分应 merge: s=%v d=%s", s, d)
	}
	// 权重异常（全零）→ new，不 panic
	if _, d := ScoreTopicPair(SimParts{EntityJaccard: 1, SimHashSim: 1, HasEmbedding: false, SameLang: true},
		ClusterConf{ThetaMerge: 0.82, ThetaReview: 0.70}); d != ClusterNew {
		t.Fatalf("Σw=0 应 new: d=%s", d)
	}
}

// TestPickLabel M-19 全序：gate=pass 优先 → workability 降序 → 互动降序 → 时间升序。
func TestPickLabel(t *testing.T) {
	// 空候选
	if title, ok := PickLabel(nil); title != "" || ok {
		t.Fatalf("空候选应返回空: %q %v", title, ok)
	}
	// gate=pass 压过一切高分项
	cands := []LabelCandidate{
		{Title: "高分未过闸", GatePass: false, Workability: 99, Engagement: 9999, CollectedAt: 100},
		{Title: "过闸普通", GatePass: true, Workability: 1, Engagement: 1, CollectedAt: 200},
	}
	if title, ok := PickLabel(cands); title != "过闸普通" || !ok {
		t.Fatalf("gate=pass 优先失败: %q %v", title, ok)
	}
	// 同 gate：workability 降序
	cands = []LabelCandidate{
		{Title: "低可工作高互动", GatePass: true, Workability: 10, Engagement: 500, CollectedAt: 100},
		{Title: "高可工作", GatePass: true, Workability: 80, Engagement: 1, CollectedAt: 200},
	}
	if title, _ := PickLabel(cands); title != "高可工作" {
		t.Fatalf("workability 降序失败: %q", title)
	}
	// 同 workability：互动降序
	cands = []LabelCandidate{
		{Title: "低互动早", GatePass: true, Workability: 50, Engagement: 5, CollectedAt: 100},
		{Title: "高互动晚", GatePass: true, Workability: 50, Engagement: 50, CollectedAt: 300},
	}
	if title, _ := PickLabel(cands); title != "高互动晚" {
		t.Fatalf("互动降序失败: %q", title)
	}
	// 全平：collected_at 升序（最早者优先，稳定可复现）
	cands = []LabelCandidate{
		{Title: "晚", GatePass: true, Workability: 50, Engagement: 50, CollectedAt: 300},
		{Title: "早", GatePass: true, Workability: 50, Engagement: 50, CollectedAt: 100},
		{Title: "中", GatePass: true, Workability: 50, Engagement: 50, CollectedAt: 200},
	}
	if title, _ := PickLabel(cands); title != "早" {
		t.Fatalf("时间升序平局失败: %q", title)
	}
	// 未过闸组内部同样按全序
	cands = []LabelCandidate{
		{Title: "低", GatePass: false, Workability: 1, Engagement: 1, CollectedAt: 100},
		{Title: "高", GatePass: false, Workability: 9, Engagement: 1, CollectedAt: 200},
	}
	if title, ok := PickLabel(cands); title != "高" || ok {
		t.Fatalf("未过闸组全序失败: %q ok=%v（应 false）", title, ok)
	}
}
