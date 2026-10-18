import fs from 'fs';
import path from 'path';

const dataPath = path.join(process.argv[2], 'internal/data/data.go');
let s = fs.readFileSync(dataPath, 'utf8');
const start = s.indexOf('var ProviderSet = wire.NewSet(');
if (start < 0) throw new Error('ProviderSet not found');
const after = s.slice(start);
// Match closing paren of ProviderSet: line that is only ")"
const m = after.match(/\r?\n\)\r?\n/);
if (!m) {
  // debug: show nearby
  const idx = after.indexOf('wire.Bind(new(biz.AirportBusinessInfoRepo)');
  console.log('sample around binds', JSON.stringify(after.slice(idx, idx+80)));
  throw new Error('ProviderSet end not found');
}
const end = start + m.index + m[0].length;

const inner = `var ProviderSet = wire.NewSet(
	NewData,

	NewEntClient,
	NewLogEntClient,

	NewAuthenticator,
	NewAuthorizer,

	NewPasswordCrypto,

	NewLocalStorage,

	NewMenuRepo,
	NewDictRepo,
	NewTaskRepo,
	NewTaskExecutionRepo,
	NewAdminLoginRestrictionRepo,
	NewToolLinkRepo,
	NewToolLinkCategoryRepo,
	NewToolLinkUserMarkRepo,
	NewToolLinkSsoTicketRepo,
	NewApiResourceRepo,

	NewOrganizationRepo,
	NewDepartmentRepo,
	NewPositionRepo,
	NewRoleRepo,
	NewUserRepo,
	NewTenantRepo,
	NewUserCredentialRepo,

	NewAdminLoginLogRepo,
	NewAdminOperationLogRepo,

	NewFileRepo,

	NewNotificationMessageRepo,
	NewNotificationMessageCategoryRepo,
	NewNotificationMessageRecipientRepo,
	NewPrivateMessageRepo,

	NewUserTokenRepo,

	NewCasbinRuleRepo,
	NewRoleResourceGroupRepo,

	NewRedisClient,

	NewResourceGroupRepo,

	NewLoginFailCounterRepo,
	NewPasswordHistoryRepo,
	NewSystemConfigRepo,
	NewDataPermissionRepo,
	NewRoleDeptRepo,

	wire.Bind(new(biz.MenuRepo), new(*MenuRepo)),
	wire.Bind(new(biz.ApiResourceRepo), new(*ApiResourceRepo)),
	wire.Bind(new(biz.NotificationMessageRepo), new(*NotificationMessageRepo)),
	wire.Bind(new(biz.NotificationMessageCategoryRepo), new(*NotificationMessageCategoryRepo)),
	wire.Bind(new(biz.NotificationMessageRecipientRepo), new(*NotificationMessageRecipientRepo)),
	wire.Bind(new(biz.PrivateMessageRepo), new(*PrivateMessageRepo)),
	wire.Bind(new(biz.AdminLoginRestrictionRepo), new(*AdminLoginRestrictionRepo)),
	wire.Bind(new(biz.UserTokenRepo), new(*UserTokenCacheRepo)),
	wire.Bind(new(biz.LoginFailCounterRepo), new(*LoginFailCounterRepo)),
	wire.Bind(new(biz.PasswordHistoryRepo), new(*PasswordHistoryRepo)),
	wire.Bind(new(biz.UserRepo), new(*UserRepo)),
	wire.Bind(new(biz.UserCredentialRepo), new(*UserCredentialRepo)),
	wire.Bind(new(biz.RoleRepo), new(*RoleRepo)),
	wire.Bind(new(biz.DepartmentRepo), new(*DepartmentRepo)),
	wire.Bind(new(biz.PositionRepo), new(*PositionRepo)),
	wire.Bind(new(biz.OrganizationRepo), new(*OrganizationRepo)),
	wire.Bind(new(biz.TenantRepo), new(*TenantRepo)),
	wire.Bind(new(biz.AdminLoginLogRepo), new(*AdminLoginLogRepo)),
	wire.Bind(new(biz.AdminOperationLogRepo), new(*AdminOperationLogRepo)),
	wire.Bind(new(biz.RoleResourceGroupReader), new(*RoleResourceGroupRepo)),
	wire.Bind(new(biz.AuthorizerPort), new(*Authorizer)),
	wire.Bind(new(biz.InternalMessageNotifier), new(*NotificationMessageRepo)),
	wire.Bind(new(biz.RedisStore), new(*RedisClient)),
)
`;

s = s.slice(0, start) + inner + s.slice(end);
fs.writeFileSync(dataPath, s);
console.log('ok', 'has Airport', /NewAirport/.test(s), 'has MenuRepo bind', /biz\.MenuRepo/.test(s));
