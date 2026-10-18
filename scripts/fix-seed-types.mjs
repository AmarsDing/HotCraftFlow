import fs from 'fs';
const p = 'E:/project/TwinMonitor/FrameWork/apps/server/app/admin/internal/service/seed_service.go';
let s = fs.readFileSync(p, 'utf8');

s = s.replace(
  /func \(s \*SeedService\) seedRoles\(ctx context\.Context, apiIds \[\]uint32\) error \{[\s\S]*?\n\}/,
  `func (s *SeedService) seedRoles(ctx context.Context, apiIds []uint32) error {
	menus, err := s.menuUC.ListAll(ctx)
	if err != nil {
		return err
	}
	menuIDs := make([]uint32, 0, len(menus))
	for _, m := range menus {
		if m.GetId() != 0 {
			menuIDs = append(menuIDs, uint32(m.GetId()))
		}
	}
	if _, err := s.roleUC.GetRoleByCode(ctx, "super"); err == nil {
		return nil
	}
	if err := s.roleUC.Create(ctx, &userV1.CreateRoleRequest{Data: &userV1.Role{
		Name:   trans.Ptr("超级管理员"),
		Code:   trans.Ptr("super"),
		Status: trans.Ptr("ON"),
		Remark: trans.Ptr("framework skeleton"),
		Menus:  menuIDs,
		Apis:   apiIds,
	}}); err != nil {
		return fmt.Errorf("seed super role: %w", err)
	}
	return nil
}`
);

s = s.replace(
  /func \(s \*SeedService\) seedOrganizations\(ctx context\.Context\) error \{[\s\S]*?\n\}/,
  `func (s *SeedService) seedOrganizations(ctx context.Context) error {
	if _, err := s.organizationUC.Get(ctx, &userV1.GetOrganizationRequest{Id: 1}); err == nil {
		return nil
	}
	return s.organizationUC.Create(ctx, &userV1.CreateOrganizationRequest{Data: &userV1.Organization{
		Id:     trans.Ptr(uint32(1)),
		Name:   trans.Ptr("默认组织"),
		Status: trans.Ptr("ON"),
	}})
}`
);

s = s.replace(
  /func \(s \*SeedService\) seedDepartments\(ctx context\.Context\) error \{[\s\S]*?\n\}/,
  `func (s *SeedService) seedDepartments(ctx context.Context) error {
	if _, err := s.departmentUC.Get(ctx, &userV1.GetDepartmentRequest{Id: 1}); err == nil {
		return nil
	}
	return s.departmentUC.Create(ctx, &userV1.CreateDepartmentRequest{Data: &userV1.Department{
		Id:             trans.Ptr(uint32(1)),
		Name:           trans.Ptr("默认部门"),
		OrganizationId: trans.Ptr(int32(1)),
		Status:         trans.Ptr("ON"),
	}})
}`
);

s = s.replace(
  /func \(s \*SeedService\) seedPositions\(ctx context\.Context\) error \{[\s\S]*?\n\}/,
  `func (s *SeedService) seedPositions(ctx context.Context) error {
	if _, err := s.positionUC.Get(ctx, &userV1.GetPositionRequest{Id: 1}); err == nil {
		return nil
	}
	return s.positionUC.Create(ctx, &userV1.CreatePositionRequest{Data: &userV1.Position{
		Id:     trans.Ptr(uint32(1)),
		Name:   trans.Ptr("管理员"),
		Code:   trans.Ptr("admin"),
		Status: trans.Ptr("ON"),
	}})
}`
);

fs.writeFileSync(p, s);
console.log('seed entity creates fixed');
