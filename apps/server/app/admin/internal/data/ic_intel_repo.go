package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"

	"github.com/go-kratos/kratos/v2/log"

	icV1 "framework/api/gen/go/media/v1/ic"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/evidencesnapshot"
	"framework/app/admin/internal/data/ent/intelitem"
	contract "framework/pkg/mediacontract"
)

// IntelItemRepo 情报条目仓储（评审 B-6 幂等 / B-26 pigeonhole / B-25 重发）。
type IntelItemRepo struct {
	data *Data
	log  *log.Helper
}

func NewIntelItemRepo(data *Data, logger log.Logger) biz.IntelItemRepo {
	return &IntelItemRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/intel/repo/admin-service")),
		data: data,
	}
}

// icPageClamp 分页参数钳制（page≥1，1≤size≤200，默认20）。
func icPageClamp(page, size int32) (p, s int) {
	p, s = int(page), int(size)
	if p < 1 {
		p = 1
	}
	if s <= 0 {
		s = 20
	}
	if s > 200 {
		s = 200
	}
	return p, s
}

// ---------- 查询 ----------

func (r *IntelItemRepo) GetByID(ctx context.Context, id string) (*biz.IntelItemRow, error) {
	e, err := r.data.db.Client().IntelItem.Query().
		Where(intelitem.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query intel item failed (id=%s): %s", id, err.Error())
		return nil, icV1.ErrorIcSchemaInvalid("query intel item failed")
	}
	ev, _ := r.latestEvidence(ctx, id)
	return toBizIntelItem(e, ev), nil
}

func (r *IntelItemRepo) GetByDedupKey(ctx context.Context, dedupKey string) (*biz.IntelItemRow, error) {
	e, err := r.data.db.Client().IntelItem.Query().
		Where(intelitem.DedupKeyEQ(dedupKey)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query intel item by dedup_key failed: %s", err.Error())
		return nil, icV1.ErrorIcSchemaInvalid("query intel item failed")
	}
	return toBizIntelItem(e, nil), nil
}

