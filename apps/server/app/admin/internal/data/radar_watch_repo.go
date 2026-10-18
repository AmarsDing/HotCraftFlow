package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/radarwatch"

	adminV1 "framework/api/gen/go/admin/v1"
)

// RadarWatchRepo 盯盘仓储：行级强制 user_id 条件。
type RadarWatchRepo struct {
	data *Data
	log  *log.Helper
}

func NewRadarWatchRepo(data *Data, logger log.Logger) biz.RadarWatchRepo {
	return &RadarWatchRepo{
		log:  log.NewHelper(log.With(logger, "module", "radar/watch/repo/admin-service")),
		data: data,
	}
}

func (r *RadarWatchRepo) ListByUser(ctx context.Context, userID uint32) ([]*biz.RadarWatch, error) {
	entities, err := r.data.db.Client().RadarWatch.Query().
		Where(radarwatch.UserIDEQ(userID)).
		Order(ent.Asc(radarwatch.FieldCreateTime)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query radar watches failed (user_id=%d): %s", userID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query radar watches failed")
	}
	out := make([]*biz.RadarWatch, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizRadarWatch(e))
	}
	return out, nil
}

func (r *RadarWatchRepo) GetByFullName(ctx context.Context, userID uint32, fullName string) (*biz.RadarWatch, error) {
	entity, err := r.data.db.Client().RadarWatch.Query().
		Where(radarwatch.UserIDEQ(userID), radarwatch.FullNameEQ(fullName)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query radar watch failed (user_id=%d, full_name=%s): %s", userID, fullName, err.Error())
		return nil, adminV1.ErrorInternalServerError("query radar watch failed")
	}
	return toBizRadarWatch(entity), nil
}

func (r *RadarWatchRepo) Upsert(ctx context.Context, userID uint32, fullName string, baselineStars int64) (*biz.RadarWatch, error) {
	existing, err := r.GetByFullName(ctx, userID, fullName)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}
	now := time.Now()
	entity, err := r.data.db.Client().RadarWatch.Create().
		SetUserID(userID).
		SetFullName(fullName).
		SetBaselineStars(baselineStars).
		SetHTMLURL("https://github.com/" + fullName).
		SetSparkline([]int64{}).
		SetCreateTime(now).
		SetUpdateTime(now).
		Save(ctx)
	if err != nil {
		r.log.Errorf("create radar watch failed (user_id=%d, full_name=%s): %s", userID, fullName, err.Error())
		return nil, adminV1.ErrorInternalServerError("create radar watch failed")
	}
	return toBizRadarWatch(entity), nil
}

func (r *RadarWatchRepo) Delete(ctx context.Context, userID uint32, id uint32) (bool, error) {
	n, err := r.data.db.Client().RadarWatch.Delete().
		Where(radarwatch.UserIDEQ(userID), radarwatch.IDEQ(id)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete radar watch failed (user_id=%d, id=%d): %s", userID, id, err.Error())
		return false, adminV1.ErrorInternalServerError("delete radar watch failed")
	}
	return n > 0, nil
}

func (r *RadarWatchRepo) UpdateStars(ctx context.Context, userID uint32, fullName string, stars, delta int64, sparkline []int64) error {
	_, err := r.data.db.Client().RadarWatch.Update().
		Where(radarwatch.UserIDEQ(userID), radarwatch.FullNameEQ(fullName)).
		SetStars(stars).
		SetStarsDelta(delta).
		SetSparkline(sparkline).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update radar watch stars failed (user_id=%d, full_name=%s): %s", userID, fullName, err.Error())
		return adminV1.ErrorInternalServerError("update radar watch failed")
	}
	return nil
}

func (r *RadarWatchRepo) SetUnreadCount(ctx context.Context, userID uint32, fullName string, n uint32) error {
	_, err := r.data.db.Client().RadarWatch.Update().
		Where(radarwatch.UserIDEQ(userID), radarwatch.FullNameEQ(fullName)).
		SetUnreadCount(n).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("update radar watch unread failed (user_id=%d, full_name=%s): %s", userID, fullName, err.Error())
		return adminV1.ErrorInternalServerError("update radar watch failed")
	}
	return nil
}

func toBizRadarWatch(e *ent.RadarWatch) *biz.RadarWatch {
	w := &biz.RadarWatch{
		ID:            e.ID,
		FullName:      e.FullName,
		BaselineStars: e.BaselineStars,
		Sparkline:     []int64{},
	}
	if e.CreateTime != nil {
		w.CreateTime = *e.CreateTime
	}
	if e.UpdateTime != nil {
		w.UpdateTime = *e.UpdateTime
	}
	if e.Stars != nil {
		w.Stars = *e.Stars
	}
	if e.StarsDelta != nil {
		w.StarsDelta = *e.StarsDelta
	}
	if e.UnreadCount != nil {
		w.UnreadCount = *e.UnreadCount
	}
	if e.HTMLURL != nil {
		w.HTMLURL = *e.HTMLURL
	}
	if e.Sparkline != nil {
		w.Sparkline = e.Sparkline
	}
	return w
}
