// Package jobstore 提供 media 任务的持久化状态机。
//
// 核心纪律（对标调研报告 §任务三件套）：
//   - 每次状态迁移 = UPDATE jobs + INSERT event，同一事务提交；
//   - 终态（done|failed|cancelled）必落 terminal 事件，SSE 据此关流；
//   - 事件序号 seq 每任务单调自增，断线重连用 after_seq 回放；
//   - 事件仅作刷新信号，权威状态永远在 jobs 行；
//   - 服务启动必须把 orphaned running 归因 failed（service_restart）。
package jobstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v4/stdlib"
)

// State 任务状态机。
type State string

const (
	StateQueued    State = "queued"
	StateRunning   State = "running"
	StateDone      State = "done"
	StateFailed    State = "failed"
	StateCancelled State = "cancelled"
)

// Terminal 判断终态。
func (s State) Terminal() bool {
	return s == StateDone || s == StateFailed || s == StateCancelled
}

// EventKind 事件词表（SSE event 字段 + WS kind）。
type EventKind string

const (
	EvQueued    EventKind = "job.queued"
	EvRunning   EventKind = "job.running"
	EvProgress  EventKind = "job.progress"
	EvDone      EventKind = "job.done"
	EvFailed    EventKind = "job.failed"
	EvCancelled EventKind = "job.cancelled"
)

// KindOfState 状态对应的 terminal 事件 kind（非终态返回空）。
func KindOfState(s State) EventKind {
	switch s {
	case StateDone:
		return EvDone
	case StateFailed:
		return EvFailed
	case StateCancelled:
		return EvCancelled
	}
	return ""
}

// Job 任务视图（media_jobs 行）。
type Job struct {
	ID             string
	Lane           string
	Type           string
	State          State
	Progress       float64
	PayloadJSON    string
	ResultJSON     string
	ErrorCode      string
	ErrorMsg       string
	Attempt        int32
	WorkerID       string
	IdempotencyKey string
	QueuedAt       time.Time
	StartedAt      *time.Time
	FinishedAt     *time.Time
	LastSeq        int64
}

// Event 任务事件（media_job_events 行）。
type Event struct {
	JobID   string
	Seq     int64
	Kind    EventKind
	Payload json.RawMessage
	Ts      time.Time
}

// PayloadFor 便捷构造事件载荷 JSON。
func PayloadFor(v any) json.RawMessage {
	if v == nil {
		return json.RawMessage(`{}`)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return b
}

// Store 任务存取。
type Store struct {
	db       *sql.DB
	eventCap int
}

// Open 打开 PG 连接（pgx stdlib）；dsn 来自 conf/pb Data.Database.Source。
func Open(dsn string, eventCap int) (*Store, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	if eventCap < 16 {
		eventCap = 16
	}
	return &Store{db: db, eventCap: eventCap}, nil
}

// Close 关闭连接池。
func (s *Store) Close() error { return s.db.Close() }

// Ping 供 /ready 探针。
func (s *Store) Ping(ctx context.Context) error { return s.db.PingContext(ctx) }

// ---------------------------------------------------------------- schema

// EnsureSchema 幂等建表（v1 简化迁移：与 data.database.migrate=true 语义一致）。
func (s *Store) EnsureSchema(ctx context.Context) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS media_jobs (
			id               TEXT PRIMARY KEY,
			lane             TEXT NOT NULL,
			type             TEXT NOT NULL,
			state            TEXT NOT NULL,
			progress         DOUBLE PRECISION NOT NULL DEFAULT 0,
			payload_json     TEXT NOT NULL DEFAULT '{}',
			result_json      TEXT NOT NULL DEFAULT '{}',
			error_code       TEXT NOT NULL DEFAULT '',
			error_msg        TEXT NOT NULL DEFAULT '',
			attempt          INT NOT NULL DEFAULT 0,
			worker_id        TEXT NOT NULL DEFAULT '',
			idempotency_key  TEXT,
			queued_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
			started_at       TIMESTAMPTZ,
			finished_at      TIMESTAMPTZ
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS media_jobs_idem_ux ON media_jobs (idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> ''`,
		`CREATE INDEX IF NOT EXISTS media_jobs_state_lane_idx ON media_jobs (state, lane, queued_at)`,
		`CREATE TABLE IF NOT EXISTS media_job_events (
			job_id  TEXT NOT NULL REFERENCES media_jobs(id) ON DELETE CASCADE,
			seq     BIGINT NOT NULL,
			kind    TEXT NOT NULL,
			payload JSONB NOT NULL DEFAULT '{}',
			ts      TIMESTAMPTZ NOT NULL DEFAULT now(),
			PRIMARY KEY (job_id, seq)
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("ensure schema: %w", err)
		}
	}
	return nil
}

// FailOrphaned 启动时归因：上一实例残留 running 一律 failed。
// 返回受影响任务数（观测用）。
func (s *Store) FailOrphaned(ctx context.Context, workerID string) (int64, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.QueryContext(ctx,
		`SELECT id FROM media_jobs WHERE state = $1 FOR UPDATE`, string(StateRunning))
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, 8)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}

	for _, id := range ids {
		p := Patch{
			To:         StateFailed,
			ErrorCode:  strPtr("MEDIA_SERVICE_RESTART"),
			ErrorMsg:   strPtr("服务重启导致运行中任务中断（孤儿归因）"),
			FinishedAt: timePtr(time.Now()),
		}
		if err := patchLocked(ctx, tx, id, p, string(EvFailed),
			PayloadFor(map[string]any{"reason": "service_restart", "prev_worker": workerID}), 0); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return int64(len(ids)), nil
}

