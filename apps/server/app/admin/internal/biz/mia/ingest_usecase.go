package mia

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	algomia "framework/app/admin/internal/algo/mia"
	"framework/pkg/mediacontract"
)

// IngestUsecase MIA 主分析管线（概要设计 §5.1 主链路）。
//
//	IntelItem(C_market) / TopicCandidate(route=mia)
//	 → 实体链接（映射表 A）
//	 → 情感与方向（FinBERT / LLM / unknown，禁止猜测）
//	 → 事件聚类（多源转载合并为 EventCard）
//	 → 叙事匹配（映射表 B，补充加密标的）
//	 → 影响面评分（四因子 + TTL）
//	 → 跨资产传导（全部带 confidence ≤0.6）
//	 → 影响路径生成（每结论必须有 path）
//	 → 🔴 三项硬校验（disclaimer/path/confidence 任一不过拒绝落库）
//	 → 落 ImpactCard + 发布 mm.impact.card.v1 + 高分推送
type IngestUsecase struct {
	cfg         Config
	sentiment   *SentimentOrchestrator
	mappings    EntityMappingRepo
	narratives  NarrativeMappingRepo
	events      EventCardRepo
	impacts     ImpactCardRepo
	rules       TransmissionRuleRepo
	publisher   ImpactPublisher
	notifier    *Notifier
	titleSim    func(a, b string) float64

	log *log.Helper
}

// NewIngestUsecase 构造 MIA 主分析管线。
func NewIngestUsecase(
	cfg Config,
	sentiment *SentimentOrchestrator,
	mappings EntityMappingRepo,
	narratives NarrativeMappingRepo,
	events EventCardRepo,
	impacts ImpactCardRepo,
	rules TransmissionRuleRepo,
	publisher ImpactPublisher,
	notifier *Notifier,
	logger log.Logger,
) *IngestUsecase {
	uc := &IngestUsecase{
		cfg:        cfg,
		sentiment:  sentiment,
		mappings:   mappings,
		narratives: narratives,
		events:     events,
		impacts:    impacts,
		rules:      rules,
		publisher:  publisher,
		notifier:   notifier,
		titleSim:   simhashTitleSimilarity,
		log:        log.NewHelper(log.With(logger, "module", "biz/mia/ingest")),
	}
	// 启动播种 6 条核心传导规则（表空时；详细设计 §2.5 例表）。失败仅告警不阻断启动，
	// 传导候选缺失时管线仍可产出直接命中卡（干跑可观测）。
	if cfg.Enabled && rules != nil {
		if err := SeedDefaultRules(context.Background(), rules, logger); err != nil {
			uc.log.Warnf("seed default transmission rules failed: %v", err)
		}
	}
	return uc
}

// simhashTitleSimilarity 标题相似度：字符 bigram Jaccard → [0,1]。
// SimHash 对短文本（<80 字符）不稳定（textutil 设计约束 valid=false），
// 新闻/热榜标题多不足 80 字符，故用 bigram Jaccard 作轻量语义代理。
func simhashTitleSimilarity(a, b string) float64 {
	if a == "" || b == "" {
		return 0
	}
	na := algomia.NormalizeEntity(a)
	nb := algomia.NormalizeEntity(b)
	if na == "" || nb == "" {
		return 0
	}
	ga, gb := runeBigrams(na), runeBigrams(nb)
	if len(ga) == 0 || len(gb) == 0 {
		if na == nb {
			return 1
		}
		return 0
	}
	inter := 0
	for g := range ga {
		if gb[g] {
			inter++
		}
	}
	union := len(ga) + len(gb) - inter
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}

func runeBigrams(s string) map[string]bool {
	r := []rune(s)
	out := make(map[string]bool, len(r)+1)
	for i := 0; i+1 < len(r); i++ {
		out[string(r[i:i+2])] = true
	}
	if len(r) == 1 {
		out[string(r)] = true
	}
	if len(r) == 2 {
		out[s] = true
	}
	return out
}

