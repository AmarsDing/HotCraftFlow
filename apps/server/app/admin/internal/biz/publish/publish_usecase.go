package publish

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
	"framework/pkg/mediacontract"
)

// PublishUsecase 发布编排用例（详细设计 §4）。
//
// 顺序即安全序：五校验 → 二次确认 → 幂等锁 → 逐 target 串行（可预期、可中止、
// 失败现场最小）→ 回执落库（唯一索引幂等兜底）→ 健康复位/熔断 → 审计。
//
// 🔴 凭据纪律：解密后的 Credential 仅存在于本平台发布调用期间，defer Zero() 清零；
// 任何分支不得把 Secret 写入日志/错误消息/回执。
type PublishUsecase struct {
	conf      *Conf
	dispatch  DispatchRepo
	accounts  AccountRepo
	channels  ChannelRepo
	receipts  ReceiptRepo
	records   ApprovalRepo
	vault     CredentialVault
	locker    PublishLocker // 可为 nil（data 层无 Redis 时降级——仅日志告警，见下）
	adapters  AdapterLookup
	breaker   *BreakerUsecase
	quota     *QuotaUsecase
	clock     func() time.Time
	log       *log.Helper
}

// NewPublishUsecase 构造发布编排用例。
func NewPublishUsecase(
	conf *Conf,
	dispatch DispatchRepo,
	accounts AccountRepo,
	channels ChannelRepo,
	receipts ReceiptRepo,
	records ApprovalRepo,
	vault CredentialVault,
	locker PublishLocker,
	adapters AdapterLookup,
	breaker *BreakerUsecase,
	quota *QuotaUsecase,
	logger log.Logger,
) *PublishUsecase {
	return &PublishUsecase{
		conf:     conf,
		dispatch: dispatch,
		accounts: accounts,
		channels: channels,
		receipts: receipts,
		records:  records,
		vault:    vault,
		locker:   locker,
		adapters: adapters,
		breaker:  breaker,
		quota:    quota,
		clock:    time.Now,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish")),
	}
}

// PublishOutcome 发布结果（receipts 为本 unit 全量回执，warnings 为降级提示）。
type PublishOutcome struct {
	Unit     *UnitRow
	Receipts []*ReceiptRow
	Warnings []string
}