// FindRecentByURLKey L1 去重：窗口内同 url_key 命中即合并（03 §4.1）。
func (r *IntelItemRepo) FindRecentByURLKey(ctx context.Context, urlKey string, window time.Duration) (*biz.IntelItemRow, error) {
	since := time.Now().UTC().Add(-window)
	e, err := r.data.db.Client().IntelItem.Query().
		Where(
			intelitem.URLKeyNotNil(),
			intelitem.URLKeyEQ(urlKey),
			intelitem.CollectedAtGT(since),
		).
		Order(ent.Desc(intelitem.FieldCollectedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query intel item by url_key failed: %s", err.Error())
		return nil, icV1.ErrorIcSchemaInvalid("query intel item failed")
	}
	return toBizIntelItem(e, nil), nil
}

// FindSimHashCandidates B-26：四段任一等值即候选；精确汉明在 Go 侧计算。
func (r *IntelItemRepo) FindSimHashCandidates(ctx context.Context, segs [4]int16, window time.Duration, limit int) ([]*biz.IntelItemRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	since := time.Now().UTC().Add(-window)
	entities, err := r.data.db.Client().IntelItem.Query().
		Where(
			intelitem.FingerprintValid(true),
			intelitem.CollectedAtGT(since),
			intelitem.Or(
				intelitem.FpB0EQ(segs[0]),
				intelitem.FpB1EQ(segs[1]),
				intelitem.FpB2EQ(segs[2]),
				intelitem.FpB3EQ(segs[3]),
			),
		).
		Order(ent.Desc(intelitem.FieldCollectedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("query simhash candidates failed: %s", err.Error())
		return nil, icV1.ErrorIcSchemaInvalid("query simhash candidates failed")
	}
	return toBizIntelItems(entities), nil
}

func (r *IntelItemRepo) List(ctx context.Context, q *biz.IntelListQuery) ([]*biz.IntelItemRow, int, error) {
	if q == nil {
		return nil, 0, icV1.ErrorIcConfigInvalid("empty query")
	}
	p, s := icPageClamp(q.Page, q.PageSize)

	predicates := make([]func(*sql.Selector), 0, 10)
	if q.Platform != "" {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.EQ(intelitem.FieldPlatform, q.Platform))
		})
	}
	if q.Region != "" {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.EQ(intelitem.FieldRegion, q.Region))
		})
	}
	if q.Category != "" {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.EQ(intelitem.FieldCategory, q.Category))
		})
	}
	if q.ClusterID != "" {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.EQ(intelitem.FieldClusterID, q.ClusterID))
		})
	}
	if q.CollectMethod != "" {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.EQ(intelitem.FieldCollectMethod, q.CollectMethod))
		})
	}
	if q.Gate != "" {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.EQ(intelitem.FieldComplianceGate, q.Gate))
		})
	}
	if q.Keyword != "" {
		kw := "%" + q.Keyword + "%"
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.Or(
				sql.Like(intelitem.FieldTitle, kw),
				sql.Like(intelitem.FieldText, kw), // V1 全文降级 ILIKE（评审 B-19）
			))
		})
	}
	if !q.CollectedFrom.IsZero() {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.GTE(intelitem.FieldCollectedAt, q.CollectedFrom.UTC()))
		})
	}
	if !q.CollectedTo.IsZero() {
		predicates = append(predicates, func(sel *sql.Selector) {
			sel.Where(sql.LTE(intelitem.FieldCollectedAt, q.CollectedTo.UTC()))
		})
	}

	countQuery := r.data.db.Client().IntelItem.Query()
	listQuery := r.data.db.Client().IntelItem.Query()
	if len(predicates) > 0 {
		countQuery.Modify(predicates...)
		listQuery.Modify(predicates...)
	}
	// 实体过滤：JSONB 包含匹配（精确 value 相等，大小写敏感；语义正确优于关键字 ILIKE）
	if q.Entity != "" {
		entity := q.Entity
		contains := func(sel *sql.Selector) {
			sel.Where(sqljson.ValueContains(intelitem.FieldEntities,
				[]map[string]interface{}{{"value": entity}}))
		}
		countQuery.Modify(contains)
		listQuery.Modify(contains)
	}

	total, err := countQuery.Count(ctx)
	if err != nil {
		r.log.Errorf("count intel items failed: %s", err.Error())
		return nil, 0, icV1.ErrorIcSchemaInvalid("count intel items failed")
	}
	entities, err := listQuery.
		Order(ent.Desc(intelitem.FieldCollectedAt)).
		Offset((p - 1) * s).
		Limit(s).
		All(ctx)
	if err != nil {
		r.log.Errorf("list intel items failed: %s", err.Error())
		return nil, 0, icV1.ErrorIcSchemaInvalid("list intel items failed")
	}
	return toBizIntelItems(entities), total, nil
}