// IngestItem 接入输入（IntelItem 契约事件归一化后的处理视图）。
type IngestItem struct {
	IntelID     string // it_ 前缀
	TraceID     string // NATS 链路 trace_id（透传不重建）
	Title       string
	Text        string
	Lang        string
	TopicID     string
	SourceURL   string
	OccurredAt  time.Time // 事件发生时间 UTC（优先 published_at，缺失回退 collected_at）
	Entities    []mediacontract.Entity
	IsMacro     bool // 时政/宏观类（走 LLM 引擎优先级）
}

// IngestResult 管线结果。
type IngestResult struct {
	EventIntake *EventCardRow // 新事件卡或合并到既有卡
	Merged      bool
	Produced    *ImpactCardRow // 生成的影响卡（合规校验不过则 nil + err）
	Pushed      bool
	Skipped     bool   // 降频/去重等跳过
	SkipReason  string
}

// Ingest 处理一条 C_market 类情报。主链路（概要设计 §5.1）。
func (u *IngestUsecase) Ingest(ctx context.Context, in IngestItem) (*IngestResult, error) {
	if !u.cfg.Enabled {
		return &IngestResult{Skipped: true, SkipReason: "mia disabled"}, nil
	}
	// 🔴 合规前置：disclaimer 为空视为配置错误，整条管线拒绝产出。
	if strings.TrimSpace(u.cfg.Disclaimer) == "" {
		return nil, ErrDisclaimerEmpty
	}
	result := &IngestResult{}

	// ---------- 1. 事件聚类（FR-MIA-05）----------
	eventCard, merged, err := u.clusterEvent(ctx, in)
	if err != nil {
		return nil, err
	}
	result.EventIntake = eventCard
	result.Merged = merged

	// ---------- 2. 实体链接（FR-MIA-01）----------
	mappings, err := u.mappings.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	mappingsView := toAlgoMappings(mappings)
	hits := make([]algomia.AssetHit, 0, 8)
	for _, e := range in.Entities {
		if e.Confidence < u.cfg.MinEntityConfidence {
			continue // 契约 §1：低于阈值不进 MIA
		}
		hits = append(hits, algomia.LinkEntity(e, mappingsView)...)
	}

	// ---------- 3. 叙事匹配（FR-MIA-02）----------
	narrItems, err := u.narratives.ListEnabled(ctx)
	if err != nil {
		return nil, err
	}
	narrHits := matchNarratives(in.Title+" "+in.Text, narrItems)

	// ---------- 4. 情感与方向（FR-MIA-03/04，禁止猜测）----------
	sent, engine := u.sentiment.Classify(ctx, SentimentInput{
		Title:   in.Title,
		Text:    in.Text,
		Lang:    normalizeLang(in.Lang),
		IsMacro: in.IsMacro || eventCard.Category == "policy" || eventCard.Category == "macro",
	})

	// ---------- 5. 影响面评分（FR-MIA-06，四因子 + TTL）----------
	confidences := make([]float64, 0, len(hits))
	for _, h := range hits {
		confidences = append(confidences, h.Confidence)
	}
	assetForTTL := dominantAssetClass(hits, narrHits)
	ttl := u.ttlFor(assetForTTL, narrHits)
	ageHours := time.Since(in.OccurredAt).Hours()
	if ageHours < 0 {
		ageHours = 0
	}
	scored := algomia.ScoreImpact(algomia.ImpactInput{
		SentimentScore:      sent.Score,
		DistinctDomainCount: givenMax(eventCard.SourceCount, u.distinctDomainsOf(eventCard)),
		SourceCredibility:   sourceCredibility(eventCard),
		TopConfidences:      confidences,
		AgeHours:            ageHours,
		TTLHours:            ttl,
	})

	// ---------- 6. 受影响标的面板（映射 + 叙事 + 传导）----------
	affected, err := u.buildAffected(ctx, hits, narrHits, scored, in)
	if err != nil {
		return nil, err
	}

	// ---------- 🔴 7. 三项硬校验（详细设计 §5 / 需求 §9.5）----------
	card := &ImpactCardRow{
		ImpactID:        mediacontract.NewImpactCardID(),
		EventID:         eventCard.EventID,
		TopicID:         in.TopicID,
		Title:           firstNonEmpty(in.Title, eventCard.Title),
		Summary:         firstNonEmpty(in.Text, eventCard.Summary),
		Lang:            normalizeLang(in.Lang),
		SentimentLabel:  sent.Label,
		SentimentScore:  sent.Score,
		SentimentEngine: engine,
		ImpactScore:     scored.Score,
		Direction:       scored.Direction,
		TTLHours:        ttl,
		ExpiresAt:       in.OccurredAt.Add(time.Duration(ttl) * time.Hour),
		Factors:         scored.Factors,
		Affected:        affected,
		NoMapping:       scored.NoMapping,
		Disclaimer:      u.cfg.Disclaimer,
		RuleVersion:     RuleVersion,
		WeightsSnapshot: u.weightsSnapshot(),
		OccurredAt:      in.OccurredAt,
	}
	if err := ValidateImpactCardForStore(card); err != nil {
		// 🔴 合规违约：production 不应到达这里（防线在配置层），打错也不落库。
		u.log.WithContext(ctx).Errorf("compliance gate rejected impact card %s: %v", card.ImpactID, err)
		return nil, err
	}
	saved, err := u.impacts.Create(ctx, card)
	if err != nil {
		return nil, err
	}
	result.Produced = saved
	u.log.WithContext(ctx).Infof("impact card produced: %s score=%.1f dir=%s engine=%s affected=%d ttl=%dh",
		saved.ImpactID, saved.ImpactScore, saved.Direction, engine, len(affected), ttl)

	// ---------- 8. 发布事件 + 高分推送 ----------
	if u.publisher != nil {
		evCard := saved.ToContract()
		evCard.Event.Sources = eventCard.SourceIntelIDs
		if pErr := u.publisher.PublishImpactCard(ctx, evCard, in.TraceID); pErr != nil {
			u.log.WithContext(ctx).Warnf("publish impact card %s failed: %v", saved.ImpactID, pErr)
		}
	}
	if u.notifier != nil && saved.ImpactScore >= u.cfg.MinScoreToPush && time.Now().UTC().Before(saved.ExpiresAt) {
		if pushed, pErr := u.notifier.PushImpactCard(ctx, u.cfg, saved, eventCard); pErr != nil {
			u.log.WithContext(ctx).Warnf("push impact card %s failed: %v", saved.ImpactID, pErr)
		} else {
			result.Pushed = pushed
		}
	}
	return result, nil
}

