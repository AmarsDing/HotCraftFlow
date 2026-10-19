// Package ttdata 是 ② TT 热点追踪的数据层实现。
//
// 纪律（02-TT 03-详细设计.md §3.1）：本包是 InfluxDB 的唯一接触面，不含业务逻辑；
// 评审 S-12/S-13：tag 只保留 topic_id + platform（region/category 走 PG join 查询），
// 否则幂等被 tag 设计破坏、series 基数失控。
package ttdata

import (
	"context"
	"fmt"
	"os"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api/write"

	"github.com/go-kratos/kratos/v2/log"

	biztt "framework/app/admin/internal/biz/tt"
)

// measurement 时序主表（详设 §1.1）。
const measurementTopicHeat = "topic_heat"

// InfluxTS InfluxDB 2.x 客户端（biz/tt.TimeSeries 实现）。
type InfluxTS struct {
	client influxdb2.Client
	conf   biztt.InfluxConf
	log    *log.Helper
}

// NewInfluxTS 创建客户端。token 支持 ${ENV_VAR} 引用（详设 §4：禁止明文入库）。
// ping_on_start=true 时做健康检查；失败仅告警返回（降级期由 SnapshotBuffer 兜底）。
func NewInfluxTS(conf *biztt.Conf, logger log.Logger) (*InfluxTS, func()) {
	l := log.NewHelper(log.With(logger, "module", "data/tt-influx"))
	token := os.ExpandEnv(conf.Influx.Token)

	ts := &InfluxTS{conf: conf.Influx, log: l}
	if token == "" {
		// 无 token：保持降级态，客户端为 nil，写操作全部走缓冲。
		l.Warn("influx token empty, TT timeseries in degraded mode (writes will be buffered)")
		return ts, func() {}
	}

	ts.client = influxdb2.NewClient(conf.Influx.URL, token)

	if conf.Influx.PingOnStart {
		ctx, cancel := context.WithTimeout(context.Background(), conf.Influx.Timeout)
		defer cancel()
		ok, err := ts.client.Ping(ctx)
		if err != nil || !ok {
			l.Warnf("influx ping failed (degraded, writes will be buffered): ok=%v err=%v", ok, err)
		} else {
			l.Infof("influx connected: %s (org=%s raw=%s)", conf.Influx.URL, conf.Influx.Org, conf.Influx.BucketRaw)
		}
	}

	return ts, func() {
		if ts.client != nil {
			ts.client.Close()
		}
	}
}