// RecoverQueued 返回仍处于 queued 的任务（启动时回填内存队列）。
func (s *Store) RecoverQueued(ctx context.Context) ([]Job, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+jobCols+` FROM media_jobs WHERE state = $1 ORDER BY queued_at ASC`, string(StateQueued))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

// ---------------------------------------------------------------- submit

// NewID 生成任务 id（job_ 前缀 + 短 uuid）。
func NewID() string {
	u := uuid.New()
	return fmt.Sprintf("job_%02x%02x%02x%02x%02x%02x", u[0], u[1], u[2], u[3], u[10], u[11])
}

// Submit 新建 queued 任务并落首个事件（同事务）。
// idempotencyKey 命中唯一约束时返回既有任务与 created=false。
func (s *Store) Submit(ctx context.Context, j Job, evPayload json.RawMessage) (Job, bool, error) {
	if j.ID == "" {
		j.ID = NewID()
	}
	if j.PayloadJSON == "" {
		j.PayloadJSON = "{}"
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	var idem any
	if j.IdempotencyKey != "" {
		idem = j.IdempotencyKey
	}
	var insertedID string
	err = tx.QueryRowContext(ctx,
		`INSERT INTO media_jobs (id, lane, type, state, payload_json, idempotency_key)
		 VALUES ($1,$2,$3,$4,$5,$6)
		 ON CONFLICT (idempotency_key) WHERE idempotency_key IS NOT NULL AND idempotency_key <> '' DO NOTHING
		 RETURNING id`,
		j.ID, j.Lane, j.Type, string(StateQueued), j.PayloadJSON, idem).Scan(&insertedID)
	if errors.Is(err, sql.ErrNoRows) {
		// 幂等命中：返回既有任务。
		_ = tx.Rollback()
		existing, gerr := s.getByIdem(ctx, j.IdempotencyKey)
		if gerr != nil {
			return Job{}, false, gerr
		}
		return existing, false, nil
	}
	if err != nil {
		return Job{}, false, err
	}
	j.ID = insertedID
	j.State = StateQueued

	if err := insertEventLocked(ctx, tx, insertedID, string(EvQueued), evPayload); err != nil {
		return Job{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	fresh, err := s.Get(ctx, insertedID)
	return fresh, true, err
}

// ---------------------------------------------------------------- transition

// Patch 一次状态迁移（To 必须与 From 状态机一致；WorkerID/Progress 等按需填）。
type Patch struct {
	From       []State
	To         State
	Progress   *float64
	ResultJSON *string
	ErrorCode  *string
	ErrorMsg   *string
	WorkerID   *string
	Attempt    *int32
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// Transition 应用迁移：UPDATE（带 from 状态门禁）+ INSERT 事件，同事务。
// 返回迁移后的任务与是否实际迁移（from 不匹配时 ok=false，调用方按并发丢弃处理）。
func (s *Store) Transition(ctx context.Context, id string, p Patch, kind EventKind, evPayload json.RawMessage) (Job, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, false, err
	}
	defer func() { _ = tx.Rollback() }()

	states := p.From
	if len(states) == 0 {
		states = []State{p.To}
	}
	args := []any{string(p.To)}
	q := `UPDATE media_jobs SET state = $1`
	n := 1
	set := func(col string, v any) {
		n++
		q += fmt.Sprintf(`, %s = $%d`, col, n)
		args = append(args, v)
	}
	if p.Progress != nil {
		set(`progress`, *p.Progress)
	}
	if p.ResultJSON != nil {
		set(`result_json`, *p.ResultJSON)
	}
	if p.ErrorCode != nil {
		set(`error_code`, *p.ErrorCode)
	}
	if p.ErrorMsg != nil {
		set(`error_msg`, *p.ErrorMsg)
	}
	if p.WorkerID != nil {
		set(`worker_id`, *p.WorkerID)
	}
	if p.Attempt != nil {
		set(`attempt`, *p.Attempt)
	}
	if p.StartedAt != nil {
		set(`started_at`, *p.StartedAt)
	}
	if p.FinishedAt != nil {
		set(`finished_at`, *p.FinishedAt)
	}
	if p.To.Terminal() {
		q += `, finished_at = COALESCE(finished_at, now())`
	}

	n++
	q += fmt.Sprintf(` WHERE id = $%d AND state = ANY($%d)`, n, n+1)
	args = append(args, id, stateArray(states))

	res, err := tx.ExecContext(ctx, q, args...)
	if err != nil {
		return Job{}, false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Job{}, false, err
	}
	if affected == 0 {
		return Job{}, false, nil
	}
	if kind != "" {
		if err := insertEventLocked(ctx, tx, id, string(kind), evPayload); err != nil {
			return Job{}, false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return Job{}, false, err
	}
	fresh, err := s.Get(ctx, id)
	return fresh, true, err
}

// patchLocked FailOrphaned 内部复用（调用方已持 FOR UPDATE 锁，直接 UPDATE 无 from 门禁）。
func patchLocked(ctx context.Context, tx *sql.Tx, id string, p Patch, kind string, evPayload json.RawMessage, _ int) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE media_jobs SET state=$1, error_code=COALESCE($2,error_code), error_msg=COALESCE($3,error_msg),
		 finished_at=COALESCE($4, finished_at, now()) WHERE id=$5`,
		string(p.To), p.ErrorCode, p.ErrorMsg, p.FinishedAt, id)
	if err != nil {
		return err
	}
	return insertEventLocked(ctx, tx, id, kind, evPayload)
}

// insertEventLocked 插入事件（seq 本任务内自增；同事务内 MAX(seq)+1 无竞争，因为只有单执行器写某任务）。
func insertEventLocked(ctx context.Context, tx *sql.Tx, jobID, kind string, payload json.RawMessage) error {
	if len(payload) == 0 {
		payload = json.RawMessage(`{}`)
	}
	_, err := tx.ExecContext(ctx,
		`INSERT INTO media_job_events (job_id, seq, kind, payload)
		 SELECT $1, COALESCE(MAX(seq), 0) + 1, $2, $3
		 FROM media_job_events WHERE job_id = $1`,
		jobID, kind, payload)
	return err
}

// Requeue 终态重入队：failed|cancelled → queued，attempt+1，
// 清空 error/result/worker/计时字段，落 job.queued 事件（同事务）。
func (s *Store) Requeue(ctx context.Context, id string) (Job, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Job{}, err
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`UPDATE media_jobs SET state=$1, progress=0, result_json='{}', error_code='', error_msg='',
		 worker_id='', started_at=NULL, finished_at=NULL, attempt=attempt+1
		 WHERE id=$2 AND state = ANY($3)`,
		string(StateQueued), id, stateArray([]State{StateFailed, StateCancelled}))
	if err != nil {
		return Job{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Job{}, err
	}
	if affected == 0 {
		return Job{}, ErrNotTerminal
	}
	if err := insertEventLocked(ctx, tx, id, string(EvQueued),
		PayloadFor(map[string]any{"reason": "retry"})); err != nil {
		return Job{}, err
	}
	if err := tx.Commit(); err != nil {
		return Job{}, err
	}
	return s.Get(ctx, id)
}

// ErrNotTerminal 非终态任务不允许重入队（与 executor 的 RetryRejected 对应）。
var ErrNotTerminal = errors.New("job is not in a terminal state")

// ---------------------------------------------------------------- queries

const jobCols = `id, lane, type, state, progress, payload_json, result_json, error_code, error_msg,
	attempt, worker_id, COALESCE(idempotency_key,''), queued_at, started_at, finished_at,
	(SELECT COALESCE(MAX(seq),0) FROM media_job_events WHERE job_id = media_jobs.id) AS last_seq`

type rowScanner interface {
	Scan(dest ...any) error
}

func scanJob(r rowScanner) (Job, error) {
	var j Job
	var state string
	err := r.Scan(&j.ID, &j.Lane, &j.Type, &state, &j.Progress, &j.PayloadJSON, &j.ResultJSON,
		&j.ErrorCode, &j.ErrorMsg, &j.Attempt, &j.WorkerID, &j.IdempotencyKey,
		&j.QueuedAt, &j.StartedAt, &j.FinishedAt, &j.LastSeq)
	j.State = State(state)
	return j, err
}

// Get 按 id 取任务。
func (s *Store) Get(ctx context.Context, id string) (Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM media_jobs WHERE id = $1`, id)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

// ErrNotFound 任务不存在。
var ErrNotFound = errors.New("media job not found")

func (s *Store) getByIdem(ctx context.Context, key string) (Job, error) {
	row := s.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM media_jobs WHERE idempotency_key = $1`, key)
	j, err := scanJob(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Job{}, ErrNotFound
	}
	return j, err
}

// ListFilter 列表过滤。
type ListFilter struct {
	State    string
	Lane     string
	Type     string
	Page     uint32
	PageSize uint32
}

// List 分页列出任务（按 queued_at 倒序）。
func (s *Store) List(ctx context.Context, f ListFilter) ([]Job, uint32, error) {
	if f.Page == 0 {
		f.Page = 1
	}
	if f.PageSize == 0 || f.PageSize > 200 {
		f.PageSize = 20
	}
	where := `TRUE`
	args := []any{}
	add := func(cond string, v any) {
		args = append(args, v)
		where += fmt.Sprintf(` AND %s = $%d`, cond, len(args))
	}
	if f.State != "" {
		add(`state`, f.State)
	}
	if f.Lane != "" {
		add(`lane`, f.Lane)
	}
	if f.Type != "" {
		add(`type`, f.Type)
	}

	var total uint32
	if err := s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_jobs WHERE `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limitArgs := append(append([]any{}, args...), f.PageSize, (f.Page-1)*f.PageSize)
	rows, err := s.db.QueryContext(ctx,
		fmt.Sprintf(`SELECT %s FROM media_jobs WHERE %s ORDER BY queued_at DESC LIMIT $%d OFFSET $%d`,
			jobCols, where, len(limitArgs)-1, len(limitArgs)), limitArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []Job{}
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, j)
	}
	return out, total, rows.Err()
}

// QueuePosition 返回排队位置（含自己为 0；非 queued 返回 0）。
func (s *Store) QueuePosition(ctx context.Context, j Job) int32 {
	if j.State != StateQueued {
		return 0
	}
	var pos int32
	_ = s.db.QueryRowContext(ctx,
		`SELECT count(*) FROM media_jobs WHERE state = $1 AND lane = $2 AND queued_at < $3`,
		string(StateQueued), j.Lane, j.QueuedAt).Scan(&pos)
	return pos
}

// EventsSince 返回 seq > afterSeq 的事件（断线回放数据源），按 seq 升序。
func (s *Store) EventsSince(ctx context.Context, jobID string, afterSeq int64, limit int) ([]Event, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT seq, kind, payload, ts FROM media_job_events WHERE job_id = $1 AND seq > $2 ORDER BY seq ASC LIMIT $3`,
		jobID, afterSeq, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		e.JobID = jobID
		if err := rows.Scan(&e.Seq, &e.Kind, &e.Payload, &e.Ts); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// TrimEvents 按封顶裁剪最早事件（每任务封顶纪律；事务外单飞也无害）。
func (s *Store) TrimEvents(ctx context.Context, jobID string) error {
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM media_job_events WHERE job_id = $1 AND seq <=
			(SELECT COALESCE(MAX(seq),0) - $2 FROM media_job_events WHERE job_id = $1)`,
		jobID, s.eventCap)
	return err
}

// ---------------------------------------------------------------- helpers

func strPtr(s string) *string     { return &s }
func timePtr(t time.Time) *time.Time { return &t }

// stateArray 转为 Postgres text[] 字面量参数格式。
func stateArray(states []State) string {
	out := "{"
	for i, st := range states {
		if i > 0 {
			out += ","
		}
		out += string(st)
	}
	return out + "}"
}
