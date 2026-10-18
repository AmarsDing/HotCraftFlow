package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 归一化流水线（评审 B-8 第 5-8 步：normalize → dedup → extract → gate → publish）
// 输入：mm.intel.raw 消费到的 notice（worker 解信封后传入，trace_id 透传不重建）。
// 失败纪律：硬错误返回 error 由 worker 写死信；NATS 发布失败不写死信——
// publish_state 保持 pending，republish 任务 30s 打捞（B-25）。
// =============================================================================

// IcNormalizeUsecase 归一化编排。
type IcNormalizeUsecase struct {
	cfg       *IcConfig
	intel     IntelItemRepo
	sources   IntelSourceRepo
	evidence  EvidenceRepo
	shortlink ShortlinkExpanderPort // 可 nil（配置关闭短链展开）
	extract   *IcExtractUsecase
	dedup     *IcDedupUsecase
	gate      *GovernanceGate
	publisher IntelPublisherPort
	idf       *IDFTable
	log       *log.Helper
}

func NewIcNormalizeUsecase(
	cfg *IcConfig,
	intel IntelItemRepo,
	sources IntelSourceRepo,
	evidence EvidenceRepo,
	shortlink ShortlinkExpanderPort,
	extract *IcExtractUsecase,
	dedup *IcDedupUsecase,
	gate *GovernanceGate,
	publisher IntelPublisherPort,
	logger log.Logger,
) *IcNormalizeUsecase {
	return &IcNormalizeUsecase{
		cfg: cfg, intel: intel, sources: sources, evidence: evidence, shortlink: shortlink,
		extract: extract, dedup: dedup, gate: gate, publisher: publisher,
		idf: LoadIDFTable(cfg.IDFPath),
		log: log.NewHelper(log.With(logger, "module", "ic/normalize/biz/admin-service")),
	}
}

// NormalizeResult 处理产物（观测与重放审计）。
type NormalizeResult struct {
	IntelID   string
	Gate      string
	Category  string
	ClusterID string
	DedupLevel string
	Published bool
}

