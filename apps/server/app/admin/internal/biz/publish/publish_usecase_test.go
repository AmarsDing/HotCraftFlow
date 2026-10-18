package publish

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
	"framework/pkg/mediacontract"
)

// publishFixture 发布编排测试夹具。
type publishFixture struct {
	conf     Conf
	channels *fakeChannels
	accounts *fakeAccounts
	dispatch *fakeDispatch
	receipts *fakeReceipts
	records  *fakeRecords
	vault    *fakeVault
	locker   *fakeLocker
	ads      *fakeAdapters
	breaker  *BreakerUsecase
	quota    *QuotaUsecase
}

// approvedUnit 已审批单元（submitted_by=alice / approver=bob，🔴 异人）。
func approvedUnit(unitID string, targets ...string) *UnitRow {
	now := time.Now()
	return &UnitRow{
		UnitID:            unitID,
		Targets:           targets,
		ApprovalState:     string(mediacontract.ApprovalApproved),
		SubmittedBy:       "alice",
		Approver:          strPtr("bob"),
		ApprovedAt:        &now,
		PublishState:      "pending",
		VisibilityDefault: string(mediacontract.VisibilityPrivate),
		ScheduleMode:      "none",
	}
}

func newPublishFixture(t *testing.T, unit *UnitRow) *publishFixture {
	t.Helper()
	fx := &publishFixture{conf: DefaultConf()}
	fx.channels = newFakeChannels(
		&ChannelRow{ChannelID: "ch_yt", PlatformKey: "youtube", Slot: "main", Enabled: true},
		&ChannelRow{ChannelID: "ch_bili", PlatformKey: "bilibili", Slot: "main", Enabled: true},
		&ChannelRow{ChannelID: "ch_rd", PlatformKey: "reddit", Slot: "main", Enabled: true},
	)
	fx.accounts = newFakeAccounts(
		&AccountRow{AccountID: "pa_yt1", ChannelID: "ch_yt", Enabled: true, Health: AccountHealthHealthy},
		&AccountRow{AccountID: "pa_bili1", ChannelID: "ch_bili", Enabled: true, Health: AccountHealthHealthy},
		&AccountRow{AccountID: "pa_rd1", ChannelID: "ch_rd", Enabled: true, Health: AccountHealthHealthy},
	)
	fx.dispatch = newFakeDispatch(unit)
	fx.receipts = newFakeReceipts()
	fx.records = &fakeRecords{}
	fx.vault = newFakeVault()
	fx.locker = newFakeLocker()
	fx.ads = &fakeAdapters{pubs: map[string]Publisher{}}
	fx.breaker = NewBreakerUsecase(&fx.conf, fx.accounts, log.DefaultLogger)
	fx.quota = NewQuotaUsecase(&fx.conf, fx.receipts, log.DefaultLogger)
	return fx
}

func (fx *publishFixture) usecase() *PublishUsecase {
	return NewPublishUsecase(&fx.conf, fx.dispatch, fx.accounts, fx.channels,
		fx.receipts, fx.records, fx.vault, fx.locker, fx.ads, fx.breaker, fx.quota, log.DefaultLogger)
}

// Happy path：双 target 串行发布（顺序可预期）→ published + 2 回执 + 操作者落库 + allow 审计。
func TestPublish_HappyPath_SerialTargets(t *testing.T) {
	unit := approvedUnit("du_p1", "youtube:main", "bilibili:main")
	fx := newPublishFixture(t, unit)
	var mu sync.Mutex
	order := []string{}
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", &order, &mu)
	fx.ads.pubs["bilibili"] = newSuccessPublisher("bilibili", &order, &mu)

	out, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{IP: "9.9.9.9"})
	if err != nil {
		t.Fatalf("发布失败: %v", err)
	}
	if out.Unit.PublishState != "published" {
		t.Fatalf("应 published，实际 %s", out.Unit.PublishState)
	}
	if len(out.Receipts) != 2 {
		t.Fatalf("应有 2 条回执，实际 %d", len(out.Receipts))
	}
	if len(order) != 2 || order[0] != "youtube" || order[1] != "bilibili" {
		t.Fatalf("targets 必须串行有序: %v", order)
	}
	if out.Unit.Operator == nil || *out.Unit.Operator != "carol" {
		t.Fatalf("操作者未落库: %+v", out.Unit.Operator)
	}
	if out.Unit.PostID == nil || *out.Unit.PostID != "post_youtube" {
		t.Fatalf("首条回执 post_id 应回写单元: %+v", out.Unit.PostID)
	}
	// 回执可见性继承 private（NFR-MAP-01）。
	for _, r := range out.Receipts {
		if r.Visibility != "private" {
			t.Fatalf("回执可见性应 private，实际 %q", r.Visibility)
		}
		// 🔴 审计存证：request_payload_hash 必填（防篡改；哈希输入绝不含凭据）。
		if r.RequestPayloadHash == nil || len(*r.RequestPayloadHash) != 64 {
			t.Fatalf("回执缺 request_payload_hash: %+v", r)
		}
	}
	if fx.records.countByResult("allow") != 1 {
		t.Fatalf("应有 1 条 publish allow 审计，实际 %d", fx.records.countByResult("allow"))
	}
	// 成功复位与 last_published 刷新。
	if fx.accounts.byID["pa_yt1"].LastPublishedAt == nil {
		t.Fatal("last_published_at 未刷新")
	}
	// 锁已释放（defer Unlock）。
	if fx.locker.unlocks != 1 {
		t.Fatalf("锁应已释放，unlocks=%d", fx.locker.unlocks)
	}
}

