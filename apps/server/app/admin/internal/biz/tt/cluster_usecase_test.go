package tt

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---- fakes（专用；不依赖 tt_test.go 的 fakeTopicRepo，以便捕获 topicID）----

type fakeClusterRepo struct {
	cands     []ClusterCandidateRow
	err       error
	lastQuery CandidateQuery
}

func (f *fakeClusterRepo) FindClusterCandidates(_ context.Context, q CandidateQuery) ([]ClusterCandidateRow, error) {
	f.lastQuery = q
	return f.cands, f.err
}

type attachCall struct {
	topicID string
	in      AttachSourceInput
}

type fakeClusterTopics struct {
	attached []attachCall
	err      error
}

func (f *fakeClusterTopics) FindTopicIDByClusterKey(context.Context, string) (string, bool, error) {
	return "", false, nil
}
func (f *fakeClusterTopics) CreateTopicWithSource(context.Context, CreateTopicInput) (string, error) {
	return "", nil
}
func (f *fakeClusterTopics) AttachSource(_ context.Context, topicID string, in AttachSourceInput) error {
	f.attached = append(f.attached, attachCall{topicID, in})
	return f.err
}
func (f *fakeClusterTopics) TouchSeen(context.Context, string, time.Time) error { return nil }
func (f *fakeClusterTopics) GetTopicHeatMeta(context.Context, string, time.Time) (TopicHeatMeta, error) {
	return TopicHeatMeta{}, nil
}
func (f *fakeClusterTopics) UpdateTopicHeatState(context.Context, string, TopicHeatUpdate) error {
	return nil
}

type fakeReviewStore struct {
	marked []string
	err    error
}

func (f *fakeReviewStore) MarkNeedsReview(_ context.Context, topicID string) error {
	f.marked = append(f.marked, topicID)
	return f.err
}

type fakeEmbedder struct {
	vecs  map[string][]float32
	err   error
	calls int
}

func (f *fakeEmbedder) EmbedItem(_ context.Context, id string) ([]float32, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if v, ok := f.vecs[id]; ok {
		return v, nil
	}
	return nil, nil // 未注册 id = 无向量（视同通道缺失）
}

// ---- helpers ----

const refIntelID = "it_01JTEST00000000000000000"

func fixedTime() time.Time { return time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) }

type clusterFixture struct {
	recall *fakeClusterRepo
	topics *fakeClusterTopics
	review *fakeReviewStore
	emb    *fakeEmbedder
}

func newFixture(emb *fakeEmbedder) (*ClusterUsecase, *clusterFixture) {
	fx := &clusterFixture{
		recall: &fakeClusterRepo{},
		topics: &fakeClusterTopics{},
		review: &fakeReviewStore{},
		emb:    emb,
	}
	var embPort Embedder
	if emb != nil {
		embPort = emb
	}
	uc, err := NewClusterUsecase(fx.recall, fx.topics, fx.review, embPort, DefaultClusterConf(), DefaultClusterConfExtra())
	if err != nil {
		panic(err) // Default 配置必须过 Validate 红线
	}
	return uc, fx
}

func candRow(topicID string, fp uint64, ents []string) ClusterCandidateRow {
	return ClusterCandidateRow{TopicID: topicID, Lang: "zh", Fp: fp, Entities: ents}
}

func baseClusterInput(fp uint64, ents []string) ClusterInput {
	return ClusterInput{
		IntelID: refIntelID, ClusterKey: "cl_01JX", Platform: "weibo",
		Entities: ents, Lang: "zh", Fp: fp, CollectedAt: fixedTime(),
	}
}

// ---- tests ----

