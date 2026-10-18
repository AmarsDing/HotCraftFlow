package biz

import (
	"context"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/go-kratos/kratos/v2/log"

	contract "framework/pkg/mediacontract"
)

// =============================================================================
// L4 理解层（评审 B-18 规则版可执行规格；LLM 增强可降级，ADR-IC-07）
//  FR-INT-08 实体：词典/正则，置信度标定 词典 0.9 / 正则 0.7 / LLM 0.6~0.95，<0.6 丢
//  FR-INT-09 分类：严格者优先 D > C > A > B（合规红线纪律：判错方向的代价不对称）
//  FR-INT-10 可加工性：score = 0.4*has_material + 0.3*has_viewpoint + 0.3*narratable
// =============================================================================

// entityConfidenceFloor 契约：confidence < 0.6 不进 MIA 实体链接，直接在抽取侧丢。
const entityConfidenceFloor = 0.6

var (
	// 中文书名号强信号 → policy/作品/文件，按 policy 记（词典级 0.9）。
	reBookTitle = regexp.MustCompile(`《([^》]{2,30})》`)
	// 中文引号短语 → topic（正则级 0.7）。
	reQuoted = regexp.MustCompile(`[“「]([^”」]{2,20})[”」]`)
	// 职务前缀 + 中文姓名 → person（0.7）：如「市长张三表示」。
	rePersonWithTitle = regexp.MustCompile(`(?:市长|省长|县长|主席|总裁|董事长|总经理|教授|院士|发言人|局长|CEO|CTO|CFO)([一-龥]{2,4})`)
	// 中文行政地名后缀 → place（0.7）。
	rePlaceCN = regexp.MustCompile(`([一-龥]{2,6}(?:省|市|县|区))`)
	// 英文连续大写词 → org/person 候选（0.7），如 "Federal Reserve"。
	reCapitalizedEN = regexp.MustCompile(`\b([A-Z][a-z]+(?:\s+[A-Z][a-z]+){1,3})\b`)
	// 公司后缀组织 → org（词典级 0.9）。
	reOrgSuffix = regexp.MustCompile(`([一-龥A-Za-z0-9]{2,20}(?:公司|集团|银行|证券|基金|控股|科技|汽车|能源|医药|大学|研究院|委员会|证监会|交易所))`)
)

// 词典（V1 内置种子表；周任务语料统计后再扩。词典命中 0.9）。
var (
	dictPolicyKeywords = []string{"规定", "处罚", "下架", "侵权", "新规", "禁令", "立案", "通报批评", "行政处罚", "条例", "办法", "实施细则"}
	dictMarketKeywords = []string{"政策", "央行", "SEC", "美联储", "财报", "年报", "降息", "加息", "IPO", "并购", "市值", "股价", "期货", "汇率"}
	dictTechTerms      = []string{"AI", "大模型", "GPT", "LLM", "芯片", "5G", "6G", "新能源", "光伏", "锂电池", "半导体", "量子", "机器人", "自动驾驶", "Sora", "OpenAI"}
)

