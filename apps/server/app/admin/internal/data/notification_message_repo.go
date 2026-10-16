package data

import (
	"context"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/copierutil"
	entgo "github.com/tx7do/go-utils/entgo/query"
	entgoUpdate "github.com/tx7do/go-utils/entgo/update"
	"github.com/tx7do/go-utils/fieldmaskutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/notificationmessage"
	"framework/app/admin/internal/data/ent/notificationmessagecategory"
	"framework/app/admin/internal/data/ent/notificationmessagerecipient"

	internalMessageV1 "framework/api/gen/go/internal_message/v1"
)

type NotificationMessageRepo struct {
	data *Data
	log  *log.Helper

	mapper          *mapper.CopierMapper[internalMessageV1.NotificationMessage, ent.NotificationMessage]
	statusConverter *mapper.EnumTypeConverter[internalMessageV1.MessageStatus, notificationmessage.Status]
}

func NewNotificationMessageRepo(data *Data, logger log.Logger) *NotificationMessageRepo {
	repo := &NotificationMessageRepo{
		log:             log.NewHelper(log.With(logger, "module", "notification-message/repo/admin-service")),
		data:            data,
		mapper:          mapper.NewCopierMapper[internalMessageV1.NotificationMessage, ent.NotificationMessage](),
		statusConverter: mapper.NewEnumTypeConverter[internalMessageV1.MessageStatus, notificationmessage.Status](internalMessageV1.MessageStatus_name, internalMessageV1.MessageStatus_value),
	}

	repo.init()

	return repo
}

func (r *NotificationMessageRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
}

// CreateSystemMessage 创建一条 PUBLISHED 系统消息并批量投递给接收人（RECEIVED），同一事务。
// 实现 biz.InternalMessageNotifier 端口：供库存预警等子系统按规则 notify_users 直达站内信。
func (r *NotificationMessageRepo) CreateSystemMessage(ctx context.Context, subject, content string, recipientIDs []uint32) error {
	if subject == "" || len(recipientIDs) == 0 {
		return internalMessageV1.ErrorBadRequest("invalid parameter: subject and recipientIDs are required")
	}

	return RunInTx(ctx, r.data.db.Client(), func(tx *ent.Tx) error {
		now := time.Now()
		msg, err := tx.NotificationMessage.Create().
			SetSubject(subject).
			SetContent(content).
			SetStatus(notificationmessage.StatusPublished).
			SetCreateTime(now).
			Save(ctx)
		if err != nil {
			r.log.Errorf("insert system message failed: %s", err.Error())
			return internalMessageV1.ErrorInternalServerError("insert system message failed")
		}

		builders := make([]*ent.NotificationMessageRecipientCreate, 0, len(recipientIDs))
		for _, uid := range recipientIDs {
			if uid == 0 {
				continue
			}
			builders = append(builders, tx.NotificationMessageRecipient.Create().
				SetMessageID(msg.ID).
				SetRecipientID(uid).
				SetStatus(notificationmessagerecipient.StatusReceived).
				SetCreateTime(now))
		}
		if len(builders) == 0 {
			return internalMessageV1.ErrorBadRequest("invalid parameter: recipientIDs are all zero")
		}
		if err := tx.NotificationMessageRecipient.CreateBulk(builders...).Exec(ctx); err != nil {
			r.log.Errorf("insert system message recipients failed: %s", err.Error())
			return internalMessageV1.ErrorInternalServerError("insert system message recipients failed")
		}
		return nil
	})
}

