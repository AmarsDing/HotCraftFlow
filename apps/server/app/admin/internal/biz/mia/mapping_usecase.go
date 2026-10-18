package mia

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// MappingUsecase 映射表 A/B 的 CRUD 与 CSV 导入导出（FR-MIA-01/02）。
// 映射表是长期运营资产，必须非技术可维护（ADR-MIA-08）。
type MappingUsecase struct {
	repo  EntityMappingRepo
	nrepo NarrativeMappingRepo
	log   *log.Helper
}

// NewMappingUsecase 构造映射用例。
func NewMappingUsecase(repo EntityMappingRepo, nrepo NarrativeMappingRepo, logger log.Logger) *MappingUsecase {
	return &MappingUsecase{
		repo:  repo,
		nrepo: nrepo,
		log:   log.NewHelper(log.With(logger, "module", "biz/mia/mapping")),
	}
}

// ---------- 表 A：实体 → 标的 ----------

// ListEntityMappings 分页查询。
func (u *MappingUsecase) ListEntityMappings(ctx context.Context, q MappingQuery) ([]*MappingItem, int, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 500 {
		q.PageSize = 50
	}
	return u.repo.List(ctx, q)
}

// CreateEntityMapping 新增映射行（notes 必填）。
func (u *MappingUsecase) CreateEntityMapping(ctx context.Context, item *MappingItem) (*MappingItem, error) {
	if err := item.Validate(); err != nil {
		return nil, err
	}
	if item.Source == "" {
		item.Source = "manual"
	}
	return u.repo.Create(ctx, item)
}

// UpdateEntityMapping 更新映射行。
func (u *MappingUsecase) UpdateEntityMapping(ctx context.Context, id int64, item *MappingItem) (*MappingItem, error) {
	if err := item.Validate(); err != nil {
		return nil, err
	}
	return u.repo.Update(ctx, id, item)
}

// DeleteEntityMapping 删除映射行。
func (u *MappingUsecase) DeleteEntityMapping(ctx context.Context, id int64) error {
	return u.repo.Delete(ctx, id)
}

// entityCSVHeader 表 A CSV 表头（导入导出共用）。
var entityCSVHeader = []string{
	"entity_name", "entity_alias", "entity_type", "asset_class",
	"symbol", "direction_bias", "confidence", "source", "notes", "enabled",
}

// ImportEntityCSV 导入表 A CSV（按唯一键幂等 upsert）。
// 行级错误不中止导入，累计进 errors 返回（分析师批量导入场景）。
func (u *MappingUsecase) ImportEntityCSV(ctx context.Context, r io.Reader) (created, updated, skipped int, errs []string, err error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) == 0 {
		return 0, 0, 0, nil, nil
	}
	start := 0
	if rows[0][0] == "entity_name" { // 跳过表头
		start = 1
	}
	for i, row := range rows[start:] {
		lineNo := start + i + 1
		if len(row) < len(entityCSVHeader) {
			errs = append(errs, fmt.Sprintf("L%d: 列数不足（需 %d 列）", lineNo, len(entityCSVHeader)))
			skipped++
			continue
		}
		item := MappingItem{
			EntityName:    strings.TrimSpace(row[0]),
			EntityAlias:   splitCSVList(row[1], ';'),
			EntityType:    parseEntityType(row[2]),
			AssetClass:    parseAssetClass(row[3]),
			Symbol:        strings.TrimSpace(row[4]),
			DirectionBias: parseDirection(row[5]),
			Confidence:    parseFloatDefault(row[6], 0.6),
			Source:        strings.TrimSpace(row[7]),
			Notes:         strings.TrimSpace(row[8]),
			Enabled:       parseBoolDefault(row[9], true),
		}
		if item.Source == "" {
			item.Source = "import"
		}
		if item.EntityType == "" || item.AssetClass == "" || item.DirectionBias == "" {
			errs = append(errs, fmt.Sprintf("L%d: 非法枚举（type=%s class=%s bias=%s）", lineNo, row[2], row[3], row[5]))
			skipped++
			continue
		}
		if item.Notes == "" {
			errs = append(errs, fmt.Sprintf("L%d: notes 必填（无理由的映射不可信）", lineNo))
			skipped++
			continue
		}
		wasCreated, uerr := u.repo.UpsertByKey(ctx, &item)
		if uerr != nil {
			errs = append(errs, fmt.Sprintf("L%d: %v", lineNo, uerr))
			skipped++
			continue
		}
		if wasCreated {
			created++
		} else {
			updated++
		}
	}
	return created, updated, skipped, errs, nil
}