// Publish 执行发布（🔴 唯一对外发布入口；approve 与 publish 严格分离，ADR-MAP-03）。
func (uc *PublishUsecase) Publish(ctx context.Context, unitID string, operator Actor, confirmPublic bool, meta AuditMeta) (*PublishOutcome, error) {
	unit, err := uc.dispatch.GetUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}

	// 幂等快速路径：已发布 → 直接返回既有回执（重复点击/重放安全）。
	if unit.PublishState == "published" {
		receipts, lerr := uc.receipts.ListReceiptsByUnit(ctx, unitID)
		if lerr != nil {
			return nil, lerr
		}
		return &PublishOutcome{Unit: unit, Receipts: receipts}, nil
	}

	// 1) 目标解析 + 渠道/账号指派（先于门校验，熔断/配额结论要用到账号）。
	resolved, err := uc.resolveTargets(ctx, unit)
	if err != nil {
		return nil, err
	}

	// 2) 收集熔断/配额结论 → 审批门五校验（algo 纯算法）。
	// 🔴 门检与第 6 步同一「未发平台」口径：已有回执的平台本轮必然跳过
	// （适配器边界幂等，发帖不可逆），不参与熔断/配额预检——否则 A 成 B 败后
	// 立即重试会被 A 的最小间隔配额整体拒死，跳过分支永远走不到（重试不可达）。
	existing, err := uc.receipts.ListReceiptsByUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	sentSet := make(map[string]struct{}, len(existing))
	for _, r := range existing {
		sentSet[r.PlatformKey] = struct{}{}
	}
	pending := make([]*ResolvedTarget, 0, len(resolved))
	for _, rt := range resolved {
		if _, done := sentSet[rt.Target.Platform]; done {
			continue
		}
		pending = append(pending, rt)
	}
	now := uc.clock()
	accountPaused := false
	for _, rt := range pending {
		if !Usable(rt.Account, now) {
			accountPaused = true
			break
		}
	}
	quotaErr := uc.firstQuotaError(ctx, pending)
	if err := CheckPublish(unit, operator, accountPaused, quotaErr != nil); err != nil {
		outErr := err
		if errors.Is(err, algopublisher.ErrQuotaExceeded) && quotaErr != nil {
			outErr = quotaErr // 透出具体配额命中项
		}
		uc.auditDeny(ctx, unit, operator, outErr, meta)
		return nil, outErr
	}

	// 3) 二次确认（🔴 非 private 必须显式 confirm_public，否则拒绝并留痕）。
	if unit.VisibilityDefault != string(mediacontract.VisibilityPrivate) && !confirmPublic {
		err := fmt.Errorf("%w: 可见性为 %s，发布必须显式 confirm_public=true", ErrPublicConfirmRequired, unit.VisibilityDefault)
		uc.auditDeny(ctx, unit, operator, err, meta)
		return nil, err
	}

	// 4) 幂等锁（Redis SETNX 10min；无锁实现时降级仅日志——DB 唯一索引仍兜底）。
	if uc.locker != nil {
		ok, lerr := uc.locker.Lock(ctx, unitID, 10*time.Minute)
		if lerr != nil {
			return nil, fmt.Errorf("%w: 获取发布锁失败: %v", ErrInternal, lerr)
		}
		if !ok {
			return nil, fmt.Errorf("%w: 单元 %s 正在发布中", ErrAlreadyPublishing, unitID)
		}
		defer func() {
			if uerr := uc.locker.Unlock(context.Background(), unitID); uerr != nil {
				uc.log.Warnf("[map-publish] 释放发布锁失败 unit=%s err=%v", unitID, uerr)
			}
		}()
		// 锁后重读（双检）：等待锁期间可能已被另一操作者发完。
		unit, err = uc.dispatch.GetUnit(ctx, unitID)
		if err != nil {
			return nil, err
		}
		if unit.PublishState == "published" {
			receipts, lerr := uc.receipts.ListReceiptsByUnit(ctx, unitID)
			if lerr != nil {
				return nil, lerr
			}
			return &PublishOutcome{Unit: unit, Receipts: receipts}, nil
		}
		// 🔴 锁内复核审批态：等锁期间审批可能被并发撤回（Cancel approved→draft）
		// 或抢占修改——锁只防并发发布，不防状态倒退，必须再完整过一次门。
		if cerr := CheckPublish(unit, operator, accountPaused, quotaErr != nil); cerr != nil {
			uc.auditDeny(ctx, unit, operator, cerr, meta)
			return nil, cerr
		}
	} else {
		uc.log.Warn("[map-publish] PublishLocker 未配置，幂等仅靠 DB 唯一索引兜底（dev 退化模式）")
	}

	// 5) 置发布中（崩溃残留 publishing 时：published 判定仍兜底，人工核查后可重发——评论即事故处理约定）。
	publishing := "publishing"
	if _, err = uc.dispatch.UpdateStates(ctx, unitID, UnitPatch{PublishState: &publishing}); err != nil {
		return nil, err
	}

	// 6) 逐 target 串行发布。
	// 🔴 适配器边界幂等：外部发帖不可逆，DB 唯一索引只能挡回执行、挡不住
	// 平台侧重复发帖。此前 A 成功/B 失败后重试会重发 A——先读既有回执，已发的
	// 平台直接跳过（失败重试 = 只补未成的平台）。
	// 第 6 步锁后必须新读回执：等锁期间另一操作者可能已补发部分平台（第 2 步的
	// 预检集合会滞后），以本处的 fresh 集合为跳过依据，防重复发帖红线。
	fresh, ferr := uc.receipts.ListReceiptsByUnit(ctx, unitID)
	if ferr != nil {
		return nil, ferr
	}
	sent := make(map[string]struct{}, len(fresh)+len(resolved))
	for _, r := range fresh {
		sent[r.PlatformKey] = struct{}{}
	}

	var warnings []string
	for _, rt := range resolved {
		if _, done := sent[rt.Target.Platform]; done {
			continue
		}
		if perr := uc.publishOne(ctx, unit, rt, operator, &warnings); perr != nil {
			// 失败：unit 置 failed，错误直传（重试/死信由 M4 asynq worker 承担，V1 人工重发）。
			failed := "failed"
			if _, uerr := uc.dispatch.UpdateStates(ctx, unitID, UnitPatch{PublishState: &failed}); uerr != nil {
				uc.log.Errorf("[map-publish] 置 failed 失败 unit=%s err=%v", unitID, uerr)
			}
			uc.auditDeny(ctx, unit, operator, perr, meta)
			return nil, perr
		}
		sent[rt.Target.Platform] = struct{}{}
	}

	// 7) 回读全量回执（既有 + 本轮补发），取首个平台帖回写 unit 便捷展示字段。
	receipts, err := uc.receipts.ListReceiptsByUnit(ctx, unitID)
	if err != nil {
		return nil, err
	}
	var firstPostID, firstPostURL *string
	for _, r := range receipts {
		if r.PostID != nil {
			firstPostID = r.PostID
			firstPostURL = r.PostURL
			break
		}
	}

	// 8) 置 published + 操作者/时间（approval_state 保持 approved，契约审计字段不回写）。
	now = uc.clock()
	pubState := "published"
	patch := UnitPatch{
		PublishState: &pubState,
		Operator:     strPtr(operator.ID),
		PublishedAt:  &now,
		PostID:       firstPostID,
		PostURL:      firstPostURL,
	}
	unit, err = uc.dispatch.UpdateStates(ctx, unitID, patch)
	if err != nil {
		return nil, err
	}

	// 9) 审计（allow）。
	recordAudit(ctx, uc.records, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionPublish,
		Actor:              operator.ID,
		ActorType:          string(operator.Type),
		FromState:          strPtr("publishing"),
		ToState:            strPtr("published"),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "allow",
	}, uc.log)

	// TODO(M4/事件接线): 发布 mm.map.feedback.v1 事件回流表现抓取（Phase 3）。
	return &PublishOutcome{Unit: unit, Receipts: receipts, Warnings: warnings}, nil
}

