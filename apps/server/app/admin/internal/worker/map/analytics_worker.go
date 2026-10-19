// 表现回流 worker（⑤ MAP，FR-PUB-08 拉取侧；与排期 worker 同包同型）。
//
// 设计依据：详细设计 §4「发 mm.map.feedback.v1（若平台支持 analytics，延迟拉取）」
// + 概要设计 §8.1 mm.map.feedback.v1 契约 + map.yaml feedback 段。落地差异（03-详细设计
// §7.4 如实补记）：MM_PUB stream 未建（进度文档 §6.1 依赖表），V1 以 asynq 周期轮询回执
// 直接写快照表替代事件扇出；跨系统事件发射（TT/IC 消费侧）待 stream 建成后补发——
// 快照表本身即回流真值，事件只是传播媒介，不影响数据完备性。
//
// 🔴 feedback.enabled=false 为运维总闸（双层把关：Start 不注册周期任务 + pull 内部短路）。
// 拉取期每条回执含 2N+1 次 DB 调用（N=渠道下账号数）：回执在单次发布事件后低频写入、
// 渠道账号槽 ≤3（概要设计 §6.3 V1 约束），行政后台量级下直读路径可接受，故不做缓存/批取
// （缓存引入「启停后重试可能拿到陈旧账号」的二次失败源）。平台渠道枚举走
// ChannelRepo.ListChannelsByPlatform 直查（不走 REST 分页 ListChannels——后者有
// pageSize≤200 的后台护栏，内部「取全量」语义不得叠加该钳制）。
package mapworker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	asynqServer "github.com/tx7do/kratos-transport/transport/asynq"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/pkg/mediacontract"
)

const (
	// TaskAnalyticsScan 回流扫描任务（节奏 = conf.feedback.fetch_interval，默认 6h）。
	TaskAnalyticsScan = "map.analytics.scan"

	// jobAnalyticsScan 周期调度器内的登记 ID（幂等重建锚点）。
	jobAnalyticsScan = "map:analytics-scan"
	// pullReceiptWindow 回流回执窗：30 天（= bizpub.FeedbackScanWindow，设计常量集中在
	// biz 层定义——读侧 stale 判定用同一值，口径同源）。表现回流的价值密度集中在发布
	// 早期，长尾回执再拉已无观察意义（且平台多有限流）——固定窗口保拉取量可控。
	// 窗沿语义：出窗回执的快照停止追加（冻结），读侧如实标注 stale（见 service.GetAnalytics）。
	pullReceiptWindow = bizpub.FeedbackScanWindow
	// pullScanBatch 单页拉取回执上限（每回执最多一次平台 API 调用，节奏治理）。
	pullScanBatch = 256
	// pullMaxPages 单轮翻页上限（M6：单轮内把 30 天窗口拉满；256×20=5120 条窗口容量
	// 对行政后台量级充裕，达上限如实记日志、下轮续拉——防单轮失控）。
	pullMaxPages = 20
	// pullScanTimeout 单轮扫描总时限（256 条 × 平台 API 调用，需余量）。
	pullScanTimeout = 10 * time.Minute
)

type analyticsScanPayload struct{}

// AnalyticsWorker 表现回流 worker（transport.Server 生命周期；asynq 未配置 / 总闸
// 关闭时全面降级——扫描不注册，快照表仍可由 REST 读空 note 如实呈现「尚无数据」）。
type AnalyticsWorker struct {
	asynq       *asynqServer.Server
	conf        *bizpub.Conf
	receipts    bizpub.ReceiptRepo
	performance bizpub.PerformanceRepo
	dispatch    bizpub.DispatchRepo
	channels    bizpub.ChannelRepo
	accounts    bizpub.AccountRepo
	adapters    bizpub.AdapterLookup
	log         *log.Helper
	clock       func() time.Time
}

