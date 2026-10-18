package mediacontract

import "time"

// DispatchUnit 是 ⑤ MAP 的输入契约（契约 §5），由 VariantSet 派生。
type DispatchUnit struct {
	APIVersion   string         `json:"api_version"`
	UnitID       string         `json:"unit_id"` // du_ 前缀
	VariantSetID string         `json:"variant_set_id"`
	Targets      []string       `json:"targets"` // "平台:账号槽位"，槽位指向 PublishAccount，凭据不进契约
	ScheduledFor time.Time      `json:"scheduled_for"` // UTC
	Visibility   VisibilitySpec `json:"visibility"`
	Approval     Approval       `json:"approval"`
	Publish      Publish        `json:"publish"`
	FeedbackRef  *string        `json:"feedback_ref"`
	SubmittedBy  string         `json:"submitted_by"` // 提交者（禁自批判据）
}

// VisibilitySpec 可见性。default 恒为 private；改公开必须填 override_reason 且经审批（NFR-17）。
type VisibilitySpec struct {
	Default         Visibility `json:"default"`
	OverrideReason  *string    `json:"override_reason"`
}

// Approval 审批块。approved 且 approver != submitted_by 才允许 publish。
type Approval struct {
	State    ApprovalState `json:"state"`
	Approver *string       `json:"approver"`
	At       *time.Time    `json:"at"`
}

// Publish 发布块。operator 不得与提交者相同（禁自批）。
type Publish struct {
	PostID   *string    `json:"post_id"`
	URL      *string    `json:"url"`
	At       *time.Time `json:"at"`
	Operator *string    `json:"operator"`
}

// Validate 校验 DispatchUnit。
func (x *DispatchUnit) Validate() error {
	if err := ValidateIDFormat(x.UnitID, PrefixDispatchUnit); err != nil {
		return err
	}
	if !x.Approval.State.Valid() {
		return errField("approval.state", "非法取值")
	}
	if !x.Visibility.Default.Valid() {
		return errField("visibility.default", "非法取值")
	}
	return nil
}

// PerformanceSnapshot 是 ⑤ MAP 表现回流契约（契约 §8.3，第八份契约）。
// 消费者：② TT（选题反馈）、① IC（信源权重）、④ MME（变体效果）。
type PerformanceSnapshot struct {
	APIVersion     string            `json:"api_version"`
	SnapshotID     string            `json:"snapshot_id"`      // ps_ 前缀
	DispatchUnitID string            `json:"dispatch_unit_id"`
	VariantID      string            `json:"variant_id"`
	Platform       string            `json:"platform"`
	AccountSlot    string            `json:"account_slot"`
	PostURL        string            `json:"post_url"`
	Metrics        PerformanceMetric `json:"metrics"`
	CapturedAt     time.Time         `json:"captured_at"` // UTC
	LagHours       int               `json:"lag_hours"`   // 发布后多久抓的（平台 analytics 通常 T+24h）
}

// PerformanceMetric 平台回流的真实表现数。
type PerformanceMetric struct {
	Views     int64 `json:"views"`
	Likes     int64 `json:"likes"`
	Comments  int64 `json:"comments"`
	Shares    int64 `json:"shares"`    // V1.1 补登
	Favorites int64 `json:"favorites"` // V1.1 补登
}

// Validate 校验 PerformanceSnapshot。
func (x *PerformanceSnapshot) Validate() error {
	if err := ValidateIDFormat(x.SnapshotID, PrefixPerformanceSnap); err != nil {
		return err
	}
	return nil
}
