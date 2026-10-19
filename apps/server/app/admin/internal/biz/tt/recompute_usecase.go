package tt

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
)

// ============================================================================
// 迟到回填重算（评审 S-23 + A-14 第 3 条）。
//
// 触发：ingest 对 `_as_of` 落后 > late_hours 的样本写入后落 Redis 标记
//       `tt:recompute:{topic_id}`（含首迟到 epoch，窗口合并；7d TTL 自愈）。
// 动作：按话题逐平台从 InfluxDB 升序重读 [seed, t_now] 序列，自种子前一点
//       状态回放 EWMA 链，对每点重算 heat_score/velocity/accel（权重/因子按
//       样本时刻口径），WriteSnapshots 同 (series, t_bucket) 覆盖写（NFR-TT-07
//       幂等，重跑/交叠窗口收敛到同一结果）。
// 抑制：重算期间 tt_topics.recompute_lock 非空（生命周期扫描集排除；M4 告警
//       评估同键抑制——§4.3 挂账项的联动位）。
// 清理：标记 Del 后 ent 实体索引过期行清 7d 窗（Step2 召回卫生）。
//
// 已知边界（诚实口径）：乱序迟到点只修正速度链口径，不重排 t_raw 插入；
// 因子分位按样本时刻重取（B-4 历史可重现），eng 直方图不可回放——迟到点落
// 库后 ObserveAndPercentile 会二次 HINCRBY 同一日键（HINCRBY 幂等性不在），
// 故重算不重复观测直方图，仅读分位（EngHistogram.ReadPercentile 走读路径）。
// ============================================================================

// RecomputeRepo 重算所需的 PG 端口（data/tt 实现）。
type RecomputeRepo interface {
	// SetRecomputeLock 置/清 recompute_lock（nil = 清除）。置锁为条件写：
	// 仅当当前无锁或锁已过期（崩溃自愈）才成功，返回 false = 他人持锁。
	SetRecomputeLock(ctx context.Context, topicID string, at *time.Time, staleBefore time.Time) (bool, error)
}

// EntityIndexCleaner 实体索引过期清理窄端口（S-23 重算轮顺带卫生任务）。
type EntityIndexCleaner interface {
	// CleanupExpiredEntities 清 indexed_at < before 的倒排行（Step2 召回 7d 窗对齐）。
	CleanupExpiredEntities(ctx context.Context, before time.Time) (int, error)
}

// RecomputeUsecase 单话题迟到回填重算编排器。
type RecomputeUsecase struct {
	conf    *Conf
	repo    RecomputeRepo
	topic   TopicRepo // category/first_seen 锚点（eng 日键与 freshness 因子）
	ts      TimeSeries
	dyn     DynamicsStateStore
	hist    EngHistogram
	weights WeightConfigRepo
	redis   biz.RedisStore
	cleaner EntityIndexCleaner // 实体索引过期清理（7d 窗）；可 nil
	log     *log.Helper
}

// NewRecomputeUsecase 构造（hist/cleaner 可 nil 降级：分位读跳过 / 索引清理跳过）。
func NewRecomputeUsecase(
	conf *Conf,
	repo RecomputeRepo,
	topic TopicRepo,
	ts TimeSeries,
	dyn DynamicsStateStore,
	hist EngHistogram,
	weights WeightConfigRepo,
	redis biz.RedisStore,
	cleaner EntityIndexCleaner,
	logger log.Logger,
) *RecomputeUsecase {
	return &RecomputeUsecase{
		conf:    conf,
		repo:    repo,
		topic:   topic,
		ts:      ts,
		dyn:     dyn,
		hist:    hist,
		weights: weights,
		redis:   redis,
		cleaner: cleaner,
		log:     log.NewHelper(log.With(logger, "module", "biz/tt-recompute")),
	}
}

