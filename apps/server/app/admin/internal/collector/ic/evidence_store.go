package iccollector

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	contract "framework/pkg/mediacontract"
)

// =============================================================================
// 证据存储（S-27 V1 本机目录；B-17 只追加语义：同 intel 二次写入拒覆盖，
// 幂等重放按内容一致判定成功）。文件名 = intel_id，目录按月分片（YYYYMM）。
// PIPL：落盘前邮箱/手机号/身份证正则脱敏（评审 gap#15）。
// =============================================================================

var (
	rePIIEmail = regexp.MustCompile(`[\w.+-]+@[\w-]+\.[\w.-]+`)
	// RE2 不支持 look-around（(?<!/(?!)，init 即 panic，radar_user_data.go 已有同款踩坑）：
	// 数字边界改捕获组实现，替换时回写边界字符（见 redactPII 注释两遍语义）。
	rePIIPhone  = regexp.MustCompile(`(^|[^\d])(1[3-9]\d{9})([^\d]|$)`)
	rePIIIDCard = regexp.MustCompile(`(^|[^\d])([0-9]{17}[\dXx])([^\d]|$)`)
)

// redactPII 最佳努力脱敏（字节级；gzip 等压缩体不展开——采集 HTTP 栈不请求压缩编码，见 httpdo 无 Accept-Encoding 设置）。
// 每个数字类规则跑两遍：首遍命中的尾部边界字符被消费，可能挡住紧邻的下一条命中，
// 二遍扫描兜底；两遍收敛（每遍至少吃掉一半共享边界）。
func redactPII(b []byte) []byte {
	b = rePIIEmail.ReplaceAll(b, []byte("[email-redacted]"))
	s := string(b)
	s = rePIIPhone.ReplaceAllString(s, "${1}[phone-redacted]${3}")
	s = rePIIPhone.ReplaceAllString(s, "${1}[phone-redacted]${3}")
	s = rePIIIDCard.ReplaceAllString(s, "${1}[id-redacted]${3}")
	s = rePIIIDCard.ReplaceAllString(s, "${1}[id-redacted]${3}")
	return []byte(s)
}

// FSEvidenceStore biz.EvidenceStorePort 实现。
type FSEvidenceStore struct {
	dir      string
	evidence biz.EvidenceRepo // 导出链文件需要 DB 视角（B-17 chain.json）
	log      *log.Helper
}

func NewFSEvidenceStore(cfg *biz.IcConfig, evidence biz.EvidenceRepo, logger log.Logger) (*FSEvidenceStore, error) {
	dir := cfg.EvidenceStorageDir
	if dir == "" {
		return nil, errors.New("ic.evidence.storage_dir 必填")
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, err
	}
	return &FSEvidenceStore{
		dir: dir, evidence: evidence,
		log: log.NewHelper(log.With(logger, "module", "ic/evidence-store")),
	}, nil
}

// pathFor intel_id 的相对路径（月分片）。
func (s *FSEvidenceStore) pathFor(intelID string, at time.Time) string {
	return filepath.Join(at.UTC().Format("200601"), intelID+".raw")
}

// Save 落盘 + 截断 + 返回元信息。rawHash 基于落盘字节（截断/脱敏后——
// 与 B-17 链一致口径：链提交的是"在档证据"，非"线上原文"）。
func (s *FSEvidenceStore) Save(_ context.Context, intelID string, fragment *contract.RawFragment, maxBytes int64) (rawPath, rawHash string, size int64, truncated bool, err error) {
	content := fragment.Content
	size = int64(len(content))
	if maxBytes > 0 && size > maxBytes {
		content = content[:maxBytes]
		truncated = true
	}
	content = redactPII(content)
	rawHash = contract.RawSHA256(content)

	rawPath = s.pathFor(intelID, time.Now())
	full, err := s.absInDir(rawPath)
	if err != nil {
		return "", "", 0, false, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", "", 0, false, err
	}

	// B-17 只追加：已存在 → 校验同 hash（幂等重放）否则拒覆盖。
	if existing, err := os.ReadFile(full); err == nil {
		if contract.RawSHA256(existing) == rawHash {
			return rawPath, rawHash, size, truncated, nil
		}
		return "", "", 0, false, fmt.Errorf("evidence exists with different hash (append-only violated): %s", intelID)
	}
	tmp := full + ".tmp"
	if err := os.WriteFile(tmp, content, 0o640); err != nil {
		return "", "", 0, false, err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return "", "", 0, false, err
	}
	return rawPath, rawHash, size, truncated, nil
}