// Process 处理一条 raw notice。traceID 取自生产信封（契约：trace_id 透传不重建）。
func (u *IcNormalizeUsecase) Process(ctx context.Context, notice *contract.RawIntelNotice, traceID string) (*NormalizeResult, error) {
	row, err := u.intel.GetByID(ctx, notice.IntelID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		// 条目不可见（上游半事务失败/保留期清扫）：重投无意义，交由 worker 直接 ack。
		u.log.Warnf("intel %s not found, skip normalize", notice.IntelID)
		return nil, nil
	}

	// ---- S-6 短链展开（先展开再归一，03 §4.1 注意；失败按原 URL 继续并留观测）----
	rekeyedURLKey, err := u.expandShortlink(ctx, row)
	if err != nil {
		return nil, err
	}

	// ---- S-25 语言判定：上游声明 → Unicode 区块统计 → und ----
	corpus := row.Payload.Title + "\n" + row.Payload.Text
	lang := row.Payload.Lang
	if lang == "" || lang == "und" {
		lang = DetectLangByScript(corpus)
	}
	row.Payload.Lang = lang
	row.Payload.Title = strings.Join(strings.Fields(row.Payload.Title), " ") // 空白折叠（展示态同证据态分离：原文在证据盘）

	// ---- L2 指纹（B-9）----
	sim := ComputeSimHash(row.Payload.Title, row.Payload.Text, lang, u.idf)
	row.Dedup.Fingerprint = sim.Hex
	row.FingerprintValid = sim.Valid
	row.FpSegments = sim.Segments

	// ---- L4 理解层（B-18；提取分类/实体/可加工性后闸门判定）----
	ext := u.extract.Extract(ctx, &row.IntelItem)
	row.Entities = ext.Entities
	row.Category = contract.Category(ext.Category)
	row.Workability = contract.IntelWorkability{Score: ext.WorkabilityScore, Reason: ext.WorkabilityReason}

	// ---- 合规闸门（B-16 fail-closed；robots 结论来自取证快照，S-7）----
	gc, gerr := u.buildGateContext(ctx, row)
	if gerr != nil {
		return nil, gerr
	}
	gateOut, gateErr := u.gate.Evaluate(ctx, &row.IntelItem, gc)
	if gateErr != nil {
		// Evaluate 已保证错误时 outcome=review；记录后继续（fail-closed 不是 fail-stop）。
		u.log.Warnf("gate evaluate partial failure (intel=%s, gate=%s): %s", row.IntelID, gateOut.Gate, gateErr.Error())
	}
	row.Compliance = contract.IntelCompliance{Gate: contract.Gate(gateOut.Gate), Reasons: gateOut.Reasons}

	// ---- 三级去重（闸门之后：block 条目也须入簇留档，便于审计同款来源聚类）----
	dedupOut, derr := u.dedup.Resolve(ctx, row, &sim)
	if derr != nil {
		return nil, derr
	}
	if dedupOut != nil {
		row.Dedup.ClusterID = dedupOut.ClusterID
	}

	// ---- 落库回写（一次 patch 写完归一化字段，闸门单列写）----
	patch := &IntelNormalizePatch{
		IntelID:            row.IntelID,
		FingerprintHex:     &sim.Hex,
		FingerprintValid:   sim.Valid,
		FpSegments:         sim.Segments,
		Entities:           row.Entities,
		Category:           string(row.Category),
		WorkabilityScore:   row.Workability.Score,
		WorkabilityReason:  row.Workability.Reason,
		Title:              row.Payload.Title,
		Text:               row.Payload.Text,
		Lang:               lang,
	}
	if rekeyedURLKey != "" { // 短链展开改过 key
		patch.URLKey = &rekeyedURLKey
	}
	if err := u.intel.UpdateAfterNormalize(ctx, patch); err != nil {
		return nil, err
	}
	if err := u.intel.UpdateGate(ctx, row.IntelID, string(row.Compliance.Gate), row.Compliance.Reasons); err != nil {
		return nil, err
	}
	if dedupOut != nil && dedupOut.EmbeddingID != "" {
		if err := u.intel.UpdateEmbedding(ctx, row.IntelID, dedupOut.EmbeddingID); err != nil {
			return nil, err
		}
	}

	// ---- 发布（B-8 末步；pass/review → normalized，block → blocked）----
	subject := contract.SubjectIntelNormalized
	if row.Compliance.Gate == contract.GateBlock {
		subject = contract.SubjectIntelBlocked
	}
	published := false
	if err := u.publish(ctx, subject, &row.IntelItem, traceID); err != nil {
		u.log.Warnf("publish %s failed, keep pending for republish (intel=%s): %s", subject, row.IntelID, err.Error())
	} else {
		published = true
		if err := u.intel.SetPublishState(ctx, row.IntelID, "published"); err != nil {
			return nil, err
		}
	}

	res := &NormalizeResult{
		IntelID: row.IntelID, Gate: string(row.Compliance.Gate), Category: string(row.Category),
		ClusterID: row.Dedup.ClusterID, Published: published,
	}
	if dedupOut != nil {
		res.DedupLevel = dedupOut.Level
	}
	return res, nil
}

