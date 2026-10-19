package tt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// ---- fakes ----

type fakeIngestTopicRepo struct {
	findByClusterKeyFn func(clusterKey string) (string, bool, error)
	createTopicFn      func(in CreateTopicInput) (string, error)
	attachSourceFn     func(topicID string, in AttachSourceInput) error
	touchedIDs         []string
	heatMeta           TopicHeatMeta
	updatedHeatState   []string
}

func (f *fakeIngestTopicRepo) FindTopicIDByClusterKey(_ context.Context, clusterKey string) (string, bool, error) {
	return f.findByClusterKeyFn(clusterKey)
}
func (f *fakeIngestTopicRepo) CreateTopicWithSource(_ context.Context, in CreateTopicInput) (string, error) {
	return f.createTopicFn(in)
}
func (f *fakeIngestTopicRepo) AttachSource(_ context.Context, topicID string, in AttachSourceInput) error {
	return f.attachSourceFn(topicID, in)
}
func (f *fakeIngestTopicRepo) TouchSeen(_ context.Context, topicID string, _ time.Time) error {
	f.touchedIDs = append(f.touchedIDs, topicID)
	return nil
}
func (f *fakeIngestTopicRepo) GetTopicHeatMeta(_ context.Context, _ string, _ time.Time) (TopicHeatMeta, error) {
	return f.heatMeta, nil
}
func (f *fakeIngestTopicRepo) UpdateTopicHeatState(_ context.Context, topicID string, _ TopicHeatUpdate) error {
	f.updatedHeatState = append(f.updatedHeatState, topicID)
	return nil
}

type fakeIngestCluster struct {
	resolveFn func(in ClusterInput) (ClusterOutcome, error)
}

func (f *fakeIngestCluster) Resolve(_ context.Context, in ClusterInput) (ClusterOutcome, error) {
	return f.resolveFn(in)
}

// ---- assignTopic 集成测试 ----

// newTestIngestUsecase 构造带默认 logger 的 usecase（避免 nil log panic）。
func newTestIngestUsecase(repo TopicRepo, cluster ClusterResolver) *IngestUsecase {
	return &IngestUsecase{
		repo:    repo,
		cluster: cluster,
		clock:   time.Now,
		log:     log.NewHelper(log.DefaultLogger),
	}
}