func (r *IntelItemRepo) ListByClusterID(ctx context.Context, clusterID string, limit int) ([]*biz.IntelItemRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	entities, err := r.data.db.Client().IntelItem.Query().
		Where(intelitem.ClusterIDEQ(clusterID)).
		Order(ent.Desc(intelitem.FieldCollectedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list intel items by cluster failed (cluster=%s): %s", clusterID, err.Error())
		return nil, icV1.ErrorIcSchemaInvalid("list intel items failed")
	}
	return toBizIntelItems(entities), nil
}

func (r *IntelItemRepo) ListPendingPublish(ctx context.Context, limit int) ([]*biz.IntelItemRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	// B-25：只打捞 1h 窗口内的 pending（超过视为终态 failed，由 retention 兜底）
	since := time.Now().UTC().Add(-time.Hour)
	entities, err := r.data.db.Client().IntelItem.Query().
		Where(
			intelitem.PublishStateEQ("pending"),
			intelitem.CollectedAtGT(since),
		).
		Order(ent.Asc(intelitem.FieldCollectedAt)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list pending publish failed: %s", err.Error())
		return nil, icV1.ErrorIcSchemaInvalid("list pending publish failed")
	}
	return toBizIntelItems(entities), nil
}

// ---------- 写入（B-6 幂等 + B-8 同事务） ----------

// UpsertWithEvidenceTx 条目 + 取证快照同事务落库。
// 调用方必须已生成 intelID（B-8：intel_id 先于取证存在）与哈希链。
func (r *IntelItemRepo) UpsertWithEvidenceTx(ctx context.Context, upsert *biz.IntelItemUpsert, ev *biz.EvidenceSnapshotUpsert) (string, bool, error) {
	if upsert == nil || upsert.Contract.IntelID == "" || upsert.DedupKey == "" {
		return "", false, icV1.ErrorIcConfigInvalid("intel_id / dedup_key 必填")
	}

	// 快路径：已存在 → 幂等返回（NFR-IC-03）
	if existing, err := r.GetByDedupKey(ctx, upsert.DedupKey); err != nil {
		return "", false, err
	} else if existing != nil {
		return existing.IntelID, false, nil
	}

	evID := ""
	now := time.Now().UTC()
	err := RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
		// 1) 取证先落（B-8：先取证后归一；哈希链已由 usecase 计算）
		if ev != nil {
			if ev.ID == "" {
				ev.ID = contract.NewEvidenceID()
			}
			evID = ev.ID
			builder := tx.EvidenceSnapshot.Create().
				SetID(ev.ID).
				SetIntelID(upsert.Contract.IntelID).
				SetCollectedAt(ev.CollectedAt.UTC()).
				SetMethod(ev.Method).
				SetRequestURL(ev.RequestURL).
				SetHTTPStatus(ev.HTTPStatus).
				SetPrevHash(ev.PrevHash).
				SetChainHash(ev.ChainHash).
				SetRobotsChecked(ev.RobotsChecked).
				SetRobotsAllowed(ev.RobotsAllowed).
				SetCrawlDelayMs(ev.CrawlDelayMs).
				SetRetainUntil(ev.RetainUntil.UTC()).
				SetRawSizeBytes(ev.RawSizeBytes).
				SetCreateTime(now).
				SetUpdateTime(now)
			if ev.FinalURL != "" {
				builder.SetFinalURL(ev.FinalURL)
			}
			if ev.HTTPHeaders != nil {
				builder.SetHTTPHeaders(ev.HTTPHeaders)
			}
			if ev.RawPath != "" {
				builder.SetRawPath(ev.RawPath)
			}
			if ev.RawHash != "" {
				builder.SetRawHash(ev.RawHash)
			}
			if _, err := builder.Save(ctx); err != nil {
				r.log.Errorf("create evidence snapshot failed (intel=%s): %s", upsert.Contract.IntelID, err.Error())
				return icV1.ErrorIcSchemaInvalid("create evidence snapshot failed")
			}
		}

		// 2) 条目落库；dedup_key 冲突 → ON CONFLICT 刷新可变字段（幂等，B-6）
		c := &upsert.Contract
		builder := tx.IntelItem.Create().
			SetID(c.IntelID).
			SetAPIVersion(c.APIVersion).
			SetPlatform(c.Source.Platform).
			SetRegion(string(c.Source.Region)).
			SetCollectMethod(string(c.Source.CollectMethod)).
			SetCollectedAt(c.Source.CollectedAt.UTC()).
			SetTitle(c.Payload.Title).
			SetMediaRefs(c.Payload.MediaRefs).
			SetLang(c.Payload.Lang).
			SetEntities(contractEntitiesToMaps(c.Entities)).
			SetCategory(string(c.Category)).
			SetRank(c.Metrics.Rank).
			SetRankTotal(c.Metrics.RankTotal).
			SetViews(c.Metrics.Views).
			SetLikes(c.Metrics.Likes).
			SetComments(c.Metrics.Comments).
			SetCycleBucket(upsert.CycleBucket).
			SetDedupKey(upsert.DedupKey).
			SetFingerprintValid(false).
			SetFpB0(0).
			SetFpB1(0).
			SetFpB2(0).
			SetFpB3(0).
			SetComplianceGate(string(c.Compliance.Gate)).
			SetComplianceReasons(c.Compliance.Reasons).
			SetWorkabilityScore(c.Workability.Score).
			SetSourceID(upsert.SourceID).
			SetPublishState("pending"). // B-25：先 pending，NATS 发布成功后转 published
			SetRawSize(upsert.RawSize).
			SetRawTruncated(upsert.RawTruncated).
			SetPublishedAtSuspect(upsert.PublishedAtSuspect).
			SetCreateTime(now).
			SetUpdateTime(now)
		if evID != "" {
			builder.SetEvidenceID(evID)
		}
		if c.Source.URL != "" {
			builder.SetSourceURL(c.Source.URL)
		}
		if c.Source.Author != "" {
			builder.SetAuthor(c.Source.Author)
		}
		if c.Source.PublishedAt != nil {
			builder.SetPublishedAt(c.Source.PublishedAt.UTC())
		}
		if c.Payload.Text != "" {
			builder.SetText(c.Payload.Text)
		}
		if upsert.URLKey != "" {
			builder.SetURLKey(upsert.URLKey)
		}
		if c.Metrics.AsOf != nil {
			builder.SetMetricsAsOf(c.Metrics.AsOf.UTC())
		}
		if c.Workability.Reason != "" {
			builder.SetWorkabilityReason(c.Workability.Reason)
		}

		// 仓库 ent 模板不含 OnConflict 方法：约束碰撞由外层 IsConstraintError 判定降级，
		// 与唯一索引 + ON CONFLICT 语义等价（B-6 幂等）。
		if err := builder.Exec(ctx); err != nil {
			r.log.Errorf("create intel item failed (dedup_key=%s): %s", upsert.DedupKey, err.Error())
			return err // 需保留原始错误供外层判定约束冲突，勿包装
		}
		return nil
	})
	if err != nil {
		if ent.IsConstraintError(err) {
			// 同 dedup_key 并发插入的罕见竞态：降级为既有行 + 可变字段刷新。
			// 不补插证据：B-17 链哈希以竞速赢家的 intel_id 计算，事后重算会破坏链。
			return r.refreshUpsertedRow(ctx, upsert)
		}
		return "", false, icV1.ErrorIcSchemaInvalid("create intel item failed")
	}
	return upsert.Contract.IntelID, true, nil
}