// resolveTargets 解析目标并指派账号（第一个可用账号；全部不可用 → ErrAccountPaused）。
func (uc *PublishUsecase) resolveTargets(ctx context.Context, unit *UnitRow) ([]*ResolvedTarget, error) {
	targets, err := unit.ParsedTargets()
	if err != nil {
		return nil, err
	}
	now := uc.clock()
	out := make([]*ResolvedTarget, 0, len(targets))
	for _, t := range targets {
		ch, err := uc.channels.GetChannelBySlot(ctx, t.Platform, t.Slot)
		if err != nil {
			return nil, fmt.Errorf("%w: %s", ErrChannelNotFound, t)
		}
		if !ch.Enabled {
			return nil, fmt.Errorf("%w: 渠道 %s 已停用", ErrChannelNotFound, t)
		}
		accounts, err := uc.accounts.ListAccountsByChannel(ctx, ch.ChannelID)
		if err != nil {
			return nil, err
		}
		var picked *AccountRow
		for _, a := range accounts {
			if Usable(a, now) {
				picked = a
				picked.PlatformKey = ch.PlatformKey
				picked.Slot = ch.Slot
				break
			}
		}
		if picked == nil {
			return nil, fmt.Errorf("%w: 渠道 %s 无可用账号（全部停用/暂停/凭据失效）", algopublisher.ErrAccountPaused, t)
		}
		out = append(out, &ResolvedTarget{Target: t, Channel: ch, Account: picked})
	}
	return out, nil
}

// firstQuotaError 汇总配额结论（命中第一项即返回，包装消息含具体命中）。
func (uc *PublishUsecase) firstQuotaError(ctx context.Context, resolved []*ResolvedTarget) error {
	checked := make(map[string]struct{}, len(resolved))
	for _, rt := range resolved {
		key := rt.Target.Platform + "/" + rt.Account.AccountID
		if _, ok := checked[key]; ok {
			continue
		}
		checked[key] = struct{}{}
		if err := uc.quota.Check(ctx, rt.Target.Platform, rt.Account); err != nil {
			return err
		}
	}
	return nil
}