// 🔴 未审批发布必败（整链口径）：draft 单元直接 Publish → MAP_NOT_APPROVED + deny 审计。
func TestPublish_NotApproved_Denied(t *testing.T) {
	unit := draftUnit("du_p2", "alice")
	fx := newPublishFixture(t, unit)
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("bob"), false, AuditMeta{})
	if !errors.Is(err, algopublisher.ErrNotApproved) {
		t.Fatalf("期望 MAP_NOT_APPROVED，实际 %v", err)
	}
	if fx.records.countByResult("deny") != 1 {
		t.Fatalf("应有 deny 审计，实际 %d", fx.records.countByResult("deny"))
	}
	if fx.receipts.createCalls != 0 {
		t.Fatalf("未审批不得产生回执: %d", fx.receipts.createCalls)
	}
}

// 🔴 非 private 必须二次确认：confirm_public=false → MAP_PUBLIC_CONFIRM_REQUIRED。
func TestPublish_NonPrivateRequiresConfirm(t *testing.T) {
	unit := approvedUnit("du_p3", "youtube:main")
	unit.VisibilityDefault = string(mediacontract.VisibilityUnlisted)
	unit.VisibilityOverrideReason = strPtr("品牌预热不公开索引")
	fx := newPublishFixture(t, unit)
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, ErrPublicConfirmRequired) {
		t.Fatalf("期望 MAP_PUBLIC_CONFIRM_REQUIRED，实际 %v", err)
	}
	// 确认后放行。
	out, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), true, AuditMeta{})
	if err != nil {
		t.Fatalf("二次确认后应放行: %v", err)
	}
	if out.Unit.PublishState != "published" {
		t.Fatalf("应 published，实际 %s", out.Unit.PublishState)
	}
}

// 幂等 1：已 published → 快速路径直返既有回执（不再调用适配器/不拿锁）。
func TestPublish_Idempotent_AlreadyPublished(t *testing.T) {
	unit := approvedUnit("du_p4", "youtube:main")
	unit.PublishState = "published"
	fx := newPublishFixture(t, unit)
	p := newSuccessPublisher("youtube", nil, nil)
	fx.ads.pubs["youtube"] = p
	existing := &ReceiptRow{ReceiptID: "pr_exist", DispatchUnitID: unit.UnitID, PlatformKey: "youtube", Visibility: "private", Operator: "bob", PublishedAt: time.Now()}
	fx.receipts.CreateReceipt(context.Background(), existing)

	out, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if err != nil {
		t.Fatalf("幂等重放不应报错: %v", err)
	}
	if len(out.Receipts) != 1 || out.Receipts[0].ReceiptID != "pr_exist" {
		t.Fatalf("应返回既有回执: %+v", out.Receipts)
	}
	if fx.locker.locks != 0 {
		t.Fatalf("快速路径不应拿锁: locks=%d", fx.locker.locks)
	}
}

// 幂等 2：锁被占 → MAP_ALREADY_PUBLISHING（并发点击/重放拦截）。
func TestPublish_Idempotent_LockContention(t *testing.T) {
	unit := approvedUnit("du_p5", "youtube:main")
	fx := newPublishFixture(t, unit)
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)
	fx.locker.denyAll = true // 模拟并发持锁

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, ErrAlreadyPublishing) {
		t.Fatalf("期望 MAP_ALREADY_PUBLISHING，实际 %v", err)
	}
	if fx.receipts.createCalls != 0 {
		t.Fatalf("锁竞争不得发布: createCalls=%d", fx.receipts.createCalls)
	}
}

