package jobstore

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"
)

// testSchema 本包测试的专属 PG schema。
//
// 为什么需要：go test 默认并行跑不同包的测试二进制，所有 touch PG 的包
// 若共享 public schema 会互相踩（如今真案例：sseapi 包 Start→FailOrphaned
// 把 jobstore 包迁移中途的 running 行判成孤儿 failed，反之亦然——
// 生产单实例纪律下 FailOrphaned 无过滤是正确语义，缺陷只在测试环境）。
// 解法：每包独立 schema 彻底隔离，收尾 DROP CASCADE 一次清干净。
const testSchema = "jobstore_test"

// openTestStore DSN 门控：无 MEDIA_TEST_DSN 一律 skip（CI 无 PG 不红）。
func openTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := os.Getenv("MEDIA_TEST_DSN")
	if dsn == "" {
		t.Skip("MEDIA_TEST_DSN 未设置，跳过 PG 集成测试")
	}
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open admin: %v", err)
	}
	if _, err := admin.ExecContext(t.Context(), `CREATE SCHEMA IF NOT EXISTS `+testSchema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	_ = admin.Close()

	s, err := Open(dsn+" search_path="+testSchema, 16)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := s.EnsureSchema(t.Context()); err != nil {
		t.Fatalf("ensure schema: %v", err)
	}
	t.Cleanup(func() {
		admin, err := sql.Open("pgx", dsn)
		if err == nil {
			_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+testSchema+` CASCADE`)
			_ = admin.Close()
		}
		_ = s.Close()
	})
	return s
}

func TestSubmitIdempotencyAndEventMirror(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()

	j := Job{ID: "job_test_idem_1", Lane: "io", Type: "exec.probe", PayloadJSON: `{"bin":"ffmpeg"}`}
	got, created, err := s.Submit(ctx, j, PayloadFor(map[string]any{"a": 1}))
	if err != nil || !created {
		t.Fatalf("首次提交: created=%v err=%v", created, err)
	}
	if got.State != StateQueued || got.LastSeq != 1 {
		t.Fatalf("首事件镜像: state=%v lastSeq=%d", got.State, got.LastSeq)
	}

	// 同 idempotency_key 重复提交：返回原任务、不重排队。
	// 注意必须用另一个主键 ID——复用旧 ID 只换 key 会撞 media_jobs_pkey。
	j2 := Job{ID: "job_test_idem_2", Lane: "io", Type: "exec.probe",
		PayloadJSON: `{"bin":"ffmpeg"}`, IdempotencyKey: "idem-key-1"}
	first2, createdK, err := s.Submit(ctx, j2, nil)
	if err != nil || !createdK {
		t.Fatalf("带 key 首次提交: createdK=%v err=%v", createdK, err)
	}
	again, created2, err := s.Submit(ctx, j2, nil)
	if err != nil || created2 {
		t.Fatalf("幂等重复: created2=%v err=%v", created2, err)
	}
	if again.ID != first2.ID || again.LastSeq != first2.LastSeq {
		t.Fatalf("幂等命中应返回原任务不新增事件: %s/%d → %s/%d",
			first2.ID, first2.LastSeq, again.ID, again.LastSeq)
	}
}