// publishOne 单目标发布：适配器 → 凭据解密（用完即清）→ 文案/规格降级 → 平台调用 → 回执 + 熔断。
func (uc *PublishUsecase) publishOne(ctx context.Context, unit *UnitRow, rt *ResolvedTarget, operator Actor, warnings *[]string) error {
	pub, ok := uc.adapters.Get(rt.Target.Platform)
	if !ok {
		return fmt.Errorf("%w: 平台 %s 适配器未注册", ErrAdapterNotFound, rt.Target.Platform)
	}

	// 凭据（🔴 解密使用 + 用完清零；Secret 不进任何日志）。
	var cred *Credential
	if rt.Account.HasCredential && rt.Account.CredentialRef != nil {
		var lerr error
		cred, lerr = uc.vault.Load(ctx, *rt.Account.CredentialRef)
		if lerr != nil {
			return fmt.Errorf("%w: 凭据解密失败（account=%s）", ErrCredentialInvalid, rt.Account.AccountID)
		}
		defer cred.Zero()
	}

	// 文案降级（V1 文案源未接 MME VariantSet：占位空文案 + 诚实警告；联调后替换）。
	// TODO(MME 联调): 从 unit.VariantID 取 copy[platform] 喂入。
	title, desc := "", ""
	var tags []string
	cap := CapabilityOf(rt.Channel, pub)
	rw := RewriteForPlatform(cap, title, desc, tags)
	*warnings = append(*warnings, withPlatform(rt.Target.Platform, rw.Warnings)...)
	spec := CheckMediaSpec(cap, 0, 0, "") // V1 媒体元数据未接入 → 传未知跳过（TODO MME 联调）
	*warnings = append(*warnings, withPlatform(rt.Target.Platform, spec.Warnings)...)

	pubReq := PublishRequest{
		Account:      rt.Account,
		Credential:   cred,
		Title:        rw.Title,
		Desc:         rw.Desc,
		Tags:         rw.Tags,
		Visibility:   unit.VisibilityDefault,
		ScheduledFor: unit.ScheduledFor,
		// VideoPath/CoverPath: V1 媒体工件路径未接入（MME 联调 TODO）。
	}
	res, err := pub.Publish(ctx, pubReq)
	if err != nil {
		sanitized := SanitizeUpstreamError(err)
		if errors.Is(err, ErrCredentialInvalid) {
			if berr := uc.breaker.MarkCredentialInvalid(ctx, rt.Account.AccountID, sanitized); berr != nil {
				uc.log.Errorf("[map-publish] 凭据失效标记失败 account=%s err=%v", rt.Account.AccountID, berr)
			}
			return fmt.Errorf("%w: 平台 %s 凭据失效，账号已停用待重新授权", ErrCredentialInvalid, rt.Target.Platform)
		}
		if berr := uc.breaker.OnFailure(ctx, rt.Account, sanitized); berr != nil {
			uc.log.Errorf("[map-publish] 熔断记账失败 account=%s err=%v", rt.Account.AccountID, berr)
		}
		if alerted, cerr := uc.breaker.CheckChannelContagion(ctx, rt.Channel.ChannelID); cerr != nil {
			uc.log.Errorf("[map-publish] 渠道熔断检查失败 channel=%s err=%v", rt.Channel.ChannelID, cerr)
		} else if alerted {
			*warnings = append(*warnings, fmt.Sprintf("[%s] 渠道账号大面积异常，已触发渠道级熔断（近似判定）", rt.Target.Platform))
		}
		return fmt.Errorf("%w: 平台 %s 发布失败: %s", ErrUpstream, rt.Target.Platform, sanitized)
	}

	// 成功 → 回执（request_payload_hash 为审计线索——防篡改存证，🔴 哈希输入绝不含凭据；
	// 唯一索引冲突=幂等重放，按成功处理）。
	receipt := &ReceiptRow{
		ReceiptID:          mediacontract.NewPublishReceiptID(),
		DispatchUnitID:     unit.UnitID,
		AccountID:          strPtr(rt.Account.AccountID),
		PlatformKey:        rt.Target.Platform,
		Visibility:         res.Visibility,
		Operator:           operator.ID,
		RequestPayloadHash: strPtr(PayloadHash(pubReq)),
	}
	if receipt.Visibility == "" {
		receipt.Visibility = unit.VisibilityDefault
	}
	if res.PostID != "" {
		receipt.PostID = strPtr(res.PostID)
	}
	if res.PostURL != "" {
		receipt.PostURL = strPtr(res.PostURL)
	}
	if !res.PublishedAt.IsZero() {
		receipt.PublishedAt = res.PublishedAt.UTC()
	} else {
		receipt.PublishedAt = uc.clock().UTC()
	}
	if cerr := uc.receipts.CreateReceipt(ctx, receipt); cerr != nil {
		if errors.Is(cerr, ErrReceiptConflict) {
			uc.log.Warnf("[map-publish] 回执唯一索引命中（幂等重放） unit=%s platform=%s", unit.UnitID, rt.Target.Platform)
		} else {
			return fmt.Errorf("%w: 回执落库失败: %v", ErrInternal, cerr)
		}
	}
	if berr := uc.breaker.OnSuccess(ctx, rt.Account.AccountID); berr != nil {
		uc.log.Errorf("[map-publish] 熔断复位失败 account=%s err=%v", rt.Account.AccountID, berr)
	}
	if terr := uc.accounts.TouchLastPublished(ctx, rt.Account.AccountID, uc.clock()); terr != nil {
		uc.log.Warnf("[map-publish] 刷新 last_published_at 失败 account=%s err=%v", rt.Account.AccountID, terr)
	}
	return nil
}

