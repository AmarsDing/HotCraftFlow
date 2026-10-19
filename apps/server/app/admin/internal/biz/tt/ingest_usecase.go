package tt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/pkg/mediacontract"
)

// HandleResult 消费一条 IntelItem 事件的结果（供订阅器决定 ACK 语义）。
type HandleResult int

const (
	// ResultOK 正常受理（快照已发布到 mm.trend.snapshot.v1）。
	ResultOK HandleResult = iota
	// ResultDedup event_id 幂等去重命中（JetStream 至少一次重投），ACK。
	ResultDedup
	// ResultGate 合规门禁阻断（defense-in-depth：normalized 流本不应含），ACK。
	ResultGate
	// ResultBadTime 时间戳不可用/未来时间，毒消息语义，Warn 后 ACK 丢弃（A-14）。
	ResultBadTime
	// ResultStale 同桶内 _as_of 未超已有值，丢弃（last-wins 守卫），ACK。
	ResultStale
	// ResultError 处理错误，须 NAK 让 JetStream 重投。
	ResultError
)

func (r HandleResult) String() string {
	switch r {
	case ResultOK:
		return "ok"
	case ResultDedup:
		return "dedup"
	case ResultGate:
		return "gate"
	case ResultBadTime:
		return "bad_time"
	case ResultStale:
		return "stale"
	case ResultError:
		return "error"
	}
	return "unknown"
}

// labelMaxLen 话题标签长度上限（IntelItem title 可能过长）。
const labelMaxLen = 200

// IngestUsecase 消费 mm.intel.normalized.v1 → 话题归并 + 时间戳规则 +
// 构点 → 发布 mm.trend.snapshot.v1（C-9：InfluxDB 写入只发生在 snapshot 消费端）。
//
// 幂等三层防线：
//  1. event_id Redis SET NX 去重（JetStream 至少一次重投，TTL DedupTTL）。
//  2. 同桶 last-wins 守卫（A-14：_as_of 最大者胜出，防乱序覆盖）。
//  3. InfluxDB (measurement+tag set+ts) 覆盖写（NFR-TT-07 兜底）。
// ClusterResolver 聚类决策端口（接口化以便测试注入 fake）。
type ClusterResolver interface {
	Resolve(ctx context.Context, in ClusterInput) (ClusterOutcome, error)
}

type IngestUsecase struct {
	conf    *Conf
	repo    TopicRepo
	guard   GuardStore
	pub     SnapshotPublisher
	redis   biz.RedisStore
	heat    *HeatEnricher   // M2 归一化编排器；nil 时保持 M1 哨兵行为（测试可注入）
	cluster ClusterResolver // M3 语义聚类编排器（Step2-4）；nil 时降级 Step1 miss→新建话题
	clock   func() time.Time // 可注入便于测试；默认 time.Now
	log     *log.Helper
}

// NewIngestUsecase 构造（redis 用于 event_id 去重与 recompute 标记；可为 nil 降级放行）。
// cluster 可 nil：未注入时 Step1 硬键未命中 → 直接新建话题（降级路径，不阻断主链）。
func NewIngestUsecase(
	conf *Conf,
	repo TopicRepo,
	guard GuardStore,
	pub SnapshotPublisher,
	redis biz.RedisStore,
	heat *HeatEnricher,
	cluster *ClusterUsecase,
	logger log.Logger,
) *IngestUsecase {
	var cr ClusterResolver
	if cluster != nil {
		cr = cluster
	}
	return &IngestUsecase{
		conf:    conf,
		repo:    repo,
		guard:   guard,
		pub:     pub,
		redis:   redis,
		heat:    heat,
		cluster: cr,
		clock:   time.Now,
		log:     log.NewHelper(log.With(logger, "module", "biz/tt-ingest")),
	}
}

