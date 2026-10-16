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
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/app/admin/internal/data/ent/role"
	_ "framework/app/admin/internal/data/ent/runtime"
	"framework/app/admin/internal/data/ent/user"
	"framework/app/admin/internal/data/ent/usercredential"
	"framework/app/admin/internal/data/ent/userpost"
	"framework/app/admin/internal/data/ent/userrole"
	"framework/app/admin/internal/data/ent/usertoken"
	"framework/pkg/crypto"

	userV1 "framework/api/gen/go/user/v1"
)

// ErrOperationPasswordNotSet 用户未设置独立操作密码（§2.6.6）。
// 调用方应据此返回 422 OPERATION_PASSWORD_NOT_SET，引导用户在个人中心设置。
// 哨兵本体在 biz 层（service 经 biz.UserUsecase 校验时以 errors.Is 判定），此处别名保证全链路同一实例。
var ErrOperationPasswordNotSet = biz.ErrOperationPasswordNotSet

type UserRepo struct {
	data *Data
	log  *log.Helper

	mapper             *mapper.CopierMapper[userV1.User, ent.User]
	statusConverter    *mapper.EnumTypeConverter[userV1.UserStatus, user.Status]
	genderConverter    *mapper.EnumTypeConverter[userV1.UserGender, user.Gender]
	authorityConverter *mapper.EnumTypeConverter[userV1.UserAuthority, user.Authority]
}

func NewUserRepo(logger log.Logger, data *Data) *UserRepo {
	repo := &UserRepo{
		log:                log.NewHelper(log.With(logger, "module", "user/repo/admin-service")),
		data:               data,
		mapper:             mapper.NewCopierMapper[userV1.User, ent.User](),
		statusConverter:    mapper.NewEnumTypeConverter[userV1.UserStatus, user.Status](userV1.UserStatus_name, userV1.UserStatus_value),
		genderConverter:    mapper.NewEnumTypeConverter[userV1.UserGender, user.Gender](userV1.UserGender_name, userV1.UserGender_value),
		authorityConverter: mapper.NewEnumTypeConverter[userV1.UserAuthority, user.Authority](userV1.UserAuthority_name, userV1.UserAuthority_value),
	}

	repo.init()

	return repo
}

func (r *UserRepo) init() {
	r.mapper.AppendConverters(copierutil.NewTimeStringConverterPair())
	r.mapper.AppendConverters(copierutil.NewTimeTimestamppbConverterPair())
	r.mapper.AppendConverters(r.statusConverter.NewConverterPair())
	r.mapper.AppendConverters(r.genderConverter.NewConverterPair())
	r.mapper.AppendConverters(r.authorityConverter.NewConverterPair())
}

func (r *UserRepo) Count(ctx context.Context, whereCond []func(s *sql.Selector)) (int, error) {
	builder := r.data.db.Client().User.Query()
	if len(whereCond) != 0 {
		builder.Modify(whereCond...)
	}

	count, err := builder.Count(ctx)
	if err != nil {
		r.log.Errorf("query count failed: %s", err.Error())
		return 0, userV1.ErrorInternalServerError("query count failed")
	}

	return count, nil
}

// userVirtualMaskFields 是 ap_org_user 物理列之外的 DTO 虚拟字段（由关联表回填）。
// go-utils BuildQuerySelector 会把 fieldMask 路径 snake_case 后直接作为 SELECT 列，
// 虚拟字段参与裁剪会生成不存在列导致 SQL 报错（500），须在传入前剥离。
var userVirtualMaskFields = map[string]struct{}{
	"postIds":  {},
	"post_ids": {},
	"roles":    {},
}

// userMaskPathAliases：DTO 字段名 → 物理列名。
// 例如前端 fieldMask=orgId 会被转成 org_id，但表列实际是 dept_id（fillOrgID 从 DeptID 回填 OrgId）。
var userMaskPathAliases = map[string]string{
	"orgId":  "dept_id",
	"org_id": "dept_id",
}