// refreshUpsertedRow 约束碰撞降级路径：刷新既有行内容/指标并返回其 ID（created=false）。
func (r *IntelItemRepo) refreshUpsertedRow(ctx context.Context, upsert *biz.IntelItemUpsert) (string, bool, error) {
	row, err := r.GetByDedupKey(ctx, upsert.DedupKey)
	if err != nil {
		return "", false, err
	}
	if row == nil {
		return "", false, icV1.ErrorIcSchemaInvalid("竞态冲突后行不可见")
	}
	c := &upsert.Contract
	builder := r.data.db.Client().IntelItem.UpdateOneID(row.IntelID).
		SetTitle(c.Payload.Title).
		SetMediaRefs(c.Payload.MediaRefs).
		SetLang(c.Payload.Lang).
		SetRank(c.Metrics.Rank).
		SetRankTotal(c.Metrics.RankTotal).
		SetViews(c.Metrics.Views).
		SetLikes(c.Metrics.Likes).
		SetComments(c.Metrics.Comments).
		SetRawSize(upsert.RawSize).
		SetRawTruncated(upsert.RawTruncated).
		SetUpdateTime(time.Now().UTC())
	if c.Payload.Text != "" {
		builder.SetText(c.Payload.Text)
	}
	if c.Source.URL != "" {
		builder.SetSourceURL(c.Source.URL)
	}
	if c.Source.Author != "" {
		builder.SetAuthor(c.Source.Author)
	}
	if c.Source.PublishedAt != nil {
		builder.SetPublishedAt(c.Source.PublishedAt.UTC())
	}
	if c.Metrics.AsOf != nil {
		builder.SetMetricsAsOf(c.Metrics.AsOf.UTC())
	}
	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf("refresh intel item failed (id=%s): %s", row.IntelID, err.Error())
		return "", false, icV1.ErrorIcSchemaInvalid("refresh intel item failed")
	}
	return row.IntelID, false, nil
}

