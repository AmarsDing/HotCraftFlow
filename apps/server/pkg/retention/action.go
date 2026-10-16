// action.go 保留动作框架（2026-08-21 设计 §6.5.1）。
// 在 Worker 既有"单 Job 清理"之上扩展多阶段编排：
//
//	days=7（旧配置）                → LegacyStages → 单阶段 hard_delete（向后兼容）
//	stages: downgrade→hard_delete   → Executor 按 AfterDays 升序逐阶段路由到对应 Handler
//
// 三种动作实现：
//   - HardDeleteHandler：通用物理删除（DELETE FROM {table} WHERE {time_field} < cutoff）
//   - DowngradeHandler：热→温→冷文件迁移（复用 pkg/storage.LayeredStorage，storage_layers 由存储层内部同步）
//   - AggregateHandler：降采样到派生表（聚合 SQL 域特定，以 AggregateFunc 注入，如 metar §6.5.3 矢量平均）
package retention

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/storage"
)

// Action 保留动作类型。
type Action string

const (
	// ActionHardDelete 物理删除（现状行为）。
	ActionHardDelete Action = "hard_delete"
	// ActionDowngrade 层间迁移文件（热→温→冷）。
	ActionDowngrade Action = "downgrade"
	// ActionAggregate 降采样写派生表。
	ActionAggregate Action = "aggregate"
	// ActionArchive 导出外部归档（远期预留，当前无实现）。
	ActionArchive Action = "archive"
)

// Valid 动作是否已定义。
func (a Action) Valid() bool {
	switch a {
	case ActionHardDelete, ActionDowngrade, ActionAggregate, ActionArchive:
		return true
	}
	return false
}

// StageConfig 单阶段配置（设计 §6.5.1）。
type StageConfig struct {
	// AfterDays 数据超过该天数后触发本阶段。
	AfterDays int `json:"after_days" yaml:"after_days"`
	// Action 动作类型。
	Action Action `json:"action" yaml:"action"`
	// Params 动作参数（aggregate: {granularity, target_table} / downgrade: {target_layer, keep_forecast_hours?}）。
	Params map[string]any `json:"params" yaml:"params"`
}

// DomainConfig 域保留配置（各服务 yaml 解析目标；Stages 按 AfterDays 升序）。
type DomainConfig struct {
	// Table 业务表名（hard_delete/aggregate 的源表；标识符白名单校验）。
	Table string `json:"table" yaml:"table"`
	// TimeField 业务时间字段（cutoff 判定列）。
	TimeField string `json:"time_field" yaml:"time_field"`
	// Stages 阶段列表（AfterDays 升序）。
	Stages []StageConfig `json:"stages" yaml:"stages"`
}

var identPattern = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// Validate 配置合法性（启动期暴露优于运行期失败）。
func (d DomainConfig) Validate() error {
	if !identPattern.MatchString(d.Table) {
		return fmt.Errorf("retention: invalid table %q", d.Table)
	}
	if !identPattern.MatchString(d.TimeField) {
		return fmt.Errorf("retention: invalid time_field %q", d.TimeField)
	}
	if len(d.Stages) == 0 {
		return fmt.Errorf("retention: empty stages for %s", d.Table)
	}
	prev := 0
	for i, s := range d.Stages {
		if !s.Action.Valid() {
			return fmt.Errorf("retention: stage[%d] invalid action %q", i, s.Action)
		}
		if s.Action == ActionArchive {
			return fmt.Errorf("retention: stage[%d] action archive reserved, not implemented", i)
		}
		if s.AfterDays <= 0 {
			return fmt.Errorf("retention: stage[%d] after_days must be > 0, got %d", i, s.AfterDays)
		}
		if s.AfterDays <= prev {
			return fmt.Errorf("retention: stages must ascend by after_days (stage[%d]=%d <= %d)", i, s.AfterDays, prev)
		}
		prev = s.AfterDays
	}
	return nil
}

// LegacyStages 旧版"仅 days"配置 → 单阶段 hard_delete（T3 向后兼容口径：现有 metar/satellite/flight 零改动）。
func LegacyStages(days int) []StageConfig {
	return []StageConfig{{AfterDays: days, Action: ActionHardDelete}}
}