func filterUserVirtualMaskPaths(paths []string) []string {
	if len(paths) == 0 {
		return paths
	}
	out := make([]string, 0, len(paths))
	seen := make(map[string]struct{}, len(paths))
	for _, p := range paths {
		if _, virtual := userVirtualMaskFields[p]; virtual {
			continue
		}
		if alias, ok := userMaskPathAliases[p]; ok {
			p = alias
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out
}

func (r *UserRepo) List(ctx context.Context, req *pagination.PagingRequest) (*userV1.ListUserResponse, error) {
	if req == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().User.Query()

	err, whereSelectors, querySelectors := entgo.BuildQuerySelector(
		req.GetQuery(), req.GetOrQuery(),
		req.GetPage(), req.GetPageSize(), req.GetNoPaging(),
		req.GetOrderBy(), user.FieldCreateTime,
		filterUserVirtualMaskPaths(req.GetFieldMask().GetPaths()),
	)
	if err != nil {
		r.log.Errorf("parse list param error [%s]", err.Error())
		return nil, userV1.ErrorBadRequest("invalid query parameter")
	}

	if querySelectors != nil {
		builder.Modify(querySelectors...)
	}

	entities, err := builder.All(ctx)
	if err != nil {
		r.log.Errorf("query list failed: %s", err.Error())
		return nil, userV1.ErrorInternalServerError("query list failed")
	}

	dtos := make([]*userV1.User, 0, len(entities))
	for _, entity := range entities {
		dto := r.mapper.ToDTO(entity)
		fillOrgID(entity, dto)
		fillPositionID(entity, dto)
		fillSecurityPolicyFields(entity, dto)
		dtos = append(dtos, dto)
	}
	// 批量填充角色，避免循环内 N+1 查询
	r.fillUserRolesBatch(ctx, entities, dtos)
	// 批量填充岗位（ap_org_user_post → airport_posts.id 列表）
	r.fillUserPostsBatch(ctx, entities, dtos)

	count, err := r.Count(ctx, whereSelectors)
	if err != nil {
		return nil, err
	}

	return &userV1.ListUserResponse{
		Total: uint32(count),
		Items: dtos,
	}, nil
}

func (r *UserRepo) IsExist(ctx context.Context, id uint32) (bool, error) {
	exist, err := r.data.db.Client().User.Query().
		Where(user.IDEQ(id)).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return false, userV1.ErrorInternalServerError("query exist failed")
	}
	return exist, nil
}

// fillUserRoles 查询 ap_org_user_role + ap_org_role 表填充 user.Roles（角色 code 列表）。
func (r *UserRepo) fillUserRoles(ctx context.Context, userID uint32, dto *userV1.User) {
	if userID == 0 || dto == nil {
		return
	}
	// 1. 查 user_role 关联表获取 role IDs
	urEntities, err := r.data.db.Client().UserRole.Query().
		Where(userrole.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query user roles failed [%d]: %s", userID, err.Error())
		return
	}
	if len(urEntities) == 0 {
		return
	}
	roleIDs := make([]uint32, 0, len(urEntities))
	for _, e := range urEntities {
		roleIDs = append(roleIDs, e.RoleID)
	}
	// 2. 查 role 表获取 code
	roleEntities, err := r.data.db.Client().Role.Query().
		Where(role.IDIn(roleIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query role codes failed [%v]: %s", roleIDs, err.Error())
		return
	}
	codes := make([]string, 0, len(roleEntities))
	for _, re := range roleEntities {
		if re.Code != nil {
			codes = append(codes, *re.Code)
		}
	}
	dto.Roles = codes
}

// fillUserRolesBatch 批量查询所有用户的角色 code，回填到对应 dto。
// 仅 3 次 SQL（user_role 1 次 + role 1 次 + 主查询 1 次），避免循环内 N+1。
func (r *UserRepo) fillUserRolesBatch(ctx context.Context, entities []*ent.User, dtos []*userV1.User) {
	if len(entities) == 0 || len(entities) != len(dtos) {
		return
	}
	userIDs := make([]uint32, 0, len(entities))
	for _, e := range entities {
		userIDs = append(userIDs, e.ID)
	}
	// 1. 一次查所有 user_role 关联
	urEntities, err := r.data.db.Client().UserRole.Query().
		Where(userrole.UserIDIn(userIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("batch query user roles failed: %s", err.Error())
		return
	}
	if len(urEntities) == 0 {
		return
	}
	// 2. 收集去重 roleID，一次查 role 取 code
	roleIDSet := make(map[uint32]struct{})
	userToRoleIDs := make(map[uint32][]uint32)
	for _, e := range urEntities {
		userToRoleIDs[e.UserID] = append(userToRoleIDs[e.UserID], e.RoleID)
		roleIDSet[e.RoleID] = struct{}{}
	}
	roleIDs := make([]uint32, 0, len(roleIDSet))
	for id := range roleIDSet {
		roleIDs = append(roleIDs, id)
	}
	roleEntities, err := r.data.db.Client().Role.Query().
		Where(role.IDIn(roleIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("batch query roles failed: %s", err.Error())
		return
	}
	roleCodeByID := make(map[uint32]string, len(roleEntities))
	for _, re := range roleEntities {
		if re.Code != nil {
			roleCodeByID[re.ID] = *re.Code
		}
	}
	// 3. 按 userID 回填到 dto
	dtoByUserID := make(map[uint32]*userV1.User, len(dtos))
	for i, e := range entities {
		dtoByUserID[e.ID] = dtos[i]
	}
	for uid, rids := range userToRoleIDs {
		dto, ok := dtoByUserID[uid]
		if !ok || dto == nil {
			continue
		}
		codes := make([]string, 0, len(rids))
		for _, rid := range rids {
			if code, ok := roleCodeByID[rid]; ok {
				codes = append(codes, code)
			}
		}
		dto.Roles = codes
	}
}

func (r *UserRepo) Get(ctx context.Context, userId uint32) (*userV1.User, error) {
	if userId == 0 {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().User.Get(ctx, userId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorUserNotFound("user not found")
		}

		r.log.Errorf("query one data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	dto := r.mapper.ToDTO(entity)
	fillOrgID(entity, dto)
	fillPositionID(entity, dto)
	fillSecurityPolicyFields(entity, dto)
	r.fillUserRoles(ctx, userId, dto)
	r.fillUserPosts(ctx, userId, dto)
	return dto, nil
}

// fillOrgID 回填部门 ID：ent 字段 DeptID 与 DTO 字段 OrgId 名称不一致，copier 按名映射会漏掉。
func fillOrgID(entity *ent.User, dto *userV1.User) {
	if entity != nil && entity.DeptID != nil && dto != nil {
		dto.OrgId = entity.DeptID
	}
}

// fillPositionID 回填职位 ID：ent 字段 PositionID 与 DTO 字段 PositionId 仅大小写差异，copier 按名映射不保证命中，显式回填（G8 接线）。
func fillPositionID(entity *ent.User, dto *userV1.User) {
	if entity != nil && entity.PositionID != nil && dto != nil {
		dto.PositionId = entity.PositionID
	}
}

// fillSecurityPolicyFields 回填安全策略字段（B7 proto 契约补齐）：
// max_concurrent/mfa_enabled 已入 proto DTO，但 copier 不做 int→*uint32 / bool→*bool 的类型转换，需显式填充。
// mfa_secret 属敏感材料，永不下发 DTO（启用/确认/禁用流程走专用接口）。
func fillSecurityPolicyFields(entity *ent.User, dto *userV1.User) {
	if entity == nil || dto == nil {
		return
	}
	maxConcurrent := uint32(max(entity.MaxConcurrent, 0)) //nolint:gosec // 负值防御性钳 0
	dto.MaxConcurrent = &maxConcurrent
	dto.MfaEnabled = &entity.MfaEnabled
}

// fillUserPosts 查询 ap_org_user_post 表填充 user.PostIds（岗位 ID 列表，关联 airport_posts.id）。
func (r *UserRepo) fillUserPosts(ctx context.Context, userID uint32, dto *userV1.User) {
	if userID == 0 || dto == nil {
		return
	}
	upEntities, err := r.data.db.Client().UserPost.Query().
		Where(userpost.UserIDEQ(userID)).
		All(ctx)
	if err != nil {
		r.log.Errorf("query user posts failed [%d]: %s", userID, err.Error())
		return
	}
	if len(upEntities) == 0 {
		return
	}
	postIDs := make([]uint32, 0, len(upEntities))
	for _, e := range upEntities {
		postIDs = append(postIDs, e.PostID)
	}
	dto.PostIds = postIDs
}

// fillUserPostsBatch 批量查询所有用户的岗位 ID，回填到对应 dto（避免循环内 N+1）。
func (r *UserRepo) fillUserPostsBatch(ctx context.Context, entities []*ent.User, dtos []*userV1.User) {
	if len(entities) == 0 || len(entities) != len(dtos) {
		return
	}
	userIDs := make([]uint32, 0, len(entities))
	for _, e := range entities {
		userIDs = append(userIDs, e.ID)
	}
	upEntities, err := r.data.db.Client().UserPost.Query().
		Where(userpost.UserIDIn(userIDs...)).
		All(ctx)
	if err != nil {
		r.log.Errorf("batch query user posts failed: %s", err.Error())
		return
	}
	userToPostIDs := make(map[uint32][]uint32)
	for _, e := range upEntities {
		userToPostIDs[e.UserID] = append(userToPostIDs[e.UserID], e.PostID)
	}
	dtoByUserID := make(map[uint32]*userV1.User, len(dtos))
	for i, e := range entities {
		dtoByUserID[e.ID] = dtos[i]
	}
	for uid, pids := range userToPostIDs {
		dto, ok := dtoByUserID[uid]
		if !ok || dto == nil {
			continue
		}
		dto.PostIds = pids
	}
}

// syncUserPosts 全量重建用户岗位关联（先删后建，空列表等价清空）。
func (r *UserRepo) syncUserPosts(ctx context.Context, userID uint32, postIDs []uint32) error {
	if _, err := r.data.db.Client().UserPost.Delete().
		Where(userpost.UserIDEQ(userID)).
		Exec(ctx); err != nil {
		r.log.Errorf("delete user posts failed [%d]: %s", userID, err.Error())
		return userV1.ErrorInternalServerError("update user posts failed")
	}
	if len(postIDs) == 0 {
		return nil
	}
	builders := make([]*ent.UserPostCreate, 0, len(postIDs))
	for _, pid := range postIDs {
		if pid == 0 {
			continue
		}
		builders = append(builders, r.data.db.Client().UserPost.Create().
			SetUserID(userID).
			SetPostID(pid).
			SetCreateTime(time.Now()))
	}
	if len(builders) == 0 {
		return nil
	}
	if err := r.data.db.Client().UserPost.CreateBulk(builders...).Exec(ctx); err != nil {
		r.log.Errorf("bulk create user posts failed [%d]: %s", userID, err.Error())
		return userV1.ErrorInternalServerError("update user posts failed")
	}
	return nil
}

// ListDeptIDs 批量查询用户部门（id → dept_id）；未分配部门（NULL）的用户不出现在返回 map 中。
func (r *UserRepo) ListDeptIDs(ctx context.Context, ids []uint32) (map[uint32]uint32, error) {
	result := make(map[uint32]uint32, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	entities, err := r.data.db.Client().User.Query().
		Where(user.IDIn(ids...), user.DeptIDNotNil()).
		Select(user.FieldID, user.FieldDeptID).
		All(ctx)
	if err != nil {
		r.log.Errorf("query user dept ids failed: %s", err.Error())
		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	for _, entity := range entities {
		if entity.DeptID != nil {
			result[entity.ID] = *entity.DeptID
		}
	}
	return result, nil
}

func (r *UserRepo) Create(ctx context.Context, req *userV1.CreateUserRequest) (*userV1.User, error) {
	if req == nil || req.Data == nil {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	builder := r.data.db.Client().User.Create().
		SetNillableUsername(req.Data.Username).
		SetNillableNickname(req.Data.Nickname).
		SetNillableRealname(req.Data.Realname).
		SetNillableAvatar(req.Data.Avatar).
		SetNillableEmail(req.Data.Email).
		SetNillableMobile(req.Data.Mobile).
		SetNillableTelephone(req.Data.Telephone).
		SetNillableRegion(req.Data.Region).
		SetNillableAddress(req.Data.Address).
		SetNillableDescription(req.Data.Description).
		SetNillableRemark(req.Data.Remark).
		SetNillableLastLoginTime(timeutil.TimestamppbToTime(req.Data.LastLoginTime)).
		SetNillableLastLoginIP(req.Data.LastLoginIp).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableGender(r.genderConverter.ToEntity(req.Data.Gender)).
		SetNillableAuthority(r.authorityConverter.ToEntity(req.Data.Authority)).
		SetNillableDeptID(req.Data.OrgId).
		SetNillablePositionID(req.Data.PositionId).
		SetNillableWorkID(req.Data.WorkId).
		SetNillableTenantID(req.Data.TenantId).
		SetNillableCreateBy(req.Data.CreateBy).
		SetNillableCreateTime(timeutil.TimestamppbToTime(req.Data.CreateTime))

	if req.Data.CreateTime == nil {
		builder.SetCreateTime(time.Now())
	}

	if req.Data.Id != nil {
		builder.SetID(req.Data.GetId())
	}

	if ret, err := builder.Save(ctx); err != nil {
		r.log.Errorf("insert one data failed: %s", err.Error())
		return nil, userV1.ErrorInternalServerError("insert data failed")
	} else {
		// 岗位关联：仅在显式传入（非 nil）时同步，空数组表示清空
		if req.Data.PostIds != nil {
			if err := r.syncUserPosts(ctx, ret.ID, req.Data.PostIds); err != nil {
				return nil, err
			}
		}
		dto := r.mapper.ToDTO(ret)
		fillOrgID(ret, dto)
		fillSecurityPolicyFields(ret, dto)
		return dto, nil
	}
}

func (r *UserRepo) Update(ctx context.Context, req *userV1.UpdateUserRequest) error {
	if req == nil || req.Data == nil {
		return userV1.ErrorBadRequest("invalid parameter")
	}

	// 优先取顶层 id（来自 URL 路径），回退 data.id（UserProfile 端点在 service 中赋值）。
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
			createReq := &userV1.CreateUserRequest{Data: req.Data}
			createReq.Data.CreateBy = createReq.Data.UpdateBy
			createReq.Data.UpdateBy = nil
			_, err = r.Create(ctx, createReq)
			return err
		}
	}

	if req.UpdateMask != nil {
		for i := 0; i < len(req.UpdateMask.GetPaths()); i++ {
			if req.UpdateMask.Paths[i] == "password" {
				req.UpdateMask.Paths = append(req.UpdateMask.Paths[:i], req.UpdateMask.Paths[i+1:]...)
				i = i - 1
				continue
			}
		}

		req.UpdateMask.Normalize()
		if !req.UpdateMask.IsValid(req.Data) {
			r.log.Errorf("invalid field mask [%v]", req.UpdateMask)
			return userV1.ErrorBadRequest("invalid field mask")
		}

		fieldmaskutil.Filter(req.GetData(), req.UpdateMask.GetPaths())
	}

	builder := r.data.db.Client().User.
		UpdateOneID(id).
		SetNillableNickname(req.Data.Nickname).
		SetNillableRealname(req.Data.Realname).
		SetNillableAvatar(req.Data.Avatar).
		SetNillableEmail(req.Data.Email).
		SetNillableMobile(req.Data.Mobile).
		SetNillableTelephone(req.Data.Telephone).
		SetNillableRegion(req.Data.Region).
		SetNillableAddress(req.Data.Address).
		SetNillableDescription(req.Data.Description).
		SetNillableRemark(req.Data.Remark).
		SetNillableLastLoginTime(timeutil.TimestamppbToTime(req.Data.LastLoginTime)).
		SetNillableLastLoginIP(req.Data.LastLoginIp).
		SetNillableStatus(r.statusConverter.ToEntity(req.Data.Status)).
		SetNillableGender(r.genderConverter.ToEntity(req.Data.Gender)).
		SetNillableAuthority(r.authorityConverter.ToEntity(req.Data.Authority)).
		SetNillableDeptID(req.Data.OrgId).
		SetNillablePositionID(req.Data.PositionId).
		SetNillableWorkID(req.Data.WorkId).
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
		return userV1.ErrorInternalServerError("update data failed")
	}

	// 岗位关联：仅在显式传入（非 nil）时同步，空数组表示清空。
	// 带 UpdateMask 的请求经 fieldmaskutil.Filter 后未授权字段已被清零（PostIds=nil），此处天然跳过。
	if req.Data.PostIds != nil {
		if err := r.syncUserPosts(ctx, id, req.Data.PostIds); err != nil {
			return err
		}
	}

	return nil
}

func (r *UserRepo) Delete(ctx context.Context, userId uint32) error {
	tx, err := r.data.db.Client().Tx(ctx)
	if err != nil {
		r.log.Errorf("begin tx failed [%d]: %s", userId, err.Error())
		return userV1.ErrorInternalServerError("delete failed")
	}

	if err := tx.User.DeleteOneID(userId).Exec(ctx); err != nil {
		_ = tx.Rollback()
		if ent.IsNotFound(err) {
			return userV1.ErrorNotFound("user not found")
		}

		r.log.Errorf("delete one data failed: %s", err.Error())

		return userV1.ErrorInternalServerError("delete failed")
	}

	// 级联清理登录凭据：孤儿行（identifier=username）会让同名重建用户在登录时触发 "not singular" 503
	if _, err := tx.UserCredential.Delete().Where(usercredential.UserIDEQ(userId)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete user credentials failed [%d]: %s", userId, err.Error())
		return userV1.ErrorInternalServerError("delete failed")
	}

	// 级联清理令牌：用户已删除，其 access/refresh token 不应再可用
	if _, err := tx.UserToken.Delete().Where(usertoken.UserIDEQ(userId)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete user tokens failed [%d]: %s", userId, err.Error())
		return userV1.ErrorInternalServerError("delete failed")
	}

	// 级联清理岗位关联（ap_org_user_post），避免孤儿行
	if _, err := tx.UserPost.Delete().Where(userpost.UserIDEQ(userId)).Exec(ctx); err != nil {
		_ = tx.Rollback()
		r.log.Errorf("delete user posts failed [%d]: %s", userId, err.Error())
		return userV1.ErrorInternalServerError("delete failed")
	}

	if err := tx.Commit(); err != nil {
		r.log.Errorf("commit tx failed [%d]: %s", userId, err.Error())
		return userV1.ErrorInternalServerError("delete failed")
	}

	return nil
}

// GetMFAStatus 读取用户 MFA 启用状态与 TOTP 密钥。
// mfa_enabled 已入 proto DTO（B7，列表/详情下发）；mfa_secret 属敏感材料不进 DTO，
// 启用/确认/禁用流程经此专用方法读取。
func (r *UserRepo) GetMFAStatus(ctx context.Context, userID uint32) (enabled bool, secret string, err error) {
	u, err := r.data.db.Client().User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, "", userV1.ErrorNotFound("user not found")
		}
		return false, "", userV1.ErrorInternalServerError("query user mfa failed")
	}
	if u.MfaSecret != nil {
		secret = *u.MfaSecret
	}
	return u.MfaEnabled, secret, nil
}

// SetMFASecret 写入 TOTP 密钥（不启用，等用户验证一次 code 后才调用 EnableMFA）。
func (r *UserRepo) SetMFASecret(ctx context.Context, userID uint32, secret string) error {
	if err := r.data.db.Client().User.UpdateOneID(userID).
		SetMfaSecret(secret).
		SetMfaEnabled(false).
		Exec(ctx); err != nil {
		r.log.Errorf("set mfa secret failed: %s", err.Error())
		return userV1.ErrorInternalServerError("set mfa secret failed")
	}
	return nil
}

// EnableMFA 启用用户 MFA（用户扫码并验证一次 code 后调用）。
func (r *UserRepo) EnableMFA(ctx context.Context, userID uint32) error {
	if err := r.data.db.Client().User.UpdateOneID(userID).
		SetMfaEnabled(true).
		Exec(ctx); err != nil {
		r.log.Errorf("enable mfa failed: %s", err.Error())
		return userV1.ErrorInternalServerError("enable mfa failed")
	}
	return nil
}

// DisableMFA 禁用用户 MFA 并清空密钥。
func (r *UserRepo) DisableMFA(ctx context.Context, userID uint32) error {
	if err := r.data.db.Client().User.UpdateOneID(userID).
		SetMfaEnabled(false).
		SetMfaSecret("").
		Exec(ctx); err != nil {
		r.log.Errorf("disable mfa failed: %s", err.Error())
		return userV1.ErrorInternalServerError("disable mfa failed")
	}
	return nil
}

// UpdatePasswordTimestamps 更新密码修改时间与过期时间。
// expireDays <= 0 表示永不过期。
func (r *UserRepo) UpdatePasswordTimestamps(ctx context.Context, userID uint32, expireDays int) error {
	now := time.Now()
	builder := r.data.db.Client().User.UpdateOneID(userID).
		SetPwdChangedAt(now)
	if expireDays > 0 {
		builder.SetPwdExpireAt(now.AddDate(0, 0, expireDays))
	}
	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("update password timestamps failed: %s", err.Error())
		return userV1.ErrorInternalServerError("update password timestamps failed")
	}
	return nil
}

// UpdateLastLogin 登录成功后回写最后登录时间与来源 IP。
// ip 为空时仅更新时间（保留历史 IP，避免回环取址失败时清空既有值）。
func (r *UserRepo) UpdateLastLogin(ctx context.Context, userID uint32, ip string) error {
	builder := r.data.db.Client().User.UpdateOneID(userID).
		SetLastLoginTime(time.Now())
	if ip != "" {
		builder.SetLastLoginIP(ip)
	}
	if err := builder.Exec(ctx); err != nil {
		r.log.Errorf("update last login failed: %s", err.Error())
		return userV1.ErrorInternalServerError("update last login failed")
	}
	return nil
}

// GetPasswordExpireAt 读取用户密码过期时间；返回 nil 表示永不过期或未设置。
func (r *UserRepo) GetPasswordExpireAt(ctx context.Context, userID uint32) (*time.Time, error) {
	u, err := r.data.db.Client().User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorNotFound("user not found")
		}
		return nil, userV1.ErrorInternalServerError("query user pwd expire failed")
	}
	return u.PwdExpireAt, nil
}

// GetOperationPassword 读取用户的独立操作密码（AES-256-GCM 加密）。
// 返回 nil 表示用户未设置独立操作密码（§2.6.6 调用方应返回 422 OPERATION_PASSWORD_NOT_SET）。
func (r *UserRepo) GetOperationPassword(ctx context.Context, userID uint32) (*string, error) {
	u, err := r.data.db.Client().User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorUserNotFound("user not found")
		}
		return nil, userV1.ErrorInternalServerError("query operation password failed")
	}
	return u.OperationPassword, nil
}

