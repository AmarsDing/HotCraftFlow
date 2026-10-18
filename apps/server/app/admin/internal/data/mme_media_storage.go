package data

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-kratos/kratos/v2/log"

	bizmme "framework/app/admin/internal/biz/mme"
)

// MmeMediaStorage MME 媒体文件接触面（本地文件系统实现）。
// 布局：
//
//	<media_dir>/assets/<asset_id>/        素材原件 + 源/ASR 字幕
//	<media_dir>/variants/<variant_id>/    成片/封面/字幕/manifest
//	<media_dir>/work/<job_id>/            渲染中间产物（失败保留）
//	<export_dir>/<set_id>.zip             导出包
//
// 🔴 路径安全：所有外部传入 id 先经 sanitizeID（剔除分隔符），防目录穿越。
type MmeMediaStorage struct {
	cfg bizmme.Config
	log *log.Helper
}

// NewMmeMediaStorage 构造。
func NewMmeMediaStorage(cfg bizmme.Config, logger log.Logger) bizmme.MediaStorage {
	return &MmeMediaStorage{cfg: cfg, log: log.NewHelper(log.With(logger, "module", "mme/media-storage"))}
}

// sanitizeID 防路径穿越：只允许字母数字下划线连字符冒号点。
func sanitizeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9',
			r == '_', r == '-', r == '.', r == ':':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	return b.String()
}

// safeJoin 组合并校验逃逸（双保险）。
func (s *MmeMediaStorage) safeJoin(base, name string) string {
	p := filepath.Join(base, sanitizeID(name))
	if !strings.HasPrefix(filepath.Clean(p), filepath.Clean(base)) {
		s.log.Errorf("path escape blocked: %s", p)
		return filepath.Join(base, "_blocked_")
	}
	return p
}

func (s *MmeMediaStorage) MediaDir() string { return s.cfg.MediaDir }

func (s *MmeMediaStorage) AssetDir(assetID string) string {
	p := filepath.Join(s.cfg.MediaDir, "assets", sanitizeID(assetID))
	_ = os.MkdirAll(p, 0o755)
	return p
}

func (s *MmeMediaStorage) VariantDir(variantID string) string {
	p := filepath.Join(s.cfg.MediaDir, "variants", sanitizeID(variantID))
	_ = os.MkdirAll(p, 0o755)
	return p
}

func (s *MmeMediaStorage) WorkDir(jobID string) string {
	p := filepath.Join(s.cfg.MediaDir, "work", sanitizeID(jobID))
	_ = os.MkdirAll(p, 0o755)
	return p
}

// ExportZipDir 导出目录（export_dir 与 media_dir 分离，NFR：导出可挂不同盘）。
func (s *MmeMediaStorage) exportZipDir() string {
	_ = os.MkdirAll(s.cfg.ExportDir, 0o755)
	return s.cfg.ExportDir
}

// IngestFile 把本地文件复制进资产库（复制非移动——不破坏用户原件）。
func (s *MmeMediaStorage) IngestFile(_ context.Context, srcPath, assetID string) (string, int64, error) {
	src, err := os.Open(filepath.Clean(srcPath))
	if err != nil {
		return "", 0, fmt.Errorf("open local file: %w", err)
	}
	defer func() { _ = src.Close() }()
	dst := s.AssetDir(assetID) + string(filepath.Separator) + "src" + strings.ToLower(filepath.Ext(srcPath))
	out, err := os.Create(dst)
	if err != nil {
		return "", 0, fmt.Errorf("create asset file: %w", err)
	}
	defer func() { _ = out.Close() }()
	n, err := io.Copy(out, src)
	if err != nil {
		_ = os.Remove(dst)
		return "", 0, fmt.Errorf("copy asset file: %w", err)
	}
	return dst, n, nil
}

// ExportZip 把簇内全部 selected 产物打包（变体按其 id 分目录，含 manifest.json）。
func (s *MmeMediaStorage) ExportZip(ctx context.Context, setID string, variants []*bizmme.Variant) (string, error) {
	zipPath := filepath.Join(s.exportZipDir(), sanitizeID(setID)+".zip")
	out, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}
	defer func() { _ = out.Close() }()
	zw := zip.NewWriter(out)
	defer func() { _ = zw.Close() }()
	add := func(variantID, name, path string) error {
		if path == "" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		f, err := os.Open(filepath.Clean(path))
		if err != nil {
			return fmt.Errorf("zip add %s: %w", path, err)
		}
		defer func() { _ = f.Close() }()
		w, err := zw.Create(sanitizeID(variantID) + "/" + name)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, f)
		return err
	}
	for _, v := range variants {
		if !v.Selected {
			continue
		}
		if err = add(v.ID, "video.mp4", v.VideoPath); err != nil {
			return "", err
		}
		if err = add(v.ID, "cover.jpg", v.CoverPath); err != nil {
			return "", err
		}
		if err = add(v.ID, "subtitle.srt", v.SrtPath); err != nil {
			return "", err
		}
		if err = add(v.ID, "manifest.json", v.ManifestPath); err != nil {
			return "", err
		}
	}
	return zipPath, nil
}

func (s *MmeMediaStorage) Remove(_ context.Context, path string) error {
	if path == "" {
		return nil
	}
	return os.Remove(filepath.Clean(path))
}
