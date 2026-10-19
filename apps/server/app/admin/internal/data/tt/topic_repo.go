package ttdata

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/predicate"
	"framework/app/admin/internal/data/ent/topic"
	"framework/app/admin/internal/data/ent/topicsource"
	biztt "framework/app/admin/internal/biz/tt"
	"framework/pkg/mediacontract"
)

// EntTopicRepo biz/tt.TopicRepo 的 Ent 实现。
//
// 纪律：
//   - 表无跨表 FK edge（topic_id 为业务键，ADR-TT-02），关联为二级查询。
//   - FindTopicIDByClusterKey 只命中 merge_lock=false 且未归档的话题
//     （B-1 Step2 合并锁定语义：锁话题不得再自动吸附新来源）。
//   - TouchSeen 用条件 UPDATE（LastSeenAtLT / FirstSeenAtGT），并发乱序安全。
type EntTopicRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntTopicRepo 构造。
func NewEntTopicRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntTopicRepo {
	return &EntTopicRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/tt-topic-repo")),
	}
}

// FindTopicIDByClusterKey 按 IC cluster_id 查最近贡献来源，二级查话题过滤锁定/归档。
func (r *EntTopicRepo) FindTopicIDByClusterKey(ctx context.Context, clusterKey string) (string, bool, error) {
	// 一条 cluster_key 可能贡献过多个话题（历史合并/拆分残留），取最近贡献。
	src, err := r.cli.Client().TopicSource.Query().
		Where(topicsource.ClusterKeyEQ(clusterKey)).
		Order(ent.Desc(topicsource.FieldContributedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}

	t, err := r.cli.Client().Topic.Query().
		Where(topic.TopicID(src.TopicID)).
		Select(topic.FieldTopicID, topic.FieldMergeLock, topic.FieldIsArchived).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", false, nil
		}
		return "", false, err
	}
	if t.IsArchived || t.MergeLock {
		// B-1：锁定/归档话题不再自动吸附，按未命中处理（事件另立新话题）。
		return "", false, nil
	}
	return t.TopicID, true, nil
}

// CreateTopicWithSource 事务新建话题 + 首条来源（B-1 Step1 未命中路径）。
func (r *EntTopicRepo) CreateTopicWithSource(ctx context.Context, in biztt.CreateTopicInput) (string, error) {
	topicID := mediacontract.NewID("tp")

	err := runInTx(ctx, r.cli.Client(), func(tx *ent.Tx) error {
		create := tx.Topic.Create().
			SetTopicID(topicID).
			SetLabel(in.Label).
			SetStageSince(in.SeenAt).
			SetPlatformCount(1).
			SetPlatforms([]string{in.Platform}).
			SetFirstSeenAt(in.SeenAt).
			SetLastSeenAt(in.SeenAt).
			SetRepresentativeIntelID(in.IntelID).
			SetOrigin(map[string]interface{}{
				"url":      in.OriginURL,
				"platform": in.Platform,
				"intel_id": in.IntelID,
			})
		switch topic.Category(in.Category) {
		case topic.CategoryHot, topic.CategoryMaterial, topic.CategoryMarket, topic.CategoryCompliance:
			create.SetCategory(topic.Category(in.Category))
		}
		switch topic.Region(in.Region) {
		case topic.RegionCn, topic.RegionIntl, topic.RegionGlobal:
			create.SetRegion(topic.Region(in.Region))
		}
		if _, err := create.Save(ctx); err != nil {
			return err
		}

		sc := tx.TopicSource.Create().
			SetTopicID(topicID).
			SetIntelID(in.IntelID).
			SetPlatform(in.Platform).
			SetContributedAt(in.SeenAt).
			SetSimilarity(1.0)
		if in.ClusterKey != "" {
			sc.SetClusterKey(in.ClusterKey)
		}
		// B-1 Step2：SimHash 分段 + 完整指纹（召回倒排键 + 打分通道）
		if in.TitleSimhash != 0 {
			segs := biztt.SimHashSegments(in.TitleSimhash)
			sc.SetTitleSimhash(in.TitleSimhash).
				SetSimhashSeg0(segs[0]).
				SetSimhashSeg1(segs[1]).
				SetSimhashSeg2(segs[2]).
				SetSimhashSeg3(segs[3])
		}
		_, err := sc.Save(ctx)
		return err
	})
	if err != nil {
		return "", err
	}
	return topicID, nil
}