// extractEntities 规则优先抽取。title 权重并入同一文本（正文缺省时兜底）。
// 置信度：词典 0.9 / 正则 0.7；去重按 (type,value) 保最高分。
func extractEntities(title, text string) []contract.Entity {
	corpus := strings.TrimSpace(title + "\n" + text)
	if corpus == "" {
		return nil
	}
	best := map[string]contract.Entity{}
	put := func(t contract.EntityType, value string, conf float64) {
		value = strings.TrimSpace(value)
		if value == "" || conf < entityConfidenceFloor {
			return
		}
		key := string(t) + "|" + value
		if cur, ok := best[key]; !ok || cur.Confidence < conf {
			best[key] = contract.Entity{Type: t, Value: value, Confidence: conf}
		}
	}

	for _, m := range reBookTitle.FindAllStringSubmatch(corpus, -1) {
		put(contract.EntityPolicy, m[1], 0.9)
	}
	for _, m := range reQuoted.FindAllStringSubmatch(corpus, -1) {
		put(contract.EntityTopic, m[1], 0.7)
	}
	for _, m := range rePersonWithTitle.FindAllStringSubmatch(corpus, -1) {
		put(contract.EntityPerson, m[1], 0.7)
	}
	for _, m := range rePlaceCN.FindAllStringSubmatch(corpus, -1) {
		put(contract.EntityPlace, m[1], 0.7)
	}
	for _, m := range reCapitalizedEN.FindAllStringSubmatch(corpus, 32) { // 防长英文正文噪音爆炸
		put(contract.EntityOrg, m[1], 0.7)
	}
	for _, m := range reOrgSuffix.FindAllStringSubmatch(corpus, -1) {
		put(contract.EntityOrg, m[1], 0.9)
	}
	for _, term := range dictTechTerms {
		if strings.Contains(corpus, term) {
			put(contract.EntityTech, term, 0.9)
		}
	}

	out := make([]contract.Entity, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	return out
}

// hitAny 词典命中检测。
func hitAny(corpus string, words []string) bool {
	for _, w := range words {
		if strings.Contains(corpus, w) {
			return true
		}
	}
	return false
}

// classifyCategory B-18 规则版。判定顺序 D > C > A > B：
// B-18 文本并列四规则未指明冲突优先级；合规信号判成热点会绕过 D 路由
// （D_compliance 不进自动流程），代价不对称 → 严格者优先，与 B-16 fail-closed 同源。
func classifyCategory(m *contract.IntelMetrics, payload *contract.IntelPayload) contract.Category {
	corpus := payload.Title + "\n" + payload.Text
	switch {
	case hitAny(corpus, dictPolicyKeywords):
		return contract.CategoryDCompliance
	case hitAny(corpus, dictMarketKeywords):
		return contract.CategoryCMarket
	case m.Rank > 0 && m.RankTotal > 0 && float64(m.Rank)/float64(m.RankTotal) <= 0.3:
		return contract.CategoryAHot
	default:
		// B-18「否则 B_material(text 长度 ≥800 或 media_refs 含长视频)」：
		// 该条件是 B 的可加工性信号而非其他类的归口，因此不满足时仍归 B（兜底桶），
		// 差异由 workability.has_material / narratable 分项表达，不再另设第五类。
		return contract.CategoryBMaterial
	}
}

// asrSupportedLangs narratable 语言系数：支持 ASR = 1，否则 0.5（B-18）。
func asrSupportedLangs(lang string) bool {
	switch strings.ToLower(lang) {
	case "zh", "zh-cn", "zh-tw", "zh-hans", "zh-hant", "en", "en-us", "en-gb":
		return true
	}
	return false
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// scoreWorkability B-18 公式精确实现；reason 低于阈值必填（四词表）。
func scoreWorkability(entities []contract.Entity, payload *contract.IntelPayload, minScore float64) (score float64, reason string) {
	hasMaterial := clamp01(float64(len(payload.MediaRefs)) / 3.0)
	hasViewpoint := clamp01(float64(len(entities)) / 5.0)
	textRunes := min(utf8.RuneCountInString(payload.Text), 3000)
	langFactor := 0.5
	if asrSupportedLangs(payload.Lang) {
		langFactor = 1.0
	}
	narratable := clamp01(float64(textRunes)/3000.0) * langFactor
	score = 0.4*hasMaterial + 0.3*hasViewpoint + 0.3*narratable

	if score >= minScore {
		return score, ""
	}
	// reason 必填：主缺口优先（语言不支持独立于分数构成，先表达）。
	switch {
	case langFactor < 1:
		reason = "语言不支持"
	case hasMaterial <= 1.0/3.0:
		reason = "素材不足"
	case textRunes < 800:
		reason = "时长不足"
	default:
		reason = "无明确观点"
	}
	return score, reason
}

// IcExtractUsecase L4 编排：规则恒定产出；LLM 开启时合并（ADR-IC-07 降级纪律）。
type IcExtractUsecase struct {
	cfg *IcConfig
	llm LLMEnhancePort // 可为 nil（纯规则降级态）
	log *log.Helper
}

func NewIcExtractUsecase(cfg *IcConfig, llm LLMEnhancePort, logger log.Logger) *IcExtractUsecase {
	return &IcExtractUsecase{cfg: cfg, llm: llm, log: log.NewHelper(log.With(logger, "module", "ic/extract/biz/admin-service"))}
}

// ExtractResult 归一化阶段的 L4 输出。
type ExtractResult struct {
	Entities          []contract.Entity
	Category          contract.Category
	WorkabilityScore  float64
	WorkabilityReason string
	LLMDegraded       bool // LLM 开启但失败回退规则（观测）
}

// Extract 主入口：规则版先算；LLM 可用则合并实体/取其 workability 建议，
// 分类按 B-18「与规则不一致时以规则为准并记样本」。
func (u *IcExtractUsecase) Extract(ctx context.Context, item *contract.IntelItem) *ExtractResult {
	entities := extractEntities(item.Payload.Title, item.Payload.Text)
	category := classifyCategory(&item.Metrics, &item.Payload)
	score, reason := scoreWorkability(entities, &item.Payload, u.cfg.WorkabilityMinScore)
	res := &ExtractResult{Entities: entities, Category: category, WorkabilityScore: score, WorkabilityReason: reason}

	if u.llm == nil || !u.cfg.LLMEnabled || !u.llm.Enabled() {
		return res
	}
	sug, err := u.llm.Extract(ctx, item)
	if err != nil {
		u.log.Warnf("llm extract failed, fallback to rule (intel=%s): %s", item.IntelID, err.Error())
		res.LLMDegraded = true
		return res
	}
	if sug == nil {
		res.LLMDegraded = true
		return res
	}
	// 实体合并：LLM 置信度钳制 [0.6,0.95]（B-18），与规则按 (type,value) 保最高。
	res.Entities = mergeLLMEntities(entities, sug.Entities)
	// 分类以规则为准；不一致记样本（落空：DedupSample 属去重域，这里落观测日志）。
	if sug.Category != "" && sug.Category != string(category) {
		u.log.Infof("llm category %s differs from rule %s, keep rule (intel=%s)", sug.Category, category, item.IntelID)
	}
	// workability 取 LLM 建议仅当它给出且规则分贴阈值带（±0.05）内——避免 LLM 漂移改写门禁。
	if sug.WorkabilityScore > 0 && abs(score-sug.WorkabilityScore) <= 0.05 {
		res.WorkabilityScore = sug.WorkabilityScore
		if sug.WorkabilityReason != "" && res.WorkabilityScore < u.cfg.WorkabilityMinScore {
			res.WorkabilityReason = sug.WorkabilityReason
		}
	}
	return res
}

func mergeLLMEntities(rule []contract.Entity, llm []contract.Entity) []contract.Entity {
	best := map[string]contract.Entity{}
	for _, e := range rule {
		best[string(e.Type)+"|"+e.Value] = e
	}
	for _, e := range llm {
		if !e.Type.Valid() {
			continue
		}
		if e.Confidence < entityConfidenceFloor {
			continue
		}
		if e.Confidence > 0.95 { // B-18：LLM 0.6~0.95
			e.Confidence = 0.95
		}
		key := string(e.Type) + "|" + e.Value
		if cur, ok := best[key]; !ok || cur.Confidence < e.Confidence {
			best[key] = e
		}
	}
	out := make([]contract.Entity, 0, len(best))
	for _, e := range best {
		out = append(out, e)
	}
	return out
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
