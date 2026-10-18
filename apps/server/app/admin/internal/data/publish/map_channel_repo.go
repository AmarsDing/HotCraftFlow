// Package pubdata 是 ⑤ MAP biz/publish 持久化端口的 Ent/Redis/Crypto 实现。
// 目录 data/publish，包名 pubdata（与 biz/publish 区分；模式对齐 data/tt → ttdata）。
package pubdata

import (
	"context"
	"fmt"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/entgo"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/mappublishchannel"
)

// EntChannelRepo bizpub.ChannelRepo 的 Ent 实现。
type EntChannelRepo struct {
	cli *entgo.EntClient[*ent.Client]
	log *log.Helper
}

// NewEntChannelRepo 构造。
func NewEntChannelRepo(cli *entgo.EntClient[*ent.Client], logger log.Logger) *EntChannelRepo {
	return &EntChannelRepo{
		cli: cli,
		log: log.NewHelper(log.With(logger, "module", "data/map-channel-repo")),
	}
}

// CreateChannel 新建渠道（platform:slot 唯一冲突 → 400 语义）。
func (r *EntChannelRepo) CreateChannel(ctx context.Context, row *bizpub.ChannelRow) error {
	builder := r.cli.Client().MapPublishChannel.Create().
		SetChannelID(row.ChannelID).
		SetPlatformKey(row.PlatformKey).
		SetSlot(row.Slot).
		SetRegion(mappublishchannel.Region(row.Region)).
		SetEnabled(row.Enabled).
		SetPriority(row.Priority)
	if row.Capability != nil {
		builder.SetCapability(row.Capability)
	}
	if row.AdapterVersion != nil {
		builder.SetNillableAdapterVersion(row.AdapterVersion)
	}
	e, err := builder.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return fmt.Errorf("%w: 渠道 %s:%s 已存在", bizpub.ErrBadRequest, row.PlatformKey, row.Slot)
		}
		r.log.Errorf("create map channel failed: %v", err)
		return fmt.Errorf("%w: 创建渠道失败", bizpub.ErrInternal)
	}
	row.ID = uint32(e.ID)
	row.CreateTime = derefTime(e.CreateTime)
	return nil
}

// GetChannelBySlot 按 platform:slot 查渠道。
func (r *EntChannelRepo) GetChannelBySlot(ctx context.Context, platform, slot string) (*bizpub.ChannelRow, error) {
	e, err := r.cli.Client().MapPublishChannel.Query().
		Where(
			mappublishchannel.PlatformKeyEQ(platform),
			mappublishchannel.SlotEQ(slot),
		).
		Only(ctx)
	if err != nil {
		return nil, mapQueryErr(err, "渠道")
	}
	return channelToRow(e), nil
}

// GetChannel 按 channel_id 查渠道。
func (r *EntChannelRepo) GetChannel(ctx context.Context, channelID string) (*bizpub.ChannelRow, error) {
	e, err := r.cli.Client().MapPublishChannel.Query().
		Where(mappublishchannel.ChannelIDEQ(channelID)).
		Only(ctx)
	if err != nil {
		return nil, mapQueryErr(err, "渠道")
	}
	return channelToRow(e), nil
}

// ListChannels 分页列出（create_time 倒序）。
func (r *EntChannelRepo) ListChannels(ctx context.Context, pageNo, pageSize int) ([]*bizpub.ChannelRow, int, error) {
	pageNo, pageSize = clampPage(pageNo, pageSize)
	q := r.cli.Client().MapPublishChannel.Query()
	total, err := q.Clone().Count(ctx)
	if err != nil {
		r.log.Errorf("count map channels failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询渠道总数失败", bizpub.ErrInternal)
	}
	entities, err := q.
		Order(ent.Desc(mappublishchannel.FieldCreateTime)).
		Offset((pageNo - 1) * pageSize).
		Limit(pageSize).
		All(ctx)
	if err != nil {
		r.log.Errorf("list map channels failed: %v", err)
		return nil, 0, fmt.Errorf("%w: 查询渠道列表失败", bizpub.ErrInternal)
	}
	rows := make([]*bizpub.ChannelRow, 0, len(entities))
	for _, e := range entities {
		rows = append(rows, channelToRow(e))
	}
	return rows, total, nil
}

func channelToRow(e *ent.MapPublishChannel) *bizpub.ChannelRow {
	return &bizpub.ChannelRow{
		ID:             uint32(e.ID),
		ChannelID:      e.ChannelID,
		PlatformKey:    e.PlatformKey,
		Slot:           e.Slot,
		Region:         string(e.Region),
		Enabled:        e.Enabled,
		Capability:     e.Capability,
		AdapterVersion: e.AdapterVersion,
		Priority:       e.Priority,
		CreateTime:     derefTime(e.CreateTime),
	}
}

// clampPage 分页参数护栏（防 0/负/超大页）。
func clampPage(pageNo, pageSize int) (int, int) {
	if pageNo < 1 {
		pageNo = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return pageNo, pageSize
}

// mapQueryErr ent 查询错误 → biz 语义错误（NotFound/Internal；调用方 wording 传资源名）。
func mapQueryErr(err error, what string) error {
	if ent.IsNotFound(err) {
		return fmt.Errorf("%w: %s不存在", bizpub.ErrNotFound, what)
	}
	return fmt.Errorf("%w: 查询%s失败: %v", bizpub.ErrInternal, what, err)
}

// enumPtrToStr nillable 枚举指针 → 字符串（ent Nillable 枚举实体字段为指针）。
func enumPtrToStr[T ~string](p *T) string {
	if p == nil {
		return ""
	}
	return string(*p)
}

// derefTime mixin.Time{} 生成的 create_time 为 *time.Time（本项目 ent 配置），零值安全解引用。
func derefTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}