// HandleIntelData 处理一条 mm.intel.normalized.v1 原始消息体。
//
// 返回错误 == ResultError（由订阅器 NAK）；其余结果一律返回 nil error（订阅器 ACK，
// 毒消息不重投，对齐 worker/jetstream_policy.go 口径）。
func (uc *IngestUsecase) HandleIntelData(ctx context.Context, data []byte) (HandleResult, error) {
	// 1) 信封解码（契约 §8.1：不得裸传业务载荷）。
	env, err := mediacontract.DecodeEnvelope(data)
	if err != nil {
		uc.log.Warnf("decode envelope failed (poison, ack & drop): %v", err)
		return ResultBadTime, nil // 格式毒消息：不重投
	}

	// 2) event_id 幂等去重。
	if uc.redis != nil && uc.redis.IsConfigured() && env.EventID != "" {
		_, got, err := uc.redis.TryLock(ctx, dedupKey(env.EventID), uc.conf.Ingest.DedupTTL)
		if err != nil {
			uc.log.Warnf("dedup check failed (fail-open): %v", err)
		} else if !got {
			IngestEventTotal.WithLabelValues("dedup").Inc()
			return ResultDedup, nil
		}
	}

	// 3) 载荷解码 + 契约校验。
	var item mediacontract.IntelItem
	if err := env.DecodePayload(&item); err != nil {
		uc.log.Warnf("decode IntelItem failed (poison, ack & drop): %v", err)
		IngestEventTotal.WithLabelValues("bad_payload").Inc()
		return ResultBadTime, nil
	}
	if err := item.Validate(); err != nil {
		uc.log.Warnf("IntelItem validate failed (poison, ack & drop): %v", err)
		IngestEventTotal.WithLabelValues("invalid").Inc()
		return ResultBadTime, nil
	}

	// 4) 合规门禁（defense-in-depth：blocked 条目不得进自动流程，契约 §1）。
	if item.BlocksAutoFlow() {
		IngestEventTotal.WithLabelValues("gate").Inc()
		return ResultGate, nil
	}

	// 5) 时间戳规则（A-14）。
	t, kind := ResolveSampleTime(&item, uc.clock(), uc.conf.Sampling)
	switch kind {
	case TimeMissing:
		IngestBadTimestampTotal.WithLabelValues("missing").Inc()
		uc.log.Warnf("intel %s 无 _as_of/collected_at，丢弃（禁止用 now 兜底）", item.IntelID)
		return ResultBadTime, nil
	case TimeFuture:
		IngestBadTimestampTotal.WithLabelValues("future").Inc()
		uc.log.Warnf("intel %s 时间戳超前 %v，丢弃（上游采集器时钟异常）", item.IntelID, t)
		return ResultBadTime, nil
	case TimeLate:
		IngestBadTimestampTotal.WithLabelValues("late").Inc()
	}

	// 6) 话题归并（B-1 Step1 硬键；M3 语义召回后置）。
	topicID, err := uc.assignTopic(ctx, &item, t)
	if err != nil {
		return ResultError, fmt.Errorf("assign topic: %w", err)
	}

	// 7) 构点 + 同桶 last-wins 守卫（A-14）。
	point := BuildSnapshotPoint(topicID, &item, t, kind, uc.conf.Sampling)
	if uc.guard != nil {
		accepted, err := uc.guard.TryAcceptSample(ctx,
			LastWinsKey(topicID, point.Platform, point.TBucket),
			point.TRaw.Unix(),
			uc.conf.Ingest.LastWinsGuardTTL)
		if err != nil {
			// 守卫故障不应阻断主链路（fail-open 让覆盖语义兜底）。
			uc.log.Warnf("last-wins guard failed (fail-open): %v", err)
		} else if !accepted {
			IngestStaleBucketTotal.Inc()
			return ResultStale, nil
		}
	}

	// 8) M2 归一化回填（B-3/C-15：因子取值、生效权重、weight_epoch、动态随点落盘）。
	enriched := false
	var heatMeta TopicHeatMeta
	if uc.heat != nil {
		meta, err := uc.heat.Enrich(ctx, &point)
		if err != nil {
			if errors.Is(err, ErrNoFactor) {
				// A-6：heat=null，不写 InfluxDB（避免 null 污染曲线），ACK 丢弃。
				uc.log.Warnf("topic %s 四因子全缺失，跳过发布（TT_HEAT_NO_FACTOR）", topicID)
				return ResultOK, nil
			}
			return ResultError, fmt.Errorf("heat enrich: %w", err)
		}
		heatMeta, enriched = meta, true
	}

	// 9) 迟到标记（Redis SET 首迟到样本时间为值；重算 worker 扫描消费，S-23/A-14）。
	if kind == TimeLate {
		uc.markRecompute(ctx, topicID, t)
	}

	// 10) 发布快照事件；失败 → ResultError → NAK 重投（不重发已去重 event_id，第 7 步守卫兜底幂等）。
	if err := uc.pub.PublishSnapshot(ctx, point, env.TraceID); err != nil {
		return ResultError, fmt.Errorf("publish snapshot: %w", err)
	}

	// 11) 推进话题活跃时间（样本时间口径，B-4）。
	if err := uc.repo.TouchSeen(ctx, topicID, t); err != nil {
		// 非致命：元数据滞后不影响时序写入，告警即可。
		uc.log.Warnf("touch topic %s seen_at failed: %v", topicID, err)
	}

	// 12) 回写话题热度状态（B-3：Topic 行冗余最新热度/因子/生效权重，门禁可查）。
	if enriched {
		if err := uc.repo.UpdateTopicHeatState(ctx, topicID, TopicHeatUpdate{
			HeatScore: point.HeatScore, Velocity: point.Velocity, Accel: point.Accel,
			RankPct: point.RankPct, EngPct: point.EngPct,
			Freshness: point.Freshness, Breadth: point.Breadth,
			WeightsEff: HeatWeights{
				WRank: point.WRankEff, WEng: point.WEngEff,
				WFresh: point.WFreshEff, WBreadth: point.WBreadthEff,
			},
			Missing:          point.Missing,
			WeightEpoch:      point.WeightEpoch,
			PlatformCount24h: heatMeta.PlatformCount24h,
			Platforms24h:     heatMeta.Platforms24h,
		}); err != nil {
			uc.log.Warnf("update topic %s heat state failed: %v", topicID, err)
		}
	}

	IngestEventTotal.WithLabelValues("ok").Inc()
	return ResultOK, nil
}