// SetOperationPassword 设置用户的独立操作密码（§2.6.6）。
// newPassword 必须为 6 位数字明文，oldPassword 在用户已设置过时必填用于校验。
// 存储使用 AES-256-GCM 加密。
func (r *UserRepo) SetOperationPassword(ctx context.Context, userID uint32, newPassword, oldPassword string) error {
	if len(newPassword) != 6 {
		return userV1.ErrorBadRequest("operation password must be 6 digits")
	}
	for _, c := range newPassword {
		if c < '0' || c > '9' {
			return userV1.ErrorBadRequest("operation password must be 6 digits")
		}
	}

	u, err := r.data.db.Client().User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return userV1.ErrorUserNotFound("user not found")
		}
		return userV1.ErrorInternalServerError("query user failed")
	}

	// 已设置过则必须校验旧密码
	if u.OperationPassword != nil && *u.OperationPassword != "" {
		if oldPassword == "" {
			return userV1.ErrorBadRequest("old operation password is required")
		}
		c := crypto.GetCrypto()
		decrypted, err := c.Decrypt(*u.OperationPassword)
		if err != nil {
			r.log.Errorf("decrypt old operation password failed: %s", err.Error())
			return userV1.ErrorInternalServerError("verify old password failed")
		}
		if decrypted != oldPassword {
			return userV1.ErrorForbidden("old operation password incorrect")
		}
	}

	c := crypto.GetCrypto()
	encrypted, err := c.Encrypt(newPassword)
	if err != nil {
		r.log.Errorf("encrypt operation password failed: %s", err.Error())
		return userV1.ErrorInternalServerError("encrypt password failed")
	}

	if err := r.data.db.Client().User.UpdateOneID(userID).
		SetOperationPassword(encrypted).
		Exec(ctx); err != nil {
		r.log.Errorf("set operation password failed: %s", err.Error())
		return userV1.ErrorInternalServerError("set operation password failed")
	}
	return nil
}

