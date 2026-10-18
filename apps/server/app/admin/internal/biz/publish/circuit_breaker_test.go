package publish

import (
	"context"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
)

// 熔断阶梯：1 次失败带 lastErr；阈值-1 → degraded；阈值 → paused + paused_until≈+30m。
func TestBreaker_FailureLadder(t *testing.T) {
	conf := DefaultConf() // 阈值 3 / 暂停 30m
	acct := &AccountRow{AccountID: "pa_a1", ChannelID: "ch_1", Enabled: true, Health: AccountHealthHealthy}
	accounts := newFakeAccounts(acct)
	breaker := NewBreakerUsecase(&conf, accounts, log.DefaultLogger)
	ctx := context.Background()

	start := time.Now()
	// 第 1 次：健康态不变，仅刷 last_error。
	if err := breaker.OnFailure(ctx, acct, "platform 500"); err != nil {
		t.Fatal(err)
	}
	if acct.Health != AccountHealthHealthy {
		t.Fatalf("1 次失败不该变健康态: %s", acct.Health)
	}
	if acct.LastError == nil || *acct.LastError != "platform 500" {
		t.Fatalf("last_error 未记录: %+v", acct.LastError)
	}

	// 第 2 次（阈值-1）：degraded 预警。
	if err := breaker.OnFailure(ctx, acct, "timeout"); err != nil {
		t.Fatal(err)
	}
	if acct.Health != AccountHealthDegraded {
		t.Fatalf("阈值-1 应 degraded，实际 %s", acct.Health)
	}

	// 第 3 次：paused + 暂停至 ≈ now+30m。
	if err := breaker.OnFailure(ctx, acct, "still failing"); err != nil {
		t.Fatal(err)
	}
	if acct.Health != AccountHealthPaused {
		t.Fatalf("达阈值应 paused，实际 %s", acct.Health)
	}
	if acct.PausedUntil == nil {
		t.Fatal("paused_until 未设置")
	}
	delta := acct.PausedUntil.Sub(start.Add(conf.Breaker.PauseDuration))
	if delta < -5*time.Second || delta > 5*time.Second {
		t.Fatalf("paused_until 应 ≈ +30m，偏差 %v", delta)
	}
}

// 成功复位：失败数清零 + 健康恢复（但已 paused 的账号不由 OnSuccess 擅自复活——
// OnSuccess 仅把 health 置 healthy 是 breaker 语义的一部分，懒惰过期靠 Usable）。
func TestBreaker_OnSuccessResets(t *testing.T) {
	conf := DefaultConf()
	acct := &AccountRow{AccountID: "pa_b1", ChannelID: "ch_1", Enabled: true, Health: AccountHealthDegraded, ConsecutiveFailures: 2}
	accounts := newFakeAccounts(acct)
	breaker := NewBreakerUsecase(&conf, accounts, log.DefaultLogger)

	if err := breaker.OnSuccess(context.Background(), acct.AccountID); err != nil {
		t.Fatal(err)
	}
	if acct.ConsecutiveFailures != 0 || acct.Health != AccountHealthHealthy {
		t.Fatalf("复位失败: failures=%d health=%s", acct.ConsecutiveFailures, acct.Health)
	}
}

// 🔴 凭据失效：health=credential_invalid 且凭据引用原样保留（不清凭据铁律）。
func TestBreaker_CredentialInvalid_KeepsCredential(t *testing.T) {
	conf := DefaultConf()
	ref := "enc:v1:AAAA"
	acct := &AccountRow{AccountID: "pa_c1", ChannelID: "ch_1", Enabled: true,
		Health: AccountHealthHealthy, CredentialRef: &ref, HasCredential: true}
	accounts := newFakeAccounts(acct)
	vault := newFakeVault()
	breaker := NewBreakerUsecase(&conf, accounts, log.DefaultLogger)

	if err := breaker.MarkCredentialInvalid(context.Background(), acct.AccountID, "token expired"); err != nil {
		t.Fatal(err)
	}
	if acct.Health != AccountHealthCredentialInvalid {
		t.Fatalf("应 credential_invalid，实际 %s", acct.Health)
	}
	if acct.CredentialRef == nil || *acct.CredentialRef != ref {
		t.Fatalf("🔴 凭据引用被清除（违反铁律）: %+v", acct.CredentialRef)
	}
	if vault.deletes != 0 {
		t.Fatalf("🔴 vault.Delete 被调用（凭据失效铁律禁止）: %d 次", vault.deletes)
	}
	// 凭据失效账号不可用于发布。
	if Usable(acct, time.Now()) {
		t.Fatal("credential_invalid 账号不应可用")
	}
}