// auditDeny 发布侧越权/违规留痕。
func (uc *PublishUsecase) auditDeny(ctx context.Context, unit *UnitRow, operator Actor, cause error, meta AuditMeta) {
	uc.log.Warnf("[map-publish] deny unit=%s operator=%s(%s) cause=%v", unit.UnitID, operator.ID, operator.Type, cause)
	recordAudit(ctx, uc.records, &RecordEntry{
		DispatchUnitID:     unit.UnitID,
		Action:             AuditActionPublish,
		Actor:              operator.ID,
		ActorType:          string(operator.Type),
		FromState:          strPtr(unit.ApprovalState),
		VisibilityAtAction: strPtr(unit.VisibilityDefault),
		IP:                 strPtrOrNil(meta.IP),
		UserAgent:          strPtrOrNil(meta.UserAgent),
		Result:             "deny",
	}, uc.log)
}

// PayloadHash 审计线索：发布请求体的 sha256（存 receipt.request_payload_hash）。
func PayloadHash(req PublishRequest) string {
	// 🔴 哈希内容绝不包含凭据（Secret 不进哈希输入，防离线反推）。
	clone := struct {
		Platform   string     `json:"platform"`
		AccountID  string     `json:"account_id"`
		Title      string     `json:"title"`
		Desc       string     `json:"desc"`
		Tags       []string   `json:"tags"`
		Visibility string     `json:"visibility"`
		Scheduled  *time.Time `json:"scheduled_for"`
	}{
		Platform:   req.Account.PlatformKey,
		AccountID:  req.Account.AccountID,
		Title:      req.Title,
		Desc:       req.Desc,
		Tags:       req.Tags,
		Visibility: req.Visibility,
		Scheduled:  req.ScheduledFor,
	}
	b, _ := json.Marshal(clone)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func withPlatform(platform string, warnings []string) []string {
	out := make([]string, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, fmt.Sprintf("[%s] %s", platform, w))
	}
	return out
}

// SanitizeUpstreamError 上游错误脱敏（适配器契约：消息不含凭据/堆栈原文；
// 此处再做一次保守截断，防止超长 dump 刷屏与夹带）。
func SanitizeUpstreamError(err error) string {
	const maxLen = 200
	msg := err.Error()
	runes := []rune(msg)
	if len(runes) > maxLen {
		msg = string(runes[:maxLen]) + "…"
	}
	return msg
}