// TestResolveQueryShape Step2 查询参数形态：±7d 双向窗、上限 200、四段拆分、排除源。
func TestResolveQueryShape(t *testing.T) {
	uc, fx := newFixture(nil)
	fp := TitleSimHash64("标题")
	in := baseClusterInput(fp, []string{"person:甲"})
	in.ExcludeID = "it_01JSELF"
	if _, err := uc.Resolve(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	q := fx.recall.lastQuery
	if q.Limit != 200 {
		t.Fatalf("召回上限应为 200: %d", q.Limit)
	}
	wantFrom, wantTo := fixedTime().Add(-7*24*time.Hour), fixedTime().Add(7*24*time.Hour)
	if !q.From.Equal(wantFrom) || !q.To.Equal(wantTo) {
		t.Fatalf("时间窗应 ±7d: %v ~ %v", q.From, q.To)
	}
	if q.ExcludeID != "it_01JSELF" {
		t.Fatalf("应排除源条目: %q", q.ExcludeID)
	}
	if q.Segments != SimHashSegments(fp) {
		t.Fatal("四段应由输入指纹拆分")
	}
}

// TestResolveNoCandidates 零候选 → new 且不写库。
func TestResolveNoCandidates(t *testing.T) {
	uc, fx := newFixture(nil)
	out, err := uc.Resolve(context.Background(), baseClusterInput(0, nil))
	if err != nil || out.Decision != ClusterNew || out.TopicID != "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(fx.topics.attached) != 0 || len(fx.review.marked) != 0 {
		t.Fatal("new 决策不得落位/置标")
	}
}

// TestResolveMergeBand 高相似 merge → AttachSource(similarity=s)，不置 review。
func TestResolveMergeBand(t *testing.T) {
	fp := TitleSimHash64("新能源汽车补贴政策延续至明年")
	ents := []string{"topic:新能源", "org:工信部"}
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{candRow("tp_01JA", fp, ents)}

	out, err := uc.Resolve(context.Background(), baseClusterInput(fp, ents))
	if err != nil || out.Decision != ClusterMerge || out.TopicID != "tp_01JA" {
		t.Fatalf("merge 分支: out=%+v err=%v", out, err)
	}
	if len(fx.topics.attached) != 1 {
		t.Fatalf("merge 应 AttachSource: %+v", fx.topics.attached)
	}
	got := fx.topics.attached[0]
	if got.topicID != "tp_01JA" || got.in.IntelID != refIntelID || got.in.ClusterKey != "cl_01JX" ||
		got.in.Platform != "weibo" || !got.in.ContributedAt.Equal(fixedTime()) {
		t.Fatalf("attach 参数: %+v", got)
	}
	if got.in.Similarity < 0.999 {
		t.Fatalf("全同相似度应=1: %v", got.in.Similarity)
	}
	if len(fx.review.marked) != 0 {
		t.Fatal("merge 档不得置 needs_review")
	}
}

// TestResolveReviewBand 中带 → AttachSource + needs_review=true。
// s = (0.45·(3/5) + 0.25·(1−6/64))/0.70 = (0.27+0.2265625)/0.70 = 0.709375 ∈ [0.70,0.82)。
func TestResolveReviewBand(t *testing.T) {
	fp := TitleSimHash64("央行宣布降准释放流动性")
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{
		candRow("tp_01JB", fp^0b111111, []string{"a", "b", "c", "x"}),
	}
	out, err := uc.Resolve(context.Background(), baseClusterInput(fp, []string{"a", "b", "c", "d"}))
	if err != nil {
		t.Fatal(err)
	}
	if out.Decision != ClusterReview {
		t.Fatalf("应为 review(s≈0.709): %+v", out)
	}
	if len(fx.topics.attached) != 1 || fx.topics.attached[0].topicID != "tp_01JB" {
		t.Fatalf("review 档也应归入: %+v", fx.topics.attached)
	}
	if len(fx.review.marked) != 1 || fx.review.marked[0] != "tp_01JB" {
		t.Fatalf("review 档须置 needs_review: %+v", fx.review.marked)
	}
}

// TestResolveBelowReviewNew s<0.70 → new 且不写库。
func TestResolveBelowReviewNew(t *testing.T) {
	fp := TitleSimHash64("新能源汽车补贴政策延续")
	fpLow := fp ^ 0xFF // 翻 8 位 → sim=0.875
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{candRow("tp_01JC", fpLow, []string{"z1", "z2"})}
	// s = (0.45·0 + 0.25·0.875)/0.70 ≈ 0.3125 → new
	out, err := uc.Resolve(context.Background(), baseClusterInput(fp, []string{"a"}))
	if err != nil || out.Decision != ClusterNew || out.TopicID != "" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if len(fx.topics.attached) != 0 {
		t.Fatal("低相似不得落位")
	}
	if out.Scanned != 1 {
		t.Fatalf("Scanned=1: %d", out.Scanned)
	}
	if out.Score < 0.31 || out.Score > 0.32 {
		t.Fatalf("Score 应≈0.3125: %v", out.Score)
	}
}