// Handler 动作执行体。cutoff 由 Executor 按 stage.AfterDays 统一计算（UTC，与 Cutoff 同口径）。
type Handler interface {
	Apply(ctx context.Context, stage StageConfig, cutoff time.Time) error
}

// HandlerFunc 函数适配器。
type HandlerFunc func(ctx context.Context, stage StageConfig, cutoff time.Time) error

// Apply 实现 Handler。
func (f HandlerFunc) Apply(ctx context.Context, stage StageConfig, cutoff time.Time) error {
	return f(ctx, stage, cutoff)
}

// Executor 阶段编排器：按 Stages 顺序逐阶段计算 cutoff 并路由到对应 Handler。
// 单阶段失败记日志并继续后续阶段（各阶段 cutoff 独立判定，互不阻塞），错误聚合返回。
type Executor struct {
	handlers map[Action]Handler
	log      *log.Helper
	now      func() time.Time // 可注入时钟（单测用）
}

// NewExecutor 构造编排器。handlers 至少覆盖配置中出现的动作，否则 Run 时报错。
func NewExecutor(handlers map[Action]Handler, logger log.Logger) (*Executor, error) {
	if len(handlers) == 0 {
		return nil, fmt.Errorf("retention: executor requires at least one handler")
	}
	return &Executor{
		handlers: handlers,
		log:      log.NewHelper(log.With(logger, "module", "retention/executor")),
		now:      time.Now,
	}, nil
}

// Run 执行一个域的全部阶段。
func (e *Executor) Run(ctx context.Context, domain DomainConfig) error {
	if err := domain.Validate(); err != nil {
		return err
	}
	var errs []error
	for _, stage := range domain.Stages {
		h, ok := e.handlers[stage.Action]
		if !ok {
			errs = append(errs, fmt.Errorf("retention: no handler for action %q", stage.Action))
			continue
		}
		cutoff := e.now().UTC().Add(-time.Duration(stage.AfterDays) * 24 * time.Hour)
		if err := h.Apply(ctx, stage, cutoff); err != nil {
			e.log.Errorf("stage failed: table=%s action=%s afterDays=%d err=%v", domain.Table, stage.Action, stage.AfterDays, err)
			errs = append(errs, fmt.Errorf("stage %s@%dd: %w", stage.Action, stage.AfterDays, err))
			continue
		}
		e.log.Infof("stage done: table=%s action=%s afterDays=%d cutoff=%s", domain.Table, stage.Action, stage.AfterDays, cutoff.Format(time.RFC3339))
	}
	return errors.Join(errs...)
}

// ExecQuerier 最小 Exec 接口（*sql.DB / *sql.Tx 均满足）。
type ExecQuerier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// HardDeleteHandler 通用物理删除：`DELETE FROM {table} WHERE {time_field} < $1`。
// 标识符白名单校验防注入（表名/列名无法参数绑定）。
type HardDeleteHandler struct {
	db        ExecQuerier
	table     string
	timeField string
	log       *log.Helper
}

// NewHardDeleteHandler 构造通用删除 handler。
func NewHardDeleteHandler(db ExecQuerier, table, timeField string, logger log.Logger) (*HardDeleteHandler, error) {
	if db == nil {
		return nil, fmt.Errorf("retention: hard_delete nil db")
	}
	if !identPattern.MatchString(table) || !identPattern.MatchString(timeField) {
		return nil, fmt.Errorf("retention: hard_delete unsafe identifier table=%q field=%q", table, timeField)
	}
	return &HardDeleteHandler{
		db:        db,
		table:     table,
		timeField: timeField,
		log:       log.NewHelper(log.With(logger, "module", "retention/hard-delete")),
	}, nil
}