// ---------- 阶段回写 ----------

func (r *IntelItemRepo) UpdateAfterNormalize(ctx context.Context, patch *biz.IntelNormalizePatch) error {
	if patch == nil || patch.IntelID == "" {
		return icV1.ErrorIcConfigInvalid("intel_id 必填")
	}
	builder := r.data.db.Client().IntelItem.UpdateOneID(patch.IntelID).
		SetFingerprintValid(patch.FingerprintValid).
		SetFpB0(patch.FpSegments[0]).
		SetFpB1(patch.FpSegments[1]).
		SetFpB2(patch.FpSegments[2]).
		SetFpB3(patch.FpSegments[3]).
		SetEntities(contractEntitiesToMaps(patch.Entities)).
		SetCategory(patch.Category).
		SetWorkabilityScore(patch.WorkabilityScore).
		SetTitle(patch.Title).
		SetText(patch.Text).
		SetLang(patch.Lang).
		SetUpdateTime(time.Now().UTC())
	if patch.URLKey != nil {
		builder.SetURLKey(*patch.URLKey)
	}
	if patch.FingerprintHex != nil {
		builder.SetFingerprintHex(*patch.FingerprintHex)
	}
	if patch.WorkabilityReason != "" {
		builder.SetWorkabilityReason(patch.WorkabilityReason)
	}
	if _, err := builder.Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return icV1.ErrorIcSrcNotFound("intel item not found")
		}
		r.log.Errorf("update normalize failed (id=%s): %s", patch.IntelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("update normalize failed")
	}
	return nil
}

func (r *IntelItemRepo) UpdateDedupKey(ctx context.Context, intelID, urlKey, dedupKey string) error {
	if _, err := r.data.db.Client().IntelItem.UpdateOneID(intelID).
		SetURLKey(urlKey).
		SetDedupKey(dedupKey).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("update dedup_key failed (id=%s): %s", intelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("update dedup_key failed")
	}
	return nil
}

func (r *IntelItemRepo) UpdateGate(ctx context.Context, intelID, gate string, reasons []string) error {
	if reasons == nil {
		reasons = []string{}
	}
	if _, err := r.data.db.Client().IntelItem.UpdateOneID(intelID).
		SetComplianceGate(gate).
		SetComplianceReasons(reasons).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("update gate failed (id=%s): %s", intelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("update gate failed")
	}
	return nil
}