// 幂等 3：回执唯一索引冲突 → 按幂等成功处理（DB 双保险）。
func TestPublish_Idempotent_ReceiptConflictAsSuccess(t *testing.T) {
	unit := approvedUnit("du_p6", "youtube:main")
	fx := newPublishFixture(t, unit)
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)
	fx.receipts.conflictOnCreate = true // 模拟并发下另一请求已落回执

	out, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if err != nil {
		t.Fatalf("回执冲突应按幂等成功: %v", err)
	}
	if out.Unit.PublishState != "published" {
		t.Fatalf("应 published，实际 %s", out.Unit.PublishState)
	}
}

// 🔴 适配器边界幂等：A 成功 B 失败 → 修复后重试，A 平台不得重发（外部发帖不可逆，
// DB 唯一索引只是兜底，业务判定必须用既有回执跳过）。
func TestPublish_RetrySkipsSentPlatform(t *testing.T) {
	unit := approvedUnit("du_p12", "youtube:main", "bilibili:main")
	fx := newPublishFixture(t, unit)
	var mu sync.Mutex
	order := []string{}
	yt := newSuccessPublisher("youtube", &order, &mu)
	bili := newSuccessPublisher("bilibili", &order, &mu)
	bili.errOnPublish = errors.New("upstream boom")
	fx.ads.pubs["youtube"] = yt
	fx.ads.pubs["bilibili"] = bili

	// 首跑：youtube 成功落回执 → bilibili 失败（unit failed，可人工重发）。
	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, ErrUpstream) {
		t.Fatalf("首跑应 ErrUpstream，实际 %v", err)
	}
	if fx.receipts.createCalls != 1 {
		t.Fatalf("首跑应仅 1 条回执（youtube），实际 %d", fx.receipts.createCalls)
	}

	// 修复后重试：youtube 已有回执必须跳过，只补 bilibili。
	bili.errOnPublish = nil
	out, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if err != nil {
		t.Fatalf("重试应成功: %v", err)
	}
	if len(order) != 3 || order[2] != "bilibili" {
		t.Fatalf("重试不得重发已成平台（发帖不可逆）: %v", order)
	}
	if len(out.Receipts) != 2 {
		t.Fatalf("回执应补满 2 条，实际 %d", len(out.Receipts))
	}
	if out.Unit.PublishState != "published" {
		t.Fatalf("补发后应 published，实际 %s", out.Unit.PublishState)
	}
}

// 🔴 锁内复核审批态：等锁期间审批被并发撤回（approved→draft）→ 锁后重门检拒发。
func TestPublish_LockRecheck_DeniesRevokedApproval(t *testing.T) {
	unit := approvedUnit("du_p13", "youtube:main")
	fx := newPublishFixture(t, unit)
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)
	// onLock 钩子在每次 GetUnit 前触发（GetUnit 持锁中调用，钩子内不得再走
	// UpdateStates——sync.Mutex 非重入会死锁，直改存储行即可；第 1 次 GetUnit 是
	// 门检前取单元（正常 approved），第 2 次是锁后重读——此处模拟并发撤回）。
	getCalls := 0
	fx.dispatch.onLock = func() {
		getCalls++
		if getCalls == 2 {
			fx.dispatch.units[unit.UnitID].ApprovalState = string(mediacontract.ApprovalDraft)
		}
	}

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, algopublisher.ErrNotApproved) {
		t.Fatalf("撤回后锁内复核应拒发（MAP_NOT_APPROVED），实际 %v", err)
	}
	if fx.receipts.createCalls != 0 {
		t.Fatalf("拒发不得产生回执: %d", fx.receipts.createCalls)
	}
	if fx.locker.unlocks != 1 {
		t.Fatalf("defer 应释放锁: unlocks=%d", fx.locker.unlocks)
	}
}

func TestPublish_AdapterMissing(t *testing.T) {
	unit := approvedUnit("du_p7", "youtube:main")
	fx := newPublishFixture(t, unit) // youtube 无适配器注册

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, ErrAdapterNotFound) {
		t.Fatalf("期望 MAP_ADAPTER_NOT_FOUND，实际 %v", err)
	}
	got, _ := fx.dispatch.GetUnit(context.Background(), unit.UnitID)
	if got.PublishState != "failed" {
		t.Fatalf("失败应置 failed，实际 %s", got.PublishState)
	}
}

