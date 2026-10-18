package tt

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// ============================================================================
// A-14 快照时间戳规则 + ingest 全链路单测（M1 第 9 项）。
// RedisStore 传 nil 走降级放行分支；ports 用内存 fake。
// ============================================================================

func mustTime(t *testing.T, s string) time.Time {
	t.Helper()
	tm, err := time.Parse(time.RFC3339, s)
	if err != nil {
		t.Fatalf("parse time: %v", err)
	}
	return tm.UTC()
}

func TestResolveSampleTime(t *testing.T) {
	now := mustTime(t, "2026-10-18T12:00:00Z")
	cfg := DefaultConf().Sampling // LateHours=24, FutureSkewMinutes=5

	asOf := mustTime(t, "2026-10-18T10:22:00Z")
	collected := mustTime(t, "2026-10-18T11:00:00Z")

	t.Run("as_of 优先于 collected_at", func(t *testing.T) {
		it := &mediacontract.IntelItem{}
		it.Metrics.AsOf = &asOf
		it.Source.CollectedAt = collected
		got, kind := ResolveSampleTime(it, now, cfg)
		if kind != TimeOK || !got.Equal(asOf) {
			t.Fatalf("want (as_of, TimeOK), got (%v, %v)", got, kind)
		}
	})

	t.Run("无 as_of 时 collected_at 兜底", func(t *testing.T) {
		it := &mediacontract.IntelItem{}
		it.Source.CollectedAt = collected
		got, kind := ResolveSampleTime(it, now, cfg)
		if kind != TimeOK || !got.Equal(collected) {
			t.Fatalf("want (collected_at, TimeOK), got (%v, %v)", got, kind)
		}
	})

	t.Run("两个都没有 → TimeMissing（禁止 now 兜底）", func(t *testing.T) {
		it := &mediacontract.IntelItem{}
		got, kind := ResolveSampleTime(it, now, cfg)
		if kind != TimeMissing || !got.IsZero() {
			t.Fatalf("want (zero, TimeMissing), got (%v, %v)", got, kind)
		}
	})

	t.Run("超前 future_skew → TimeFuture 丢弃", func(t *testing.T) {
		fut := now.Add(10 * time.Minute)
		it := &mediacontract.IntelItem{}
		it.Metrics.AsOf = &fut
		_, kind := ResolveSampleTime(it, now, cfg)
		if kind != TimeFuture {
			t.Fatalf("want TimeFuture, got %v", kind)
		}
	})

	t.Run("超前但未越线 → TimeOK", func(t *testing.T) {
		fut := now.Add(4 * time.Minute)
		it := &mediacontract.IntelItem{}
		it.Metrics.AsOf = &fut
		_, kind := ResolveSampleTime(it, now, cfg)
		if kind != TimeOK {
			t.Fatalf("want TimeOK, got %v", kind)
		}
	})

	t.Run("落后 late_hours → TimeLate 写入", func(t *testing.T) {
		late := now.Add(-25 * time.Hour)
		it := &mediacontract.IntelItem{}
		it.Metrics.AsOf = &late
		got, kind := ResolveSampleTime(it, now, cfg)
		if kind != TimeLate || !got.Equal(late) {
			t.Fatalf("want TimeLate, got (%v, %v)", got, kind)
		}
	})

	t.Run("时区归一：+08:00 输入转 UTC", func(t *testing.T) {
		loc := time.FixedZone("CST", 8*3600)
		as := time.Date(2026, 10, 18, 20, 0, 0, 0, loc) // = 12:00:00Z
		it := &mediacontract.IntelItem{}
		it.Metrics.AsOf = &as
		got, kind := ResolveSampleTime(it, now.Add(1*time.Hour), cfg)
		if kind != TimeOK || got.Location() != time.UTC || !got.Equal(as.UTC()) {
			t.Fatalf("want UTC normalized, got (%v @ %v, %v)", got, got.Location(), kind)
		}
	})
}