// clusterEvent 事件聚类：与近窗事件卡相似度比对，并入或新建。
func (u *IngestUsecase) clusterEvent(ctx context.Context, in IngestItem) (*EventCardRow, bool, error) {
	category := classifyEventCategory(in)
	domain := domainOfURL(in.SourceURL)

	recent, err := u.events.ListRecent(ctx, time.Duration(u.cfg.ClusterWindowHours)*time.Hour)
	if err != nil {
		return nil, false, err
	}
	seenTopics := map[string]bool{}
	for _, c := range recent {
		if c.TopicID != "" && in.TopicID != "" && c.TopicID == in.TopicID {
			seenTopics[c.TopicID] = true
		}
	}

	ev := &EventUsecase{}
	match := ev.FindClusterMatch(ctx, in.Title, in.Entities, in.OccurredAt, u.cfg.ClusterWindowHours, u.cfg.ClusterSimilarityThreshold, recent, u.titleSim)

	// 同 topic 直接并入（TT 已经把同一话题的多源报到上层，属天然多源转载证据）。
	if in.TopicID != "" && seenTopics[in.TopicID] {
		for _, c := range recent {
			if c.TopicID == in.TopicID && !c.SplitLocked {
				match = &ClusterMatch{Card: c, Similarity: 1}
				break
			}
		}
	}

	if match != nil {
		if err := u.events.AbsorbSource(ctx, match.Card.EventID, in.IntelID, domain, in.OccurredAt); err != nil {
			u.log.WithContext(ctx).Warnf("absorb source %s into event %s failed: %v", in.IntelID, match.Card.EventID, err)
		}
		updated, gErr := u.events.Get(ctx, match.Card.EventID)
		if gErr == nil && updated != nil {
			return updated, true, nil
		}
		match.Card.SourceIntelIDs = append(match.Card.SourceIntelIDs, in.IntelID)
		return match.Card, true, nil
	}

	// 新建事件卡。
	ec := &EventCardRow{
		EventID:        mediacontract.NewEventCardID(),
		Title:          firstNonEmpty(in.Title, "(untitled event)"),
		Summary:        truncate(in.Text, 1024),
		OccurredAt:     in.OccurredAt,
		TopicID:        in.TopicID,
		SourceIntelIDs: []string{in.IntelID},
		SourceCount:    1,
		Domains:        brandDomains(domain),
		Entities:       in.Entities,
		Category:       category,
		Lang:           normalizeLang(in.Lang),
		MergedBy:       "auto",
	}
	created, err := u.events.Create(ctx, ec)
	if err != nil {
		return nil, false, err
	}
	return created, false, nil
}