// AttachSource 追加来源关联。同 (topic_id, intel_id) 唯一约束命中视为幂等成功。
func (r *EntTopicRepo) AttachSource(ctx context.Context, topicID string, in biztt.AttachSourceInput) error {
	create := r.cli.Client().TopicSource.Create().
		SetTopicID(topicID).
		SetIntelID(in.IntelID).
		SetPlatform(in.Platform).
		SetContributedAt(in.ContributedAt).
		SetSimilarity(in.Similarity)
	if in.ClusterKey != "" {
		create.SetClusterKey(in.ClusterKey)
	}
	// B-1 Step2：SimHash 分段 + 完整指纹（召回倒排键 + 打分通道）
	if in.TitleSimhash != 0 {
		segs := biztt.SimHashSegments(in.TitleSimhash)
		create.SetTitleSimhash(in.TitleSimhash).
			SetSimhashSeg0(segs[0]).
			SetSimhashSeg1(segs[1]).
			SetSimhashSeg2(segs[2]).
			SetSimhashSeg3(segs[3])
	}
	_, err := create.Save(ctx)
	if err != nil && ent.IsConstraintError(err) {
		return nil // 重复关联：幂等
	}
	return err
}

// TouchSeen 条件推进 first/last_seen_at（乱序安全：只前进 last、只后退 first）。
func (r *EntTopicRepo) TouchSeen(ctx context.Context, topicID string, seenAt time.Time) error {
	wh := []predicate.Topic{topic.TopicID(topicID)}

	if err := r.cli.Client().Topic.Update().
		Where(append(wh, topic.LastSeenAtLT(seenAt))...).
		SetLastSeenAt(seenAt).
		Exec(ctx); err != nil {
		return err
	}
	if err := r.cli.Client().Topic.Update().
		Where(append(wh, topic.FirstSeenAtGT(seenAt))...).
		SetFirstSeenAt(seenAt).
		Exec(ctx); err != nil {
		return err
	}
	return nil
}

// GetTopicHeatMeta normalize 取数（M2）：
//   - first_seen_at / category 直取 Topic 行；
//   - pc(t) = contributed_at ∈ (at-24h, at] 的 distinct platform（A-5/S-17 样本窗口，
//     B-4 历史可重现——TopicSource 只追加，重算迟到点得当时窗口）。
func (r *EntTopicRepo) GetTopicHeatMeta(ctx context.Context, topicID string, at time.Time) (biztt.TopicHeatMeta, error) {
	var out biztt.TopicHeatMeta

	t, err := r.cli.Client().Topic.Query().
		Where(topic.TopicID(topicID)).
		Select(topic.FieldFirstSeenAt, topic.FieldCategory).
		First(ctx)
	if err != nil {
		return out, err
	}
	out.FirstSeenAt = t.FirstSeenAt.UTC()
	out.Category = string(t.Category)

	// 样本窗口内 distinct 平台（GROUP BY platform 后聚合）。数据量级：
	// 单话题日来源数十条，索引 (topic_id, contributed_at) 覆盖范围扫描。
	platforms, err := r.cli.Client().TopicSource.Query().
		Where(
			topicsource.TopicID(topicID),
			topicsource.ContributedAtGT(at.UTC().Add(-24*time.Hour)),
			topicsource.ContributedAtLTE(at.UTC()),
		).
		GroupBy(topicsource.FieldPlatform).
		Strings(ctx)
	if err != nil {
		return out, err
	}
	out.Platforms24h = platforms
	out.PlatformCount24h = len(platforms)
	return out, nil
}

// UpdateTopicHeatState 回写热度/动态/因子明细（B-3 落库，NFR-TT-05 门禁）。
// 全量字段覆盖式 UPDATE（同字段并发写由调用方串行度兜底——权重 epoch 不变时
// 各点单调推进，偶发乱序只影响下个点自愈）。
func (r *EntTopicRepo) UpdateTopicHeatState(ctx context.Context, topicID string, in biztt.TopicHeatUpdate) error {
	missing := in.Missing
	if missing == nil {
		missing = []string{}
	}
	platforms := in.Platforms24h
	if platforms == nil {
		platforms = []string{}
	}
	return r.cli.Client().Topic.Update().
		Where(topic.TopicID(topicID)).
		SetHeatScore(in.HeatScore).
		SetVelocity(in.Velocity).
		SetAccel(in.Accel).
		SetRankPct(in.RankPct).
		SetEngPct(in.EngPct).
		SetFreshness(in.Freshness).
		SetBreadth(in.Breadth).
		SetWRankEff(in.WeightsEff.WRank).
		SetWEngEff(in.WeightsEff.WEng).
		SetWFreshEff(in.WeightsEff.WFresh).
		SetWBreadthEff(in.WeightsEff.WBreadth).
		SetMissingFactors(missing).
		SetWeightEpoch(in.WeightEpoch).
		SetPlatformCount(in.PlatformCount24h).
		SetPlatforms(platforms).
		Exec(ctx)
}

// runInTx 局部事务助手（避免 ttdata 依赖整个 data 包）。
func runInTx(ctx context.Context, client *ent.Client, fn func(tx *ent.Tx) error) error {
	tx, err := client.Tx(ctx)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

// 编译期接口校验。
var _ biztt.TopicRepo = (*EntTopicRepo)(nil)