func TestAlignToBucket(t *testing.T) {
	t.Run("同桶对齐（S-25 跨平台）", func(t *testing.T) {
		a := mustTime(t, "2026-10-18T10:05:00Z")
		b := mustTime(t, "2026-10-18T10:22:00Z")
		want := mustTime(t, "2026-10-18T10:00:00Z")
		if got := AlignToBucket(a, 30); !got.Equal(want) {
			t.Fatalf("10:05 → %v, want %v", got, want)
		}
		if got := AlignToBucket(b, 30); !got.Equal(want) {
			t.Fatalf("10:22 → %v, want %v", got, want)
		}
	})

	t.Run("非 UTC 输入转 UTC 再对齐", func(t *testing.T) {
		loc := time.FixedZone("CST", 8*3600)
		in := time.Date(2026, 10, 18, 18, 5, 0, 0, loc) // = 10:05:00Z
		got := AlignToBucket(in, 30)
		if got.Location() != time.UTC || !got.Equal(mustTime(t, "2026-10-18T10:00:00Z")) {
			t.Fatalf("got %v @ %v", got, got.Location())
		}
	})

	t.Run("bucket=0 回退 30 分钟", func(t *testing.T) {
		got := AlignToBucket(mustTime(t, "2026-10-18T10:05:00Z"), 0)
		if !got.Equal(mustTime(t, "2026-10-18T10:00:00Z")) {
			t.Fatalf("got %v", got)
		}
	})

	t.Run("秒纳秒清零", func(t *testing.T) {
		in := mustTime(t, "2026-10-18T10:05:59.999999999Z")
		got := AlignToBucket(in, 30)
		if got.Second() != 0 || got.Nanosecond() != 0 {
			t.Fatalf("bucket must zero sub-minute parts, got %v", got)
		}
	})
}

func TestBuildSnapshotPoint(t *testing.T) {
	cfg := DefaultConf().Sampling
	raw := mustTime(t, "2026-10-18T10:22:00Z")

	t.Run("哨兵映射：无榜/无曝光", func(t *testing.T) {
		it := &mediacontract.IntelItem{}
		it.Source.Platform = "weibo"
		p := BuildSnapshotPoint("tp_x", it, raw, TimeOK, cfg)
		if p.Rank != SentinelRank || p.RankTotal != SentinelRankTotal {
			t.Fatalf("rank sentinels: %d/%d", p.Rank, p.RankTotal)
		}
		if p.EngRate != SentinelEngRate {
			t.Fatalf("eng_rate sentinel: %v", p.EngRate)
		}
		if p.HeatScore != SentinelHeat || p.RankPct != SentinelFactor {
			t.Fatalf("M1 must write sentinel heat/factors, got heat=%v rank_pct=%v", p.HeatScore, p.RankPct)
		}
		if !p.TBucket.Equal(mustTime(t, "2026-10-18T10:00:00Z")) || !p.TRaw.Equal(raw) {
			t.Fatalf("t_bucket/t_raw mismatch: %v / %v", p.TBucket, p.TRaw)
		}
	})

	t.Run("eng_rate = (likes+comments)/views", func(t *testing.T) {
		it := &mediacontract.IntelItem{}
		it.Metrics.Views, it.Metrics.Likes, it.Metrics.Comments = 1000, 30, 20
		p := BuildSnapshotPoint("tp_x", it, raw, TimeOK, cfg)
		if p.EngRate != 0.05 {
			t.Fatalf("eng_rate: %v", p.EngRate)
		}
	})

	t.Run("late 标记透传", func(t *testing.T) {
		it := &mediacontract.IntelItem{}
		if p := BuildSnapshotPoint("tp_x", it, raw, TimeLate, cfg); !p.Late {
			t.Fatal("TimeLate must set Late=true")
		}
		if p := BuildSnapshotPoint("tp_x", it, raw, TimeOK, cfg); p.Late {
			t.Fatal("TimeOK must set Late=false")
		}
	})
}

// ---------- ingest usecase：fake ports ----------

type fakeTopicRepo struct {
	byCluster map[string]string
	created   []CreateTopicInput
	attached  []AttachSourceInput
	touched   []string
	nextTopic int
	err       error
}

func (f *fakeTopicRepo) FindTopicIDByClusterKey(_ context.Context, key string) (string, bool, error) {
	v, ok := f.byCluster[key]
	return v, ok, f.err
}

func (f *fakeTopicRepo) CreateTopicWithSource(_ context.Context, in CreateTopicInput) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	f.created = append(f.created, in)
	f.nextTopic++
	id := "tp_fake_" + string(rune('a'+f.nextTopic-1))
	if in.ClusterKey != "" {
		f.byCluster[in.ClusterKey] = id
	}
	return id, nil
}

func (f *fakeTopicRepo) AttachSource(_ context.Context, _ string, in AttachSourceInput) error {
	f.attached = append(f.attached, in)
	return nil
}

func (f *fakeTopicRepo) TouchSeen(_ context.Context, id string, _ time.Time) error {
	f.touched = append(f.touched, id)
	return nil
}

// M2 新增端口方法（M1 测试场景不走归一化，heat=nil 时不会触达）。
func (f *fakeTopicRepo) GetTopicHeatMeta(context.Context, string, time.Time) (TopicHeatMeta, error) {
	return TopicHeatMeta{}, nil
}