// expandShortlink S-6：候选短链 → 展开 → canonical → 重算 url_key/dedup_key。
// 返回非空串表示 key 已变（调用方须随归一化 patch 回写 url_key 列）。
// 展开失败按原 URL 继续（03 §4.1 注意：标记即可，不构成 normalize 失败）；
// 换 key 后与新 key 持有者冲突 → 走 L1 合并（ic_repo.go UpdateDedupKey 约定）。
func (u *IcNormalizeUsecase) expandShortlink(ctx context.Context, row *IntelItemRow) (string, error) {
	if u.shortlink == nil || !IsURLCandidateForShortlink(row.Source.URL) {
		return "", nil
	}
	final, err := u.shortlink.Expand(ctx, row.Source.URL)
	if err != nil {
		u.log.Warnf("shortlink expand failed, keep original (intel=%s, url=%s): %s", row.IntelID, row.Source.URL, err.Error())
		return "", nil
	}
	if final == "" || final == row.Source.URL {
		return "", nil
	}
	urlKey, canonical, err := NormalizeURLKey(final)
	if err != nil {
		u.log.Warnf("canonicalize expanded url failed (intel=%s): %s", row.IntelID, err.Error())
		return "", nil
	}
	newDedupKey := ComputeDedupKey(urlKey, row.SourceID, row.CycleBucket)
	if urlKey == row.Dedup.URLKey && newDedupKey == row.DedupKey {
		return "", nil
	}
	if err := u.intel.UpdateDedupKey(ctx, row.IntelID, urlKey, newDedupKey); err != nil {
		// 另一行已占该键：L1 语义合并（本行就是对同 URL 的迟到重复采样）。
		peer, qerr := u.intel.FindRecentByURLKey(ctx, urlKey, icDedupMergeWindow)
		if qerr != nil {
			return "", qerr
		}
		if peer != nil && peer.IntelID != row.IntelID {
			if _, merr := u.dedup.ForceL1Merge(ctx, row, peer); merr != nil {
				return "", merr
			}
			row.Dedup.ClusterID = peer.Dedup.ClusterID
			return "", nil
		}
		return "", err // 查无对端却冲突：判数据异常上抛
	}
	row.Source.URL = canonical
	row.Dedup.URLKey = urlKey
	row.DedupKey = newDedupKey
	return urlKey, nil
}

// buildGateContext 组闸门上下文：robots 三态取最近取证快照（S-7），豁免读信源（S-8）。
func (u *IcNormalizeUsecase) buildGateContext(ctx context.Context, row *IntelItemRow) (GateContext, error) {
	gc := GateContext{Method: string(row.Source.CollectMethod)}
	if ev, err := u.evidence.GetLatestByIntelID(ctx, row.IntelID); err != nil {
		return gc, err
	} else if ev != nil {
		gc.RobotsChecked = ev.RobotsChecked
		gc.RobotsAllowed = ev.RobotsAllowed
	}
	if src, err := u.sources.GetByID(ctx, row.SourceID); err != nil {
		return gc, err
	} else if src != nil {
		gc.RobotsExemptSource = src.RobotsExempt
	}
	return gc, nil
}

// publish 组信封并发 NATS（幂等键 = intel_id）。
func (u *IcNormalizeUsecase) publish(ctx context.Context, subject string, item *contract.IntelItem, traceID string) error {
	if err := item.Validate(); err != nil {
		// 契约组装失败 = 实现缺陷级错误，必须暴露（死信由 worker 兜底）。
		return err
	}
	env, err := contract.NewEnvelope("ic", subject, traceID, item)
	if err != nil {
		return err
	}
	return u.publisher.PublishIntel(ctx, subject, env, item.IntelID)
}

// RepublishPending B-25 打捞入口：pending ≤1h 的条目重发 normalized/blocked。
// 供 worker republish 任务循环调用；返回发布成功数。
func (u *IcNormalizeUsecase) RepublishPending(ctx context.Context, limit int) (int, error) {
	rows, err := u.intel.ListPendingPublish(ctx, limit)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, row := range rows {
		// 重发前校验契约仍完整（归一化半途崩掉的行：指纹空 → 重新进 normalize 更稳，跳过让重试链路处理）
		if !row.FingerprintValid && row.Dedup.Fingerprint == "" {
			continue
		}
		subject := contract.SubjectIntelNormalized
		if row.Compliance.Gate == contract.GateBlock {
			subject = contract.SubjectIntelBlocked
		}
		if err := u.publish(ctx, subject, &row.IntelItem, ""); err != nil {
			u.log.Warnf("republish failed (intel=%s): %s", row.IntelID, err.Error())
			continue
		}
		if err := u.intel.SetPublishState(ctx, row.IntelID, "published"); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}