// NewAnalyticsWorker 构造（处理器必须构造期注册——asynq 禁止 Start 后注册）。
func NewAnalyticsWorker(
	asynqSrv *asynqServer.Server,
	conf *bizpub.Conf,
	receipts bizpub.ReceiptRepo,
	performance bizpub.PerformanceRepo,
	dispatch bizpub.DispatchRepo,
	channels bizpub.ChannelRepo,
	accounts bizpub.AccountRepo,
	adapters bizpub.AdapterLookup,
	logger log.Logger,
) *AnalyticsWorker {
	w := &AnalyticsWorker{
		asynq:       asynqSrv,
		conf:        conf,
		receipts:    receipts,
		performance: performance,
		dispatch:    dispatch,
		channels:    channels,
		accounts:    accounts,
		adapters:    adapters,
		log:         log.NewHelper(log.With(logger, "module", "worker/map-analytics")),
		clock:       time.Now,
	}
	if asynqSrv != nil {
		err := asynqServer.RegisterSubscriberWithCtx(asynqSrv, TaskAnalyticsScan,
			func(ctx context.Context, _ string, _ *analyticsScanPayload) error {
				ctx, cancel := context.WithTimeout(ctx, pullScanTimeout)
				defer cancel()
				return w.pullAll(ctx)
			})
		if err != nil {
			panic("register map subscriber " + TaskAnalyticsScan + " failed: " + err.Error())
		}
	}
	return w
}

// Start 登记周期扫描（幂等；总闸关闭则不注册——运维级 kill-switch，重启生效）。
// cron 由 conf.feedback.fetch_interval 动态换算（asynq 接受 @every <duration> 语法；
// 时长非整分可用 @every Ns/Nh 原样表达，FetchInterval 经 Normalize 保证 >0）。
func (w *AnalyticsWorker) Start(context.Context) error {
	if w.asynq == nil {
		w.log.Warn("asynq not configured, map analytics worker disabled")
		return nil
	}
	if w.conf == nil || !w.conf.Feedback.Enabled {
		w.log.Info("map feedback.enabled=false，表现回流拉取未启用（回执 REST/CSV 兜底不受影响）")
		return nil
	}
	interval := w.conf.Feedback.FetchInterval
	if interval <= 0 {
		interval = 6 * time.Hour // 双保险：Normalize 缺席时仍回落文档默认
	}
	cron := fmt.Sprintf("@every %s", interval)
	if _, err := w.asynq.NewPeriodicTask(cron, jobAnalyticsScan, TaskAnalyticsScan, &analyticsScanPayload{}); err != nil {
		return fmt.Errorf("register %s: %w", jobAnalyticsScan, err)
	}
	w.log.Infof("map analytics worker registered (%s, cron=%s)", TaskAnalyticsScan, cron)
	return nil
}

// Stop 周期任务体由共享 asynq server 生命周期管理，本层无自有 goroutine。
func (w *AnalyticsWorker) Stop(context.Context) error { return nil }

// pullAll 单轮拉取：扫 30 天内带 post_id 的回执 → 逐条解析渠道/账号/适配器 →
// 能力门（analytics 未声明即跳过，终态）→ 平台拉取 → 追加时序快照。
// 单条失败不阻断整轮（下一条继续）；汇总在日志与计数器里呈现。
func (w *AnalyticsWorker) pullAll(ctx context.Context) error {
	if w.conf == nil || !w.conf.Feedback.Enabled {
		return nil
	}
	now := w.clock()
	since := now.Add(-pullReceiptWindow)
	// M6 修复：单轮内分页拉满 30 天窗口——以本页最旧 published_at 为下一页 before
	// 游标翻页，窗口内回执再无「最老 44 条永远进不了任何一轮」的盲区。窗口分页
	// 上限 pullMaxPages 防单轮失控（一页 256，30 天 5120 条内 20 页拉满，行政后台
	// 量级充裕）；达上限即如实记日志，下一轮周期自然续拉（published_at 滚动前移）。
	var receipts []*bizpub.ReceiptRow
	before := time.Time{} // 零值 = 首页（窗口上界 now）
	for page := 0; page < pullMaxPages; page++ {
		batch, err := w.receipts.ListReceiptsPublishedSinceBefore(ctx, since, before, pullScanBatch)
		if err != nil {
			return err // 扫描源失败如实上报，asynq 按周期任务失败重投
		}
		if len(batch) == 0 {
			break
		}
		receipts = append(receipts, batch...)
		if len(batch) < pullScanBatch {
			break // 不足一页 = 窗口已尽
		}
		// 游标前移：本页最旧行的 published_at（倒序，末行即最旧）。
		oldest := batch[len(batch)-1]
		if oldest.PublishedAt.IsZero() {
			break // 脏数据防线：无 published_at 无法翻页，停在当前进度
		}
		before = oldest.PublishedAt
	}
	var saved, skipped, failed int
	for _, rc := range receipts {
		outcome := w.pullOne(ctx, rc, now)
		switch outcome {
		case "saved":
			saved++
		case "failed":
			failed++
		default:
			skipped++
		}
	}
	w.log.Infof("analytics pull round done: receipts=%d saved=%d skipped=%d failed=%d",
		len(receipts), saved, skipped, failed)
	return nil // 单条失败属于该条终态/重试决策，不回掷整轮
}