// Delete 保留期清扫（仅 retention 任务调用；删除须留审计日志——调用方职责）。
func (s *FSEvidenceStore) Delete(_ context.Context, rawPath string) error {
	if rawPath == "" {
		return nil
	}
	full, err := s.absInDir(rawPath)
	if err != nil {
		return err
	}
	if err := os.Remove(full); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// Export 打包 ZIP（B-17/M-19）：条目全部证据文件 + chain.json（链全量）+ meta.json
// （含导出审计字段），返回 zip 相对路径与内容 sha256（meta.json 审计字段由调用方注入）。
func (s *FSEvidenceStore) Export(ctx context.Context, intelID string, meta map[string]string) (zipPath, zipSHA256 string, err error) {
	snapshots, err := s.evidence.ListByIntelID(ctx, intelID)
	if err != nil {
		return "", "", err
	}
	if len(snapshots) == 0 {
		return "", "", errors.New("no evidence snapshots to export")
	}
	zipPath = filepath.Join("export", intelID+"-"+time.Now().UTC().Format("20060102150405")+".zip")
	full, err := s.absInDir(zipPath)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return "", "", err
	}

	tmp := full + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", "", err
	}
	zw := zip.NewWriter(f)

	// 1) 原文文件（按采集时刻排序，只读追加，不修改原件）
	ordered := append([]*biz.EvidenceSnapshotRow{}, snapshots...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].CollectedAt.Before(ordered[j].CollectedAt) })
	for i, ev := range ordered {
		entry := fmt.Sprintf("raw/%02d-%s.raw", i, ev.ID)
		if werr := s.addFile(zw, entry, ev.RawPath); werr != nil {
			_ = zw.Close()
			_ = f.Close()
			_ = os.Remove(tmp)
			return "", "", werr
		}
	}

	// 2) chain.json（B-17 导出复核材料）
	type chainEntry struct {
		EvidenceID  string    `json:"evidence_id"`
		CollectedAt time.Time `json:"collected_at"`
		RawHash     string    `json:"raw_hash"`
		PrevHash    string    `json:"prev_hash"`
		ChainHash   string    `json:"chain_hash"`
	}
	chain := make([]chainEntry, 0, len(ordered))
	for _, ev := range ordered {
		chain = append(chain, chainEntry{
			EvidenceID: ev.ID, CollectedAt: ev.CollectedAt.UTC(),
			RawHash: ev.RawHash, PrevHash: ev.PrevHash, ChainHash: ev.ChainHash,
		})
	}
	if err := addJSONEntry(zw, "chain.json", map[string]any{
		"intel_id": intelID, "count": len(chain), "entries": chain,
	}); err != nil {
		_ = zw.Close()
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", "", err
	}

	// 3) meta.json（审计字段由调用方注入：导出者/时刻/用途）
	if err := addJSONEntry(zw, "meta.json", meta); err != nil {
		_ = zw.Close()
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", "", err
	}

	if err := zw.Close(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return "", "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	sum, err := fileSHA256(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	if err := os.Rename(tmp, full); err != nil {
		_ = os.Remove(tmp)
		return "", "", err
	}
	return zipPath, sum, nil
}

// addFile 把一条在档证据复制进 zip。
func (s *FSEvidenceStore) addFile(zw *zip.Writer, entry, rawPath string) error {
	full, err := s.absInDir(rawPath)
	if err != nil {
		return err
	}
	src, err := os.Open(full)
	if err != nil {
		return fmt.Errorf("open evidence %s: %w", rawPath, err)
	}
	defer src.Close()
	dst, err := zw.Create(entry)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	return err
}

func addJSONEntry(zw *zip.Writer, name string, v any) error {
	dst, err := zw.Create(name)
	if err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = dst.Write(b)
	return err
}

// absInDir 路径逃逸防御：返回 dir 内的绝对路径，越界报错。
func (s *FSEvidenceStore) absInDir(rel string) (string, error) {
	root, err := filepath.Abs(s.dir)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(filepath.Join(root, rel))
	if err != nil {
		return "", err
	}
	relToRoot, err := filepath.Rel(root, abs)
	if err != nil || relToRoot == ".." || (len(relToRoot) > 2 && relToRoot[:3] == ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes storage dir: %s", rel)
	}
	return abs, nil
}

func fileSHA256(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return contract.RawSHA256(b), nil
}

var _ biz.EvidenceStorePort = (*FSEvidenceStore)(nil)
