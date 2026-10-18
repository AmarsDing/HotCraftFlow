package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/radarwatchevent"

	adminV1 "framework/api/gen/go/admin/v1"
)

// RadarWatchEventRepo 盯盘事件仓储：行级强制 user_id，写入后按用户裁剪保留量。
type RadarWatchEventRepo struct {
	data *Data
	log  *log.Helper
}

func NewRadarWatchEventRepo(data *Data, logger log.Logger) biz.RadarWatchEventRepo {
	return &RadarWatchEventRepo{
		log:  log.NewHelper(log.With(logger, "module", "radar/watch-event/repo/admin-service")),
		data: data,
	}
}

// radarEventListLimit 单次列表返回上限（事件流视图，避免无限增长）。
const radarEventListLimit = 100

func (r *RadarWatchEventRepo) ListByUser(ctx context.Context, userID uint32, unreadOnly bool) ([]*biz.RadarWatchEvent, error) {
	q := r.data.db.Client().RadarWatchEvent.Query().
		Where(radarwatchevent.UserIDEQ(userID))
	if unreadOnly {
		q = q.Where(radarwatchevent.ReadEQ(false))
	}
	entities, err := q.
		Order(ent.Desc(radarwatchevent.FieldCreateTime), ent.Desc(radarwatchevent.FieldID)).
		Limit(radarEventListLimit).
		All(ctx)
	if err != nil {
		r.log.Errorf("query radar watch events failed (user_id=%d): %s", userID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query radar watch events failed")
	}
	out := make([]*biz.RadarWatchEvent, 0, len(entities))
	for _, e := range entities {
		out = append(out, toBizRadarWatchEvent(e))
	}
	return out, nil
}

func (r *RadarWatchEventRepo) CreateBatch(ctx context.Context, userID uint32, events []*biz.RadarWatchEvent) error {
	if len(events) == 0 {
		return nil
	}
	now := time.Now()
	builders := make([]*ent.RadarWatchEventCreate, 0, len(events))
	for _, ev := range events {
		builders = append(builders, r.data.db.Client().RadarWatchEvent.Create().
			SetUserID(userID).
			SetFullName(ev.FullName).
			SetKind(radarwatchevent.Kind(ev.Kind)).
			SetMessage(ev.Message).
			SetDelta(ev.Delta).
			SetRead(false).
			SetCreateTime(now).
			SetUpdateTime(now))
	}
	if err := r.data.db.Client().RadarWatchEvent.CreateBulk(builders...).Exec(ctx); err != nil {
		r.log.Errorf("create radar watch events failed (user_id=%d): %s", userID, err.Error())
		return adminV1.ErrorInternalServerError("create radar watch events failed")
	}
	return r.trimPerUser(ctx, userID)
}

// trimPerUser 裁剪单用户事件到保留上限（最旧优先删）。
func (r *RadarWatchEventRepo) trimPerUser(ctx context.Context, userID uint32) error {
	total, err := r.data.db.Client().RadarWatchEvent.Query().
		Where(radarwatchevent.UserIDEQ(userID)).
		Count(ctx)
	if err != nil {
		return err
	}
	overflow := total - biz.RadarMaxWatchEventsPerUser
	if overflow <= 0 {
		return nil
	}
	stale, err := r.data.db.Client().RadarWatchEvent.Query().
		Where(radarwatchevent.UserIDEQ(userID)).
		Order(ent.Asc(radarwatchevent.FieldCreateTime), ent.Asc(radarwatchevent.FieldID)).
		Limit(overflow).
		IDs(ctx)
	if err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	_, err = r.data.db.Client().RadarWatchEvent.Delete().
		Where(radarwatchevent.IDIn(stale...)).
		Exec(ctx)
	return err
}

func (r *RadarWatchEventRepo) MarkRead(ctx context.Context, userID uint32, ids []uint32) (int, error) {
	if len(ids) == 0 {
		return 0, nil // 空 ids 不做全量标记（语义见 biz 层）
	}
	n, err := r.data.db.Client().RadarWatchEvent.Update().
		Where(radarwatchevent.UserIDEQ(userID), radarwatchevent.IDIn(ids...)).
		SetRead(true).
		SetUpdateTime(time.Now()).
		Save(ctx)
	if err != nil {
		r.log.Errorf("mark radar watch events read failed (user_id=%d): %s", userID, err.Error())
		return 0, adminV1.ErrorInternalServerError("mark radar watch events read failed")
	}
	return n, nil
}

func (r *RadarWatchEventRepo) CountUnreadByRepo(ctx context.Context, userID uint32, fullName string) (uint32, error) {
	n, err := r.data.db.Client().RadarWatchEvent.Query().
		Where(radarwatchevent.UserIDEQ(userID), radarwatchevent.FullNameEQ(fullName), radarwatchevent.ReadEQ(false)).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count unread radar watch events failed (user_id=%d, full_name=%s): %s", userID, fullName, err.Error())
		return 0, adminV1.ErrorInternalServerError("count radar watch events failed")
	}
	return uint32(n), nil
}

func (r *RadarWatchEventRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int, error) {
	n, err := r.data.db.Client().RadarWatchEvent.Delete().
		Where(radarwatchevent.CreateTimeLT(cutoff)).
		Exec(ctx)
	if err != nil {
		r.log.Errorf("delete expired radar watch events failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("delete radar watch events failed")
	}
	return n, nil
}

func toBizRadarWatchEvent(e *ent.RadarWatchEvent) *biz.RadarWatchEvent {
	ev := &biz.RadarWatchEvent{
		ID:       e.ID,
		FullName: e.FullName,
		Kind:     string(e.Kind),
		Message:  e.Message,
		Read:     e.Read,
	}
	if e.CreateTime != nil {
		ev.CreateTime = *e.CreateTime
	}
	if e.Delta != nil {
		ev.Delta = *e.Delta
	}
	return ev
}