// 渠道级熔断近似：enabled 账号暂停占比 > 阈值 → 其余全部暂停。
func TestBreaker_ChannelContagion(t *testing.T) {
	conf := DefaultConf() // 阈值 0.5
	pausedUntil := time.Now().Add(time.Hour)
	a1 := &AccountRow{AccountID: "pa_d1", ChannelID: "ch_yt", Enabled: true, Health: AccountHealthPaused, PausedUntil: &pausedUntil}
	a2 := &AccountRow{AccountID: "pa_d2", ChannelID: "ch_yt", Enabled: true, Health: AccountHealthPaused, PausedUntil: &pausedUntil}
	a3 := &AccountRow{AccountID: "pa_d3", ChannelID: "ch_yt", Enabled: true, Health: AccountHealthHealthy}
	disabled := &AccountRow{AccountID: "pa_d4", ChannelID: "ch_yt", Enabled: false, Health: AccountHealthHealthy}
	accounts := newFakeAccounts(a1, a2, a3, disabled)
	breaker := NewBreakerUsecase(&conf, accounts, log.DefaultLogger)

	alerted, err := breaker.CheckChannelContagion(context.Background(), "ch_yt")
	if err != nil {
		t.Fatal(err)
	}
	if !alerted {
		t.Fatal("2/3 paused > 0.5 应触发渠道熔断")
	}
	if a3.Health != AccountHealthPaused {
		t.Fatalf("传染暂停未生效，a3=%s", a3.Health)
	}
	if disabled.Health != AccountHealthHealthy {
		t.Fatalf("停用账号不应被传染暂停: %s", disabled.Health)
	}
}

// 占比未超阈：不触发。
func TestBreaker_ChannelContagion_NoTriggerBelowThreshold(t *testing.T) {
	conf := DefaultConf()
	pausedUntil := time.Now().Add(time.Hour)
	a1 := &AccountRow{AccountID: "pa_e1", ChannelID: "ch_x", Enabled: true, Health: AccountHealthPaused, PausedUntil: &pausedUntil}
	a2 := &AccountRow{AccountID: "pa_e2", ChannelID: "ch_x", Enabled: true, Health: AccountHealthHealthy}
	a3 := &AccountRow{AccountID: "pa_e3", ChannelID: "ch_x", Enabled: true, Health: AccountHealthHealthy}
	accounts := newFakeAccounts(a1, a2, a3)
	breaker := NewBreakerUsecase(&conf, accounts, log.DefaultLogger)

	alerted, err := breaker.CheckChannelContagion(context.Background(), "ch_x")
	if err != nil || alerted {
		t.Fatalf("1/3 不应触发: alerted=%v err=%v", alerted, err)
	}
	if a2.Health != AccountHealthHealthy || a3.Health != AccountHealthHealthy {
		t.Fatalf("未超阈不该暂停他人: %s/%s", a2.Health, a3.Health)
	}
}

// Usable 判定全矩阵：停用 / 凭据失效 / 熔断中 / 暂停过期（懒惰恢复）/ degraded（可发）。
func TestUsable_Matrix(t *testing.T) {
	now := time.Now()
	future := now.Add(30 * time.Minute)
	past := now.Add(-time.Minute)
	cases := []struct {
		name string
		acct *AccountRow
		want bool
	}{
		{"停用", &AccountRow{Enabled: false, Health: AccountHealthHealthy}, false},
		{"凭据失效", &AccountRow{Enabled: true, Health: AccountHealthCredentialInvalid}, false},
		{"熔断中", &AccountRow{Enabled: true, Health: AccountHealthPaused, PausedUntil: &future}, false},
		{"暂停已过期", &AccountRow{Enabled: true, Health: AccountHealthPaused, PausedUntil: &past}, true},
		{"degraded 可发", &AccountRow{Enabled: true, Health: AccountHealthDegraded}, true},
		{"healthy", &AccountRow{Enabled: true, Health: AccountHealthHealthy}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Usable(tc.acct, now); got != tc.want {
				t.Fatalf("Usable=%v，期望 %v", got, tc.want)
			}
		})
	}
}
