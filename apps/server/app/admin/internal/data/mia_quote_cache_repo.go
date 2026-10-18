package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	bizmia "framework/app/admin/internal/biz/mia"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/miaquotecache"
)

// MiaQuoteCacheRepo 行情缓存仓储（FR-MIA-07）。
// 本表只存 OHLCV 只读快照（NFR-MIA-02：绝不存交易凭据）。
type MiaQuoteCacheRepo struct {
	data *Data
	log  *log.Helper
}

// NewMiaQuoteCacheRepo 构造。
func NewMiaQuoteCacheRepo(data *Data, logger log.Logger) bizmia.QuoteCacheRepo {
	return &MiaQuoteCacheRepo{
		data: data,
		log:  log.NewHelper(log.With(logger, "module", "mia/quote-cache/repo/admin-service")),
	}
}

// Get 查询 symbol+provider 的缓存快照；maxAge 内无数据返回 (nil, nil)。
func (r *MiaQuoteCacheRepo) Get(ctx context.Context, symbol, provider string, maxAge time.Duration) (*bizmia.QuoteSnapshot, error) {
	query := r.data.db.Client().MiaQuoteCache.Query().
		Where(miaquotecache.SymbolEQ(symbol), miaquotecache.ProviderEQ(provider))
	if maxAge > 0 {
		query = query.Where(miaquotecache.FetchedAtGTE(time.Now().UTC().Add(-maxAge)))
	}
	e, err := query.First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("get quote cache failed (symbol=%s provider=%s): %s", symbol, provider, err.Error())
		return nil, adminV1.ErrorInternalServerError("get quote cache failed")
	}
	return &bizmia.QuoteSnapshot{
		Symbol:    e.Symbol,
		Provider:  e.Provider,
		Bars:      ohlcvFromJSON(e.Ohlcv),
		FetchedAt: e.FetchedAt,
		FromCache: true,
	}, nil
}

// Upsert 按复合键 (symbol, provider) 覆盖快照。
func (r *MiaQuoteCacheRepo) Upsert(ctx context.Context, snap *bizmia.QuoteSnapshot) error {
	existing, err := r.data.db.Client().MiaQuoteCache.Query().
		Where(miaquotecache.SymbolEQ(snap.Symbol), miaquotecache.ProviderEQ(snap.Provider)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		r.log.Errorf("upsert quote cache lookup failed (symbol=%s): %s", snap.Symbol, err.Error())
		return adminV1.ErrorInternalServerError("upsert quote cache failed")
	}
	now := time.Now()
	if existing == nil {
		_, err = r.data.db.Client().MiaQuoteCache.Create().
			SetSymbol(snap.Symbol).
			SetProvider(snap.Provider).
			SetOhlcv(ohlcvToJSON(snap.Bars)).
			SetFetchedAt(snap.FetchedAt.UTC()).
			SetCreateTime(now).
			SetUpdateTime(now).
			Save(ctx)
	} else {
		_, err = r.data.db.Client().MiaQuoteCache.UpdateOneID(existing.ID).
			SetOhlcv(ohlcvToJSON(snap.Bars)).
			SetFetchedAt(snap.FetchedAt.UTC()).
			SetUpdateTime(now).
			Save(ctx)
	}
	if err != nil {
		r.log.Errorf("upsert quote cache failed (symbol=%s): %s", snap.Symbol, err.Error())
		return adminV1.ErrorInternalServerError("upsert quote cache failed")
	}
	return nil
}

// ohlcvToJSON K 线 → JSON 列（紧凑键名控存储体积）。
func ohlcvToJSON(bars []bizmia.OhlcvBar) []map[string]any {
	out := make([]map[string]any, 0, len(bars))
	for _, b := range bars {
		out = append(out, map[string]any{
			"t": b.T.UTC().Unix(),
			"o": b.Open,
			"h": b.High,
			"l": b.Low,
			"c": b.Close,
			"v": b.Volume,
		})
	}
	return out
}

// ohlcvFromJSON JSON 列 → K 线（字段缺失容忍零值）。
func ohlcvFromJSON(rows []map[string]any) []bizmia.OhlcvBar {
	out := make([]bizmia.OhlcvBar, 0, len(rows))
	num := func(m map[string]any, k string) float64 {
		if v, ok := m[k].(float64); ok {
			return v
		}
		return 0
	}
	for _, row := range rows {
		var b bizmia.OhlcvBar
		if ts, ok := row["t"].(float64); ok {
			b.T = time.Unix(int64(ts), 0).UTC()
		}
		b.Open = num(row, "o")
		b.High = num(row, "h")
		b.Low = num(row, "l")
		b.Close = num(row, "c")
		b.Volume = num(row, "v")
		out = append(out, b)
	}
	return out
}
