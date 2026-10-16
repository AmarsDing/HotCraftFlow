package data

import (
	"context"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/toollinkusermark"

	adminV1 "framework/api/gen/go/admin/v1"
)

// ToolLinkUserMarkRepo 用户标记仓储：收藏 + 个人点击统计。
type ToolLinkUserMarkRepo struct {
	data *Data
	log  *log.Helper
}

func NewToolLinkUserMarkRepo(data *Data, logger log.Logger) biz.ToolLinkUserMarkRepo {
	return &ToolLinkUserMarkRepo{
		log:  log.NewHelper(log.With(logger, "module", "tool-link-user-mark/repo/admin-service")),
		data: data,
	}
}

func (r *ToolLinkUserMarkRepo) ListByUser(ctx context.Context, userID uint32) ([]*biz.ToolLinkUserMark, error) {
	entities, err := r.data.db.Client().ToolLinkUserMark.Query().
		Where(toollinkusermark.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query user marks failed (user_id=%d): %s", userID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query user marks failed")
	}

	marks := make([]*biz.ToolLinkUserMark, 0, len(entities))
	for _, entity := range entities {
		mark := &biz.ToolLinkUserMark{
			ToolLinkID: entity.ToolLinkID,
			Favorite:   entity.Favorite != nil && *entity.Favorite,
		}
		if entity.ClickCount != nil {
			mark.ClickCount = *entity.ClickCount
		}
		if entity.LastClickAt != nil {
			mark.LastClickAt = *entity.LastClickAt
		}
		marks = append(marks, mark)
	}
	return marks, nil
}

// SetFavorite 设置/取消收藏（不存在则插入）。
func (r *ToolLinkUserMarkRepo) SetFavorite(ctx context.Context, userID, toolLinkID uint32, favorite bool) error {
	entity, err := r.findOne(ctx, userID, toolLinkID)
	if err != nil {
		return err
	}

	if entity == nil {
		return r.data.db.Client().ToolLinkUserMark.Create().
			SetUserID(userID).
			SetToolLinkID(toolLinkID).
			SetFavorite(favorite).
			SetCreateTime(time.Now()).
			Exec(ctx)
	}

	_, err = r.data.db.Client().ToolLinkUserMark.UpdateOneID(entity.ID).
		SetFavorite(favorite).
		SetUpdateTime(time.Now()).
		Save(ctx)
	return err
}

// RecordClick 记录一次点击（click_count+1 并刷新 last_click_at，不存在则插入）。
func (r *ToolLinkUserMarkRepo) RecordClick(ctx context.Context, userID, toolLinkID uint32) error {
	now := time.Now()

	entity, err := r.findOne(ctx, userID, toolLinkID)
	if err != nil {
		return err
	}

	if entity == nil {
		return r.data.db.Client().ToolLinkUserMark.Create().
			SetUserID(userID).
			SetToolLinkID(toolLinkID).
			SetClickCount(1).
			SetLastClickAt(now).
			SetCreateTime(now).
			Exec(ctx)
	}

	_, err = r.data.db.Client().ToolLinkUserMark.UpdateOneID(entity.ID).
		AddClickCount(1).
		SetLastClickAt(now).
		SetUpdateTime(now).
		Save(ctx)
	return err
}

// findOne 按 (user_id, tool_link_id) 唯一索引查询；未找到返回 nil,nil。
func (r *ToolLinkUserMarkRepo) findOne(ctx context.Context, userID, toolLinkID uint32) (*ent.ToolLinkUserMark, error) {
	entity, err := r.data.db.Client().ToolLinkUserMark.Query().
		Where(toollinkusermark.UserIDEQ(userID), toollinkusermark.ToolLinkIDEQ(toolLinkID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		r.log.Errorf("query user mark failed (user_id=%d, tool_link_id=%d): %s", userID, toolLinkID, err.Error())
		return nil, adminV1.ErrorInternalServerError("query user mark failed")
	}
	return entity, nil
}
