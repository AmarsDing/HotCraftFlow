package biz

import (
	"context"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/tx7do/go-utils/trans"
	pagination "github.com/tx7do/kratos-bootstrap/api/gen/go/pagination/v1"

	adminV1 "framework/api/gen/go/admin/v1"
	"framework/pkg/middleware/logging"
	"framework/pkg/password"
)

// 预置配置键
const (
	ConfigKeySystemName               = "system_name"
	ConfigKeyLogoURL                  = "logo_url"
	ConfigKeyPasswordMinLength        = "password_min_length"
	ConfigKeyPasswordExpireDays       = "password_expire_days"
	ConfigKeyPasswordRequireUppercase = "password_require_uppercase"
	ConfigKeyPasswordRequireLowercase = "password_require_lowercase"
	ConfigKeyPasswordRequireDigit     = "password_require_digit"
	ConfigKeyPasswordRequireSpecial   = "password_require_special"
	ConfigKeyPasswordHistoryCount     = "password_history_count"
	ConfigKeyLoginFailLockCount       = "login_fail_lock_count"
	ConfigKeyLoginFailLockMinutesIP   = "login_fail_lock_minutes_ip"
	ConfigKeyLoginFailLockMinutesUser = "login_fail_lock_minutes_user"
	ConfigKeySessionTimeoutMinutes    = "session_timeout_minutes"
	ConfigKeyDefaultMaxConcurrent     = "default_max_concurrent"
	ConfigKeyMFAEnabled               = "mfa_enabled"
	// 可信反向代理网段（逗号分隔 CIDR/IP）。空=信任所有 XFF（旧行为）；
	// 非空=仅直连对端在网段内才采纳 X-Forwarded-For/X-Real-IP（防 XFF 伪造绕过 IP 黑白名单）。
	ConfigKeySecurityTrustedProxies = "security_trusted_proxies"

	// 远程运维（opstools）设置项（远程运维能力增强 B10；opstools 经 admin system_configs 接口读取）。
	ConfigKeyOpsHighRiskAssetGroups    = "ops.high_risk_asset_groups"
	ConfigKeyOpsMaxSessionsPerAsset    = "ops.max_sessions_per_asset"
	ConfigKeyOpsSessionConfirmRequired = "ops.session_confirm_required"
)

// presetConfig 预置配置项定义（首次启动 seed 用）。
type presetConfig struct {
	key         string
	value       string
	configType  string
	description string
}

var presetConfigs = []presetConfig{
	{ConfigKeySystemName, "Framework 管理平台", "string", "系统名称（前端顶栏/侧栏品牌区/登录页展示，系统管理-系统配置可改）"},
	{ConfigKeyLogoURL, "", "string", "Logo 地址"},
	{ConfigKeyPasswordMinLength, "8", "number", "密码最小长度"},
	{ConfigKeyPasswordExpireDays, "90", "number", "密码过期天数（0 表示不过期）"},
	{ConfigKeyPasswordRequireUppercase, "true", "bool", "密码必须包含大写字母"},
	{ConfigKeyPasswordRequireLowercase, "true", "bool", "密码必须包含小写字母"},
	{ConfigKeyPasswordRequireDigit, "true", "bool", "密码必须包含数字"},
	{ConfigKeyPasswordRequireSpecial, "false", "bool", "密码必须包含特殊字符"},
	{ConfigKeyPasswordHistoryCount, "5", "number", "历史密码校验数量"},
	{ConfigKeyLoginFailLockCount, "5", "number", "登录失败锁定阈值（次）"},
	{ConfigKeyLoginFailLockMinutesIP, "15", "number", "IP 登录失败锁定时长（分钟）"},
	{ConfigKeyLoginFailLockMinutesUser, "30", "number", "账户登录失败锁定时长（分钟）"},
	// 可信反向代理网段：仅直连对端落在网段内才采纳 XFF（防 XFF 伪造绕过 IP 黑白名单/锁定）。
	// 默认值必须与 docker-compose.yaml 的 twinnet 子网（172.28.0.0/16）一致（gateway→admin 链路）；
	// 44 环境 twinnet 实为 172.33.0.0/16，由 post-restore-fix-44.sql 幂等覆写。
	// 多网段逗号分隔，置空字符串=不启用收敛（兼容旧行为）。
	{ConfigKeySecurityTrustedProxies, "172.28.0.0/16", "string", "可信反向代理网段（逗号分隔 CIDR/IP，仅可信对端的 XFF 才被采纳）"},
	{ConfigKeySessionTimeoutMinutes, "120", "number", "会话超时时间（分钟）"},
	{ConfigKeyDefaultMaxConcurrent, "3", "number", "默认最大并发会话数"},
	{ConfigKeyMFAEnabled, "false", "bool", "是否启用 MFA"},
	{ConfigKeyOpsHighRiskAssetGroups, "[]", "json", "高危资产分组 ID 列表（远程运维 operator 档二次确认范围）"},
	{ConfigKeyOpsMaxSessionsPerAsset, "2", "number", "单设备并发远程主控会话上限（VNC/终端）"},
	{ConfigKeyOpsSessionConfirmRequired, "true", "bool", "operator 档对高危分组远程操作是否强制二次确认"},
}