// pullOne 单条回执拉取。返回 outcome 供汇总（saved / skipped / failed）。
// 渠道解析链（两跳，恢复槽位精度）：
//  1. channelOf：按平台直查渠道——唯一渠道直接锚定；多槽位返回 (nil, nil)；
//  2. 多槽位时经回执发布账号反查其渠道（真实发布回执恒带 account_id，
//     publish_usecase 创建回执时恒写入）——锚定发布所用槽位，能力判定恢复
//     「渠道快照优先」口径（运营在槽位快照关掉 analytics 时不被旁路）；
//     回执无 account_id 且无唯一渠道可锚 → 诚实 skipped（不臆测槽位）。
//
// 🔴 凭据纪律：v1 适配器 FetchAnalytics(acct, postID) 签名不含凭据（mock 不需要）；
// 真实适配器接入时由适配器内部经 vault 取解密即清——本 worker 全程不触碰凭据。
func (w *AnalyticsWorker) pullOne(ctx context.Context, rc *bizpub.ReceiptRow, now time.Time) string {
	if rc.PostID == nil || *rc.PostID == "" {
		return "skipped" // 仓储谓词已过滤，双保险
	}
	ch, err := w.channelOf(ctx, rc)
	if err != nil {
		if errors.Is(err, bizpub.ErrNotFound) {
			w.log.Warnf("analytics pull: channel missing for receipt=%s platform=%s", rc.ReceiptID, rc.PlatformKey)
			return "skipped" // 终态：渠道被删，下轮无需再试
		}
		w.log.Warnf("analytics pull: channel lookup failed receipt=%s err=%v", rc.ReceiptID, err)
		return "failed"
	}
	if ch != nil && !ch.Enabled {
		return "skipped" // 渠道人工停用：终态（单槽位直判；多槽位锚定后再判）
	}
	acct := w.pickAccount(ctx, rc, ch)
	if acct == nil {
		return "skipped" // 渠道下无可用账号（未启用/凭据失效）或回执无账号可锚（多槽位）
	}
	if ch == nil {
		// 多槽位平台：经回执发布账号锚定槽位（账号 → 渠道）。
		anchored, aerr := w.channels.GetChannel(ctx, acct.ChannelID)
		if aerr != nil {
			if errors.Is(aerr, bizpub.ErrNotFound) {
				w.log.Warnf("analytics pull: anchored channel gone account=%s channel=%s receipt=%s",
					acct.AccountID, acct.ChannelID, rc.ReceiptID)
				return "skipped" // 终态：发布槽位已删
			}
			w.log.Warnf("analytics pull: channel anchor failed account=%s err=%v", acct.AccountID, aerr)
			return "failed"
		}
		if anchored.PlatformKey != rc.PlatformKey {
			// 数据异常：回执平台与账号所属渠道平台不一致（不应发生），不猜。
			w.log.Warnf("analytics pull: anchored platform mismatch account=%s channel=%s platform=%s receipt_platform=%s receipt=%s",
				acct.AccountID, acct.ChannelID, anchored.PlatformKey, rc.PlatformKey, rc.ReceiptID)
			return "skipped"
		}
		if !anchored.Enabled {
			return "skipped" // 发布槽位已停用：终态
		}
		ch = anchored
	}
	pub, ok := w.adapters.Get(rc.PlatformKey)
	if !ok {
		w.log.Warnf("analytics pull: adapter missing platform=%s receipt=%s", rc.PlatformKey, rc.ReceiptID)
		return "skipped" // 终态：未注册键（V1 真实适配器未装的自然分支）
	}
	if !bizpub.CapabilityOf(ch, pub).Analytics {
		return "skipped" // 终态：平台/槽位未声明 analytics 能力（渠道快照优先）
	}
	perf, err := pub.FetchAnalytics(ctx, acct, *rc.PostID)
	if err != nil {
		switch {
		case errors.Is(err, bizpub.ErrCapabilityUnsupported):
			return "skipped" // 终态：适配器侧再次声明不支持
		case errors.Is(err, bizpub.ErrCredentialInvalid):
			// 🔴 凭据失效：停账号 + 留凭据（纪律与发布路径一致），本轮终态。
			// 下次轮询时 pickAccount 会因 health 自动过滤。
			w.markCredentialInvalid(ctx, acct)
			return "failed"
		default:
			// 🔴 与发布路径同纪律：适配器原始错误先经 SanitizeUpstreamError 截断再进日志
			//（防未来真实适配器按 Go 惯例把 url/query token 带进 error 文本——回流路径
			// 不得成为发布侧双层防御（适配器契约 + 截断）之外的缺口）。
			w.log.Warnf("analytics pull fetch failed receipt=%s platform=%s err=%s",
				rc.ReceiptID, rc.PlatformKey, bizpub.SanitizeUpstreamError(err))
			return "failed" // 可重试错误：下一轮周期自然补拉（无独立重试队列——间隔足够大）
		}
	}
	row := &bizpub.PerformanceSnapshotRow{
		SnapshotID:     mediacontract.NewPerformanceSnapID(),
		ReceiptID:      rc.ReceiptID,
		PlatformKey:    rc.PlatformKey,
		DispatchUnitID: &rc.DispatchUnitID,
		Views:          perf.Views,
		Likes:          perf.Likes,
		Comments:       perf.Comments,
		Shares:         perf.Shares,
		Favorites:      perf.Favorites,
		CompletionRate: perf.CompletionRate,
		FetchedAt:      now.UTC(),
	}
	// 闭环回指：单元可查 → 回填 variant_id（回流反哺 MME 变体效果观察）。
	if unit, uerr := w.dispatch.GetUnit(ctx, rc.DispatchUnitID); uerr == nil && unit != nil && unit.VariantID != nil {
		row.VariantID = unit.VariantID
	}
	if err := w.performance.SaveSnapshot(ctx, row); err != nil {
		w.log.Warnf("analytics snapshot save failed receipt=%s err=%v", rc.ReceiptID, err)
		return "failed"
	}
	return "saved"
}