// TestResolveArgmax 多候选 argmax 取最高而非先见；同话题多行不干扰。
func TestResolveArgmax(t *testing.T) {
	fp := TitleSimHash64("标题甲")
	ents := []string{"e1", "e2"}
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{
		candRow("tp_01JA", fp^(uint64(1)<<40), []string{"e1"}), // 弱：J=0.5, sim=63/64
		candRow("tp_01JB", fp, ents),                           // 强：全同
		candRow("tp_01JB", fp, []string{"e1"}),                 // 同话题的另一来源行
	}
	out, err := uc.Resolve(context.Background(), baseClusterInput(fp, ents))
	if err != nil || out.TopicID != "tp_01JB" {
		t.Fatalf("应选 argmax 的 tp_01JB: %+v", out)
	}
	if out.Scanned != 3 {
		t.Fatalf("Scanned=3: %d", out.Scanned)
	}
}

// TestResolveCrossLangGate 跨语言无 embedding → 恒 new；cos≥0.72 → merge。
func TestResolveCrossLangGate(t *testing.T) {
	fp := TitleSimHash64("Oil price surges again")
	ents := []string{"topic:oil"}
	row := candRow("tp_01JEN", fp, ents)
	row.Lang = "en"

	// 无 embedding → new
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{row}
	out, _ := uc.Resolve(context.Background(), baseClusterInput(fp, ents))
	if out.Decision != ClusterNew {
		t.Fatalf("跨语言无 embedding 应 new: %+v", out)
	}

	in := baseClusterInput(fp, ents)
	in.Embedding = []float32{1, 0}

	// cos=0 → new
	emb := &fakeEmbedder{vecs: map[string][]float32{"tp_01JEN": {0, 1}}}
	uc2, fx2 := newFixture(emb)
	fx2.recall.cands = []ClusterCandidateRow{row}
	out, _ = uc2.Resolve(context.Background(), in)
	if out.Decision != ClusterNew {
		t.Fatalf("cos=0 应 new: %+v", out)
	}

	// cos=0.8 ≥0.72 → merge
	emb.vecs = map[string][]float32{"tp_01JEN": {0.8, 0.6}}
	uc3, fx3 := newFixture(emb)
	fx3.recall.cands = []ClusterCandidateRow{row}
	out, _ = uc3.Resolve(context.Background(), in)
	if out.Decision != ClusterMerge || out.TopicID != "tp_01JEN" {
		t.Fatalf("cos=0.8 应 merge: %+v", out)
	}
}

// TestResolveErrors 召回/落位/置标/embedding 故障各自上抛。
func TestResolveErrors(t *testing.T) {
	uc, fx := newFixture(nil)
	fx.recall.err = errors.New("recall down")
	if _, err := uc.Resolve(context.Background(), baseClusterInput(0, nil)); err == nil {
		t.Fatal("召回故障应上抛")
	}

	fp := TitleSimHash64("标题")
	ents := []string{"e1"}
	uc2, fx2 := newFixture(nil)
	fx2.recall.cands = []ClusterCandidateRow{candRow("tp_01JE", fp, ents)}
	fx2.topics.err = errors.New("attach down")
	if _, err := uc2.Resolve(context.Background(), baseClusterInput(fp, ents)); err == nil {
		t.Fatal("落位故障应上抛")
	}

	uc3, fx3 := newFixture(nil)
	fx3.review.err = errors.New("review store down")
	// review 档构造同 TestResolveReviewBand（J=3/5, S=1-6/64 → s≈0.709）
	fx3.recall.cands = []ClusterCandidateRow{candRow("tp_01JF", fp^0b111111, []string{"a", "b", "c", "x"})}
	if _, err := uc3.Resolve(context.Background(), baseClusterInput(fp, []string{"a", "b", "c", "d"})); err == nil {
		t.Fatal("置标故障应上抛")
	}

	emb := &fakeEmbedder{err: errors.New("embedder down")}
	uc4, fx4 := newFixture(emb)
	fx4.recall.cands = []ClusterCandidateRow{candRow("tp_01JG", fp, ents)}
	in := baseClusterInput(fp, ents)
	in.Embedding = []float32{1}
	if _, err := uc4.Resolve(context.Background(), in); err == nil {
		t.Fatal("embedder 故障应上抛")
	}
}

