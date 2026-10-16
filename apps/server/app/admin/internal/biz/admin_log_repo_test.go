package biz

import (
	"context"
	"testing"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
)

// 日志手动清理（Clean）单测：keep_days 校验 + cutoff 透传，fake 手写（与 api_resource_test.go 同风格）。

type fakeAdminLoginLogRepo struct {
	cutoff  time.Time
	deleted int
	err     error
}

func (f *fakeAdminLoginLogRepo) List(context.Context, *pagination.PagingRequest) (*adminV1.ListAdminLoginLogResponse, error) {
	return &adminV1.ListAdminLoginLogResponse{}, nil
}
func (f *fakeAdminLoginLogRepo) Get(context.Context, *adminV1.GetAdminLoginLogRequest) (*adminV1.AdminLoginLog, error) {
	return &adminV1.AdminLoginLog{}, nil
}
func (f *fakeAdminLoginLogRepo) DeleteOlderThan(_ context.Context, cutoff time.Time) (int, error) {
	f.cutoff = cutoff
	return f.deleted, f.err
}

type fakeAdminOperationLogRepo struct {
	cutoff  time.Time
	deleted int
	err     error
}

func (f *fakeAdminOperationLogRepo) List(context.Context, *pagination.PagingRequest) (*adminV1.ListAdminOperationLogResponse, error) {
	return &adminV1.ListAdminOperationLogResponse{}, nil
}
func (f *fakeAdminOperationLogRepo) Get(context.Context, *adminV1.GetAdminOperationLogRequest) (*adminV1.AdminOperationLog, error) {
	return &adminV1.AdminOperationLog{}, nil
}
func (f *fakeAdminOperationLogRepo) RecordAudit(context.Context, *AuditEntry) error { return nil }
func (f *fakeAdminOperationLogRepo) DeleteOlderThan(_ context.Context, cutoff time.Time) (int, error) {
	f.cutoff = cutoff
	return f.deleted, f.err
}

func TestAdminLoginLogUsecase_Clean_Validation(t *testing.T) {
	repo := &fakeAdminLoginLogRepo{}
	uc := NewAdminLoginLogUsecase(repo, log.DefaultLogger)

	for _, days := range []uint32{0, 3651} {
		if _, err := uc.Clean(context.Background(), &adminV1.CleanAdminLoginLogRequest{KeepDays: days}); err == nil {
			t.Fatalf("keep_days=%d should be rejected", days)
		}
	}
}

func TestAdminOperationLogUsecase_Clean_Validation(t *testing.T) {
	repo := &fakeAdminOperationLogRepo{}
	uc := NewAdminOperationLogUsecase(repo, nil, log.DefaultLogger)

	for _, days := range []uint32{0, 3651} {
		if _, err := uc.Clean(context.Background(), &adminV1.CleanAdminOperationLogRequest{KeepDays: days}); err == nil {
			t.Fatalf("keep_days=%d should be rejected", days)
		}
	}
}

func TestAdminOperationLogUsecase_Clean_Passthrough(t *testing.T) {
	repo := &fakeAdminOperationLogRepo{deleted: 42}
	uc := NewAdminOperationLogUsecase(repo, nil, log.DefaultLogger)

	before := time.Now().Add(-30 * 24 * time.Hour)
	resp, err := uc.Clean(context.Background(), &adminV1.CleanAdminOperationLogRequest{KeepDays: 30})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
	if resp.GetDeleted() != 42 {
		t.Fatalf("deleted=%d, want 42", resp.GetDeleted())
	}
	// cutoff 应落在 now-30d 前后一个执行窗口内
	if repo.cutoff.Before(before.Add(-time.Minute)) || repo.cutoff.After(time.Now().Add(-30*24*time.Hour).Add(time.Minute)) {
		t.Fatalf("cutoff %s not near now-30d", repo.cutoff)
	}
}

func TestAdminLoginLogUsecase_Clean_Passthrough(t *testing.T) {
	repo := &fakeAdminLoginLogRepo{deleted: 7}
	uc := NewAdminLoginLogUsecase(repo, log.DefaultLogger)

	resp, err := uc.Clean(context.Background(), &adminV1.CleanAdminLoginLogRequest{KeepDays: 90})
	if err != nil {
		t.Fatalf("Clean failed: %v", err)
	}
	if resp.GetDeleted() != 7 {
		t.Fatalf("deleted=%d, want 7", resp.GetDeleted())
	}
	if repo.cutoff.IsZero() {
		t.Fatal("DeleteOlderThan was not called")
	}
}