// buildAffected 组装受影响标的面板：映射命中 + 叙事命中 + 传导规则扩展。
// 🔴 面板每一项都必须有非空 path 与有效 confidence——否则整个卡拒绝落库。
func (u *IngestUsecase) buildAffected(ctx context.Context, hits []algomia.AssetHit, narrHits []NarrativeHit, scored algomia.ImpactOutput, in IngestItem) ([]mediacontract.AffectedItem, error) {
	out := make([]mediacontract.AffectedItem, 0, len(hits)+len(narrHits))
	seen := map[string]bool{}
	scoreDir := directionFromScore(scored)

	// 6.1 直接映射命中（源：映射表 A）。
	for _, h := range hits {
		key := string(h.AssetClass) + "|" + h.Symbol
		if seen[key] {
			continue
		}
		seen[key] = true
		direction := resolveDirection(h.DirectionBias, scoreDir)
		path := ExplainMappedPath(h, direction)
		out = append(out, mediacontract.AffectedItem{
			Symbol:     h.Symbol,
			AssetClass: h.AssetClass,
			Direction:  direction,
			Confidence: clamp01(h.Confidence),
			Path:       path,
			RuleID:     firstNonEmpty(h.RuleID, RuleVersion),
		})
	}

	// 6.2 叙事匹配命中（源：映射表 B；缩小 scope：加密面板）。
	ttlOverride := false
	_ = ttlOverride
	for _, nh := range narrHits {
		for _, coin := range nh.Item.Coins {
			symbol := coin + "/USDT"
			key := string(mediacontract.AssetCrypto) + "|" + symbol
			if seen[key] {
				continue
			}
			seen[key] = true
			dir := resolveDirection(nh.Item.DefaultSentimentHint, scoreDir)
			out = append(out, mediacontract.AffectedItem{
				Symbol:     symbol,
				AssetClass: mediacontract.AssetCrypto,
				Direction:  dir,
				Confidence: clamp01(nh.Item.Confidence * maxSco1(scored.Factors.Sentiment, 0.5)),
				Path:       ExplainNarrativePath(nh),
				RuleID:     "narrative/" + nh.Item.Narrative,
			})
		}
	}

	// 6.3 跨资产传导（FR-MIA-08；每条规则带 confidence ≤0.6，假设非因果）。
	rules, err := u.rules.List(ctx, true)
	if err != nil {
		return nil, err
	}
	baseCount := len(out)
	for _, r := range rules {
		for i := 0; i < baseCount; i++ {
			if !strings.EqualFold(out[i].Symbol, r.Trigger.Asset) {
				continue
			}
			directionMatch := directionMatchesTrigger(out[i].Direction, string(r.Trigger.Direction))
			if !directionMatch {
				continue
			}
			contractRule := mediacontract.TransmissionRule{
				RuleID:      r.RuleID,
				Trigger:     r.Trigger,
				ToAsset:     r.ToAsset,
				ToDirection: r.ToDirection,
				Confidence:  r.Confidence,
				Note:        r.Note,
			}
			newItem := algomia.ApplyTransmission(contractRule, out[i])
			newItem.AssetClass = assetClassOfSymbol(r.ToAsset)
			// 🔴 双重保险：传导项 confidence 强制 ≤0.6。
			if newItem.Confidence > 0.6 {
				newItem.Confidence = 0.6
			}
			key := "T|" + r.RuleID + "|" + newItem.Symbol
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, newItem)
		}
	}
	return out, nil
}