func TestTransitionGateAndTerminalEvent(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	j, _, err := s.Submit(ctx, Job{ID: "job_test_trans_1", Lane: "infer", Type: "echo.synthesize"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// 非法迁移门禁：queued → done 直接跳（状态机允许迁移序列由调用方保序，
	// 这里验证 from 门禁——从 running 门禁迁移必失败）。
	_, ok, err := s.Transition(ctx, j.ID, Patch{
		From: []State{StateRunning}, To: StateDone,
	}, EvDone, nil)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("from 门禁失效：queued 状态下不应从 running 迁移成功")
	}

	// 正常链路：queued→running→done，终态落 terminal 事件。
	done := "{}"
	_, ok, err = s.Transition(ctx, j.ID, Patch{
		From: []State{StateQueued}, To: StateRunning, WorkerID: strPtr("w1"),
	}, EvRunning, nil)
	if err != nil || !ok {
		t.Fatalf("running 迁移: ok=%v err=%v", ok, err)
	}
	prog := 1.0
	fin, ok, err := s.Transition(ctx, j.ID, Patch{
		From: []State{StateRunning}, To: StateDone,
		Progress: &prog, ResultJSON: &done,
	}, EvDone, nil)
	if err != nil || !ok {
		t.Fatalf("done 迁移: ok=%v err=%v", ok, err)
	}
	if fin.FinishedAt == nil {
		t.Fatal("终态必须自动落 finished_at")
	}

	evs, err := s.EventsSince(ctx, j.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []EventKind{EvQueued, EvRunning, EvDone}
	if len(evs) != len(want) {
		t.Fatalf("事件数: got %d want %d", len(evs), len(want))
	}
	for i, k := range want {
		if evs[i].Kind != k || evs[i].Seq != int64(i+1) {
			t.Fatalf("事件序列不符: [%d]=%v(seq=%d)", i, evs[i].Kind, evs[i].Seq)
		}
	}
	// after_seq 回放语义：只给增量。
	rest, err := s.EventsSince(ctx, j.ID, 1, 100)
	if err != nil || len(rest) != 2 {
		t.Fatalf("after_seq=1 回放: %v (%d 条)", err, len(rest))
	}
}

func TestFailOrphanedOnBoot(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	j, _, err := s.Submit(ctx, Job{ID: "job_test_orphan_1", Lane: "io", Type: "exec.probe"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Transition(ctx, j.ID, Patch{
		From: []State{StateQueued}, To: StateRunning,
	}, EvRunning, nil); !ok {
		t.Fatal("setup: 未进入 running")
	}

	n, err := s.FailOrphaned(ctx, "test-worker")
	if err != nil || n < 1 {
		t.Fatalf("FailOrphaned count: n=%d err=%v", n, err)
	}
	after, err := s.Get(ctx, j.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.State != StateFailed || after.ErrorCode != "MEDIA_SERVICE_RESTART" {
		t.Fatalf("孤儿归因: state=%v code=%v", after.State, after.ErrorCode)
	}
}

func TestTrimEventsCap(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	j, _, err := s.Submit(ctx, Job{ID: "job_test_trim_1", Lane: "io", Type: "exec.probe"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Transition(ctx, j.ID, Patch{
		From: []State{StateQueued}, To: StateRunning,
	}, EvRunning, nil); !ok {
		t.Fatal("setup")
	}
	// 灌 40 条进度事件（cap=16），裁剪后应只剩末 16 条。
	for i := 0; i < 40; i++ {
		p := float64(i) / 40
		if _, _, err := s.Transition(ctx, j.ID, Patch{
			From: []State{StateRunning}, To: StateRunning, Progress: &p,
		}, EvProgress, PayloadFor(map[string]any{"i": i})); err != nil {
			t.Fatal(err)
		}
	}
	evs, err := s.EventsSince(ctx, j.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) != 42 {
		t.Fatalf("裁剪前应有 42 条: %d", len(evs))
	}
	if err := s.TrimEvents(ctx, j.ID); err != nil {
		t.Fatal(err)
	}
	evs, err = s.EventsSince(ctx, j.ID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(evs) > 16 {
		t.Fatalf("裁剪后应 ≤16 条: %d", len(evs))
	}
	// seq 连续性允许断点（裁剪的代价），但末条必须是最新一条。
	last := evs[len(evs)-1]
	if last.Seq != 42 {
		t.Fatalf("末条 seq 应为 42: %d", last.Seq)
	}
}

func TestQueuePositionAndList(t *testing.T) {
	s := openTestStore(t)
	ctx := t.Context()
	// 清场后连续排 3 个任务，位次单调。
	ids := []string{"job_test_q_1", "job_test_q_2", "job_test_q_3"}
	for _, id := range ids {
		if _, _, err := s.Submit(ctx, Job{ID: id, Lane: "infer", Type: "echo.synthesize"}, nil); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	for i, id := range ids {
		j, err := s.Get(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		// 位次 = 同 lane 且早于我入队的 queued 任务数（环境残留任务会影响绝对值，
		// 只断言单调性——后入队者位次 ≥ 先入队者）。
		pos := s.QueuePosition(ctx, j)
		if i > 0 {
			prev, _ := s.Get(ctx, ids[i-1])
			if pos < s.QueuePosition(ctx, prev) {
				t.Fatalf("位次不单调: %s(%d) < %s", id, pos, ids[i-1])
			}
		}
	}

	jobs, total, err := s.List(ctx, ListFilter{Type: "echo.synthesize", Page: 1, PageSize: 5})
	if err != nil {
		t.Fatal(err)
	}
	if total < 3 || len(jobs) == 0 {
		t.Fatalf("列表: total=%d items=%d", total, len(jobs))
	}
}