// VerifyOperationPassword 校验用户的独立操作密码（§2.6.6 二次鉴权）。
// 返回 (true, nil) 表示校验通过；用户未设置时返回 (false, ErrOperationPasswordNotSet)；
// 校验失败返回 (false, nil)，由调用方根据 ErrOperationPasswordNotSet 区分场景。
func (r *UserRepo) VerifyOperationPassword(ctx context.Context, userID uint32, input string) (bool, error) {
	u, err := r.data.db.Client().User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return false, userV1.ErrorUserNotFound("user not found")
		}
		return false, userV1.ErrorInternalServerError("query user failed")
	}

	if u.OperationPassword == nil || *u.OperationPassword == "" {
		return false, ErrOperationPasswordNotSet
	}

	c := crypto.GetCrypto()
	decrypted, err := c.Decrypt(*u.OperationPassword)
	if err != nil {
		r.log.Errorf("decrypt operation password failed: %s", err.Error())
		return false, userV1.ErrorInternalServerError("verify password failed")
	}

	return decrypted == input, nil
}

func (r *UserRepo) GetUserByUserName(ctx context.Context, userName string) (*userV1.User, error) {
	if userName == "" {
		return nil, userV1.ErrorBadRequest("invalid parameter")
	}

	entity, err := r.data.db.Client().User.Query().
		Where(user.UsernameEQ(userName)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, userV1.ErrorUserNotFound("user not found")
		}

		r.log.Errorf("query user data failed: %s", err.Error())

		return nil, userV1.ErrorInternalServerError("query data failed")
	}

	dto := r.mapper.ToDTO(entity)
	fillOrgID(entity, dto)
	fillPositionID(entity, dto)
	fillSecurityPolicyFields(entity, dto)
	// 填充角色码列表：登录签发 JWT 的 roc claim 依赖此字段（缺失时 roc=null，下游 RBAC 无法识别管理员）
	r.fillUserRoles(ctx, entity.ID, dto)
	return dto, nil
}

func (r *UserRepo) UserExists(ctx context.Context, req *userV1.UserExistsRequest) (*userV1.UserExistsResponse, error) {
	exist, err := r.data.db.Client().User.Query().
		Where(user.UsernameEQ(req.GetUsername())).
		Exist(ctx)
	if err != nil {
		r.log.Errorf("query exist failed: %s", err.Error())
		return &userV1.UserExistsResponse{
			Exist: false,
		}, userV1.ErrorInternalServerError("query exist failed")
	}

	return &userV1.UserExistsResponse{
		Exist: exist,
	}, nil
}