// ttlFor 解析卡 TTL：叙事映射可覆盖；否则资产类默认。
func (u *IngestUsecase) ttlFor(ac mediacontract.AssetClass, narrHits []NarrativeHit) int {
	if len(narrHits) > 0 && ac == mediacontract.AssetCrypto {
		// 叙事命中时可取其最短 TTL（叙事衰减快者优先）。
		minTTL := 12
		for _, nh := range narrHits {
			if nh.Item.TTLHours < minTTL && nh.Item.TTLHours >= 6 {
				minTTL = nh.Item.TTLHours
			}
		}
		return minTTL
	}
	return u.cfg.TTLFor(ac)
}

// weightsSnapshot 权重快照（NFR-MIA-10 可复算）。
func (u *IngestUsecase) weightsSnapshot() map[string]any {
	return map[string]any{
		"rule_version":       RuleVersion,
		"source_weight_base": u.cfg.SourceWeightBase,
		"source_weight_log":  u.cfg.SourceWeightLogCoef,
		"similarity_threshold": u.cfg.ClusterSimilarityThreshold,
		"cluster_window_hours": u.cfg.ClusterWindowHours,
		"min_push_score":     u.cfg.MinScoreToPush,
	}
}

// distinctDomainsOf EventCard domains 字段的去重域数（sovled by repo upsert）。
func (u *IngestUsecase) distinctDomainsOf(c *EventCardRow) int {
	return len(c.Domains)
}

func givenMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// maxSco1 返回不低于 floor 的因子值——叙事匹配的置信度乘子下限保护。
func maxSco1(v, floor float64) float64 {
	if v < floor {
		return floor
	}
	return v
}

// directionFromScore 评分输出的方向；unknown 情感 → neutral（评分里 sentiment=0）。
func directionFromScore(scored algomia.ImpactOutput) mediacontract.Direction {
	if scored.Direction == "" {
		return mediacontract.DirectionNeutral
	}
	return scored.Direction
}

// resolveDirection 映射默认偏向优先，neutral 时按评分方向。
func resolveDirection(bias mediacontract.Direction, scoreDir mediacontract.Direction) mediacontract.Direction {
	if bias == mediacontract.DirectionBullish || bias == mediacontract.DirectionBearish {
		return bias
	}
	return scoreDir
}

// directionMatchesTrigger 传导触发方向与卡方向的一致性（触发 ↑ = bullish）。
func directionMatchesTrigger(cardDir mediacontract.Direction, trigDir string) bool {
	up := cardDir == mediacontract.DirectionBullish
	switch trigDir {
	case "up":
		return up
	case "down":
		return !up
	default:
		return true // 未指定方向视为任意
	}
}

// assetClassOfSymbol 按 symbol 形式猜资产类别（BTC/ETH → crypto）。
func assetClassOfSymbol(symbol string) mediacontract.AssetClass {
	s := strings.ToUpper(symbol)
	switch {
	case strings.Contains(s, "/USDT"), strings.Contains(s, "/USD"), strings.Contains(s, "-USD"):
		return mediacontract.AssetCrypto
	case strings.HasPrefix(s, "^"), strings.HasPrefix(s, "HSI"), strings.HasPrefix(s, "SHCOMP"), strings.HasPrefix(s, "SSE"), strings.Contains(s, "INDEX"):
		return mediacontract.AssetIndex
	case len(s) == 6 && isAlpha(s):
		return mediacontract.AssetFX
	default:
		return mediacontract.AssetEquity
	}
}

func isAlpha(s string) bool {
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return true
}