// RecomputeOne 重算单话题。返回覆盖写点数；无标记/无点返回 0。
// seedHint 为 ingest 标记中的首迟到样本时间（unix 秒；0 = 无提示，回退 24h 窗）。
func (uc *RecomputeUsecase) RecomputeOne(ctx context.Context, topicID string, seedHint time.Time) (int, error) {
	now := time.Now().UTC()
	// 锁陈旧阈值：锁龄超 2× interval 视为崩溃残留，允许接管（自愈）。
	staleBefore := now.Add(-2 * uc.recomputeInterval())
	ok, err := uc.repo.SetRecomputeLock(ctx, topicID, &now, staleBefore)
	if err != nil {
		return 0, fmt.Errorf("set recompute lock: %w", err)
	}
	if !ok {
		RecomputeTotal.WithLabelValues("locked_skip").Inc()
		return 0, nil // 他人持锁（并发 worker/上一轮未完），下轮再来
	}
	defer func() {
		if _, err := uc.repo.SetRecomputeLock(ctx, topicID, nil, time.Time{}); err != nil {
			uc.log.Warnf("clear recompute lock %s failed: %v", topicID, err)
		}
	}()

	// 重算窗口：[seedHint 或 now-24h, now]。A-14：>24h 的迟到已丢弃，
	// 故窗口右开 now 即可覆盖全部存活迟到点。
	from := seedHint
	if from.IsZero() || from.Before(now.Add(-24*time.Hour)) {
		from = now.Add(-24 * time.Hour)
	}

	platforms, err := uc.ts.ReadPlatforms(ctx, topicID, from, now)
	if err != nil {
		return 0, fmt.Errorf("read platforms: %w", err)
	}
	if len(platforms) == 0 {
		uc.log.Infof("recompute %s: no points in [%s, %s]", topicID, from.Format(time.RFC3339), now.Format(time.RFC3339))
		return 0, nil
	}

	// 话题锚点：category（eng 日键）+ first_seen（freshness 因子，A-4）。
	// 取数失败不阻断重算——category 空 → 直方图读跳过；first_seen 零值 → freshness
	// 自锚退化（heat 合成主导项为 rank/eng，偏差文档化接受）。
	meta, _ := uc.topic.GetTopicHeatMeta(ctx, topicID, now)

	total := 0
	for _, pf := range platforms {
		n, err := uc.recomputeSeries(ctx, topicID, pf, meta, from, now)
		if err != nil {
			RecomputeTotal.WithLabelValues("error").Inc()
			uc.log.Warnf("recompute %s/%s failed: %v", topicID, pf, err)
			continue // 单平台失败不阻断其余平台
		}
		total += n
	}
	if total > 0 {
		RecomputeTotal.WithLabelValues("ok").Inc()
		RecomputePointsTotal.Add(float64(total))
	}
	return total, nil
}

// recomputeSeries 单 (topic, platform) 序列回放。
// 种子状态 = 窗口前最后一个点的推进后状态（dyn store 若 last_t 在窗口内则无效，
// 回退 nil 首点 α=1——口径偏差收敛于窗口长度，文档化接受）。
func (uc *RecomputeUsecase) recomputeSeries(ctx context.Context, topicID, platform string, meta TopicHeatMeta, from, to time.Time) (int, error) {
	points, err := uc.ts.ReadSeriesAsc(ctx, topicID, platform, from, to)
	if err != nil {
		return 0, fmt.Errorf("read series: %w", err)
	}
	if len(points) == 0 {
		return 0, nil
	}

	// 种子：dyn store 当前态仅当其 last_t < 窗口起点时可用（在窗口内/后 = 已被污染，重置）。
	var prev *DynamicsState
	if uc.dyn != nil {
		if st, err := uc.dyn.Load(ctx, topicID, platform); err == nil && st != nil && st.LastT.Before(points[0].TBucket) {
			prev = st
		}
	}

	var lastState *DynamicsState
	for i := range points {
		p := &points[i]
		// 权重按样本时刻口径（A-15）；直查不读缓存（历史可重现，B-4）。
		weights, epoch := uc.weightsAt(ctx, p.TRaw)

		// 因子重算：rank_pct/freshness/breadth 按样本口径；eng_pct 只读分位
		// （不重复 Observe——迟到点已由 ingest 观测过日键，二次 HINCRBY 会双倍计数）。
		rankPct, rankValid := RankPct(p.Rank, p.RankTotal)
		engPct := p.EngPct
		if uc.hist != nil && meta.Category != "" {
			if pct, err := uc.hist.ReadPercentile(ctx, platform, meta.Category, p.TRaw, float64(p.Likes+p.Comments)); err == nil && pct >= 0 {
				engPct = pct
			}
		}
		// freshness（A-4）：first_seen 锚点由 PG 元数据给（与 ingest 口径一致）；
		// 锚点缺失时退化自锚（=floor，heat 合成主导项为 rank/eng，偏差文档化接受）。
		freshAnchor := meta.FirstSeenAt
		if freshAnchor.IsZero() {
			freshAnchor = p.TRaw
		}
		fresh := Freshness(p.TRaw, freshAnchor, uc.conf.Heat.FreshnessTauHours, uc.conf.Heat.FreshnessFloor)
		freshValid := !meta.FirstSeenAt.IsZero()
		breadth := p.Breadth // 保持原值（pc(t) 窗口重取代价高，迟到修正主目标是动态链）

		res, err := CombineHeat(weights, rankPct, engPct, fresh, breadth, rankValid, true, freshValid, true)
		if err != nil {
			if errors.Is(err, ErrNoFactor) {
				continue // A-6：全缺失不覆盖写（保留原行）
			}
			return 0, err
		}

		step := AdvanceDynamics(prev, res.Score, p.TBucket, epoch, uc.conf.Heat.Dynamics)
		if !step.Advanced {
			// 序列内乱序/同桶（读路径已按 t_bucket 升序，理论上仅同桶重复到）：
			// 记 0 跳过，状态不动——与 ingest 乱序口径一致。
			p.Velocity, p.Accel = 0, 0
			continue
		}
		prev = &step.Next
		lastState = &step.Next

		p.HeatScore = res.Score
		p.Velocity = step.Velocity
		p.Accel = step.Accel
		p.RankPct = res.RankPct
		p.EngPct = res.EngPct
		p.Freshness = res.Freshness
		// breadth 原地保留（见上）
		p.WRankEff = res.WeightsEff.WRank
		p.WEngEff = res.WeightsEff.WEng
		p.WFreshEff = res.WeightsEff.WFresh
		p.WBreadthEff = res.WeightsEff.WBreadth
		p.Missing = res.Missing
		p.WeightEpoch = epoch
	}

	// 覆盖写（同 (measurement, topic_id, platform, t_bucket) 幂等）。
	toWrite := make([]SnapshotPoint, 0, len(points))
	for i := range points {
		if points[i].HeatScore >= 0 { // 跳过全缺失点
			toWrite = append(toWrite, points[i])
		}
	}
	if err := uc.ts.WriteSnapshots(ctx, toWrite); err != nil {
		return 0, fmt.Errorf("overwrite snapshots: %w", err)
	}

	// 回写 dyn store 最新态（条件写：expected = 读时 last_t，并发 ingest 抢先则放弃——
	// 下个样本点自愈）。
	if lastState != nil && uc.dyn != nil {
		var expected time.Time
		if cur, err := uc.dyn.Load(ctx, topicID, platform); err == nil && cur != nil {
			expected = cur.LastT
			if !expected.Before(lastState.LastT) {
				// store 已推进到重算终点之后（ingest 新点抢先），不写旧态。
				return len(toWrite), nil
			}
		}
		if _, err := uc.dyn.SaveIfNotAdvanced(ctx, topicID, platform, *lastState, expected); err != nil {
			uc.log.Warnf("dyn save after recompute %s/%s failed: %v", topicID, platform, err)
		}
	}
	return len(toWrite), nil
}