// Apply 删除 cutoff 之前的行。
func (h *HardDeleteHandler) Apply(ctx context.Context, _ StageConfig, cutoff time.Time) error {
	q := fmt.Sprintf(`DELETE FROM %s WHERE %s < $1`, h.table, h.timeField)
	res, err := h.db.ExecContext(ctx, q, cutoff)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	h.log.Infof("hard_delete: table=%s cutoff=%s rows=%d", h.table, cutoff.Format(time.RFC3339), n)
	return nil
}

// ObjectRef 待降级对象（bucket 内相对路径两段式，与 pkg/storage 口径一致）。
type ObjectRef struct {
	Bucket string
	Object string
}

// DowngradeLister 域注入：列出 cutoff 之前待降级的对象（业务表 join storage_layers 查当前层，由域 SQL 决定）。
// limit 为单轮上限（防单轮迁太多拖垮 IO）。
type DowngradeLister func(ctx context.Context, cutoff time.Time, limit int) ([]ObjectRef, error)

// DowngradeHandler 通用降级：list 取清单 → LayeredStorage.MoveLayer 逐对象迁移。
// storage_layers 元数据由存储层 Record 内部同步，本 handler 不直接写库。
type DowngradeHandler struct {
	store storage.LayeredStorage
	list  DowngradeLister
	batch int
	log   *log.Helper
}

// NewDowngradeHandler 构造降级 handler。batch<=0 默认 500。
func NewDowngradeHandler(store storage.LayeredStorage, list DowngradeLister, batch int, logger log.Logger) (*DowngradeHandler, error) {
	if store == nil {
		return nil, fmt.Errorf("retention: downgrade nil store")
	}
	if list == nil {
		return nil, fmt.Errorf("retention: downgrade nil lister")
	}
	if batch <= 0 {
		batch = 500
	}
	return &DowngradeHandler{
		store: store,
		list:  list,
		batch: batch,
		log:   log.NewHelper(log.With(logger, "module", "retention/downgrade")),
	}, nil
}

// Apply 把 cutoff 之前的对象迁移到 params.target_layer 指定层。
func (h *DowngradeHandler) Apply(ctx context.Context, stage StageConfig, cutoff time.Time) error {
	raw, ok := stage.Params["target_layer"]
	if !ok {
		return fmt.Errorf("retention: downgrade missing params.target_layer")
	}
	target := storage.Layer(fmt.Sprint(raw))
	if !target.IsValid() || target == storage.LayerHot {
		return fmt.Errorf("retention: downgrade invalid target_layer %q", target)
	}

	refs, err := h.list(ctx, cutoff, h.batch)
	if err != nil {
		return fmt.Errorf("list downgrade objects: %w", err)
	}
	if len(refs) == 0 {
		return nil
	}
	var moved, failed int
	for _, ref := range refs {
		if err := h.store.MoveLayer(ctx, ref.Bucket, ref.Object, target); err != nil {
			failed++
			h.log.Warnf("downgrade move failed: %s/%s -> %s err=%v", ref.Bucket, ref.Object, target, err)
			continue
		}
		moved++
	}
	h.log.Infof("downgrade: target=%s cutoff=%s listed=%d moved=%d failed=%d", target, cutoff.Format(time.RFC3339), len(refs), moved, failed)
	if failed > 0 {
		return fmt.Errorf("retention: downgrade %d/%d objects failed", failed, len(refs))
	}
	return nil
}

// AggregateFunc 域注入聚合主体（聚合 SQL 域特定；cutoff 之前的原始数据聚合写入派生表）。
type AggregateFunc func(ctx context.Context, cutoff time.Time, params map[string]any) error

// AggregateHandler 降采样动作适配器。
type AggregateHandler struct {
	fn AggregateFunc
}

// NewAggregateHandler 构造聚合 handler。
func NewAggregateHandler(fn AggregateFunc) (*AggregateHandler, error) {
	if fn == nil {
		return nil, fmt.Errorf("retention: aggregate nil func")
	}
	return &AggregateHandler{fn: fn}, nil
}

// Apply 调用域聚合主体，透传 cutoff 与 params（granularity/target_table 等）。
func (h *AggregateHandler) Apply(ctx context.Context, stage StageConfig, cutoff time.Time) error {
	return h.fn(ctx, cutoff, stage.Params)
}