// dominantAssetClass 卡面主导资产类别（决定默认 TTL）。
func dominantAssetClass(hits []algomia.AssetHit, narrHits []NarrativeHit) mediacontract.AssetClass {
	if len(hits) == 0 && len(narrHits) > 0 {
		return mediacontract.AssetCrypto
	}
	classCount := map[mediacontract.AssetClass]float64{}
	for _, h := range hits {
		classCount[h.AssetClass] += h.Confidence
	}
	best := mediacontract.AssetEquity
	var score float64
	for c, s := range classCount {
		if s > score {
			score = s
			best = c
		}
	}
	return best
}

// sourceCredibility 信源可信度基准（官方/央行/交易所=1.0，自媒体=0.6）。
// V1 先按事件类别估：policy/macro 来源通常更官方；将来可接 source 表扩充。
func sourceCredibility(c *EventCardRow) float64 {
	switch c.Category {
	case "policy", "macro":
		return 0.9
	case "company", "tech":
		return 0.8
	case "crypto":
		return 0.7
	default:
		return 0.7
	}
}

// classifyEventCategory 事件粗分类（policy/company/macro/tech/crypto）。
func classifyEventCategory(in IngestItem) string {
	lower := strings.ToLower(in.Title + " " + in.Text)
	if in.IsMacro {
		return "macro"
	}
	cryptoWords := []string{"bitcoin", "btc", "eth", "solana", "sol", "crypto", "代币", "虚拟币", "比特币", "以太坊", "公链"}
	for _, w := range cryptoWords {
		if strings.Contains(lower, w) {
			return "crypto"
		}
	}
	for _, e := range in.Entities {
		switch e.Type {
		case mediacontract.EntityPolicy:
			return "policy"
		case mediacontract.EntityTech:
			return "tech"
		}
	}
	return "macro"
}

// normalizeLang 统一 lang 口径（zh-CN → zh，en-US → en，空 → en 保守英文通路）。
func normalizeLang(lang string) string {
	lang = strings.ToLower(strings.TrimSpace(lang))
	switch {
	case strings.HasPrefix(lang, "zh"):
		return "zh"
	case strings.HasPrefix(lang, "en"):
		return "en"
	case lang == "":
		return "en"
	default:
		return lang
	}
}

// domainOfURL 提取 URL 域名（source_weight 的去重域计数输入）。
func domainOfURL(raw string) string {
	if raw == "" {
		return ""
	}
	host := raw
	if i := strings.Index(host, "://"); i >= 0 {
		host = host[i+3:]
	}
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	host = strings.ToLower(strings.TrimSpace(host))
	host = strings.TrimPrefix(host, "www.")
	return host
}

func brandDomains(d string) []string {
	if d == "" {
		return nil
	}
	return []string{d}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// ---------- 🔴 合规硬校验（详细设计 §5：MIA_DISCLAIMER_EMPTY / MIA_PATH_EMPTY / MIA_CONFIDENCE_INVALID） ----------

// ValidateImpactCardForStore 三项硬校验：任一不过拒绝落库（需求 §9.5：自动化校验）。
// ① disclaimer 非空；② 每项 affected 的 path 非空；③ 每项 confidence ∈[0,1] 且传导项 ≤0.6。
func ValidateImpactCardForStore(card *ImpactCardRow) error {
	if strings.TrimSpace(card.Disclaimer) == "" {
		return ErrDisclaimerEmpty
	}
	for i, a := range card.Affected {
		if len(a.Path) == 0 {
			return ErrPathEmpty
		}
		for _, p := range a.Path {
			if strings.TrimSpace(p) == "" {
				return ErrPathEmpty
			}
		}
		if a.Confidence < 0 || a.Confidence > 1 {
			return ErrConfidenceInvalid
		}
		if a.IsTransmission && a.Confidence > 0.6 {
			return ErrConfidenceInvalid
		}
		if !a.Direction.Valid() {
			return ErrConfidenceInvalid
		}
		if !a.AssetClass.Valid() {
			return ErrConfidenceInvalid
		}
		_ = i
	}
	return nil
}

// DebugDump JSON 化（观测/调试用）。
func (r IngestResult) DebugDump() string {
	b, _ := json.Marshal(r)
	return string(b)
}
