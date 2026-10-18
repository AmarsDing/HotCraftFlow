package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/tx7do/go-utils/entgo/mixin"

	contract "framework/pkg/mediacontract"
)

// EvidenceSnapshot 取证快照（01-IC/03 §2.4 + 评审 B-8/B-17，🔴 不可后置项）。
// 哈希链防篡改：chain_hash = sha256(prev_hash || raw_hash || intel_id || collected_at.UnixNano())，
// 首条 prev_hash 为 64 个 0。只追加存储，删除仅经 pkg/retention 到期清理且写审计日志。
type EvidenceSnapshot struct {
	ent.Schema
}

func (EvidenceSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ic_evidence_snapshots",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("取证快照：时间/URL/原始片段指针/响应头/robots 结论 + 哈希链（防篡改审计链）"),
	}
}

// Fields of the EvidenceSnapshot.
func (EvidenceSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.String("id").
			Comment("快照 ID，ev_ 前缀 ULID").
			MaxLen(30).
			NotEmpty().
			Unique().
			Immutable().
			DefaultFunc(contract.NewEvidenceID),

		field.String("intel_id").
			Comment("所属情报 ID（it_ 前缀；非空外键，与 IntelItem 同事务写入，评审 B-8）").
			MaxLen(30).
			NotEmpty(),
		field.Time("collected_at").
			Comment("采集时间 UTC"),
		field.String("method").
			Comment("采集方法：hotlist|api|rss|crawl|sidecar").
			MaxLen(16),
		field.String("request_url").
			Comment("请求 URL"),
		field.String("final_url").
			Comment("重定向后最终 URL（短链展开终点，S-6）；无重定向时同 request_url").
			Optional().
			Nillable(),
		field.Int32("http_status").
			Comment("HTTP 状态码").
			Default(0),
		field.JSON("http_headers", map[string]string{}).
			Comment("脱敏后的响应头受限集合（去 Set-Cookie/Authorization 等，契约 C-16）").
			Optional(),

		field.String("raw_path").
			Comment("原始片段对象存储路径（只追加文件名 = evidence_id；不存正文进消息体，ADR-IC-04）").
			Optional().
			Nillable(),
		field.String("raw_hash").
			Comment("原始片段 SHA-256 hex（完整性校验）").
			MaxLen(64).
			Optional().
			Nillable(),

		// —— 哈希链（评审 B-17 防篡改）——
		field.String("prev_hash").
			Comment("上一条证据的 chain_hash（首条为 64 个 0）").
			MaxLen(64).
			NotEmpty(),
		field.String("chain_hash").
			Comment("本证据链哈希 sha256(prev_hash||raw_hash||intel_id||collected_at.UnixNano())").
			MaxLen(64).
			NotEmpty(),

		// —— robots 结论（评审 B-16 三态；S-7 crawl-delay）——
		field.Bool("robots_checked").
			Comment("robots.txt 是否完成检查（抓取失败保守放行时记 false → gate=review）").
			Default(false),
		field.Bool("robots_allowed").
			Comment("robots 是否允许（checked=true 且有结论时才有效）").
			Default(true),
		field.Int32("crawl_delay_ms").
			Comment("站点要求的 Crawl-delay 毫秒数（S-7；无则 0）").
			Default(0),

		field.Time("retain_until").
			Comment("留存截止（默认 collected_at + 90d，NFR-IC-08；交 pkg/retention 清理）"),

		field.Int64("raw_size_bytes").
			Comment("原始片段字节数").
			Default(0),
	}
}

// Indexes of the EvidenceSnapshot.
func (EvidenceSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("intel_id"),
		index.Fields("retain_until"),
		index.Fields("collected_at"),
	}
}

// Edges of the EvidenceSnapshot.
func (EvidenceSnapshot) Edges() []ent.Edge {
	return nil
}

// Mixin of the EvidenceSnapshot.
func (EvidenceSnapshot) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.Time{},
	}
}