// ExportEntityCSV 导出表 A 全量 CSV。
func (u *MappingUsecase) ExportEntityCSV(ctx context.Context, w io.Writer) error {
	items, _, err := u.repo.List(ctx, MappingQuery{Page: 1, PageSize: 100000})
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write(entityCSVHeader); err != nil {
		return err
	}
	for _, it := range items {
		rec := []string{
			it.EntityName,
			strings.Join(it.EntityAlias, ";"),
			string(it.EntityType),
			string(it.AssetClass),
			it.Symbol,
			string(it.DirectionBias),
			strconv.FormatFloat(it.Confidence, 'f', 2, 64),
			it.Source,
			it.Notes,
			strconv.FormatBool(it.Enabled),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

// ---------- 表 B：叙事 → 币种 ----------

// ListNarrativeMappings 分页查询。
func (u *MappingUsecase) ListNarrativeMappings(ctx context.Context, q NarrativeQuery) ([]*NarrativeItem, int, error) {
	if q.Page < 1 {
		q.Page = 1
	}
	if q.PageSize < 1 || q.PageSize > 500 {
		q.PageSize = 50
	}
	return u.nrepo.List(ctx, q)
}

// CreateNarrativeMapping 新增叙事映射。
func (u *MappingUsecase) CreateNarrativeMapping(ctx context.Context, item *NarrativeItem) (*NarrativeItem, error) {
	if err := item.Validate(); err != nil {
		return nil, err
	}
	return u.nrepo.Create(ctx, item)
}

// UpdateNarrativeMapping 更新叙事映射。
func (u *MappingUsecase) UpdateNarrativeMapping(ctx context.Context, id int64, item *NarrativeItem) (*NarrativeItem, error) {
	if err := item.Validate(); err != nil {
		return nil, err
	}
	return u.nrepo.Update(ctx, id, item)
}

// DeleteNarrativeMapping 删除叙事映射。
func (u *MappingUsecase) DeleteNarrativeMapping(ctx context.Context, id int64) error {
	return u.nrepo.Delete(ctx, id)
}

// narrativeCSVHeader 表 B CSV 表头。
var narrativeCSVHeader = []string{
	"narrative", "keywords", "coins", "default_sentiment_hint", "ttl_hours", "confidence", "enabled",
}

// ImportNarrativeCSV 导入表 B CSV（按 narrative 幂等 upsert）。
func (u *MappingUsecase) ImportNarrativeCSV(ctx context.Context, r io.Reader) (created, updated, skipped int, errs []string, err error) {
	cr := csv.NewReader(r)
	cr.TrimLeadingSpace = true
	rows, err := cr.ReadAll()
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) == 0 {
		return 0, 0, 0, nil, nil
	}
	start := 0
	if rows[0][0] == "narrative" {
		start = 1
	}
	for i, row := range rows[start:] {
		lineNo := start + i + 1
		if len(row) < len(narrativeCSVHeader) {
			errs = append(errs, fmt.Sprintf("L%d: 列数不足（需 %d 列）", lineNo, len(narrativeCSVHeader)))
			skipped++
			continue
		}
		item := NarrativeItem{
			Narrative:            strings.TrimSpace(row[0]),
			Keywords:             splitCSVList(row[1], ';'),
			Coins:                splitCSVList(row[2], ';'),
			DefaultSentimentHint: parseDirection(row[3]),
			TTLHours:             parseIntDefault(row[4], 6),
			Confidence:           parseFloatDefault(row[5], 0.6),
			Enabled:              parseBoolDefault(row[6], true),
		}
		if item.DefaultSentimentHint == "" {
			errs = append(errs, fmt.Sprintf("L%d: 非法情感提示=%s", lineNo, row[3]))
			skipped++
			continue
		}
		if vErr := item.Validate(); vErr != nil {
			errs = append(errs, fmt.Sprintf("L%d: %v", lineNo, vErr))
			skipped++
			continue
		}
		wasCreated, uerr := u.nrepo.UpsertByKey(ctx, &item)
		if uerr != nil {
			errs = append(errs, fmt.Sprintf("L%d: %v", lineNo, uerr))
			skipped++
			continue
		}
		if wasCreated {
			created++
		} else {
			updated++
		}
	}
	return created, updated, skipped, errs, nil
}

// ExportNarrativeCSV 导出表 B 全量 CSV。
func (u *MappingUsecase) ExportNarrativeCSV(ctx context.Context, w io.Writer) error {
	items, _, err := u.nrepo.List(ctx, NarrativeQuery{Page: 1, PageSize: 100000})
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	defer cw.Flush()
	if err := cw.Write(narrativeCSVHeader); err != nil {
		return err
	}
	for _, it := range items {
		rec := []string{
			it.Narrative,
			strings.Join(it.Keywords, ";"),
			strings.Join(it.Coins, ";"),
			string(it.DefaultSentimentHint),
			strconv.Itoa(it.TTLHours),
			strconv.FormatFloat(it.Confidence, 'f', 2, 64),
			strconv.FormatBool(it.Enabled),
		}
		if err := cw.Write(rec); err != nil {
			return err
		}
	}
	return nil
}

// ---------- CSV 解析辅助 ----------

// splitCSVList 分号分隔的列表单元格 → 字符串切片（去空白去空项）。
func splitCSVList(s string, sep rune) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, string(sep))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func parseFloatDefault(s string, def float64) float64 {
	if s == "" {
		return def
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return def
	}
	return v
}

func parseIntDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return v
}

func parseBoolDefault(s string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes":
		return true
	case "false", "0", "no":
		return false
	default:
		return def
	}
}

func parseEntityType(s string) mediacontract.EntityType {
	return mediacontract.EntityType(strings.TrimSpace(s))
}
func parseAssetClass(s string) mediacontract.AssetClass {
	return mediacontract.AssetClass(strings.TrimSpace(s))
}
func parseDirection(s string) mediacontract.Direction {
	return mediacontract.Direction(strings.TrimSpace(s))
}