// Ping 健康检查。降级态（client=nil）返回 (false, nil)。
func (s *InfluxTS) Ping(ctx context.Context) (bool, error) {
	if s.client == nil {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()
	return s.client.Ping(ctx)
}

// WriteSnapshots 同步写一批快照点到 bucket_raw。
//
// 幂等（NFR-TT-07）：write.Point 时间戳用 TBucket，(measurement + topic_id +
// platform + t_bucket) 唯一，重复写覆盖而非追加（S-13：region/category 不出 tag，
// 分类变更不需要重写时序点）。
func (s *InfluxTS) WriteSnapshots(ctx context.Context, points []biztt.SnapshotPoint) error {
	if len(points) == 0 {
		return nil
	}
	if s.client == nil {
		return fmt.Errorf("influx client not initialized (degraded)")
	}

	pts := make([]*write.Point, 0, len(points))
	for i := range points {
		pts = append(pts, toPoint(&points[i]))
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(ctx, s.conf.Timeout+time.Duration(len(pts))*time.Millisecond)
	defer cancel()
	err := s.client.WriteAPIBlocking(s.conf.Org, s.conf.BucketRaw).WritePoint(ctx, pts...)
	biztt.InfluxWriteDuration.Observe(time.Since(start).Seconds())
	if err != nil {
		return fmt.Errorf("influx write %d points: %w", len(pts), err)
	}
	return nil
}

// ReadRecentPoints 按时间倒序读某话题最近 N 个快照点（生命周期 streak 推导）。
// platform 语义：非空=精确平台；空串=跨全部平台（话题级视图）。
// 行形：pivot(_field) 同时取 velocity+heat_score——单字段查询曾使 HeatScore 恒 0，
// 下游规则 1（heat<20）会误判全站 long_tail。
func (s *InfluxTS) ReadRecentPoints(ctx context.Context, topicID, platform string, limit int) ([]biztt.SnapshotPoint, error) {
	if s.client == nil {
		return nil, fmt.Errorf("influx client not initialized (degraded)")
	}
	ctx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	platformFilter := ""
	if platform != "" {
		platformFilter = fmt.Sprintf(` and r.platform == "%s"`, platform)
	}
	queryAPI := s.client.QueryAPI(s.conf.Org)
	// H6/H7 修复：pivot 后先 group(topic_id) 把多平台序列合并为单表，再按原始
	// 采样时间 t_raw（写入侧已落 field，A-14）降序，最后 limit。否则：
	//  - 缺 group：每平台一张表，limit 各算一份，3 平台返回 3*limit 行，streak
	//    把微博/油管/B站 velocity 混在同一降序流里累计（跨平台抵消/凑数）；
	//  - 按 _time（桶时间）排序：样本乱序/跨桶边界时桶序≠原始时序，points[0]
	//    未必是最新样本，AgeDays/streak 取错锚点。
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: -168h)
		|> filter(fn: (r) => r._measurement == "%s" and r.topic_id == "%s"%s)
		|> filter(fn: (r) => r._field == "velocity" or r._field == "heat_score" or r._field == "t_raw")
		|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
		|> group(columns: ["topic_id"])
		|> sort(columns: ["t_raw"], desc: true)
		|> limit(n: %d)`, s.conf.BucketRaw, measurementTopicHeat, topicID, platformFilter, limit)

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influx query recent points: %w", err)
	}
	defer result.Close()

	points := make([]biztt.SnapshotPoint, 0, limit)
	for result.Next() {
		record := result.Record()
		p := biztt.SnapshotPoint{TopicID: topicID}
		// pivot 后 velocity/heat_score/t_raw 成为列，逐列提取（缺一列保持 0 值）。
		if cols := record.Values(); cols != nil {
			if hv, ok := cols["heat_score"].(float64); ok {
				p.HeatScore = hv
			}
			if vv, ok := cols["velocity"].(float64); ok {
				p.Velocity = vv
			}
			// t_raw 是 epoch 秒（int64/float64 皆兼容）；缺失时退化用桶时间，
			// 不致空指针——但正常路径写入侧恒落 t_raw。
			switch tv := cols["t_raw"].(type) {
			case int64:
				p.TRaw = time.Unix(tv, 0).UTC()
			case float64:
				p.TRaw = time.Unix(int64(tv), 0).UTC()
			default:
				p.TRaw = record.Time()
			}
		} else {
			p.TRaw = record.Time()
		}
		points = append(points, p)
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("influx query recent points result: %w", result.Err())
	}
	return points, nil
}

// ReadActiveHeatRange 读近 24h 全体活跃话题的 heat 序列（P25/P99 分位用）。
// 空窗口返回空切片（调用方禁用相对线）。
func (s *InfluxTS) ReadActiveHeatRange(ctx context.Context, from, to time.Time) ([]float64, error) {
	if s.client == nil {
		return nil, fmt.Errorf("influx client not initialized (degraded)")
	}
	ctx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	queryAPI := s.client.QueryAPI(s.conf.Org)
	// 分位语义 = 「活跃话题热度分布」：按 (topic, platform) 分组后逐小时取均值，
	// 再摊平为单切片。不做 group 的全局 hourly mean 会把全体话题揉成 24 个
	// 站点级均值，P25/P99 失去话题级分布意义（A-8 冷启动保护仍由调用方 len<4 判定）。
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: %d, stop: %d)
		|> filter(fn: (r) => r._measurement == "%s" and r._field == "heat_score")
		|> group(columns: ["topic_id", "platform"])
		|> aggregateWindow(every: 1h, fn: mean, createEmpty: false)
		|> yield(name: "mean")`,
		s.conf.BucketRaw, from.Unix(), to.Unix(), measurementTopicHeat)

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influx query active heat: %w", err)
	}
	defer result.Close()

	heats := make([]float64, 0, 48)
	for result.Next() {
		if v, ok := result.Record().Value().(float64); ok {
			heats = append(heats, v)
		}
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("influx query active heat result: %w", result.Err())
	}
	return heats, nil
}

