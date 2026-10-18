package mediacontract

// VariantSet 是 ④ MME 的输出契约（契约 §4）。消费者：⑤ MAP。
type VariantSet struct {
	APIVersion   string     `json:"api_version"`
	VariantSetID string     `json:"variant_set_id"` // vs_ 前缀
	Origin       VariantOrigin `json:"origin"`
	Variants     []Variant  `json:"variants"`
	DiffCheck    DiffCheck  `json:"diff_check"`
}

// VariantOrigin 回指来源，形成闭环。
type VariantOrigin struct {
	CandidateID   *string `json:"candidate_id"` // 回指 ② TT 选题，允许 null（手动上传素材场景）
	AssetID       string  `json:"asset_id"`     // as_ 前缀
	TranscriptRef string  `json:"transcript_ref"`
}

// Variant 单个变体（一个画幅 × 一个内容版本 × 一个 Hook）。
type Variant struct {
	VariantID    string                  `json:"variant_id"` // v_ 前缀
	Aspect       string                  `json:"aspect"`     // 9:16 / 1:1 / 16:9
	ContentVersion string                `json:"content_version,omitempty"` // B 维度：内容版本标签
	HookID       string                  `json:"hook_id,omitempty"`       // C 维度：Hook 标识（补丁级演进）
	DurationSec  float64                 `json:"duration_sec"`
	VideoPath    string                  `json:"video_path"`  // 对象存储路径，不是 HTTP URL
	CoverPath    string                  `json:"cover_path"`
	EDLRef       string                  `json:"edl_ref"`
	Captions     Captions                `json:"captions"`
	Copy         map[string]PlatformCopy `json:"copy"` // 平台键须与 MAP 适配器注册表的 platform_key 一致（评审修复：改 map 支持 12 平台）
	ManifestPath string                  `json:"manifest_path"`
	QC           QC                      `json:"qc"`
}

// Captions 字幕。
type Captions struct {
	Style   CaptionStyle `json:"style"`
	SRTPath string       `json:"srt_path"`
}

// PlatformCopy 平台差异化文案。
type PlatformCopy struct {
	Title string   `json:"title"`
	Desc  string   `json:"desc"`
	Tags  []string `json:"tags,omitempty"`
}

// QC 渲染自检。boundary_ok / subtitle_ok 任一为 false 不得进入审批门。
type QC struct {
	BoundaryOK bool    `json:"boundary_ok"`
	SubtitleOK bool    `json:"subtitle_ok"`
	Score      float64 `json:"score"`
}

// DiffCheck 差异化检查。min_pair_similarity 过高说明变体不够差异化（平台查重风险）。
type DiffCheck struct {
	MinPairSimilarity float64 `json:"min_pair_similarity"`
	Warn              bool    `json:"warn"`
}

// Validate 校验 VariantSet。
func (x *VariantSet) Validate() error {
	if err := ValidateIDFormat(x.VariantSetID, PrefixVariantSet); err != nil {
		return err
	}
	for i, v := range x.Variants {
		if err := ValidateIDFormat(v.VariantID, PrefixVariant); err != nil {
			return err
		}
		if !v.Captions.Style.Valid() {
			return errField("variants["+itoa(i)+"].captions.style", "非法取值")
		}
	}
	return nil
}

// EligibleVariants 返回通过 QC 硬门禁、可进入审批门的变体（契约 §4）。
func (x *VariantSet) EligibleVariants() []Variant {
	out := make([]Variant, 0, len(x.Variants))
	for _, v := range x.Variants {
		if v.QC.BoundaryOK && v.QC.SubtitleOK {
			out = append(out, v)
		}
	}
	return out
}
