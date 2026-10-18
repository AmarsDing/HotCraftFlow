package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	adminV1 "framework/api/gen/go/admin/v1"
	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/evidencesnapshot"
)

// EvidenceRepo 取证快照仓储（FR-INT-11 不可裁剪）。
// 插入走 IntelItemRepo.UpsertWithEvidenceTx（B-8：与条目同事务），此处为读侧 + 保留期清扫。
type EvidenceRepo struct {
	data *Data
	log  *log.Helper
}

func NewEvidenceRepo(data *Data, logger log.Logger) biz.EvidenceRepo {
	return &EvidenceRepo{
		log:  log.NewHelper(log.With(logger, "module", "ic/evidence/repo/admin-service")),
		data: data,
	}
}

func (r *EvidenceRepo) GetByID(ctx context.Context, id string) (*biz.EvidenceSnapshotRow, error) {
	e, err := r.data.db.Client().EvidenceSnapshot.Query().
		Where(evidencesnapshot.IDEQ(id)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query evidence failed (id=%s): %s", id, err.Error())
		return nil, adminV1.ErrorInternalServerError("query evidence failed")
	}
	return toBizEvidenceSnapshot(e), nil
}

func (r *EvidenceRepo) GetLatestByIntelID(ctx context.Context, intelID string) (*biz.EvidenceSnapshotRow, error) {
	e, err := r.data.db.Client().EvidenceSnapshot.Query().
		Where(evidencesnapshot.IntelIDEQ(intelID)).
		Order(ent.Desc(evidencesnapshot.FieldCollectedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query latest evidence failed (intel=%s): %s", intelID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query evidence failed")
	}
	return toBizEvidenceSnapshot(e), nil
}

func (r *EvidenceRepo) ListByIntelID(ctx context.Context, intelID string) ([]*biz.EvidenceSnapshotRow, error) {
	entities, err := r.data.db.Client().EvidenceSnapshot.Query().
		Where(evidencesnapshot.IntelIDEQ(intelID)).
		Order(ent.Asc(evidencesnapshot.FieldCollectedAt)).
		All(ctx)
	if err != nil {
		r.log.Errorf("list evidence failed (intel=%s): %s", intelID, err.Error())
		return nil, adminV1.ErrorInternalServerError("list evidence failed")
	}
	out := make([]*biz.EvidenceSnapshotRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizEvidenceSnapshot(e))
	}
	return out, nil
}

// LatestChainHash B-17：返回该条目最近一次链哈希（验证器用它回放链）。
func (r *EvidenceRepo) LatestChainHash(ctx context.Context, intelID string) (string, error) {
	e, err := r.data.db.Client().EvidenceSnapshot.Query().
		Where(evidencesnapshot.IntelIDEQ(intelID)).
		Order(ent.Desc(evidencesnapshot.FieldCollectedAt)).
		First(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", nil
		}
		r.log.Errorf("query latest chain hash failed (intel=%s): %s", intelID, err.Error())
		return "", adminV1.ErrorInternalServerError("query chain hash failed")
	}
	return e.ChainHash, nil
}

// ListExpired retention（NFR-IC-08）：retain_until 到期的快照，先删文件后删行。
func (r *EvidenceRepo) ListExpired(ctx context.Context, before time.Time, limit int) ([]*biz.EvidenceSnapshotRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	entities, err := r.data.db.Client().EvidenceSnapshot.Query().
		Where(evidencesnapshot.RetainUntilLT(before.UTC())).
		Order(ent.Asc(evidencesnapshot.FieldRetainUntil)).
		Limit(limit).
		All(ctx)
	if err != nil {
		r.log.Errorf("list expired evidence failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("list expired evidence failed")
	}
	out := make([]*biz.EvidenceSnapshotRow, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizEvidenceSnapshot(e))
	}
	return out, nil
}

func (r *EvidenceRepo) DeleteIDs(ctx context.Context, ids []string) (int, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	n, err := r.data.db.Client().EvidenceSnapshot.Delete().
		Where(evidencesnapshot.IDIn(ids...)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete evidence ids failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("delete evidence failed")
	}
	return n, nil
}

func toBizEvidenceSnapshot(e *ent.EvidenceSnapshot) *biz.EvidenceSnapshotRow {
	if e == nil {
		return nil
	}
	return &biz.EvidenceSnapshotRow{
		ID:            e.ID,
		IntelID:       e.IntelID,
		CollectedAt:   e.CollectedAt.UTC(),
		Method:        e.Method,
		RequestURL:    e.RequestURL,
		FinalURL:      derefStr(e.FinalURL),
		HTTPStatus:    e.HTTPStatus,
		HTTPHeaders:   e.HTTPHeaders,
		RawPath:       derefStr(e.RawPath),
		RawHash:       derefStr(e.RawHash),
		PrevHash:      e.PrevHash,
		ChainHash:     e.ChainHash,
		RobotsChecked: e.RobotsChecked,
		RobotsAllowed: e.RobotsAllowed,
		CrawlDelayMs:  e.CrawlDelayMs,
		RetainUntil:   e.RetainUntil.UTC(),
		RawSizeBytes:  e.RawSizeBytes,
	}
}
