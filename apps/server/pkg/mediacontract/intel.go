package mediacontract

import "time"

// IntelItem 是 ① IC 的输出契约（契约 §1）。
// 消费者：② TT（全量）、③ MIA（仅 C_market）、④ MME（仅 B_material）。
type IntelItem struct {
	APIVersion  string          `json:"api_version"` // 恒 "1.0"
	IntelID     string          `json:"intel_id"`    // it_ 前缀
	Source      IntelSource     `json:"source"`
	Payload     IntelPayload    `json:"payload"`
	Entities    []Entity        `json:"entities"`
	Category    Category        `json:"category"`
	Metrics     IntelMetrics    `json:"metrics"`
	Dedup       IntelDedup      `json:"dedup"`
	Compliance  IntelCompliance `json:"compliance"`
	Workability IntelWorkability `json:"workability"`
}

// IntelSource 信源信息。
type IntelSource struct {
	Platform      string         `json:"platform"`       // 见契约 §1 平台枚举表
	Region        Region         `json:"region"`         // cn / intl / global
	CollectMethod CollectMethod  `json:"collect_method"` // 五路采集器
	URL           string         `json:"url"`
	Author        string         `json:"author"`                 // 仅存公开昵称（PIPL）
	CollectedAt   time.Time      `json:"collected_at"`           // 本地接收时间 UTC，必填
	PublishedAt   *time.Time     `json:"published_at,omitempty"` // 上游发布时间 UTC，可空
	Evidence      IntelEvidence  `json:"evidence"`
}

// IntelEvidence 取证链指针。不允许把原始 HTML 塞进消息体，只存路径/引用。
type IntelEvidence struct {
	SnapshotPath string            `json:"snapshot_path"`
	RawRef       string            `json:"raw_ref"`
	HTTPHeaders  map[string]string `json:"http_headers,omitempty"` // 脱敏后的受限集合
}

// IntelPayload 正文载荷。
type IntelPayload struct {
	Title      string   `json:"title"`
	Text       string   `json:"text"`
	MediaRefs  []string `json:"media_refs,omitempty"` // asset:// 可加工；https:// 须先下载
	Lang       string   `json:"lang"`                 // BCP-47，und 兜底
}

// Entity 实体。Confidence < 0.6 不进 MIA 实体链接。
type Entity struct {
	Type       EntityType `json:"type"`
	Value      string     `json:"value"`
	Confidence float64    `json:"confidence"`
}

// IntelMetrics 互动指标快照。rank_total=0 表示未知（契约 §0.2：0 与 null 语义不同）。
type IntelMetrics struct {
	Rank      int32      `json:"rank"`        // 无榜 = 0
	RankTotal int32      `json:"rank_total"`  // 无榜 = 0
	Views     int64      `json:"views"`
	Likes     int64      `json:"likes"`
	Comments  int64      `json:"comments"`
	AsOf      *time.Time `json:"_as_of,omitempty"` // 指标快照时间（时变数据必须带时间）
}

// IntelDedup 去重信息。
type IntelDedup struct {
	URLKey      string `json:"url_key"`     // URL 归一化键
	Fingerprint string `json:"fingerprint"` // SimHash 16 位小写 hex（契约是 string，不是 uint64）
	ClusterID   string `json:"cluster_id"`  // cl_ 前缀（评审修正：初版误写 tp_）
}

// IntelCompliance 合规闸门结论。
type IntelCompliance struct {
	Gate    Gate     `json:"gate"`
	Reasons []string `json:"reasons,omitempty"`
}

// IntelWorkability 可加工性评分（MME 准入依据）。
type IntelWorkability struct {
	Score  float64 `json:"score"`
	Reason string  `json:"reason,omitempty"`
}

// Validate 校验 IntelItem 是否满足契约必填约束。
func (x *IntelItem) Validate() error {
	if x.APIVersion == "" {
		return errField("api_version", "必填")
	}
	if err := ValidateIDFormat(x.IntelID, PrefixIntelItem); err != nil {
		return err
	}
	if !x.Category.Valid() {
		return errField("category", "非法取值 "+string(x.Category))
	}
	if !x.Source.CollectMethod.Valid() {
		return errField("source.collect_method", "非法取值")
	}
	if !x.Source.Region.Valid() {
		return errField("source.region", "非法取值")
	}
	if !x.Compliance.Gate.Valid() {
		return errField("compliance.gate", "非法取值")
	}
	for i, e := range x.Entities {
		if !e.Type.Valid() {
			return errField("entities["+itoa(i)+"].type", "非法取值")
		}
		if e.Confidence < 0 || e.Confidence > 1 {
			return errField("entities["+itoa(i)+"].confidence", "须在 [0,1]")
		}
	}
	return nil
}

// CategoryRoute 返回该条目应路由到的下游子系统（契约 §1 路由表）。
// D_compliance 返回空串（只进人工队列，不进自动流程）。
func (c Category) CategoryRoute() string {
	switch c {
	case CategoryAHot:
		return "tt"
	case CategoryBMaterial:
		return "mme"
	case CategoryCMarket:
		return "mia"
	case CategoryDCompliance:
		return "" // 人工 + 告警
	}
	return ""
}

// BlocksAutoFlow 判断该条目是否阻断自动流程。
func (x *IntelItem) BlocksAutoFlow() bool {
	return x.Compliance.Gate == GateBlock || x.Category == CategoryDCompliance
}
