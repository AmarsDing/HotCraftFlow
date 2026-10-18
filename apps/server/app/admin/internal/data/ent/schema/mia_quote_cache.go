package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// MiaQuoteCache 行情缓存（FR-MIA-07，详细设计 §1.7）。
// 行情依赖一律只读（NFR-MIA-02）；本表只落 OHLCV 快照，绝不存交易凭据。
type MiaQuoteCache struct {
	ent.Schema
}

func (MiaQuoteCache) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "mia_quote_caches", Charset: "utf8mb4", Collation: "utf8mb4_bin"},
		entsql.WithComments(true),
		schema.Comment("行情缓存：只读 OHLCV 快照（FR-MIA-07）"),
	}
}

func (MiaQuoteCache) Fields() []ent.Field {
	return []ent.Field{
		field.String("symbol").
			Comment("标的：SPY / ^GSPC / EURUSD / BTC/USDT").
			NotEmpty(),
		field.String("provider").
			Comment("行情来源：openbb|ccxt|yahoo").
			NotEmpty(),
		field.JSON("ohlcv", []map[string]any{}).
			Comment("OHLCV 快照数组"),
		field.Time("fetched_at").
			Comment("拉取时间 UTC（Redis 侧 TTL 5 分钟）"),
	}
}

func (MiaQuoteCache) Edges() []ent.Edge { return nil }

func (MiaQuoteCache) Indexes() []ent.Index {
	return []ent.Index{
		// 复合主键语义：symbol+provider 唯一（详细设计 §1.7）
		index.Fields("symbol", "provider").Unique(),
		index.Fields("fetched_at"),
	}
}

func (MiaQuoteCache) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		appmixin.TenantID{},
	}
}
