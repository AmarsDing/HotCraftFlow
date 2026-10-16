package data

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/lib/pq"

	"github.com/tx7do/go-utils/copierutil"
	entgo "github.com/tx7do/go-utils/entgo/query"
	entgoUpdate "github.com/tx7do/go-utils/entgo/update"
	"github.com/tx7do/go-utils/fieldmaskutil"
	"github.com/tx7do/go-utils/mapper"
	"github.com/tx7do/go-utils/timeutil"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/dict"
	"framework/app/admin/internal/data/ent/dictcategory"
	"framework/app/admin/internal/data/ent/predicate"

	adminV1 "framework/api/gen/go/admin/v1"
)

// dictRepo 未导出，实现 biz.DictRepo 接口（端口-适配器模式）。
// service/biz 层只依赖 biz.DictRepo interface，不感知此 struct。
type dictRepo struct {
	data *Data
	log  *log.Helper

	mapper *mapper.CopierMapper[adminV1.Dict, ent.Dict]
}

// NewDictRepo 返回 biz.DictRepo 接口，wire 据返回类型自动绑定到 Usecase 入参。
func NewDictRepo(data *Data, logger log.Logger) biz.DictRepo {
	repo := &dictRepo{
		log:    log.NewHelper(log.With(logger, "module", "dict/repo/admin-service")),
		data:   data,
		mapper: mapper.NewCopierMapper[adminV1.Dict, ent.Dict](),
	}

	repo.init()

	return repo
}

func (r *dictRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
}

func (r *dictRepo) toDTO(entity *ent.Dict) *adminV1.Dict {
	dto := r.mapper.ToDTO(entity)
	dto.Category = trans.Ptr(entity.Type)
	dto.CategoryDesc = trans.Ptr(entity.TypeName)
	dto.Key = trans.Ptr(entity.ItemKey)
	dto.Value = trans.Ptr(entity.ItemValue)
	dto.ValueDesc = entity.ValueDesc
	return dto
}

// dictQueryKeyMap DTO 查询键 → ent 字段键（BuildQuerySelector 内部再转 snake_case 列名）。
var dictQueryKeyMap = map[string]string{
	"key":          "itemKey",
	"category":     "type",
	"categoryDesc": "typeName",
	"value":        "itemValue",
	"valueDesc":    "valueDesc",
}

// rewriteDictQuery 把前端按 DTO 字段名（key/category/...）提交的查询 JSON
// 改写为 ent 列名键，保留 `__op` 操作符后缀；无法解析时原样返回。
func rewriteDictQuery(query string) string {
	if query == "" {
		return query
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(query), &m); err != nil {
		return query
	}
	out := make(map[string]interface{}, len(m))
	for k, v := range m {
		base, suffix := k, ""
		if idx := strings.Index(k, "__"); idx >= 0 {
			base, suffix = k[:idx], k[idx:]
		}
		if mapped, ok := dictQueryKeyMap[base]; ok {
			out[mapped+suffix] = v
		} else {
			out[k] = v
		}
	}
	b, err := json.Marshal(out)
	if err != nil {
		return query
	}
	return string(b)
}