// 平台失败 → breaker 记账 + unit failed + ErrUpstream；凭据失效 → credential_invalid + 凭据保留。
func TestPublish_FailurePaths(t *testing.T) {
	t.Run("普通失败熔断记账", func(t *testing.T) {
		unit := approvedUnit("du_p8", "youtube:main")
		fx := newPublishFixture(t, unit)
		p := newSuccessPublisher("youtube", nil, nil)
		p.errOnPublish = errors.New("upstream 500 boom")
		fx.ads.pubs["youtube"] = p

		_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
		if !errors.Is(err, ErrUpstream) {
			t.Fatalf("期望 MAP_UPSTREAM_ERROR，实际 %v", err)
		}
		if fx.accounts.byID["pa_yt1"].ConsecutiveFailures != 1 {
			t.Fatalf("熔断记账应 +1，实际 %d", fx.accounts.byID["pa_yt1"].ConsecutiveFailures)
		}
		got, _ := fx.dispatch.GetUnit(context.Background(), unit.UnitID)
		if got.PublishState != "failed" {
			t.Fatalf("unit 应 failed，实际 %s", got.PublishState)
		}
	})

	t.Run("凭据失效停号留凭据", func(t *testing.T) {
		unit := approvedUnit("du_p9", "youtube:main")
		fx := newPublishFixture(t, unit)
		// 有凭据的账号
		ref, _ := fx.vault.Store(context.Background(), "pa_yt1", &Credential{Type: "oauth", Secret: []byte("SECRET-DO-NOT-LEAK")})
		acct := fx.accounts.byID["pa_yt1"]
		acct.HasCredential = true
		acct.CredentialRef = &ref
		p := newSuccessPublisher("youtube", nil, nil)
		p.errOnPublish = ErrCredentialInvalid
		fx.ads.pubs["youtube"] = p

		_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
		if !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("期望 MAP_CREDENTIAL_INVALID，实际 %v", err)
		}
		if acct.Health != AccountHealthCredentialInvalid {
			t.Fatalf("应 credential_invalid，实际 %s", acct.Health)
		}
		if acct.CredentialRef == nil || *acct.CredentialRef != ref {
			t.Fatal("🔴 凭据失效不得清凭据引用")
		}
		if fx.vault.deletes != 0 {
			t.Fatalf("🔴 vault.Delete 被调用: %d", fx.vault.deletes)
		}
		// 脱敏：返回消息不得含原始 SECRET。
		if err != nil && containsSecret(err.Error()) {
			t.Fatalf("🔴 错误消息泄漏凭据: %s", err.Error())
		}
	})
}

// 账号全部不可用 → MAP_ACCOUNT_PAUSED（熔断结论喂门校验）。
func TestPublish_AllAccountsPaused(t *testing.T) {
	unit := approvedUnit("du_p10", "youtube:main")
	fx := newPublishFixture(t, unit)
	pausedUntil := time.Now().Add(time.Hour)
	fx.accounts.byID["pa_yt1"].Health = AccountHealthPaused
	fx.accounts.byID["pa_yt1"].PausedUntil = &pausedUntil
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, algopublisher.ErrAccountPaused) {
		t.Fatalf("期望 MAP_ACCOUNT_PAUSED，实际 %v", err)
	}
}

// 配额超限 → MAP_QUOTA_EXCEEDED（门校验交换出具体配额消息）。
func TestPublish_QuotaExceeded(t *testing.T) {
	unit := approvedUnit("du_p11", "youtube:main")
	fx := newPublishFixture(t, unit)
	fx.receipts.totalCount = fx.conf.Quota.DailyPublishLimit
	fx.ads.pubs["youtube"] = newSuccessPublisher("youtube", nil, nil)

	_, err := fx.usecase().Publish(context.Background(), unit.UnitID, NewHumanActor("carol"), false, AuditMeta{})
	if !errors.Is(err, algopublisher.ErrQuotaExceeded) {
		t.Fatalf("期望 MAP_QUOTA_EXCEEDED，实际 %v", err)
	}
	if fx.records.countByResult("deny") != 1 {
		t.Fatalf("配额拒绝应留 deny 审计，实际 %d", fx.records.countByResult("deny"))
	}
}

func containsSecret(s string) bool {
	return strings.Contains(s, "SECRET-DO-NOT-LEAK")
}
