package biz

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 手动采集入口（FR-INT-04 / B-13）：
//   - RunWithStats：RunCollector RPC 的带统计变体（force 忽略限流/熔断，人工补采语义）。
//   - FetchURL：单 URL 补采——02 §6.2「同主链路，但跳过 SourceRegistry 调度，直接单次执行」。
//     治理不旁路：robots 三态（S-7）、证据哈希链（B-17）、幂等键（B-6）全部保留；
//     限流走平台共享桶（S-10 对手采同样生效，force 不适用于本接口——proto 无该字段）。
// =============================================================================

var (
	// ErrIcPlatformUnknown FetchURL 的 URL host 推导不出契约平台词表值（§1 闭集，禁止造词）。
	ErrIcPlatformUnknown = errors.New("IC fetch url platform unknown")
	// ErrIcFetchMethod FetchURL method 仅收 api|crawl（hotlist/rss 走周期调度，sidecar 走授权通道）。
	ErrIcFetchMethod = errors.New("IC fetch method must be api|crawl")
	// ErrIcFetchRateLimited 手采命中平台限流桶（S-10）。
	ErrIcFetchRateLimited = errors.New("IC fetch rate limited")
	// ErrIcFetchEmpty 上游返回但无可用条目（crawl 空正文语义，02 §4.3 EMPTY_CONTENT）。
	ErrIcFetchEmpty = errors.New("IC fetch empty content")
)