func (r *dictRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().Dict.Query()
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

func (r *dictRepo) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListDictResponse, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	// categoryKind（enum/param，分类级元数据）不属于 dict 列，先剥离再按 dict_category 子集过滤，
	// 避免 rewriteDictQuery 把未知键透传给 BuildQuerySelector 生成不存在列的 WHERE
	query, categoryKind := extractCategoryKind(req.GetQuery())

	builder := r.data.db.Client().Dict.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		rewriteDictQuery(query), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), dict.FieldCreateTime,
		req.GetFieldMask().GetPaths(),
	)
	if err != nil {
		r.log.Errorf("parse list param error [%s]", err.Error())
		return nil, adminV1.ErrorBadRequest("invalid query parameter")
	}

	if querySelectors != nil {
		builder.Modify(querySelectors...)
	}

	// 分类用途过滤：type ∈ (SELECT code FROM dict_category WHERE kind=?)，列表与计数同口径
	var kindPred []predicate.Dict
	if categoryKind != "" {
		codes, kerr := r.categoryCodesByKind(ctx, categoryKind)
		if kerr != nil {
			return nil, kerr
		}
		if len(codes) == 0 {
			return &adminV1.ListDictResponse{Total: 0, Items: []*adminV1.Dict{}}, nil
		}
		kindPred = append(kindPred, dict.TypeIn(codes...))
		builder.Where(kindPred...)
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*adminV1.Dict, 0, len(entities))
	for _, entity := range entities {
		dto := r.toDTO(entity)
		dtos = append(dtos, dto)
	}

	var count int
	if len(kindPred) > 0 {
		countBuilder := r.data.db.Client().Dict.Query().Where(kindPred...)
		if len(whereSelectors) != 0 {
			countBuilder.Modify(whereSelectors...)
		}
		count, err = countBuilder.Count(ctx)
	} else {
		count, err = r.Count(ctx, whereSelectors)
	}
	if err != nil {
		return nil, err
	}

	return &adminV1.ListDictResponse{
		Total: uint32(count),
		Items: dtos,
	}, err
}

// extractCategoryKind 从查询 JSON 剥离 categoryKind 键（分类用途过滤，dict 列上不存在）。
// 返回剥离后的查询串与用途值（仅认 enum/param，其余视为未指定）。
// 空查询（含剥离后为空）必须返回 "" 而非 "{}"：BuildFilterSelector("{}") 会生成空谓词，
// 与其他谓词拼出 "WHERE  AND ..." / 单独拼出 "WHERE  ORDER BY ..." 非法 SQL（pq 42601）。
func extractCategoryKind(query string) (string, string) {
	if query == "" {
		return query, ""
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(query), &m); err != nil {
		return query, ""
	}
	v, ok := m["categoryKind"]
	if ok {
		delete(m, "categoryKind")
	}
	kind, _ := v.(string)
	if kind != "enum" && kind != "param" {
		kind = ""
	}
	if len(m) == 0 {
		return "", kind
	}
	if !ok {
		return query, ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return query, ""
	}
	return string(b), kind
}

// categoryCodesByKind 查 dict_category 指定用途的分类编码集合（小表全量，免方言子查询耦合）。
func (r *dictRepo) categoryCodesByKind(ctx context.Context, kind string) ([]string, error) {
	entities, err := r.data.db.Client().DictCategory.Query().
		Where(dictcategory.KindEQ(kind)).
		Select(dictcategory.FieldCode).
		All(ctx)
	if err != nil {
		r.log.Errorf("query dict category codes by kind failed: %s", err.Error())
		return nil, adminV1.ErrorInternalServerError("query dict categories failed")
	}
	codes := make([]string, 0, len(entities))
	for _, entity := range entities {
		codes = append(codes, entity.Code)
	}
	return codes, nil
}