// presetConfigKeys 预置配置键集合（由 presetConfigs 派生）。
// 预置项禁止删除：删除后 seed 会在下次启动以代码默认值复活（语义等同"恢复默认"但发生在
// 重启时点，不直观）；对 security_trusted_proxies 还会瞬时回退"信任所有 XFF"或错误网段。
// 需要"清空"语义的项（如可信网段置空=不启用收敛）应通过 Update 置空值，而非删除。
var presetConfigKeys = func() map[string]struct{} {
	m := make(map[string]struct{}, len(presetConfigs))
	for _, p := range presetConfigs {
		m[p.key] = struct{}{}
	}
	return m
}()

// SystemConfigRepo 系统配置仓储端口，由 data 层实现。
type SystemConfigRepo interface {
	List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListSystemConfigResponse, error)
	IsExist(ctx context.Context, id uint32) (bool, error)
	Get(ctx context.Context, req *adminV1.GetSystemConfigRequest) (*adminV1.SystemConfig, error)
	GetByKey(ctx context.Context, key string) (*adminV1.SystemConfig, error)
	GetAll(ctx context.Context) ([]*adminV1.SystemConfig, error)
	Create(ctx context.Context, req *adminV1.CreateSystemConfigRequest) error
	Update(ctx context.Context, req *adminV1.UpdateSystemConfigRequest) error
	Delete(ctx context.Context, req *adminV1.DeleteSystemConfigRequest) error
}

// SystemConfigUsecase 系统配置业务用例。
// 持有进程内缓存（atomic.Pointer 指向不可变 map），CRUD 后刷新；供其他模块通过 GetConfig 读取。
type SystemConfigUsecase struct {
	repo SystemConfigRepo
	log  *log.Helper

	cache atomic.Pointer[map[string]string] // config_key → config_value
}

