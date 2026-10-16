package data

import (
	"context"
	"database/sql"

	"github.com/go-kratos/kratos/v2/log"

	authnEngine "github.com/tx7do/kratos-authn/engine"
	"github.com/tx7do/kratos-authn/engine/jwt"

	"github.com/tx7do/go-utils/entgo"
	"github.com/tx7do/go-utils/password"

	conf "framework/app/admin/internal/conf"

	"framework/app/admin/internal/biz"
	"framework/app/admin/internal/data/ent"
	"framework/pkg/observability"
	"framework/pkg/storage"

	"github.com/google/wire"
)

// ProviderSet is data providers.
var ProviderSet = wire.NewSet(
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

// Data .
type Data struct {
	log *log.Helper

	db    *entgo.EntClient[*ent.Client] // 主库 framework
	logDb *LogDB                        // 日志库 framework_log（nil 时回退主库）

	authenticator authnEngine.Authenticator
	authorizer    *Authorizer
}

// NewData .
func NewData(
	logger log.Logger,
	db *entgo.EntClient[*ent.Client],
	logDb *LogDB,
) (*Data, func(), error) {
	l := log.NewHelper(log.With(logger, "module", "data/admin-service"))

	d := &Data{
		log: l,

		db:    db,
		logDb: logDb,
	}

	return d, func() {
		l.Info("closing the data resources")

		if d.db != nil {
			if err := d.db.Close(); err != nil {
				l.Error(err)
			}
		}

		if d.logDb != nil && d.logDb.EntClient != nil {
			if err := d.logDb.EntClient.Close(); err != nil {
				l.Error(err)
			}
		}
	}, nil
}

// MainDB 返回主库原生 *sql.DB（气象雷达向导等手写 SQL）。
func (d *Data) MainDB() *sql.DB {
	if d.db == nil {
		return nil
	}
	return d.db.DB()
}

// LogClient 返回日志库 ent client，若 logDb 未配置则回退到主库。
func (d *Data) LogClient() *ent.Client {
	if d.logDb != nil && d.logDb.EntClient != nil {
		return d.logDb.EntClient.Client()
	}
	return d.db.Client()
}

// LogDBSql 返回日志库原生 *sql.DB 连接（用于跨库只读查询 alarm_events 等），若未配置则返回 nil。
func (d *Data) LogDBSql() *sql.DB {
	if d.logDb != nil && d.logDb.EntClient != nil {
		return d.logDb.DB()
	}
	return nil
}

// PingChecks 返回 /ready 就绪探针的依赖检查集合（仅包含已配置的依赖）。
func (d *Data) PingChecks() map[string]observability.CheckFunc {
	checks := make(map[string]observability.CheckFunc, 2)
	if d.db != nil && d.db.DB() != nil {
		checks["postgres"] = func(ctx context.Context) error { return d.db.DB().PingContext(ctx) }
	}
	if d.logDb != nil && d.logDb.EntClient != nil && d.logDb.DB() != nil {
		checks["postgres_log"] = func(ctx context.Context) error { return d.logDb.DB().PingContext(ctx) }
	}
	return checks
}

// NewAuthenticator 创建认证器
func NewAuthenticator(cfg *conf.Bootstrap) authnEngine.Authenticator {
	if cfg.GetAuth() == nil || cfg.GetAuth().GetAuthn() == nil {
		return nil
	}

	switch cfg.GetAuth().GetAuthn().GetType() {
	default:
		return nil

	case "jwt":
		authenticator, err := jwt.NewAuthenticator(
			jwt.WithKey([]byte(cfg.GetAuth().GetAuthn().GetJwt().GetKey())),
			jwt.WithSigningMethod(cfg.GetAuth().GetAuthn().GetJwt().GetMethod()),
		)
		if err != nil {
			return nil
		}
		return authenticator

	case "oidc":
		return nil

	case "preshared_key":
		return nil
	}
}

func NewUserTokenRepo(logger log.Logger, data *Data, authenticator authnEngine.Authenticator, cfg *conf.Bootstrap) *UserTokenCacheRepo {
	return NewUserTokenCacheRepo(
		logger,
		data, authenticator,
		cfg.GetServer().GetRest().GetMiddleware().GetAuth().GetAccessTokenExpires().AsDuration(),
		cfg.GetServer().GetRest().GetMiddleware().GetAuth().GetRefreshTokenExpires().AsDuration(),
	)
}

// NewLocalStorage 创建本地文件存储客户端（决策 D6：替代 MinIO/OSS，storage.root 未配置时返回 nil）。
func NewLocalStorage(cfg *conf.Bootstrap, logger log.Logger) *storage.LocalStorage {
	if cfg == nil || cfg.Storage == nil {
		return nil
	}
	return storage.NewLocalStorage(cfg.Storage, logger)
}

func NewPasswordCrypto() password.Crypto {
	crypto, err := password.CreateCrypto("bcrypt")
	if err != nil {
		panic(err)
	}
	return crypto
}