// channelOf 回执 → 渠道第一跳：按平台直查该平台全量渠道（ListChannelsByPlatform——
// 与 REST 分页的 ListChannels 分离，不叠加后台页大小钳制，渠道总量再大也不会漏）。
// 唯一渠道 → 直接锚定；多槽位 → 返回 (nil, nil)，槽位精度由 pullOne 经回执发布
// 账号反查渠道恢复（不回执账号且非唯一渠道时才诚实 skipped，不臆测槽位）。
func (w *AnalyticsWorker) channelOf(ctx context.Context, rc *bizpub.ReceiptRow) (*bizpub.ChannelRow, error) {
	channels, err := w.channels.ListChannelsByPlatform(ctx, rc.PlatformKey)
	if err != nil {
		return nil, err
	}
	switch len(channels) {
	case 0:
		return nil, bizpub.ErrNotFound
	case 1:
		return channels[0], nil
	default:
		return nil, nil // 多槽位：交给账号锚链（pullOne）
	}
}

// pickAccount 选回流账号：回执有 account_id 优先（发布同账号 = 数据口径一致）；
// 否则渠道下取任一「启用且健康」的账号。凭据失效/熔断账号自动过滤（health 字段）。
// ch==nil（多槽位平台）且回执无 account_id → 返回 nil（诚实 skipped：无槽位依据）。
func (w *AnalyticsWorker) pickAccount(ctx context.Context, rc *bizpub.ReceiptRow, ch *bizpub.ChannelRow) *bizpub.AccountRow {
	if rc.AccountID != nil && *rc.AccountID != "" {
		if a, err := w.accounts.GetAccount(ctx, *rc.AccountID); err == nil && a != nil &&
			a.Enabled && a.Health == "healthy" {
			return a
		}
		// 指定账号不可用 → 不降级到其他账号（口径漂移），本轮终态
		return nil
	}
	if ch == nil {
		return nil
	}
	list, err := w.accounts.ListAccountsByChannel(ctx, ch.ChannelID)
	if err != nil {
		return nil
	}
	for _, a := range list {
		if a.Enabled && a.Health == "healthy" {
			return a
		}
	}
	return nil
}

// markCredentialInvalid 回流侧凭据失效处置：health=credential_invalid（🔴 凭据保留，
// 纪律与 circuit_breaker 一致——留待人工重新授权，绝不因拉取失败清凭据）。
func (w *AnalyticsWorker) markCredentialInvalid(ctx context.Context, acct *bizpub.AccountRow) {
	lastErr := "回流拉取鉴权失败（凭据失效，待重新授权）"
	if err := w.accounts.SetHealth(ctx, acct.AccountID, "credential_invalid", &lastErr); err != nil {
		w.log.Warnf("mark credential_invalid failed account=%s err=%v", acct.AccountID, err)
	}
}