// weightsAt 直查样本时刻生效权重（不走 enricher 缓存——重算须历史可重现）。
func (uc *RecomputeUsecase) weightsAt(ctx context.Context, t time.Time) (HeatWeights, string) {
	row, found, err := uc.weights.ActiveHeatWeights(ctx, t)
	if err != nil || !found {
		return DefaultHeatWeights(), DefaultWeightEpoch
	}
	return row.Weights, WeightEpochOf(row.ConfigID)
}

// recomputeInterval 锁陈旧阈值的 interval 来源（conf 未配时 15min）。
func (uc *RecomputeUsecase) recomputeInterval() time.Duration {
	if uc.conf != nil && uc.conf.Recompute.Interval > 0 {
		return uc.conf.Recompute.Interval
	}
	return 15 * time.Minute
}

// RecomputeScan 扫描一轮迟到标记（worker 周期调用）。
// 返回 (处理话题数, 错误)。
func (uc *RecomputeUsecase) RecomputeScan(ctx context.Context, limit int) (int, error) {
	if uc.redis == nil || !uc.redis.IsConfigured() {
		return 0, nil // 无 Redis：迟到标记本就没落，无活可干
	}
	keys, err := uc.redis.ScanKeys(ctx, "tt:recompute:*", limit)
	if err != nil {
		return 0, fmt.Errorf("scan recompute marks: %w", err)
	}
	processed := 0
	for _, key := range keys {
		topicID := strings.TrimPrefix(key, "tt:recompute:")
		if topicID == "" {
			continue
		}
		var seedHint time.Time
		if v, err := uc.redis.GetString(ctx, key); err == nil && v != "" && v != "1" {
			if sec, err := parseUnix(v); err == nil {
				seedHint = time.Unix(sec, 0).UTC()
			}
		}
		if _, err := uc.RecomputeOne(ctx, topicID, seedHint); err != nil {
			uc.log.Warnf("recompute %s failed (mark kept for retry): %v", topicID, err)
			continue // 标记保留，下轮重试
		}
		// 成功：清标记（已覆盖到 now；其间新迟到的标记会重新 SET NX）。
		if err := uc.redis.Del(ctx, key); err != nil {
			uc.log.Warnf("del recompute mark %s failed: %v", key, err)
		}
		processed++
	}

	// 实体索引过期清理（Step2 召回卫生：7d 窗对齐召回 from 边界）。
	if uc.cleaner != nil {
		cutoff := time.Now().UTC().Add(-7 * 24 * time.Hour)
		if dropped, err := uc.cleaner.CleanupExpiredEntities(ctx, cutoff); err != nil {
			uc.log.Warnf("cleanup expired entity index failed: %v", err)
		} else if dropped > 0 {
			uc.log.Infof("cleanup expired entity index: %d rows", dropped)
		}
	}
	return processed, nil
}

// parseUnix 解析 unix 秒字符串（strconv 包装便于测试替换）。
func parseUnix(s string) (int64, error) {
	var n int64
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

// sortPointsAsc 读路径升序保证（TimeSeries 实现侧已 ORDER BY，此处防御排序）。
func sortPointsAsc(points []SnapshotPoint) {
	sort.Slice(points, func(i, j int) bool { return points[i].TBucket.Before(points[j].TBucket) })
}
