package ttdata

import (
	"github.com/google/wire"
	"github.com/redis/go-redis/v9"

	"framework/app/admin/internal/data"
	biztt "framework/app/admin/internal/biz/tt"
)

// ProvideRawRedis 从 data.RedisClient 取 go-redis 客户端（可 nil，守卫/缓冲已降级设计）。
// 须导出：wire 在 cmd 包合成调用，未导出名跨包不可见。
func ProvideRawRedis(rc *data.RedisClient) *redis.Client {
	if rc == nil {
		return nil
	}
	return rc.Client()
}

// ProviderSet data/tt 注入清单：InfluxTS(TimeSeries) → InfluxSink(PointSink)、
// Redis 缓冲/守卫、Ent 话题仓库。biz/tt 端口经 wire.Bind 绑定。
var ProviderSet = wire.NewSet(
	ProvideRawRedis,
	NewInfluxTS,
	NewRedisSnapshotBuffer,
	NewRedisGuardStore,
	NewEntTopicRepo,
	NewInfluxSink,

	wire.Bind(new(biztt.TimeSeries), new(*InfluxTS)),
	wire.Bind(new(biztt.SnapshotBuffer), new(*RedisSnapshotBuffer)),
	wire.Bind(new(biztt.GuardStore), new(*RedisGuardStore)),
	wire.Bind(new(biztt.TopicRepo), new(*EntTopicRepo)),
	wire.Bind(new(biztt.PointSink), new(*InfluxSink)),
)
