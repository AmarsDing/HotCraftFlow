package mme

import (
	"context"

	"github.com/go-kratos/kratos/v2/log"
)

// AssetDeduper FR-MIX-07 素材去重者：
// 同指纹素材复用已有 asset（ref_count++），命中后删除重复文件副本，
// 不重复下载产物占双份磁盘。
type AssetDeduper struct {
	cfg     Config
	assets  AssetRepo
	storage MediaStorage
	log     *log.Helper
}

// NewAssetDeduper 构造。
func NewAssetDeduper(cfg Config, assets AssetRepo, storage MediaStorage, logger log.Logger) *AssetDeduper {
	return &AssetDeduper{cfg: cfg, assets: assets, storage: storage, log: log.NewHelper(log.With(logger, "module", "biz/mme/dedup"))}
}

// DedupeResult 去重结论。
type DedupeResult struct {
	Asset     *Asset
	Duplicate bool // true = 命中已有素材，incoming 文件已被删除
}

// Ingest 把一份已下载/已入库文件注册为 Asset：
//   - 指纹命中 → ref_count++，删除调用方手里的重复文件，返回既有 Asset
//   - 未命中 → 建档（fingerprint 唯一约束兜底）
//
// filePath/sizeBytes/durationMS 必须已由调用方实测（不猜）。
func (d *AssetDeduper) Ingest(ctx context.Context, filePath, sourceURL, title string, sizeBytes, durationMS int64, width, height int) (*DedupeResult, error) {
	fp, err := FingerprintFile(filePath, sizeBytes, durationMS)
	if err != nil {
		return nil, err
	}
	if existing, _ := d.assets.GetByFingerprint(ctx, fp); existing != nil {
		n, err := d.assets.IncrRefCount(ctx, existing.ID)
		if err != nil {
			return nil, err
		}
		existing.RefCount = n
		// 元数据回填：既有行若是早期探测缺失环境入的库（0x0/0.0s），用本次实测值补齐。
		// 只填 0 值/空串字段，绝不覆盖既有真实值；回填失败仅告警（复用主线不中断）。
		if backfillAssetMeta(existing, sizeBytes, durationMS, width, height, title) {
			if uerr := d.assets.Update(ctx, existing); uerr != nil {
				d.log.Warnf("dedup hit metadata backfill failed: %s: %v", existing.ID, uerr)
			}
		}
		// 重复副本删除失败仅告警——不阻断主流程，但日志留痕。
		if rmErr := d.storage.Remove(ctx, filePath); rmErr != nil {
			d.log.Warnf("dedup hit but remove dup file failed: %s: %v", filePath, rmErr)
		}
		d.log.Infof("asset dedup hit: %s -> existing %s (ref_count=%d)", fp[:12], existing.ID, n)
		return &DedupeResult{Asset: existing, Duplicate: true}, nil
	}
	a := &Asset{
		Fingerprint: fp,
		SourceURL:   sourceURL,
		LocalPath:   filePath,
		DurationSec: float64(durationMS) / 1000.0,
		Width:       width,
		Height:      height,
		SizeBytes:   sizeBytes,
		RefCount:    1,
		MetaTitle:   title,
	}
	if err = d.assets.Create(ctx, a); err != nil {
		return nil, err
	}
	return &DedupeResult{Asset: a}, nil
}

// backfillAssetMeta 用本次实测值补齐既有素材的零值字段（只填空位，不覆盖）。
// 返回是否有变更。
func backfillAssetMeta(a *Asset, sizeBytes, durationMS int64, width, height int, title string) bool {
	changed := false
	if a.DurationSec <= 0 && durationMS > 0 {
		a.DurationSec = float64(durationMS) / 1000.0
		changed = true
	}
	if a.Width <= 0 && width > 0 {
		a.Width, a.Height = width, height
		changed = true
	}
	if a.SizeBytes <= 0 && sizeBytes > 0 {
		a.SizeBytes = sizeBytes
		changed = true
	}
	if a.MetaTitle == "" && title != "" {
		a.MetaTitle = title
		changed = true
	}
	return changed
}