// RunWithStats RunCollector 手动触发的完整变体：返回运行统计（collected/normalized）。
// force=true 忽略熔断与限流（人工补采 B-13；熔断恢复语义不触发——人工明确要跑）。
func (u *IcCollectUsecase) RunWithStats(ctx context.Context, sourceID, trigger string, force bool) (runID string, stats *CollectRunFinish, err error) {
	src, err := u.sources.GetByID(ctx, sourceID)
	if err != nil {
		return "", nil, err
	}
	if src == nil {
		return "", nil, ErrIcSourceNotFound
	}
	now := time.Now().UTC()

	// ---- 熔断门（S-12）：force 不查看门；非 force 的 paused 窗口内直接跳过（跳过 ≠ 失败）----
	if !force {
		if !Runnable(src, now) {
			return "", nil, nil
		}
		if src.Status == "paused" {
			// 熔断窗已过（Runnable 判定）→ 拨回 active（S-12 恢复语义）
			if err := u.sources.SetStatus(ctx, sourceID, "active", nil); err != nil {
				return "", nil, err
			}
		}
	}

	collector, ok := u.registry.Get(src.Method)
	if !ok {
		return "", nil, ErrIcCollectorUnavailable
	}

	// ---- 限流（S-10）：force 跳闸 ----
	if !force {
		if allowed, rerr := u.allowRate(ctx, src); rerr != nil {
			return "", nil, rerr
		} else if !allowed {
			return "", nil, nil
		}
	}

	// ---- 运行记录开账 ----
	runID, err = u.runs.Create(ctx, sourceID, trigger, now)
	if err != nil {
		return "", nil, err
	}
	fin := &CollectRunFinish{Status: "succeeded"}
	defer func() {
		// 熔断回执（成功重置连击 / 失败累计，阈值到位自动 paused —— data 层语义）
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
		return runID, fin, cerr
	}
	fin.Fetched = int32(len(items))

	// ---- 逐条 B-8 第 2-4 步 ----
	for _, item := range items {
		_, outcome, perr := u.persistRawItem(ctx, src, item)
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
	return runID, fin, nil
}

// FetchURL 单 URL 补采（FR-INT-04 手贴续跑入口之一，02 §6.2）。
// 合成临时信源（不入信源表；不产生 run 记录——ic_collect_runs.source_id 有外键约束，
// 审计面由证据快照承担：method/url/robots 结论全留痕）。
// 返回已落库条目；幂等重放（dedup_key 命中）返回既有行。
func (u *IcCollectUsecase) FetchURL(ctx context.Context, rawURL, method string) (*IntelItemRow, error) {
	if method != string(contract.CollectAPI) && method != string(contract.CollectCrawl) {
		return nil, ErrIcFetchMethod
	}
	_, canonical, err := NormalizeURLKey(rawURL)
	if err != nil {
		return nil, err
	}
	platform, region, ok := platformForURL(canonical)
	if !ok {
		return nil, ErrIcPlatformUnknown
	}
	collector, ok := u.registry.Get(method)
	if !ok {
		return nil, ErrIcCollectorUnavailable
	}

	// 合成信源：interval 60m 只影响 cycle_bucket 粒度；crawl 路需 config.targets 数组。
	synthetic := &IntelSourceRow{
		ID: "manual:fetch", Platform: platform, Region: region, Kind: "keyword",
		Method: method, Enabled: true, Status: "active", IntervalMinutes: 60,
		Config: map[string]interface{}{"url": canonical, "targets": []string{canonical}},
	}

	// 平台共享限流桶（S-10：手采不豁免预算）。
	if allowed, aerr := u.allowRate(ctx, synthetic); aerr != nil {
		return nil, aerr
	} else if !allowed {
		return nil, ErrIcFetchRateLimited
	}

	items, cerr := collector.Collect(ctx, synthetic)
	if cerr != nil {
		return nil, cerr
	}
	if len(items) == 0 {
		return nil, ErrIcFetchEmpty
	}

	var first *IntelItemRow
	var firstErr error
	for _, item := range items {
		intelID, _, perr := u.persistRawItem(ctx, synthetic, item)
		if perr != nil {
			if firstErr == nil {
				firstErr = perr
			}
			continue
		}
		if first == nil {
			// 幂等重放时 intelID 指向既有行（UpsertWithEvidenceTx 快路径语义）。
			row, gerr := u.intel.GetByID(ctx, intelID)
			if gerr != nil {
				return nil, gerr
			}
			first = row
		}
	}
	if first == nil {
		if firstErr != nil {
			return nil, firstErr
		}
		return nil, ErrIcFetchEmpty
	}
	return first, nil
}

// icPlatformHosts URL host 后缀 → 契约平台词表（§1 闭集）+ 区域。
// 匹配规则：host 等于后缀或以 "."+后缀 结尾（防 ax.com 误命中 x.com）。
// 词表外 host 一律不匹配——FetchURL 报错引导走信源注册，保证词表干净。
var icPlatformHosts = []struct{ suffix, platform, region string }{
	{"weibo.com", "weibo", "cn"},
	{"zhihu.com", "zhihu", "cn"},
	{"baidu.com", "baidu", "cn"},
	{"toutiao.com", "toutiao", "cn"},
	{"douyin.com", "douyin", "cn"},
	{"bilibili.com", "bilibili", "cn"},
	{"xiaohongshu.com", "xiaohongshu", "cn"},
	{"xhslink.com", "xiaohongshu", "cn"},
	{"kuaishou.com", "kuaishou", "cn"},
	{"weixin.qq.com", "weixin_channels", "cn"},
	{"youtube.com", "youtube", "intl"},
	{"youtu.be", "youtube", "intl"},
	{"reddit.com", "reddit", "intl"},
	{"x.com", "x", "intl"},
	{"twitter.com", "x", "intl"},
	{"tiktok.com", "tiktok", "intl"},
	{"facebook.com", "facebook", "intl"},
	{"instagram.com", "instagram", "intl"},
	{"t.me", "telegram", "intl"},
	{"telegram.me", "telegram", "intl"},
	{"news.ycombinator.com", "hackernews", "intl"},
}

// platformForURL 以 host 推导契约平台/区域；词表外返回 ok=false。
func platformForURL(canonical string) (platform, region string, ok bool) {
	u, err := url.Parse(canonical)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	host := strings.ToLower(u.Hostname())
	for _, e := range icPlatformHosts {
		if host == e.suffix || strings.HasSuffix(host, "."+e.suffix) {
			return e.platform, e.region, true
		}
	}
	return "", "", false
}