// NewSystemConfigUsecase 创建系统配置 Usecase。
// 构造期加载全量配置到缓存，并 seed 预置项（缺失则补建）。
func NewSystemConfigUsecase(repo SystemConfigRepo, logger log.Logger) *SystemConfigUsecase {
	uc := &SystemConfigUsecase{
		repo: repo,
		log:  log.NewHelper(log.With(logger, "module", "system-config/usecase/admin-service")),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	uc.seed(ctx)
	if err := uc.refresh(ctx); err != nil {
		uc.log.Errorf("init load system configs failed (non-fatal): %v", err)
	}

	return uc
}

// seed 预置配置项：对每个 preset key 检查是否存在，缺失则创建。
// 幂等：已存在的配置不会被覆盖。
func (uc *SystemConfigUsecase) seed(ctx context.Context) {
	for _, p := range presetConfigs {
		// 空值预置项跳过播种：schema config_value NotEmpty 校验不接受空串，
		// 空值语义等同于"未配置"（如 logo_url），留待用户在系统设置中填写。
		if p.value == "" {
			continue
		}
		existing, err := uc.repo.GetByKey(ctx, p.key)
		if err != nil {
			uc.log.Warnf("seed: check config %s failed (non-fatal): %v", p.key, err)
			continue
		}
		if existing != nil {
			continue
		}
		if err := uc.repo.Create(ctx, &adminV1.CreateSystemConfigRequest{
			Data: &adminV1.SystemConfig{
				ConfigKey:   trans.Ptr(p.key),
				ConfigValue: trans.Ptr(p.value),
				ConfigType:  trans.Ptr(p.configType),
				Description: trans.Ptr(p.description),
			},
		}); err != nil {
			uc.log.Warnf("seed: create config %s failed (non-fatal): %v", p.key, err)
		}
	}
}

// refresh 从 DB 全量加载配置到缓存（构建新 map 后原子替换）。
// 同步将可信代理网段推送到边缘取 IP 逻辑（XFF 收敛），启动加载与配置变更均生效。
func (uc *SystemConfigUsecase) refresh(ctx context.Context) error {
	items, err := uc.repo.GetAll(ctx)
	if err != nil {
		return err
	}
	m := make(map[string]string, len(items))
	for _, it := range items {
		m[it.GetConfigKey()] = it.GetConfigValue()
	}
	uc.cache.Store(&m)
	logging.SetTrustedProxies(strings.Split(m[ConfigKeySecurityTrustedProxies], ","))
	return nil
}

// GetConfig 按 key 读取配置值（从缓存，未命中返回空串）。
func (uc *SystemConfigUsecase) GetConfig(key string) string {
	if p := uc.cache.Load(); p != nil {
		return (*p)[key]
	}
	return ""
}

// GetConfigInt 按 key 读取整型配置，解析失败或缺失返回 def。
func (uc *SystemConfigUsecase) GetConfigInt(key string, def int) int {
	v := uc.GetConfig(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// GetConfigBool 按 key 读取布尔配置，解析失败或缺失返回 def。
func (uc *SystemConfigUsecase) GetConfigBool(key string, def bool) bool {
	v := uc.GetConfig(key)
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

// GetPasswordPolicy 从缓存组装密码策略配置。
func (uc *SystemConfigUsecase) GetPasswordPolicy() password.PolicyConfig {
	return password.PolicyConfig{
		MinLength:         uc.GetConfigInt(ConfigKeyPasswordMinLength, 8),
		RequireUppercase:  uc.GetConfigBool(ConfigKeyPasswordRequireUppercase, true),
		RequireLowercase:  uc.GetConfigBool(ConfigKeyPasswordRequireLowercase, true),
		RequireDigit:      uc.GetConfigBool(ConfigKeyPasswordRequireDigit, true),
		RequireSpecial:    uc.GetConfigBool(ConfigKeyPasswordRequireSpecial, false),
		HistoryCheckCount: uc.GetConfigInt(ConfigKeyPasswordHistoryCount, 5),
		ExpireDays:        uc.GetConfigInt(ConfigKeyPasswordExpireDays, 90),
	}
}

// LoginLockConfig 登录失败锁定参数。
type LoginLockConfig struct {
	IPLockCount     int
	IPLockMinutes   int
	UserLockCount   int
	UserLockMinutes int
}

// GetLoginLockConfig 从缓存组装登录锁定参数。
func (uc *SystemConfigUsecase) GetLoginLockConfig() LoginLockConfig {
	return LoginLockConfig{
		IPLockCount:     uc.GetConfigInt(ConfigKeyLoginFailLockCount, 5),
		IPLockMinutes:   uc.GetConfigInt(ConfigKeyLoginFailLockMinutesIP, 15),
		UserLockCount:   uc.GetConfigInt(ConfigKeyLoginFailLockCount, 5),
		UserLockMinutes: uc.GetConfigInt(ConfigKeyLoginFailLockMinutesUser, 30),
	}
}

// GetDefaultMaxConcurrent 默认最大并发会话数。
func (uc *SystemConfigUsecase) GetDefaultMaxConcurrent() int {
	return uc.GetConfigInt(ConfigKeyDefaultMaxConcurrent, 3)
}

// IsMFAEnabled 全局是否启用 MFA。
func (uc *SystemConfigUsecase) IsMFAEnabled() bool {
	return uc.GetConfigBool(ConfigKeyMFAEnabled, false)
}

// GetSessionTimeoutMinutes 会话超时时间（分钟）。
func (uc *SystemConfigUsecase) GetSessionTimeoutMinutes() int {
	return uc.GetConfigInt(ConfigKeySessionTimeoutMinutes, 120)
}

// List 分页查询配置列表
func (uc *SystemConfigUsecase) List(ctx context.Context, req *pagination.PagingRequest) (*adminV1.ListSystemConfigResponse, error) {
	return uc.repo.List(ctx, req)
}

// Get 查询单个配置
func (uc *SystemConfigUsecase) Get(ctx context.Context, req *adminV1.GetSystemConfigRequest) (*adminV1.SystemConfig, error) {
	return uc.repo.Get(ctx, req)
}

// Create 创建配置后刷新缓存
func (uc *SystemConfigUsecase) Create(ctx context.Context, req *adminV1.CreateSystemConfigRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if err := uc.repo.Create(ctx, req); err != nil {
		return err
	}
	return uc.refresh(ctx)
}

// Update 更新配置后刷新缓存
func (uc *SystemConfigUsecase) Update(ctx context.Context, req *adminV1.UpdateSystemConfigRequest) error {
	if req.Data == nil {
		return adminV1.ErrorBadRequest("invalid parameter")
	}
	if err := uc.repo.Update(ctx, req); err != nil {
		return err
	}
	return uc.refresh(ctx)
}

// Delete 删除配置后刷新缓存
// Delete 删除配置项。预置配置项禁止删除（见 presetConfigKeys 注释）；删除成功后刷新缓存。
func (uc *SystemConfigUsecase) Delete(ctx context.Context, req *adminV1.DeleteSystemConfigRequest) error {
	item, err := uc.repo.Get(ctx, &adminV1.GetSystemConfigRequest{Id: req.GetId()})
	if err != nil {
		return err
	}
	if _, ok := presetConfigKeys[item.GetConfigKey()]; ok {
		return adminV1.ErrorForbidden("预置配置项 %s 禁止删除，可修改其值", item.GetConfigKey())
	}
	if err := uc.repo.Delete(ctx, req); err != nil {
		return err
	}
	return uc.refresh(ctx)
}
