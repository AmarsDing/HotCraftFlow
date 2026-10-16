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

	"google.golang.org/protobuf/types/known/timestamppb"

	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/notificationmessage"
	"framework/app/admin/internal/data/ent/notificationmessagerecipient"
	"framework/app/admin/internal/data/ent/predicate"

	internalMessageV1 "framework/api/gen/go/internal_message/v1"
)

type NotificationMessageRecipientRepo struct {
	data *Data
	log  *log.Helper

	mapper          *mapper.CopierMapper[internalMessageV1.NotificationMessageRecipient, ent.NotificationMessageRecipient]
	statusConverter *mapper.EnumTypeConverter[internalMessageV1.MessageStatus, notificationmessagerecipient.Status]
}

func NewNotificationMessageRecipientRepo(data *Data, logger log.Logger) *NotificationMessageRecipientRepo {
	repo := &NotificationMessageRecipientRepo{
		log:             log.NewHelper(log.With(logger, "module", "notification-message-recipient/repo/admin-service")),
		data:            data,
		mapper:          mapper.NewCopierMapper[internalMessageV1.NotificationMessageRecipient, ent.NotificationMessageRecipient](),
		statusConverter: mapper.NewEnumTypeConverter[internalMessageV1.MessageStatus, notificationmessagerecipient.Status](internalMessageV1.MessageStatus_name, internalMessageV1.MessageStatus_value),
	}

	repo.init()

	return repo
}

func (r *NotificationMessageRecipientRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
}