// fillCategoryName 批量回填分类名称。NotificationMessage 只存 categoryId，
// categoryName 是派生展示字段，需查询 notification_message_categories 表填充。
func (r *NotificationMessageRepo) fillCategoryName(ctx context.Context, dtos []*internalMessageV1.NotificationMessage) error {
	if len(dtos) == 0 {
		return nil
	}
	seen := make(map[uint32]struct{})
	ids := make([]uint32, 0, len(dtos))
	for _, dto := range dtos {
		if dto.CategoryId == nil {
			continue
		}
		id := dto.GetCategoryId()
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	categories, err := r.data.db.Client().NotificationMessageCategory.Query().
		Where(notificationmessagecategory.IDIn(ids...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query notification message categories failed: %s", err.Error())
		return internalMessageV1.ErrorInternalServerError("query categories failed")
	}
	nameByID := make(map[uint32]string, len(categories))
	for _, c := range categories {
		if c.Name != nil {
			nameByID[c.ID] = *c.Name
		}
	}
	for _, dto := range dtos {
		if dto.CategoryId == nil {
			continue
		}
		if name, ok := nameByID[dto.GetCategoryId()]; ok {
			dto.CategoryName = trans.Ptr(name)
		}
	}
	return nil
}

func (r *NotificationMessageRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().NotificationMessage.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, internalMessageV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

func (r *NotificationMessageRepo) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageResponse, error) {
	if req == nil {
		return nil, internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().NotificationMessage.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), notificationmessage.FieldCreateTime,
		req.GetFieldMask().GetPaths(),
	)
	if err != nil {
		r.log.Errorf("parse list param error [%s]", err.Error())
		return nil, internalMessageV1.ErrorBadRequest("invalid query parameter")
	}

	if querySelectors != nil {
		builder.Modify(querySelectors...)
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list failed: %s", err.Error())
		return nil, internalMessageV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*internalMessageV1.NotificationMessage, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	if err := r.fillCategoryName(ctx, dtos); err != nil {
		return nil, err
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &internalMessageV1.ListNotificationMessageResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

func (r *NotificationMessageRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().NotificationMessage.Query().
		Where(notificationmessage.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, internalMessageV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *NotificationMessageRepo) Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRequest) (*internalMessageV1.NotificationMessage, error) {
	if req == nil {
		return nil, internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().NotificationMessage.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, internalMessageV1.ErrorNotFound("message not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, internalMessageV1.ErrorInternalServerError("query data failed")
	}

	dto := r.mapper.ToDTO(entity)
	if err := r.fillCategoryName(ctx, []*internalMessageV1.NotificationMessage{dto}); err != nil {
		return nil, err
	}

	return dto, nil
}

func (r *NotificationMessageRepo) Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRequest) error {
	if req == nil || req.Data == nil {
		return internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().NotificationMessage.Create().
		SetNillableSubject(req.Data.Subject).
		SetNillableContent(req.Data.Content).
		SetNillableCategoryID(req.Data.CategoryId).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return internalMessageV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *NotificationMessageRepo) Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRequest) error {
	if req == nil || req.Data == nil {
		return internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	// 优先取顶层 id（来自 URL 路径），回退 data.id。
	// 必须在 fieldmaskutil.Filter 之前捕获：Filter 会清空不在 updateMask 中的字段（含 data.id）。
	id := req.GetId()
	if id == 0 {
		id = req.Data.GetId()
	}

	// 如果不存在则创建
	if req.GetAllowMissing() {
		exist, err := r.IsExist(ctx, id)
		if err != nil {
			return err
		}
		if !exist {
			createReq := &internalMessageV1.CreateNotificationMessageRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			return r.Create(ctx, createReq)
		}
	}

	if req.UpdateMask != nil {
		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return internalMessageV1.ErrorBadRequest("invalid field mask")
		}
		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().NotificationMessage.UpdateOneID(id).
		SetNillableSubject(req.Data.Subject).
		SetNillableContent(req.Data.Content).
		SetNillableCategoryID(req.Data.CategoryId).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	if req.Data.UpdateTime == nil {
		builder.SetUpdateTime(time.Now())
	}

	if req.UpdateMask != nil {
		nilPaths := fieldmaskutil.NilValuePaths(req.Data, req.GetUpdateMask().GetPaths())
		nilUpdater := entgoUpdate.BuildSetNullUpdater(nilPaths)
		if nilUpdater != nil {
			builder.Modify(nilUpdater)
		}
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("update one data failed: %s", err.Error())
		return internalMessageV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

func (r *NotificationMessageRepo) Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRequest) error {
	if req == nil {
		return internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().NotificationMessage.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return internalMessageV1.ErrorNotFound("notification message not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return internalMessageV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