// ReadPlatforms 列出某话题在 [from, to] 内有采样点的平台集合（迟到回填分平台回放用）。
// 空窗口/无点返回空切片。
func (s *InfluxTS) ReadPlatforms(ctx context.Context, topicID string, from, to time.Time) ([]string, error) {
	if s.client == nil {
		return nil, fmt.Errorf("influx client not initialized (degraded)")
	}
	ctx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	queryAPI := s.client.QueryAPI(s.conf.Org)
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: %d, stop: %d)
		|> filter(fn: (r) => r._measurement == "%s" and r.topic_id == "%s" and r._field == "heat_score")
		|> group(columns: ["platform"])
		|> keep(columns: ["platform"])
		|> distinct(column: "platform")`,
		s.conf.BucketRaw, from.Unix(), to.Unix(), measurementTopicHeat, topicID)

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influx query platforms: %w", err)
	}
	defer result.Close()

	platforms := make([]string, 0, 4)
	for result.Next() {
		if v, ok := result.Record().Value().(string); ok && v != "" {
			platforms = append(platforms, v)
		}
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("influx query platforms result: %w", result.Err())
	}
	return platforms, nil
}

// ReadSeriesAsc 按 t_bucket 升序读某话题某平台 [from, to] 的完整快照点
// （S-23 迟到回填：EWMA 链回放须含 rank/views/likes 等原始 field 与既有因子列，
// 覆盖写时保未被重算的 field 原值——breadth/sample_count/late 等）。
func (s *InfluxTS) ReadSeriesAsc(ctx context.Context, topicID, platform string, from, to time.Time) ([]biztt.SnapshotPoint, error) {
	if s.client == nil {
		return nil, fmt.Errorf("influx client not initialized (degraded)")
	}
	ctx, cancel := context.WithTimeout(ctx, s.conf.Timeout)
	defer cancel()

	queryAPI := s.client.QueryAPI(s.conf.Org)
	// pivot 全 field 为列；按 t_raw 升序回放（链式推进严格单调；同桶重复由
	// AdvanceDynamics Advanced=false 跳过——与 ingest 乱序口径一致）。
	query := fmt.Sprintf(`from(bucket: "%s")
		|> range(start: %d, stop: %d)
		|> filter(fn: (r) => r._measurement == "%s" and r.topic_id == "%s" and r.platform == "%s")
		|> pivot(rowKey: ["_time"], columnKey: ["_field"], valueColumn: "_value")
		|> group(columns: ["topic_id", "platform"])
		|> sort(columns: ["t_raw"], desc: false)`,
		s.conf.BucketRaw, from.Unix(), to.Unix(), measurementTopicHeat, topicID, platform)

	result, err := queryAPI.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("influx query series: %w", err)
	}
	defer result.Close()

	points := make([]biztt.SnapshotPoint, 0, 64)
	for result.Next() {
		record := result.Record()
		p := biztt.SnapshotPoint{TopicID: topicID, Platform: platform, TBucket: record.Time().UTC()}
		cols := record.Values()
		if cols == nil {
			p.TRaw = p.TBucket
			points = append(points, p)
			continue
		}
		p.Rank = colInt32(cols, "rank")
		p.RankTotal = colInt32(cols, "rank_total")
		p.Views = colInt64(cols, "views")
		p.Likes = colInt64(cols, "likes")
		p.Comments = colInt64(cols, "comments")
		p.EngRate = colFloat(cols, "eng_rate")
		p.HeatScore = colFloat(cols, "heat_score")
		p.Velocity = colFloat(cols, "velocity")
		p.Accel = colFloat(cols, "accel")
		p.RankPct = colFloat(cols, "rank_pct")
		p.EngPct = colFloat(cols, "eng_pct")
		p.Freshness = colFloat(cols, "freshness")
		p.Breadth = colFloat(cols, "breadth")
		p.WRankEff = colFloat(cols, "w_rank_eff")
		p.WEngEff = colFloat(cols, "w_eng_eff")
		p.WFreshEff = colFloat(cols, "w_fresh_eff")
		p.WBreadthEff = colFloat(cols, "w_breadth_eff")
		p.IsInterpolated = colBool(cols, "is_interpolated")
		p.Late = colBool(cols, "late")
		p.SampleCount = colInt32(cols, "sample_count")
		if v, ok := cols["weight_epoch"].(string); ok {
			p.WeightEpoch = v
		}
		switch tv := cols["t_raw"].(type) {
		case int64:
			p.TRaw = time.Unix(tv, 0).UTC()
		case float64:
			p.TRaw = time.Unix(int64(tv), 0).UTC()
		default:
			p.TRaw = p.TBucket
		}
		points = append(points, p)
	}
	if result.Err() != nil {
		return nil, fmt.Errorf("influx query series result: %w", result.Err())
	}
	return points, nil
}

// colFloat/colInt32/colInt64/colBool pivot 列提取（列缺失/型别漂移 → 零值哨兵）。
func colFloat(cols map[string]any, k string) float64 {
	switch v := cols[k].(type) {
	case float64:
		return v
	case int64:
		return float64(v)
	}
	return 0
}

func colInt32(cols map[string]any, k string) int32 {
	switch v := cols[k].(type) {
	case int64:
		return int32(v)
	case float64:
		return int32(v)
	}
	return 0
}

func colInt64(cols map[string]any, k string) int64 {
	switch v := cols[k].(type) {
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

func colBool(cols map[string]any, k string) bool {
	if v, ok := cols[k].(bool); ok {
		return v
	}
	return false
}

// toPoint 构造 line-protocol 点。tags 只有 topic_id + platform（S-12 基数治理）。
func toPoint(p *biztt.SnapshotPoint) *write.Point {
	tags := map[string]string{
		"topic_id": p.TopicID,
		"platform": p.Platform,
	}
	fields := map[string]any{
		"rank":            p.Rank,
		"rank_total":      p.RankTotal,
		"heat_score":      p.HeatScore,
		"views":           p.Views,
		"likes":           p.Likes,
		"comments":        p.Comments,
		"eng_rate":        p.EngRate,
		"velocity":        p.Velocity,
		"accel":           p.Accel,
		"is_interpolated": p.IsInterpolated,
		// —— B-3：四因子取值与生效权重落盘（NFR-TT-05）——
		"rank_pct":      p.RankPct,
		"eng_pct":       p.EngPct,
		"freshness":     p.Freshness,
		"breadth":       p.Breadth,
		"w_rank_eff":    p.WRankEff,
		"w_eng_eff":     p.WEngEff,
		"w_fresh_eff":   p.WFreshEff,
		"w_breadth_eff": p.WBreadthEff,
		// —— A-14：原始样本时间（epoch）与桶折叠数、迟到标记 ——
		"t_raw":        p.TRaw.UTC().Unix(),
		"sample_count": p.SampleCount,
		"late":         p.Late,
	}
	if p.WeightEpoch != "" {
		// A-15：权重口径断点（看板分段渲染与伪突发抑制依据）
		fields["weight_epoch"] = p.WeightEpoch
	}
	return influxdb2.NewPoint(measurementTopicHeat, tags, fields, p.TBucket.UTC())
}

// 编译期断言：端口签名漂移在编译期暴露，不在 wire 运行时。
var _ biztt.TimeSeries = (*InfluxTS)(nil)