// TestAssignTopic_Step1Hit 硬键命中路径：直接复用话题，不触发聚类。
func TestAssignTopic_Step1Hit(t *testing.T) {
	repo := &fakeIngestTopicRepo{
		findByClusterKeyFn: func(clusterKey string) (string, bool, error) {
			if clusterKey == "cl_abc123" {
				return "tp_existing", true, nil
			}
			return "", false, nil
		},
		attachSourceFn: func(topicID string, in AttachSourceInput) error {
			return nil
		},
		createTopicFn: func(in CreateTopicInput) (string, error) {
			t.Fatalf("should not create topic when cluster_key hits")
			return "", nil
		},
	}

	uc := newTestIngestUsecase(repo, nil)
	item := &mediacontract.IntelItem{
		IntelID: "it_001",
		Source:  mediacontract.IntelSource{Platform: "weibo"},
		Payload: mediacontract.IntelPayload{Title: "硬键命中的新闻"},
		Dedup:   mediacontract.IntelDedup{ClusterID: "cl_abc123"},
	}
	topicID, err := uc.assignTopic(context.Background(), item, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topicID != "tp_existing" {
		t.Errorf("expected tp_existing, got %s", topicID)
	}
}

// TestAssignTopic_Step1Miss_ClusterHit 硬键未命中 + 聚类命中：归入既有话题。
func TestAssignTopic_Step1Miss_ClusterHit(t *testing.T) {
	repo := &fakeIngestTopicRepo{
		findByClusterKeyFn: func(clusterKey string) (string, bool, error) {
			return "", false, nil // 硬键未命中
		},
		createTopicFn: func(in CreateTopicInput) (string, error) {
			t.Fatalf("should not create topic when cluster resolves to merge/review")
			return "", nil
		},
	}

	cluster := &fakeIngestCluster{
		resolveFn: func(in ClusterInput) (ClusterOutcome, error) {
			return ClusterOutcome{Decision: ClusterMerge, TopicID: "tp_clustered", Score: 0.85}, nil
		},
	}

	uc := newTestIngestUsecase(repo, cluster)
	item := &mediacontract.IntelItem{
		IntelID: "it_002",
		Source:  mediacontract.IntelSource{Platform: "weibo"},
		Payload: mediacontract.IntelPayload{Title: "某科技公司发布新品"},
		Dedup:   mediacontract.IntelDedup{ClusterID: ""}, // 无硬键
	}
	topicID, err := uc.assignTopic(context.Background(), item, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topicID != "tp_clustered" {
		t.Errorf("expected tp_clustered, got %s", topicID)
	}
}

// TestAssignTopic_Step1Miss_ClusterNew 硬键未命中 + 聚类未命中：新建话题。
func TestAssignTopic_Step1Miss_ClusterNew(t *testing.T) {
	repo := &fakeIngestTopicRepo{
		findByClusterKeyFn: func(clusterKey string) (string, bool, error) {
			return "", false, nil
		},
		createTopicFn: func(in CreateTopicInput) (string, error) {
			return "tp_new", nil
		},
	}

	cluster := &fakeIngestCluster{
		resolveFn: func(in ClusterInput) (ClusterOutcome, error) {
			return ClusterOutcome{Decision: ClusterNew}, nil
		},
	}

	uc := newTestIngestUsecase(repo, cluster)
	item := &mediacontract.IntelItem{
		IntelID: "it_003",
		Source:  mediacontract.IntelSource{Platform: "weibo"},
		Payload: mediacontract.IntelPayload{Title: "完全无关的另一条新闻"},
	}
	topicID, err := uc.assignTopic(context.Background(), item, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topicID != "tp_new" {
		t.Errorf("expected tp_new, got %s", topicID)
	}
}

// TestAssignTopic_ClusterError 聚类失败降级：不阻断主链，新建话题。
func TestAssignTopic_ClusterError(t *testing.T) {
	repo := &fakeIngestTopicRepo{
		findByClusterKeyFn: func(clusterKey string) (string, bool, error) {
			return "", false, nil
		},
		createTopicFn: func(in CreateTopicInput) (string, error) {
			return "tp_fallback", nil
		},
	}

	cluster := &fakeIngestCluster{
		resolveFn: func(in ClusterInput) (ClusterOutcome, error) {
			return ClusterOutcome{}, errors.New("cluster repo unavailable")
		},
	}

	uc := newTestIngestUsecase(repo, cluster)
	item := &mediacontract.IntelItem{
		IntelID: "it_004",
		Source:  mediacontract.IntelSource{Platform: "weibo"},
		Payload: mediacontract.IntelPayload{Title: "聚类服务故障时的兜底"},
	}
	topicID, err := uc.assignTopic(context.Background(), item, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if topicID != "tp_fallback" {
		t.Errorf("expected tp_fallback, got %s", topicID)
	}
}

// TestExtractEntityKeys 实体抽取：confidence 过滤 + (type:value) 格式化。
func TestExtractEntityKeys(t *testing.T) {
	item := &mediacontract.IntelItem{
		Entities: []mediacontract.Entity{
			{Type: "person", Value: "马云", Confidence: 0.9},
			{Type: "org", Value: "阿里巴巴", Confidence: 0.85},
			{Type: "place", Value: "杭州", Confidence: 0.5}, // <0.6 应被过滤
			{Type: "unknown", Value: "x", Confidence: 0.0},
		},
	}
	keys := extractEntityKeys(item)
	if len(keys) != 2 {
		t.Fatalf("expected 2 keys, got %d: %v", len(keys), keys)
	}
	if keys[0] != "person:马云" || keys[1] != "org:阿里巴巴" {
		t.Errorf("unexpected keys: %v", keys)
	}
}

// TestDetectLang 语言判定：CJK 字符 → zh；否则 → en。
func TestDetectLang(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"某科技公司发布新品", "zh"},
		{"Breaking: Tech Giant Acquires Startup", "en"},
		{"", "en"},
		{"中文English混合", "zh"},
	}
	for _, c := range cases {
		got := detectLang(c.title)
		if got != c.want {
			t.Errorf("detectLang(%q) = %s, want %s", c.title, got, c.want)
		}
	}
}
