package ttdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/redis/go-redis/v9"

	biztt "framework/app/admin/internal/biz/tt"
)

// ============================================================================
// EWMA 动态链状态仓（每 (topic, platform) 一条 JSON 串，TTL 14d）。
//
// 并发语义（SaveIfNotAdvanced 乐观条件写）：同一 (topic,platform) 的可能并发
// 消费方下，仅当已存状态的 last_t 未越过「读时版本」才落盘；Lua 原子化
// GET-COMPARE-SET，防止双推进把 EWMA 链打成双倍计入。失败的写入放弃
// （热度仍按时序落库，动态下个样本点自愈）。
// ============================================================================

// dynStateTTL 状态保留（话题两周无采样则重建链，首点 α=1 语义可接受）。
const dynStateTTL = 14 * 24 * time.Hour

// saveDynScript 条件写：无 key 直接写；有 key 仅当 stored.last_t ≤ expected 才写。
// ARGV: [1]=expected_last_t_unix(0=首次断言无key), [2]=state_json, [3]=ttl_ms
// 返回 1=已写，0=并发抢先放弃。
var saveDynScript = redis.NewScript(`
local cur = redis.call('GET', KEYS[1])
if not cur then
  redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
  return 1
end
if tonumber(ARGV[1]) == 0 then
  return 0
end
local ok, st = pcall(cjson.decode, cur)
if not ok or not st['last_t_unix'] then
  redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
  return 1
end
if tonumber(st['last_t_unix']) > tonumber(ARGV[1]) then
  return 0
end
redis.call('SET', KEYS[1], ARGV[2], 'PX', ARGV[3])
return 1
`)

// dynStateJSON Redis 落盘形态（last_t_unix 供 Lua 比较，免解析 RFC3339）。
type dynStateJSON struct {
	HEwma       float64 `json:"h_ewma"`
	VEwma       float64 `json:"v_ewma"`
	LastTUnix   int64   `json:"last_t_unix"`
	WeightEpoch string  `json:"weight_epoch"`
}

// RedisDynamicsStore biz/tt.DynamicsStateStore 实现。rdb=nil → 禁用态（Load 恒 nil）。
type RedisDynamicsStore struct {
	rdb *redis.Client
	log *log.Helper
}

// NewRedisDynamicsStore 构造。
func NewRedisDynamicsStore(rdb *redis.Client, logger log.Logger) *RedisDynamicsStore {
	s := &RedisDynamicsStore{rdb: rdb, log: log.NewHelper(log.With(logger, "module", "data/tt-dynamics"))}
	if rdb == nil {
		s.log.Warn("redis not configured, dynamics state disabled (velocity/accel = 0)")
	}
	return s
}

// dynKey 状态键。
func dynKey(topicID, platform string) string {
	return fmt.Sprintf("tt:dyn:%s:%s", topicID, platform)
}

// Load 读状态；无 key / 坏值 → (nil, nil) 按首点语义处理。
func (s *RedisDynamicsStore) Load(ctx context.Context, topicID, platform string) (*biztt.DynamicsState, error) {
	if s.rdb == nil {
		return nil, nil
	}
	raw, err := s.rdb.Get(ctx, dynKey(topicID, platform)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var v dynStateJSON
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		s.log.Warnf("bad dynamics state for %s/%s (reset): %v", topicID, platform, err)
		return nil, nil
	}
	return &biztt.DynamicsState{
		HEwma:       v.HEwma,
		VEwma:       v.VEwma,
		LastT:       time.Unix(v.LastTUnix, 0).UTC(),
		WeightEpoch: v.WeightEpoch,
	}, nil
}

// SaveIfNotAdvanced 乐观条件写（expectedLastT 零值 = 断言无 key/坏值可覆盖）。
func (s *RedisDynamicsStore) SaveIfNotAdvanced(
	ctx context.Context, topicID, platform string, st biztt.DynamicsState, expectedLastT time.Time,
) (bool, error) {
	if s.rdb == nil {
		return false, nil
	}
	payload, err := json.Marshal(dynStateJSON{
		HEwma:       st.HEwma,
		VEwma:       st.VEwma,
		LastTUnix:   st.LastT.Unix(),
		WeightEpoch: st.WeightEpoch,
	})
	if err != nil {
		return false, err
	}
	res, err := saveDynScript.Run(ctx, s.rdb, []string{dynKey(topicID, platform)},
		expectedLastT.Unix(), payload, dynStateTTL.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return res == 1, nil
}

// 编译期接口校验。
var _ biztt.DynamicsStateStore = (*RedisDynamicsStore)(nil)