// assignTopic B-1 话题归并：
//   - Step1 硬键：dedup.cluster_id 命中 TopicSource.cluster_key → 复用话题；
//   - Step2-4 语义召回：硬键未命中且 cluster 可用 → ClusterUsecase.Resolve
//     （实体倒排 ∪ SimHash 桶召回 → 三路打分 → merge/review/new 决策）；
//   - 兜底：Step1/Step2-4 均未命中或聚类失败 → 新建话题（topic_id 由 TT 生成 tp_
//     前缀，ADR-TT-02 与 S-22 砍单口径：不直接复用 cl_ 作 topic_id）。
func (uc *IngestUsecase) assignTopic(ctx context.Context, item *mediacontract.IntelItem, seenAt time.Time) (string, error) {
	clusterKey := item.Dedup.ClusterID
	if clusterKey != "" {
		if topicID, found, err := uc.repo.FindTopicIDByClusterKey(ctx, clusterKey); err != nil {
			return "", fmt.Errorf("find by cluster_key: %w", err)
		} else if found {
			// 命中硬键：补记本条 intel 的归属（幂等）。
			if err := uc.repo.AttachSource(ctx, topicID, AttachSourceInput{
				IntelID:       item.IntelID,
				ClusterKey:    clusterKey,
				Platform:      item.Source.Platform,
				ContributedAt: seenAt,
				Similarity:    1.0,
				TitleSimhash:  TitleSimHash64(item.Payload.Title),
				Entities:      extractEntityKeys(item),
			}); err != nil {
				uc.log.Warnf("attach source intel=%s topic=%s failed: %v", item.IntelID, topicID, err)
			}
			return topicID, nil
		}
	}

	// Step2-4 语义召回（硬键未命中；M3 落地后启用）
	if uc.cluster != nil {
		outcome, err := uc.cluster.Resolve(ctx, ClusterInput{
			IntelID:     item.IntelID,
			ClusterKey:  clusterKey,
			Platform:    item.Source.Platform,
			Entities:    extractEntityKeys(item),
			Lang:        detectLang(item.Payload.Title),
			Fp:          TitleSimHash64(item.Payload.Title),
			CollectedAt: seenAt,
		})
		if err != nil {
			// 聚类失败不阻断主链：降级为新建话题（诚实口径：计 metric 供运维观测）
			uc.log.Warnf("cluster resolve failed (intel=%s), fallback to new topic: %v", item.IntelID, err)
			ClusterFallbackTotal.WithLabelValues("resolve_error").Inc()
		} else if outcome.Decision == ClusterMerge || outcome.Decision == ClusterReview {
			// 归并命中：ClusterUsecase 内部已 AttachSource + (review 档) MarkNeedsReview
			ClusterFallbackTotal.WithLabelValues("hit_" + string(outcome.Decision)).Inc()
			return outcome.TopicID, nil
		}
		// ClusterNew：继续走下方新建话题路径
	}

	// 新建话题（Step1 未命中 + Step2-4 未归并）。
	source := "no_cluster_key"
	if clusterKey != "" {
		source = "cluster_key"
	}
	in := CreateTopicInput{
		Label:      truncate(stripSpaces(item.Payload.Title), labelMaxLen),
		Category:   string(item.Category),
		Region:     string(item.Source.Region),
		SeenAt:     seenAt,
		IntelID:    item.IntelID,
		ClusterKey: clusterKey,
		OriginURL:  item.Source.URL,
		Platform:   item.Source.Platform,
		TitleSimhash: TitleSimHash64(item.Payload.Title),
		Entities:     extractEntityKeys(item),
		Lang:         detectLang(item.Payload.Title),
	}
	if in.Label == "" {
		in.Label = fallbackLabel(item)
	}
	topicID, err := uc.repo.CreateTopicWithSource(ctx, in)
	if err != nil {
		return "", fmt.Errorf("create topic: %w", err)
	}
	TopicCreatedTotal.WithLabelValues(source).Inc()
	return topicID, nil
}

