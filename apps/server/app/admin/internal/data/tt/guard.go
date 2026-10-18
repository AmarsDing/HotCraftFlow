package ttdata

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"

	biztt "framework/app/admin/internal/biz/tt"
)

// acceptSampleScript 同桶 last-wins 原子守卫（A-14）。
// 仅当 epoch 严格大于已存值时接受并更新；无 key 总是接受。
var acceptSampleScript = redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if cur and tonumber(cur) >= tonumber(ARGV[1]) then
  return 0
end
redis.call('SET', KEYS[1], ARGV[1], 'PX', ARGV[2])
return 1
`)

// RedisGuardStore biz/tt.GuardStore 实现（GET-COMPARE-SET 经 Lua 原子化，
// 并发消费下同桶旧点不会覆盖新点）。
type RedisGuardStore struct {
	rdb  *redis.Client
	log  *log.Helper
}

// NewRedisGuardStore 构造。rdb 为 nil 时返回禁用态（TryAcceptSample 恒接受，
// 由 InfluxDB (series,ts) 覆盖语义兜底——乱序覆盖风险仅存在于守卫关闭时，日志告警）。
func NewRedisGuardStore(rdb *redis.Client, logger log.Logger) *RedisGuardStore {
	g := &RedisGuardStore{
		rdb: rdb,
		log: log.NewHelper(log.With(logger, "module", "data/tt-guard")),
	}
	if rdb == nil {
		g.log.Warn("redis not configured, last-wins guard disabled (unordered overwrites possible)")
	}
	return g
}

// TryAcceptSample 原子判定（禁用态恒 accepted=true, nil）。
func (g *RedisGuardStore) TryAcceptSample(ctx context.Context, key string, epoch int64, ttl time.Duration) (bool, error) {
	if g.rdb == nil {
		return true, nil
	}
	res, err := acceptSampleScript.Run(ctx, g.rdb, []string{key}, epoch, ttl.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

// 编译期接口校验。
var _ biztt.GuardStore = (*RedisGuardStore)(nil)
