package publish

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algopublisher "framework/app/admin/internal/algo/publisher"
)

// 24h 总量硬上限 → MAP_QUOTA_EXCEEDED（NFR-MAP-06，验收 §8 配额项）。
func TestQuota_DailyLimit(t *testing.T) {
	conf := DefaultConf() // daily 100
	receipts := newFakeReceipts()
	receipts.totalCount = conf.Quota.DailyPublishLimit // 已达上限
	quota := NewQuotaUsecase(&conf, receipts, log.DefaultLogger)

	err := quota.Check(context.Background(), "youtube", &AccountRow{AccountID: "pa_1"})
	if !errors.Is(err, algopublisher.ErrQuotaExceeded) {
		t.Fatalf("期望 MAP_QUOTA_EXCEEDED，实际 %v", err)
	}
}

// 平台小时上限 → MAP_QUOTA_EXCEEDED。
func TestQuota_PlatformHourlyLimit(t *testing.T) {
	conf := DefaultConf() // platform hourly 20
	receipts := newFakeReceipts()
	receipts.platformCnt["youtube"] = conf.Quota.PlatformHourlyLimit
	quota := NewQuotaUsecase(&conf, receipts, log.DefaultLogger)

	err := quota.Check(context.Background(), "youtube", &AccountRow{AccountID: "pa_1"})
	if !errors.Is(err, algopublisher.ErrQuotaExceeded) {
		t.Fatalf("期望 MAP_QUOTA_EXCEEDED，实际 %v", err)
	}
	// 其他平台不受影响。
	if err := quota.Check(context.Background(), "bilibili", &AccountRow{AccountID: "pa_1"}); err != nil {
		t.Fatalf("其他平台不该被限: %v", err)
	}
}

// 账号最小间隔 → MAP_QUOTA_EXCEEDED；间隔已过 → 放行。
func TestQuota_AccountMinInterval(t *testing.T) {
	conf := DefaultConf() // 5m
	receipts := newFakeReceipts()
	quota := NewQuotaUsecase(&conf, receipts, log.DefaultLogger)

	recent := time.Now().Add(-2 * time.Minute) // 仅 2m 前发过
	err := quota.Check(context.Background(), "youtube", &AccountRow{AccountID: "pa_1", LastPublishedAt: &recent})
	if !errors.Is(err, algopublisher.ErrQuotaExceeded) {
		t.Fatalf("期望 MAP_QUOTA_EXCEEDED，实际 %v", err)
	}

	old := time.Now().Add(-10 * time.Minute)
	if err := quota.Check(context.Background(), "youtube", &AccountRow{AccountID: "pa_1", LastPublishedAt: &old}); err != nil {
		t.Fatalf("间隔已过应放行: %v", err)
	}
	// 从未发过 → 放行。
	if err := quota.Check(context.Background(), "youtube", &AccountRow{AccountID: "pa_1"}); err != nil {
		t.Fatalf("新账号应放行: %v", err)
	}
}

// 均未触限 → 放行（happy path 基线）。
func TestQuota_AllClear(t *testing.T) {
	conf := DefaultConf()
	receipts := newFakeReceipts()
	receipts.totalCount = 3
	receipts.platformCnt["youtube"] = 1
	quota := NewQuotaUsecase(&conf, receipts, log.DefaultLogger)

	if err := quota.Check(context.Background(), "youtube", &AccountRow{AccountID: "pa_1"}); err != nil {
		t.Fatalf("未触限应放行: %v", err)
	}
}