func (r *IntelItemRepo) UpdateMetrics(ctx context.Context, intelID string, m *biz.IntelMetricsPatch) error {
	if m == nil {
		return nil
	}
	builder := r.data.db.Client().IntelItem.UpdateOneID(intelID).
		SetRank(m.Rank).
		SetRankTotal(m.RankTotal).
		SetViews(m.Views).
		SetLikes(m.Likes).
		SetComments(m.Comments).
		SetUpdateTime(time.Now().UTC())
	if !m.AsOf.IsZero() {
		builder.SetMetricsAsOf(m.AsOf.UTC())
	}
	if _, err := builder.Save(ctx); err != nil {
		r.log.Errorf("update metrics failed (id=%s): %s", intelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("update metrics failed")
	}
	return nil
}

func (r *IntelItemRepo) UpdateEmbedding(ctx context.Context, intelID, embeddingID string) error {
	if _, err := r.data.db.Client().IntelItem.UpdateOneID(intelID).
		SetEmbeddingID(embeddingID).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("update embedding failed (id=%s): %s", intelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("update embedding failed")
	}
	return nil
}

// SupplementText 手贴正文（评审 C-12：只 UPDATE，不新建）。
func (r *IntelItemRepo) SupplementText(ctx context.Context, intelID, text string) error {
	if _, err := r.data.db.Client().IntelItem.UpdateOneID(intelID).
		SetText(text).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		if ent.IsNotFound(err) {
			return icV1.ErrorIcSrcNotFound("intel item not found")
		}
		r.log.Errorf("supplement text failed (id=%s): %s", intelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("supplement text failed")
	}
	return nil
}

func (r *IntelItemRepo) SetPublishState(ctx context.Context, intelID, state string) error {
	if _, err := r.data.db.Client().IntelItem.UpdateOneID(intelID).
		SetPublishState(state).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("set publish state failed (id=%s): %s", intelID, err.Error())
		return icV1.ErrorIcSchemaInvalid("set publish state failed")
	}
	return nil
}

func (r *IntelItemRepo) AssignCluster(ctx context.Context, intelIDs []string, clusterID string) error {
	if len(intelIDs) == 0 {
		return nil
	}
	if _, err := r.data.db.Client().IntelItem.Update().
		Where(intelitem.IDIn(intelIDs...)).
		SetClusterID(clusterID).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx); err != nil {
		r.log.Errorf("assign cluster failed: %s", err.Error())
		return icV1.ErrorIcSchemaInvalid("assign cluster failed")
	}
	return nil
}

func (r *IntelItemRepo) MoveClusterMembers(ctx context.Context, fromClusterID, toClusterID string) (int, error) {
	n, err := r.data.db.Client().IntelItem.Update().
		Where(intelitem.ClusterIDEQ(fromClusterID)).
		SetClusterID(toClusterID).
		SetUpdateTime(time.Now().UTC()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("move cluster members failed (from=%s to=%s): %s", fromClusterID, toClusterID, err.Error())
		return 0, icV1.ErrorIcSchemaInvalid("move cluster members failed")
	}
	return n, nil
}

func (r *IntelItemRepo) DeleteBefore(ctx context.Context, before time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	ids, err := r.data.db.Client().IntelItem.Query().
		Where(intelitem.CollectedAtLT(before.UTC())).
		Limit(limit).
		IDs(ctx)
	if err != nil {
		r.log.Errorf("query retention ids failed: %s", err.Error())
		return 0, icV1.ErrorIcSchemaInvalid("query retention ids failed")
	}
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := r.data.db.Client().IntelItem.Delete().
		Where(intelitem.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete intel items before %s failed: %s", before, err.Error())
		return 0, icV1.ErrorIcSchemaInvalid("delete intel items failed")
	}
	return n, nil
}

// ---------- 内部 ----------

func (r *IntelItemRepo) latestEvidence(ctx context.Context, intelID string) (*ent.EvidenceSnapshot, error) {
	return r.data.db.Client().EvidenceSnapshot.Query().
		Where(evidencesnapshot.IntelIDEQ(intelID)).
		Order(ent.Desc(evidencesnapshot.FieldCollectedAt)).
		First(ctx)
}

// contractEntitiesToMaps 契约实体 → ent JSON 存储形态。
func contractEntitiesToMaps(entities []contract.Entity) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(entities))
	for _, e := range entities {
		out = append(out, map[string]interface{}{
			"type":       string(e.Type),
			"value":      e.Value,
			"confidence": e.Confidence,
		})
	}
	return out
}