// TestCosine32 余弦语义：正交 0、同向 1、零向量 0、长度不齐 0。
func TestCosine32(t *testing.T) {
	if got := cosine32([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Fatalf("正交应 0: %v", got)
	}
	if got := cosine32([]float32{1, 2}, []float32{2, 4}); got < 0.9999 {
		t.Fatalf("同向应≈1: %v", got)
	}
	if got := cosine32([]float32{0, 0}, []float32{1, 1}); got != 0 {
		t.Fatalf("零向量应 0: %v", got)
	}
	if got := cosine32([]float32{1}, []float32{1, 1}); got != 0 {
		t.Fatalf("长度不齐应 0: %v", got)
	}
}

// TestCosFromPair 对子级通道判定：空向量/长度不齐/零向量一律无效，可用对返回 cos。
func TestCosFromPair(t *testing.T) {
	if _, ok := cosFromPair(nil, []float32{1}); ok {
		t.Fatal("空 a 应无效")
	}
	if _, ok := cosFromPair([]float32{1}, nil); ok {
		t.Fatal("空 b 应无效")
	}
	if _, ok := cosFromPair([]float32{1, 2}, []float32{1}); ok {
		t.Fatal("长度不齐应无效")
	}
	if _, ok := cosFromPair([]float32{0, 0}, []float32{1, 2}); ok {
		t.Fatal("零向量应无效")
	}
	got, ok := cosFromPair([]float32{1, 0}, []float32{1, 0})
	if !ok || got < 0.9999 {
		t.Fatalf("同向应有效且≈1: %v %v", got, ok)
	}
}

// TestResolveDegradedEmbedding 评审对抗裁决发现：空/不齐向量曾 cos≡0 仍占 0.30
// 权重，使 merge 上限 0.70 永不可达。修复后按「对」降级重归一：
// 空 embedding 输入不再触发 embedder 调用，强实体+强指纹仍可 merge；
// 候选向量长度不齐时该对同样降级。
func TestResolveDegradedEmbedding(t *testing.T) {
	fp := TitleSimHash64("新能源汽车补贴政策延续至明年")
	ents := []string{"topic:新能源", "org:工信部"}

	// 场景1：in.Embedding 为空切片（非 nil 的坑）→ 不调用 embedder，重归一后 merge
	emb := &fakeEmbedder{vecs: map[string][]float32{"tp_01JA": {1, 0}}}
	uc, fx := newFixture(emb)
	fx.recall.cands = []ClusterCandidateRow{candRow("tp_01JA", fp, ents)}
	in := baseClusterInput(fp, ents)
	in.Embedding = []float32{} // 空切片 != nil：曾触发通道占用
	out, err := uc.Resolve(context.Background(), in)
	if err != nil || out.Decision != ClusterMerge {
		t.Fatalf("空 embedding 应按无通道重归一: %+v err=%v", out, err)
	}
	if emb.calls != 0 {
		t.Fatalf("空 embedding 不应调用 embedder: %d", emb.calls)
	}

	// 场景2：候选向量长度不齐 → 该对降级，仍可由 J/S 双满分进 merge
	emb2 := &fakeEmbedder{vecs: map[string][]float32{"tp_01JA": {1}}} // dim=1 ≠ 2
	uc2, fx2 := newFixture(emb2)
	fx2.recall.cands = []ClusterCandidateRow{candRow("tp_01JA", fp, ents)}
	in.Embedding = []float32{1, 0}
	out, err = uc2.Resolve(context.Background(), in)
	if err != nil || out.Decision != ClusterMerge {
		t.Fatalf("维度不齐应降级重归一: %+v err=%v", out, err)
	}

	// 场景3：候选无向量（未注册）→ 同场景2
	emb3 := &fakeEmbedder{vecs: map[string][]float32{}} // tp_01JA 无向量
	uc3, fx3 := newFixture(emb3)
	fx3.recall.cands = []ClusterCandidateRow{candRow("tp_01JA", fp, ents)}
	out, err = uc3.Resolve(context.Background(), baseClusterInput(fp, ents))
	if err != nil || out.Decision != ClusterMerge {
		t.Fatalf("无向量候选应降级重归一: %+v err=%v", out, err)
	}
}

// TestResolveEmbeddingByTopic Hook 模式：经 EmbeddingByTopic 取话题向量（跨语言/缓存
// 接线路径），与 EmbedItem 端口等价有效。
func TestResolveEmbeddingByTopic(t *testing.T) {
	fp := TitleSimHash64("某国央行加息周期开启")
	ents := []string{"topic:央行"}
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{
		{TopicID: "tp_01JEN", Lang: "en", Fp: fp, Entities: ents}, // 候选自身也是该话题的代表行
	}
	in := baseClusterInput(fp, ents)
	in.Lang = "zh"
	in.Embedding = []float32{1, 0}
	in.EmbeddingByTopic = func(_ context.Context, topicID string) ([]float32, error) {
		if topicID != "tp_01JEN" {
			t.Errorf("unexpected topic: %s", topicID)
		}
		return []float32{0.8, 0.6}, nil // cos=0.8 ≥0.72 → merge
	}
	out, err := uc.Resolve(context.Background(), in)
	if err != nil || out.Decision != ClusterMerge || out.TopicID != "tp_01JEN" {
		t.Fatalf("EmbeddingByTopic 应驱动跨语言 merge: %+v err=%v", out, err)
	}
}

// TestNewClusterUsecaseValidate 构造函数红线：零值/非法配置拒绝构造。
func TestNewClusterUsecaseValidate(t *testing.T) {
	ok := DefaultClusterConf()
	if _, err := NewClusterUsecase(&fakeClusterRepo{}, &fakeClusterTopics{}, &fakeReviewStore{}, nil, ok, DefaultClusterConfExtra()); err != nil {
		t.Fatalf("默认配置须构造成功: %v", err)
	}
	if _, err := NewClusterUsecase(&fakeClusterRepo{}, &fakeClusterTopics{}, &fakeReviewStore{}, nil, ClusterConf{}, DefaultClusterConfExtra()); err == nil {
		t.Fatal("零值 ClusterConf 必须拒绝")
	}
	if _, err := NewClusterUsecase(&fakeClusterRepo{}, &fakeClusterTopics{}, &fakeReviewStore{}, nil, ok, ClusterConfExtra{}); err == nil {
		t.Fatal("零值 ClusterConfExtra 必须拒绝")
	}
}

// TestResolveReviewBandAttachFirst 顺序契约：先归入后置标（置标失败时来源已存，
// 重试幂等不会丢关联——AttachSource (topic_id,intel_id) 幂等）。
func TestResolveReviewBandAttachFirst(t *testing.T) {
	fp := TitleSimHash64("某品牌发布新一代旗舰芯片")
	uc, fx := newFixture(nil)
	fx.recall.cands = []ClusterCandidateRow{candRow("tp_01JH", fp^0b111111, []string{"a", "b", "c", "x"})}
	fx.review.err = errors.New("review down")
	out, err := uc.Resolve(context.Background(), baseClusterInput(fp, []string{"a", "b", "c", "d"}))
	if err == nil {
		t.Fatal("应返回置标错误")
	}
	if len(fx.topics.attached) != 1 {
		t.Fatalf("置标失败不得回滚已归入的关联: %+v", fx.topics.attached)
	}
	if out.Decision != ClusterReview || out.TopicID != "tp_01JH" {
		t.Fatalf("out 仍应携带决策: %+v", out)
	}
}