func (f *fakeTopicRepo) UpdateTopicHeatState(context.Context, string, TopicHeatUpdate) error {
	return nil
}

type fakeGuard struct {
	accepted bool
	err      error
	keys     []string
}

func (f *fakeGuard) TryAcceptSample(_ context.Context, key string, _ int64, _ time.Duration) (bool, error) {
	f.keys = append(f.keys, key)
	return f.accepted, f.err
}

type fakePublisher struct {
	points []SnapshotPoint
	err    error
}

func (f *fakePublisher) PublishSnapshot(_ context.Context, p SnapshotPoint, _ string) error {
	if f.err != nil {
		return f.err
	}
	f.points = append(f.points, p)
	return nil
}

func newValidItem(asOf time.Time) *mediacontract.IntelItem {
	it := &mediacontract.IntelItem{
		APIVersion: "1.0",
		IntelID:    mediacontract.NewID("it"),
		Category:   mediacontract.CategoryAHot,
	}
	it.Source.Platform = "weibo"
	it.Source.Region = mediacontract.RegionCN
	it.Source.CollectMethod = mediacontract.CollectHotlist
	it.Source.URL = "https://example.com/p/1"
	it.Source.CollectedAt = asOf
	it.Payload.Title = "测试话题"
	it.Payload.Lang = "zh"
	it.Metrics.AsOf = &asOf
	it.Metrics.Rank = 3
	it.Metrics.Views = 10000
	it.Metrics.Likes = 500
	it.Compliance.Gate = mediacontract.GatePass
	return it
}

func encodeEvent(t *testing.T, payload any) []byte {
	t.Helper()
	env, err := mediacontract.NewEnvelope("ic", mediacontract.SubjectIntelNormalized, "trace-1", payload)
	if err != nil {
		t.Fatalf("new envelope: %v", err)
	}
	b, err := env.Encode()
	if err != nil {
		t.Fatalf("encode envelope: %v", err)
	}
	return b
}