// extractEntityKeys 从 IntelItem.Entities 抽取规范化实体串
// （confidence≥0.6 过滤 + (type:value) 格式化）。
func extractEntityKeys(item *mediacontract.IntelItem) []string {
	if item == nil || len(item.Entities) == 0 {
		return nil
	}
	out := make([]string, 0, len(item.Entities))
	for _, e := range item.Entities {
		if e.Confidence < 0.6 {
			continue // B-18 置信度下限
		}
		out = append(out, string(e.Type)+":"+e.Value)
	}
	return out
}

// detectLang 简易语言脚本判定（中文标题 vs 其他）。
// 完整版可引入 unicode script 检测；当前 zh/en 二分足够（Step3 跨语言判定用）。
func detectLang(title string) string {
	for _, r := range title {
		if r >= 0x4E00 && r <= 0x9FFF { // CJK Unified Ideographs
			return "zh"
		}
	}
	return "en"
}

// markRecompute 迟到回填重算标记（A-14：重算区间 [t_late, now] 内的派生指标）。
// 值为窗口种子：首迟到样本的 unix 秒（SET NX 语义由「已有值不覆盖」模拟——
// 已有标记说明该话题在排队，窗口起点取最早迟到点；覆盖会把窗口起点右移漏点）。
func (uc *IngestUsecase) markRecompute(ctx context.Context, topicID string, lateAt time.Time) {
	if uc.redis == nil || !uc.redis.IsConfigured() {
		return
	}
	key := RecomputeMarkKey(topicID)
	// 已有值 = 更早的迟到点排队中：仅在无值/占位 "1" 时写入种子（最早者胜出）。
	if cur, err := uc.redis.GetString(ctx, key); err == nil && cur != "" && cur != "1" {
		return
	}
	if err := uc.redis.SetStringEX(ctx, key, fmt.Sprintf("%d", lateAt.UTC().Unix()), 7*24*time.Hour); err != nil {
		uc.log.Warnf("mark recompute for %s failed: %v", topicID, err)
	}
}

func dedupKey(eventID string) string { return "tt:ingest:dedup:" + eventID }

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func stripSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// fallbackLabel 无标题时的兜底标签（平台 + intel_id 尾段，可辨识即可）。
func fallbackLabel(item *mediacontract.IntelItem) string {
	tail := item.IntelID
	if len(tail) > 8 {
		tail = tail[len(tail)-8:]
	}
	return fmt.Sprintf("[%s] %s", item.Source.Platform, tail)
}
