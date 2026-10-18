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
		"rank_pct":     p.RankPct,
		"eng_pct":      p.EngPct,
		"freshness":    p.Freshness,
		"breadth":      p.Breadth,
		"w_rank_eff":   p.WRankEff,
		"w_eng_eff":    p.WEngEff,
		"w_fresh_eff":  p.WFreshEff,
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