// toBizIntelItem 是评审 B-5 要求的唯一显式映射：ent → 契约 + 内部字段。
// ev 可空（手贴条目可能尚无取证）。
func toBizIntelItem(e *ent.IntelItem, ev *ent.EvidenceSnapshot) *biz.IntelItemRow {
	if e == nil {
		return nil
	}
	row := &biz.IntelItemRow{
		SourceID:    derefStr(e.SourceID),
		EvidenceID:  derefStr(e.EvidenceID),
		PublishState: e.PublishState,
		CycleBucket: e.CycleBucket,
		DedupKey:    e.DedupKey,
		FingerprintValid: e.FingerprintValid,
		FpSegments:  [4]int16{e.FpB0, e.FpB1, e.FpB2, e.FpB3},
		EmbeddingID: derefStr(e.EmbeddingID),
		PublishedAtSuspect: e.PublishedAtSuspect,
		RawSize:     e.RawSize,
		RawTruncated: e.RawTruncated,
	}
	row.IntelID = e.ID
	row.APIVersion = e.APIVersion
	row.Source = contract.IntelSource{
		Platform:      e.Platform,
		Region:        contract.Region(e.Region),
		CollectMethod: contract.CollectMethod(e.CollectMethod),
		URL:           derefStr(e.SourceURL),
		Author:        derefStr(e.Author),
		CollectedAt:   e.CollectedAt.UTC(),
		PublishedAt:   e.PublishedAt,
	}
	if row.Source.PublishedAt != nil {
		utc := row.Source.PublishedAt.UTC()
		row.Source.PublishedAt = &utc
	}
	if ev != nil {
		row.Source.Evidence = contract.IntelEvidence{
			SnapshotPath: derefStr(ev.RawPath),
			RawRef:       ev.ID,
			HTTPHeaders:  ev.HTTPHeaders,
		}
	}
	row.Payload = contract.IntelPayload{
		Title:     e.Title,
		Text:      derefStr(e.Text),
		MediaRefs: e.MediaRefs,
		Lang:      e.Lang,
	}
	for _, m := range e.Entities {
		ent0 := contract.Entity{
			Type:  contract.EntityType(strFromMap(m, "type")),
			Value: strFromMap(m, "value"),
		}
		if f, ok := m["confidence"].(float64); ok {
			ent0.Confidence = f
		}
		if ent0.Type == "" || ent0.Value == "" {
			continue // 脏数据降噪：空 type/value 不进契约
		}
		row.Entities = append(row.Entities, ent0)
	}
	row.Category = contract.Category(e.Category)
	row.Metrics = contract.IntelMetrics{
		Rank:      e.Rank,
		RankTotal: e.RankTotal,
		Views:     e.Views,
		Likes:     e.Likes,
		Comments:  e.Comments,
		AsOf:      e.MetricsAsOf,
	}
	if row.Metrics.AsOf != nil {
		utc := row.Metrics.AsOf.UTC()
		row.Metrics.AsOf = &utc
	}
	row.Dedup = contract.IntelDedup{
		URLKey:      derefStr(e.URLKey),
		Fingerprint: derefStr(e.FingerprintHex),
		ClusterID:   derefStr(e.ClusterID),
	}
	row.Compliance = contract.IntelCompliance{
		Gate:    contract.Gate(e.ComplianceGate),
		Reasons: e.ComplianceReasons,
	}
	row.Workability = contract.IntelWorkability{
		Score:  e.WorkabilityScore,
		Reason: derefStr(e.WorkabilityReason),
	}
	return row
}

func toBizIntelItems(entities []*ent.IntelItem) []*biz.IntelItemRow {
	out := make([]*biz.IntelItemRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizIntelItem(e, nil))
	}
	return out
}

// derefStr 复用 api_resource_repo.go 的包级定义。

func strFromMap(m map[string]interface{}, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}