func (r *NotificationMessageRecipientRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().NotificationMessageRecipient.Query()
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

func (r *NotificationMessageRecipientRepo) List(ctx context.Context, req *pagination.PagingRequest) (*internalMessageV1.ListNotificationMessageRecipientResponse, error) {
	if req == nil {
		return nil, internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().NotificationMessageRecipient.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), notificationmessagerecipient.FieldCreateTime,
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

	dtos := make([]*internalMessageV1.NotificationMessageRecipient, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		dtos = append(dtos, dto)
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &internalMessageV1.ListNotificationMessageRecipientResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

func (r *NotificationMessageRecipientRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().NotificationMessageRecipient.Query().
		Where(notificationmessagerecipient.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, internalMessageV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *NotificationMessageRecipientRepo) Get(ctx context.Context, req *internalMessageV1.GetNotificationMessageRecipientRequest) (*internalMessageV1.NotificationMessageRecipient, error) {
	if req == nil {
		return nil, internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().NotificationMessageRecipient.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, internalMessageV1.ErrorNotFound("message not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, internalMessageV1.ErrorInternalServerError("query data failed")
	}

	return r.mapper.ToDTO(entity), nil
}

func (r *NotificationMessageRecipientRepo) Create(ctx context.Context, req *internalMessageV1.CreateNotificationMessageRecipientRequest) error {
	if req == nil || req.Data == nil {
		return internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().NotificationMessageRecipient.Create().
		SetNillableMessageID(req.Data.MessageId).
		SetNillableRecipientID(req.Data.RecipientId).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return internalMessageV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *NotificationMessageRecipientRepo) Update(ctx context.Context, req *internalMessageV1.UpdateNotificationMessageRecipientRequest) error {
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
			createReq := &internalMessageV1.CreateNotificationMessageRecipientRequest{Data: req.Data}
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

	builder := r.data.db.Client().NotificationMessageRecipient.UpdateOneID(id).
		SetNillableMessageID(req.Data.MessageId).
		SetNillableRecipientID(req.Data.RecipientId).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
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

func (r *NotificationMessageRecipientRepo) Delete(ctx context.Context, req *internalMessageV1.DeleteNotificationMessageRecipientRequest) error {
	if req == nil {
		return internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().NotificationMessageRecipient.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return internalMessageV1.ErrorNotFound("notification message recipient not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return internalMessageV1.ErrorInternalServerError("delete failed")
	}

	return nil
}

// ListMy 查询当前登录用户的通知收件箱：收件记录联查消息主题/内容，按接收时间倒序分页。
// unread_count 与分页/筛选无关，始终返回该用户未读总数。
func (r *NotificationMessageRecipientRepo) ListMy(ctx context.Context, userID uint32, req *internalMessageV1.ListMyNotificationRequest) (*internalMessageV1.ListMyNotificationResponse, error) {
	if req == nil {
		return nil, internalMessageV1.ErrorBadRequest("invalid parameter")
	}

	baseConds := []predicate.NotificationMessageRecipient{
		notificationmessagerecipient.RecipientIDEQ(userID),
		notificationmessagerecipient.DeleteTimeIsNil(),
	}

	// 未读总数（与分页无关）
	unread, err := r.data.db.Client().NotificationMessageRecipient.Query().
		Where(append(baseConds, notificationmessagerecipient.StatusEQ(notificationmessagerecipient.StatusReceived))...).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count my unread failed: %s", err.Error())
		return nil, internalMessageV1.ErrorInternalServerError("query unread count failed")
	}

	conds := baseConds
	if req.GetOnlyUnread() {
		conds = append(conds, notificationmessagerecipient.StatusEQ(notificationmessagerecipient.StatusReceived))
	}

	page := req.GetPage()
	if page == 0 {
		page = 1
	}
	pageSize := req.GetPageSize()
	if pageSize == 0 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	query := r.data.db.Client().NotificationMessageRecipient.Query().
		Where(conds...).
		Order(ent.Desc(notificationmessagerecipient.FieldCreateTime)).
		Offset(int((page - 1) * pageSize)).
		Limit(int(pageSize))

	entities, err := query.All(ctx)
	if err != nil {
		r.log.Errorf("query my notifications failed: %s", err.Error())
		return nil, internalMessageV1.ErrorInternalServerError("query my notifications failed")
	}

	total, err := r.data.db.Client().NotificationMessageRecipient.Query().Where(conds...).Count(ctx)
	if err != nil {
		r.log.Errorf("count my notifications failed: %s", err.Error())
		return nil, internalMessageV1.ErrorInternalServerError("query count failed")
	}

	// 批量联查消息主题/内容
	messageIDs := make([]uint32, 0, len(entities))
	for _, e := range entities {
		if e.MessageID != nil {
			messageIDs = append(messageIDs, *e.MessageID)
		}
	}
	msgMap := make(map[uint32]*ent.NotificationMessage, len(messageIDs))
	if len(messageIDs) > 0 {
		msgs, merr := r.data.db.Client().NotificationMessage.Query().
			Where(notificationmessage.IDIn(messageIDs...)).
			All(ctx)
		if merr != nil {
			r.log.Errorf("query messages for my notifications failed: %s", merr.Error())
			return nil, internalMessageV1.ErrorInternalServerError("query messages failed")
		}
		for _, m := range msgs {
			msgMap[m.ID] = m
		}
	}

	items := make([]*internalMessageV1.MyNotification, 0, len(entities))
	for _, e := range entities {
		item := &internalMessageV1.MyNotification{
			RecipientId: trans.Ptr(e.ID),
			MessageId:   e.MessageID,
			Status:      r.statusConverter.ToDTO(e.Status),
		}
		if e.CreateTime != nil && !e.CreateTime.IsZero() {
			item.CreateTime = timestamppb.New(*e.CreateTime)
		}
		if e.MessageID != nil {
			if m, ok := msgMap[*e.MessageID]; ok {
				item.Subject = m.Subject
				item.Content = m.Content
			}
		}
		items = append(items, item)
	}

	return &internalMessageV1.ListMyNotificationResponse{
		Items:       items,
		Total:       uint32(total),
		UnreadCount: uint32(unread),
	}, nil
}

// CountMyUnread 查询当前登录用户未读通知数。
func (r *NotificationMessageRecipientRepo) CountMyUnread(ctx context.Context, userID uint32) (uint32, error) {
	count, err := r.data.db.Client().NotificationMessageRecipient.Query().
		Where(
			notificationmessagerecipient.RecipientIDEQ(userID),
			notificationmessagerecipient.DeleteTimeIsNil(),
			notificationmessagerecipient.StatusEQ(notificationmessagerecipient.StatusReceived),
		).
		Count(ctx)
	if err != nil {
		r.log.Errorf("count my unread failed: %s", err.Error())
		return 0, internalMessageV1.ErrorInternalServerError("query unread count failed")
	}
	return uint32(count), nil
}

// MarkMyRead 标记当前登录用户通知已读；recipientIDs 为空表示全部已读。
// 仅允许操作本人收件记录（recipient_id = userID 强制过滤）。
func (r *NotificationMessageRecipientRepo) MarkMyRead(ctx context.Context, userID uint32, recipientIDs []uint32) error {
	conds := []predicate.NotificationMessageRecipient{
		notificationmessagerecipient.RecipientIDEQ(userID),
		notificationmessagerecipient.DeleteTimeIsNil(),
		notificationmessagerecipient.StatusEQ(notificationmessagerecipient.StatusReceived),
	}
	if len(recipientIDs) > 0 {
		conds = append(conds, notificationmessagerecipient.IDIn(recipientIDs...))
	}

	if _, err := r.data.db.Client().NotificationMessageRecipient.Update().
		Where(conds...).
		SetStatus(notificationmessagerecipient.StatusRead).
		SetUpdateTime(time.Now()).
		Save(ctx); err != nil {
		r.log.Errorf("mark my notifications read failed: %s", err.Error())
		return internalMessageV1.ErrorInternalServerError("mark read failed")
	}
	return nil
}
