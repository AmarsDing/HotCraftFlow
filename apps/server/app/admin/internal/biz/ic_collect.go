package biz

import (
	"context"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 采集编排（评审 B-8 第 1-4 步；顺序写死不可换）：
//   1. 采集器产出 RawFragment
//   2. 立即生成 intel_id（ULID 本地生成）
//   3. 证据快照与条目同事务落库（B-17 哈希链）
//   4. 发布 mm.intel.raw（仅 RawMeta，原始字节永不进消息体 B-21）
// 治理：限流（S-10 平台级 > 信源级）→ 熔断（S-12）→ robots 三态（S-7/S-8）。
// =============================================================================

// IcCollectUsecase 一轮采集的编排器。
type IcCollectUsecase struct {
	cfg        *IcConfig
	sources    IntelSourceRepo
	intel      IntelItemRepo
	runs       CollectRunRepo
	registry   CollectorRegistry
	store      EvidenceStorePort
	limiter    RateLimiterPort
	robots     RobotsCheckerPort
	publisher  IntelPublisherPort
	evidences  EvidenceRepo
	log        *log.Helper
}

func NewIcCollectUsecase(
	cfg *IcConfig,
	sources IntelSourceRepo,
	intel IntelItemRepo,
	runs CollectRunRepo,
	registry CollectorRegistry,
	store EvidenceStorePort,
	limiter RateLimiterPort,
	robots RobotsCheckerPort,
	publisher IntelPublisherPort,
	evidences EvidenceRepo,
	logger log.Logger,
) *IcCollectUsecase {
	return &IcCollectUsecase{
		cfg: cfg, sources: sources, intel: intel, runs: runs, registry: registry,
		store: store, limiter: limiter, robots: robots, publisher: publisher, evidences: evidences,
		log: log.NewHelper(log.With(logger, "module", "ic/collect/biz/admin-service")),
	}
}

// RunSource 执行一轮采集；trigger = cron|manual|retry。
// 上一级 worker 的调用协议：每个信源每周期一次；本函数自含运行记录与熔断回执。
func (u *IcCollectUsecase) RunSource(ctx context.Context, sourceID, trigger string) (runID string, err error) {
	src, err := u.sources.GetByID(ctx, sourceID)
	if err != nil {
		return "", err
	}
	if src == nil {
		return "", ErrIcSourceNotFound
	}
	now := time.Now().UTC()

	// ---- 熔断门（S-12）：paused 窗口内直接跳过，不产生 run（跳过 ≠ 失败）----
	if !Runnable(src, now) {
		return "", nil
	}
	if src.Status == "paused" {
		// 能走到这 = 熔断窗已过（Runnable 判定）→ 拨回 active（S-12 恢复语义）
		if err := u.sources.SetStatus(ctx, sourceID, "active", nil); err != nil {
			return "", err
		}
	}

	collector, ok := u.registry.Get(src.Method)
	if !ok {
		return "", ErrIcCollectorUnavailable
	}

	// ---- 限流（S-10）：平台级桶 > 信源级桶；任一不足本轮跳过 ----
	if allowed, err := u.allowRate(ctx, src); err != nil {
		return "", err
	} else if !allowed {
		return "", nil
	}

	// ---- 运行记录开账 ----
	runID, err = u.runs.Create(ctx, sourceID, trigger, now)
	if err != nil {
		return "", err
	}
	fin := &CollectRunFinish{Status: "succeeded"}
	defer func() {
		// 熔断回执（成功重置连击 / 失败累计，阈值到位自动 paused 30m —— data 层语义）
		if oerr := u.sources.RecordRunOutcome(ctx, sourceID, fin.Status == "succeeded", fin.ErrorCode,
			time.Now().UTC(), u.cfg.BreakerFailThreshold, u.cfg.BreakerCooldown); oerr != nil {
			u.log.Warnf("record run outcome failed (source=%s): %s", sourceID, oerr.Error())
		}
		if ferr := u.runs.Finish(ctx, runID, fin); ferr != nil {
			u.log.Warnf("finish run %s failed: %s", runID, ferr.Error())
		}
	}()

	// ---- 采集 ----
	items, cerr := collector.Collect(ctx, src)
	if cerr != nil {
		fin.Status, fin.ErrorCode = "failed", "IC_COLLECT_FAILED"
		err = cerr
		return runID, err
	}
	fin.Fetched = int32(len(items))

	// ---- 逐条 B-8 第 2-4 步 ----
	for _, item := range items {
		outcome, perr := u.persistRawItem(ctx, src, item)
		if perr != nil {
			u.log.Warnf("persist raw item failed (source=%s, url=%s): %s", src.ID, item.SourceURL, perr.Error())
			fin.Skipped++
			continue
		}
		switch outcome {
		case rawOutcomeCreated:
			fin.Normalized++
		case rawOutcomeDuplicate:
			fin.Merged++
		}
	}
	return runID, nil
}

// allowRate 平台 + 信源双桶（S-10）。
// 平台桶水位取默认速率（平台级共享），信源桶取信源配置（未配走默认）。
func (u *IcCollectUsecase) allowRate(ctx context.Context, src *IntelSourceRow) (bool, error) {
	if u.limiter == nil {
		return true, nil
	}
	perMin := src.RateLimitPerMin
	if perMin <= 0 {
		perMin = u.cfg.RateLimitDefaultPerMin
	}
	if ok, err := u.limiter.Allow(ctx, "ic:platform:"+src.Platform, u.cfg.RateLimitDefaultPerMin); err != nil {
		return false, err
	} else if !ok {
		return false, nil
	}
	return u.limiter.Allow(ctx, "ic:source:"+src.ID, perMin)
}

type rawOutcome int

const (
	rawOutcomeCreated rawOutcome = iota + 1
	rawOutcomeDuplicate
)

// persistRawItem B-8 第 2-4 步单条实现（顺序不可换）。
func (u *IcCollectUsecase) persistRawItem(ctx context.Context, src *IntelSourceRow, item *RawCollectItem) (rawOutcome, error) {
	// 2. intel_id 立即生成（不依赖 DB）
	intelID := contract.NewIntelItemID()
	collectedAt := time.Now().UTC()

	// URL 归一 + 周期桶 + 幂等键（B-6）
	urlKey, canonical, err := NormalizeURLKey(item.SourceURL)
	if err != nil {
		return 0, err
	}
	cycleBucket := CycleBucketFor(collectedAt, src.IntervalMinutes)
	dedupKey := ComputeDedupKey(urlKey, src.ID, cycleBucket)

	// robots 三态（S-7/S-16：robots 检查在采集路由外，结论进证据快照）
	robotsChecked, robotsAllowed, crawlDelayMs, err := u.checkRobots(ctx, src, canonical)
	if err != nil {
		u.log.Warnf("robots check failed, three-state=false (source=%s): %s", src.ID, err.Error())
	}

	// S-16：发布时间不可信标记（保留原值）
	publishedAtSuspect := item.PublishedAt != nil && item.PublishedAt.After(collectedAt.Add(5*time.Minute))

	// 3. 证据落盘 + 哈希链 + 同事务落库
	ev, err := u.buildEvidence(ctx, intelID, src, item, canonical, collectedAt, robotsChecked, robotsAllowed, crawlDelayMs)
	if err != nil {
		return 0, err
	}
	upsert := &IntelItemUpsert{
		Contract:    u.buildContract(intelID, src, item, canonical, collectedAt, ev),
		SourceID:    src.ID,
		URLKey:      urlKey,
		CycleBucket: cycleBucket,
		DedupKey:    dedupKey,
		RawSize:     ev.RawSizeBytes,
		RawTruncated: false,
		PublishedAtSuspect: publishedAtSuspect,
	}
	if item.Fragment != nil && int64(len(item.Fragment.Content)) > u.cfg.EvidenceRawMaxBytes {
		upsert.RawTruncated = true
	}
	_, created, err := u.intel.UpsertWithEvidenceTx(ctx, upsert, ev)
	if err != nil {
		return 0, err
	}
	if !created {
		return rawOutcomeDuplicate, nil // 幂等重放 / 周期内重复（热榜高频属预期）
	}

	// 4. 发 mm.intel.raw（仅 RawMeta；证据盘路径 + 链哈希，B-21）
	notice := &contract.RawIntelNotice{
		APIVersion:  "1.0",
		IntelID:     intelID,
		SourceID:    src.ID,
		CollectedAt: collectedAt,
		Raw: contract.RawMeta{
			RawPath:    ev.RawPath,
			RawHash:    ev.RawHash,
			RawSize:    ev.RawSizeBytes,
			RawTruncated: upsert.RawTruncated,
			PrevHash:   ev.PrevHash,
			ChainHash:  ev.ChainHash,
			EvidenceID: ev.ID,
		},
	}
	env, err := contract.NewEnvelope("ic", contract.SubjectIntelRaw, "", notice)
	if err != nil {
		return 0, err
	}
	if err := u.publisher.PublishIntel(ctx, contract.SubjectIntelRaw, env, intelID); err != nil {
		return 0, err
	}
	return rawOutcomeCreated, nil
}

// checkRobots 包装 RobotsCheckerPort：仅 crawl/api 两路适用（S-7）。
func (u *IcCollectUsecase) checkRobots(ctx context.Context, src *IntelSourceRow, canonicalURL string) (checked, allowed bool, crawlDelayMs int32, err error) {
	if u.robots == nil || (src.Method != string(contract.CollectCrawl) && src.Method != string(contract.CollectAPI)) {
		return false, false, 0, nil
	}
	allowed, delay, found, err := u.robots.Check(ctx, src.Method, canonicalURL, src.RobotsExempt)
	if err != nil {
		return false, false, 0, err
	}
	return found, allowed, delay, nil
}

// buildEvidence 落盘 + B-17 哈希链（链路取同条目最近一条 chain_hash 作 prev；首条 64 个 0）。
func (u *IcCollectUsecase) buildEvidence(
	ctx context.Context, intelID string, src *IntelSourceRow, item *RawCollectItem,
	canonical string, collectedAt time.Time, robotsChecked, robotsAllowed bool, crawlDelayMs int32,
) (*EvidenceSnapshotRow, error) {
	ev := &EvidenceSnapshotRow{
		ID:          contract.NewEvidenceID(),
		IntelID:     intelID,
		CollectedAt: collectedAt,
		Method:      src.Method,
		RequestURL:  item.SourceURL,
		FinalURL:    canonical,
		RobotsChecked: robotsChecked,
		RobotsAllowed: robotsAllowed,
		CrawlDelayMs:  crawlDelayMs,
		RetainUntil:   collectedAt.Add(time.Duration(u.cfg.EvidenceRetainDays) * 24 * time.Hour),
	}
	frag := item.Fragment
	if frag == nil {
		// 手贴/侧车可无原文片段：占位空内容，哈希照算（链完整优先于全文留存）。
		frag = &contract.RawFragment{RequestURL: item.SourceURL, FinalURL: canonical}
	}
	rawPath, rawHash, size, _, err := u.store.Save(ctx, intelID, frag, u.cfg.EvidenceRawMaxBytes)
	if err != nil {
		return nil, err
	}
	ev.RawPath, ev.RawHash, ev.RawSizeBytes = rawPath, rawHash, size
	if frag.HTTPStatus > 0 {
		ev.HTTPStatus = int32(frag.HTTPStatus)
	}
	ev.HTTPHeaders = contract.SanitizedHeaders(frag.Headers)
	if frag.FinalURL != "" {
		ev.FinalURL = frag.FinalURL
	}

	prev, err := u.evidences.LatestChainHash(ctx, intelID)
	if err != nil {
		return nil, err
	}
	if prev == "" {
		prev = strings.Repeat("0", 64) // B-17：链首条
	}
	ev.PrevHash = prev
	ev.ChainHash = contract.ChainHash(prev, rawHash, intelID, collectedAt)
	return ev, nil
}

// buildContract 组装待入库契约（gate/fingerprint/category 由归一化阶段回填；此处给合法初值）。
func (u *IcCollectUsecase) buildContract(
	intelID string, src *IntelSourceRow, item *RawCollectItem,
	canonical string, collectedAt time.Time, ev *EvidenceSnapshotRow,
) contract.IntelItem {
	asOf := collectedAt
	return contract.IntelItem{
		APIVersion: "1.0",
		IntelID:    intelID,
		Source: contract.IntelSource{
			Platform:      src.Platform,
			Region:        contract.Region(src.Region),
			CollectMethod: contract.CollectMethod(src.Method),
			URL:           canonical,
			Author:        item.Author,
			CollectedAt:   collectedAt,
			PublishedAt:   item.PublishedAt,
			Evidence: contract.IntelEvidence{
				SnapshotPath: ev.RawPath,
				RawRef:       ev.ID,
				HTTPHeaders:  ev.HTTPHeaders,
			},
		},
		Payload: contract.IntelPayload{
			Title:     item.Title,
			Text:      item.Text,
			MediaRefs: item.MediaRefs,
			Lang:      item.Lang,
		},
		Metrics: contract.IntelMetrics{
			Rank:      item.Rank,
			RankTotal: item.RankTotal,
			Views:     item.Views,
			Likes:     item.Likes,
			Comments:  item.Comments,
			AsOf:      &asOf,
		},
		// 初值（归一化覆写）：category=B_material 兜底桶、gate=review 未判先封（B-16 方向的初值纪律）。
		Category:   contract.CategoryBMaterial,
		Compliance: contract.IntelCompliance{Gate: contract.GateReview, Reasons: []string{"pending:待归一化判定"}},
	}
}

// ErrIcCollectorUnavailable 采集器未注册/被配置禁用（M-5）。
var ErrIcCollectorUnavailable = icSentinel("IC collector unavailable")
