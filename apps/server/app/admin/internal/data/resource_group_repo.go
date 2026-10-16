package data

import (
	"context"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"

	"github.com/tx7do/go-utils/copierutil"
	entgo "github.com/tx7do/go-utils/entgo/query"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/resourcegroup"
)

// ResourceGroupRepo 资源组仓储实现（只读：列表/详情，软删记录不返回）。
type ResourceGroupRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[adminV1.ResourceGroup, ent.ResourceGroup]
}

func NewResourceGroupRepo(data *Data, logger log.Logger) biz.ResourceGroupRepo {
	repo := &ResourceGroupRepo{
		log:    log.NewHelper(log.With(logger, "module", "resource-group/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[adminV1.ResourceGroup, ent.ResourceGroup](),
	}
	repo.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	repo.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	return repo
}

func (r *ResourceGroupRepo) entityToDTO(entity *ent.ResourceGroup) *adminV1.ResourceGroup {
	if entity == nil {
		return nil
	}
	dto := r.mapper.ToDTO(entity)
	// ent Status 为字符串枚举 *resourcegroup.Status，与 DTO *string 命名类型不一致，mapper 跳过，手动转换
	if entity.Status != nil {
		dto.Status = trans.Ptr(entity.Status.String())
	}
	return dto
}

func (r *ResourceGroupRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().ResourceGroup.Query().
		Where(resourcegroup.DeleteTimeIsNil())
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}
	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, adminV1.ErrorInternalServerError("query count failed")
	}
	return count, nil
}

func (r *ResourceGroupRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListResourceGroupResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().ResourceGroup.Query().
		Where(resourcegroup.DeleteTimeIsNil())

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), resourcegroup.FieldCreateTime,
		req.GetFieldMask().GetPaths(),
	)
	if err != nil {
		r.log.Errorf("parse list param error [%s]", err.Error())
		return nil, adminV1.ErrorBadRequest("invalid query parameter")
	}

	if querySelectors != nil {
		builder.Modify(querySelectors...)
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*adminV1.ResourceGroup, 0, len(entities))
	for _, entity := range entities {
		dtos = append(dtos, r.entityToDTO(entity))
	}

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &adminV1.ListResourceGroupResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

func (r *ResourceGroupRepo) Get(ctx context.Context, id uint32) (*adminV1.ResourceGroup, error) {
	if id == 0 {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().ResourceGroup.Query().
		Where(resourcegroup.IDEQ(id), resourcegroup.DeleteTimeIsNil()).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("resource group not found")
		}
		r.log.Errorf("query resource group failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.entityToDTO(entity), nil
}