func (r *dictRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().Dict.Query().
		Where(dict.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, adminV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

func (r *dictRepo) Get(ctx context.Context, req *adminV1.GetDictRequest) (*adminV1.Dict, error) {
	if req == nil {
		return nil, adminV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().Dict.Get(ctx, req.GetId())
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, adminV1.ErrorNotFound("dict not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, adminV1.ErrorInternalServerError("query data failed")
	}

	return r.toDTO(entity), nil
}

// pgStringDataRightTruncationCode 为 PostgreSQL 字符串超长错误码（value too long for type character varying(n)）。
// 长度上限与 ent schema 对齐（key/category 64、categoryDesc 128、value 2048），超限翻译为可读 400。
const pgStringDataRightTruncationCode = "22001"

// isPgValueTooLong 判断错误是否为 PostgreSQL 字符串长度超限。
// 与 isPgUniqueViolation 同模式：兼容 pq 原生错误与 ent 包装。
func isPgValueTooLong(err error) bool {
	if err == nil {
		return false
	}
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return string(pqErr.Code) == pgStringDataRightTruncationCode
	}
	return false
}

func (r *dictRepo) Create(ctx context.Context, req *adminV1.CreateDictRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	// (type, item_key) 唯一预检：冲突返回 409 可读文案，避免存储层 500 泛化报错。
	dup, err := r.data.db.Client().Dict.Query().
		Where(dict.TypeEQ(req.Data.GetCategory()), dict.ItemKeyEQ(req.Data.GetKey())).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("check dict key duplicated failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("check dict key failed")
	}
	if dup {
		return adminV1.ErrorConflict("dict key %q already exists in category %q", req.Data.GetKey(), req.Data.GetCategory())
	}

	builder := r.data.db.Client().Dict.Create().
		SetType(req.Data.GetCategory()).
		SetTypeName(req.Data.GetCategoryDesc()).
		SetItemKey(req.Data.GetKey()).
		SetItemValue(req.Data.GetValue()).
		SetNillableValueDesc(req.Data.ValueDesc).
		SetNillableValueDataType(req.Data.ValueDataType).
		SetNillableSortID(req.Data.SortId).
		SetNillableStatus((*dict.Status)(req.Data.Status)).
		SetNillableRemark(req.Data.Remark).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if err := builder.Exec(ctx); err != nil {
		// 唯一约束兜底（并发竞态/预检漏网）与字段长度翻译：避免泛化 500。
		if isPgUniqueViolation(err) {
			return adminV1.ErrorConflict("dict key %q already exists in category %q", req.Data.GetKey(), req.Data.GetCategory())
		}
		if isPgValueTooLong(err) {
			return adminV1.ErrorBadRequest("dict field value too long (key/category <= 64, categoryDesc <= 128, value <= 2048)")
		}
		r.log.Errorf("insert one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("insert data failed")
	}

	return nil
}

func (r *dictRepo) Update(ctx context.Context, req *adminV1.UpdateDictRequest) error {
	if req == nil || req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
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
			createReq := &adminV1.CreateDictRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			return r.Create(ctx, createReq)
		}
	}

	if req.UpdateMask != nil {
		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return adminV1.ErrorBadRequest("invalid field mask")
		}
		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().Dict.
		UpdateOneID(id).
		SetNillableValueDesc(req.Data.ValueDesc).
		SetNillableValueDataType(req.Data.ValueDataType).
		SetNillableSortID(req.Data.SortId).
		SetNillableStatus((*dict.Status)(req.Data.Status)).
		SetNillableRemark(req.Data.Remark).
		SetNillableUpdateBy(req.Data.UpdateBy).
		SetNillableUpdateTime(timeutil.TimestamppbToTime(req.Data.UpdateTime))

	// 必填列按"字段是否被提交"守卫写入：部分更新（如仅切换 status）时
	// fieldmaskutil.Filter 已把未触碰字段置 nil，直接 Get 会得到空串并把列擦空。
	if req.Data.Category != nil {
		builder.SetType(req.Data.GetCategory())
	}
	if req.Data.CategoryDesc != nil {
		builder.SetTypeName(req.Data.GetCategoryDesc())
	}
	if req.Data.Key != nil {
		builder.SetItemKey(req.Data.GetKey())
	}
	if req.Data.Value != nil {
		builder.SetItemValue(req.Data.GetValue())
	}

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
		// 改 key/category 撞唯一键或字段超长时的可读翻译（同 Create 链路）。
		if isPgUniqueViolation(err) {
			return adminV1.ErrorConflict("dict key %q already exists in category %q", req.Data.GetKey(), req.Data.GetCategory())
		}
		if isPgValueTooLong(err) {
			return adminV1.ErrorBadRequest("dict field value too long (key/category <= 64, categoryDesc <= 128, value <= 2048)")
		}
		r.log.Errorf("update one data failed: %s", err.Error())
		return adminV1.ErrorInternalServerError("update data failed")
	}

	return nil
}

func (r *dictRepo) Delete(ctx context.Context, req *adminV1.DeleteDictRequest) error {
	if req == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}

	if err := r.data.db.Client().Dict.DeleteOneID(req.GetId()).Exec(ctx); err != nil {
		if ent.IsNotFound(err) {
			return adminV1.ErrorNotFound("dict not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return adminV1.ErrorInternalServerError("delete failed")
	}

	return nil
}