func TestIngestHandle(t *testing.T) {
	now := mustTime(t, "2026-10-18T12:00:00Z")
	conf := DefaultConf()

	setup := func() (*IngestUsecase, *fakeTopicRepo, *fakeGuard, *fakePublisher) {
		repo := &fakeTopicRepo{byCluster: map[string]string{}}
		guard := &fakeGuard{accepted: true}
		pub := &fakePublisher{}
		uc := NewIngestUsecase(&conf, repo, guard, pub, nil, nil, log.DefaultLogger)
		uc.clock = func() time.Time { return now } // 固定时钟（同包测试）
		return uc, repo, guard, pub
	}

	t.Run("坏信封 → 毒消息 ACK 丢弃", func(t *testing.T) {
		uc, repo, _, pub := setup()
		res, err := uc.HandleIntelData(context.Background(), []byte("{not json"))
		if err != nil || res != ResultBadTime {
			t.Fatalf("got (%v, %v)", res, err)
		}
		if len(pub.points) != 0 || len(repo.created) != 0 {
			t.Fatal("poison must not touch repo/publisher")
		}
	})

	t.Run("契约校验失败 → 丢弃", func(t *testing.T) {
		uc, _, _, pub := setup()
		bad := newValidItem(now.Add(-time.Hour))
		bad.Category = "Z_unknown"
		res, err := uc.HandleIntelData(context.Background(), encodeEvent(t, bad))
		if err != nil || res != ResultBadTime || len(pub.points) != 0 {
			t.Fatalf("got (%v, %v, pub=%d)", res, err, len(pub.points))
		}
	})

	t.Run("合规 block → ResultGate", func(t *testing.T) {
		uc, repo, _, pub := setup()
		blocked := newValidItem(now.Add(-time.Hour))
		blocked.Compliance.Gate = mediacontract.GateBlock
		res, _ := uc.HandleIntelData(context.Background(), encodeEvent(t, blocked))
		if res != ResultGate || len(pub.points) != 0 || len(repo.created) != 0 {
			t.Fatalf("got res=%v pub=%d created=%d", res, len(pub.points), len(repo.created))
		}
	})

	t.Run("无时间戳 → ResultBadTime", func(t *testing.T) {
		uc, _, _, pub := setup()
		noTime := newValidItem(time.Time{})
		noTime.Metrics.AsOf = nil
		noTime.Source.CollectedAt = time.Time{}
		res, _ := uc.HandleIntelData(context.Background(), encodeEvent(t, noTime))
		if res != ResultBadTime || len(pub.points) != 0 {
			t.Fatalf("got res=%v pub=%d", res, len(pub.points))
		}
	})

	t.Run("未来时间戳 → 丢弃", func(t *testing.T) {
		uc, _, _, pub := setup()
		fut := newValidItem(now.Add(30 * time.Minute))
		res, _ := uc.HandleIntelData(context.Background(), encodeEvent(t, fut))
		if res != ResultBadTime || len(pub.points) != 0 {
			t.Fatalf("got res=%v pub=%d", res, len(pub.points))
		}
	})

	t.Run("正常路径：新建话题 + 发布快照", func(t *testing.T) {
		uc, repo, guard, pub := setup()
		item := newValidItem(now.Add(-1 * time.Hour))
		item.Dedup.ClusterID = mediacontract.NewID("cl")
		res, err := uc.HandleIntelData(context.Background(), encodeEvent(t, item))
		if err != nil || res != ResultOK {
			t.Fatalf("got (%v, %v)", res, err)
		}
		if len(repo.created) != 1 {
			t.Fatalf("expect 1 topic created, got %d", len(repo.created))
		}
		if repo.created[0].ClusterKey != item.Dedup.ClusterID {
			t.Fatalf("cluster_key not propagated: %q", repo.created[0].ClusterKey)
		}
		if len(pub.points) != 1 {
			t.Fatalf("expect 1 point published, got %d", len(pub.points))
		}
		p := pub.points[0]
		if p.Platform != "weibo" || p.EngRate != 0.05 || p.Late {
			t.Fatalf("point fields wrong: %+v", p)
		}
		if len(guard.keys) != 1 || guard.keys[0] != LastWinsKey(p.TopicID, p.Platform, p.TBucket) {
			t.Fatalf("guard key: %v", guard.keys)
		}
	})

	t.Run("cluster_key 命中 → 复用话题且 AttachSource", func(t *testing.T) {
		uc, repo, _, pub := setup()
		ck := mediacontract.NewID("cl")
		repo.byCluster[ck] = "tp_existing"
		item := newValidItem(now.Add(-1 * time.Hour))
		item.Dedup.ClusterID = ck
		res, err := uc.HandleIntelData(context.Background(), encodeEvent(t, item))
		if err != nil || res != ResultOK {
			t.Fatalf("got (%v, %v)", res, err)
		}
		if len(repo.created) != 0 {
			t.Fatalf("found topic must not create, created=%d", len(repo.created))
		}
		if len(repo.attached) != 1 || repo.attached[0].ClusterKey != ck {
			t.Fatalf("attach: %+v", repo.attached)
		}
		if len(pub.points) != 1 || pub.points[0].TopicID != "tp_existing" {
			t.Fatalf("point topic: %+v", pub.points)
		}
	})

	t.Run("last-wins 守卫拒绝 → ResultStale 不发布", func(t *testing.T) {
		uc, repo, guard, pub := setup()
		guard.accepted = false
		item := newValidItem(now.Add(-1 * time.Hour))
		res, _ := uc.HandleIntelData(context.Background(), encodeEvent(t, item))
		if res != ResultStale || len(pub.points) != 0 {
			t.Fatalf("got res=%v pub=%d", res, len(pub.points))
		}
		// 守卫在构点之前建话题是既有语义（Step1 归并先于守卫），允许 created>0。
		_ = repo
	})

	t.Run("守卫故障 fail-open：继续发布", func(t *testing.T) {
		uc, _, guard, pub := setup()
		guard.err = errors.New("redis down")
		item := newValidItem(now.Add(-1 * time.Hour))
		res, err := uc.HandleIntelData(context.Background(), encodeEvent(t, item))
		if err != nil || res != ResultOK || len(pub.points) != 1 {
			t.Fatalf("got (%v, %v, pub=%d)", res, err, len(pub.points))
		}
	})

	t.Run("发布失败 → ResultError（订阅器 NAK）", func(t *testing.T) {
		uc, _, _, pub := setup()
		pub.err = errors.New("nats down")
		item := newValidItem(now.Add(-1 * time.Hour))
		res, err := uc.HandleIntelData(context.Background(), encodeEvent(t, item))
		if res != ResultError || err == nil {
			t.Fatalf("got (%v, %v)", res, err)
		}
	})

	t.Run("迟到数据：写入 + late=true", func(t *testing.T) {
		uc, _, _, pub := setup()
		item := newValidItem(now.Add(-30 * time.Hour))
		res, err := uc.HandleIntelData(context.Background(), encodeEvent(t, item))
		if err != nil || res != ResultOK {
			t.Fatalf("got (%v, %v)", res, err)
		}
		if len(pub.points) != 1 || !pub.points[0].Late {
			t.Fatalf("late flag: %+v", pub.points)
		}
	})
}
