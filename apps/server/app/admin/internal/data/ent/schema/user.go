package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/privacy"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/tx7do/go-utils/entgo/mixin"

	appmixin "framework/pkg/entgo/mixin"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{
			Table:     "ap_org_user",
			Charset:   "utf8mb4",
			Collation: "utf8mb4_bin",
		},
		entsql.WithComments(true),
		schema.Comment("用户表"),
	}
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.String("username").
			Comment("用户名").
			Unique().
			NotEmpty().
			Immutable().
			Optional().
			Nillable(),

		field.String("nickname").
			Comment("昵称").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("realname").
			Comment("真实名字").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("email").
			Comment("电子邮箱").
			MaxLen(320).
			Optional().
			Nillable(),

		field.String("mobile").
			Comment("手机号码").
			Default("").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("telephone").
			Comment("座机号码").
			Default("").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("avatar").
			Comment("头像").
			MaxLen(1023).
			Optional().
			Nillable(),

		field.String("address").
			Comment("地址").
			Default("").
			MaxLen(2048).
			Optional().
			Nillable(),

		field.String("region").
			Comment("国家地区").
			Default("").
			MaxLen(255).
			Optional().
			Nillable(),

		field.String("description").
			Comment("个人说明").
			MaxLen(1023).
			Optional().
			Nillable(),

		field.Enum("gender").
			Comment("性别").
			NamedValues(
				"Secret", "SECRET",
				"Male", "MALE",
				"Female", "FEMALE",
			).
			Optional().
			Nillable(),

		field.Enum("authority").
			Comment("授权").
			Optional().
			Nillable().
			NamedValues(
				"SysAdmin", "SYS_ADMIN",
				"TenantAdmin", "TENANT_ADMIN",
				"CustomerUser", "CUSTOMER_USER",
				"Guest", "GUEST",
			).
			Default("CUSTOMER_USER"),

		field.Time("last_login_time").
			Comment("最后一次登录的时间").
			Optional().
			Nillable(),

		field.String("last_login_ip").
			Comment("最后一次登录的IP").
			Default("").
			MaxLen(64).
			Optional().
			Nillable(),

		field.Uint32("dept_id").
			Comment("部门ID").
			Optional().
			Nillable(),

		field.Uint32("position_id").
			Comment("职位ID（关联 positions 表；G8 接线——proto 字段早已存在但 ent 无列致静默丢弃）").
			Optional().
			Nillable(),

		field.Uint32("work_id").
			Comment("员工工号").
			Optional().
			Nillable(),

		field.Time("pwd_changed_at").
			Comment("密码最后修改时间").
			Optional().
			Nillable(),

		field.Time("pwd_expire_at").
			Comment("密码过期时间").
			Optional().
			Nillable(),

		field.Int("login_fail_count").
			Comment("连续登录失败次数").
			Default(0),

		field.Time("locked_until").
			Comment("账户锁定截止时间").
			Optional().
			Nillable(),

		field.Int("max_concurrent").
			Comment("最大并发会话数").
			Default(3),

		field.Bool("mfa_enabled").
			Comment("是否启用MFA").
			Default(false),

		field.String("mfa_secret").
			Comment("MFA TOTP密钥").
			MaxLen(64).
			Optional().
			Nillable(),

		// operation_password 用于 §2.6.6 密码查看接口的二次鉴权
		// 存储形式：AES-256-GCM 加密后的 6 位数字明文；为空表示用户未设置独立操作密码
		field.String("operation_password").
			Comment("独立操作密码（AES-256 加密，6 位数字明文，与登录密码独立）").
			Optional().
			Nillable().
			MaxLen(255),
	}
}

// Mixin of the User.
func (User) Mixin() []ent.Mixin {
	return []ent.Mixin{
		mixin.AutoIncrementId{},
		mixin.CreateBy{},
		mixin.UpdateBy{},
		mixin.DeleteBy{},
		mixin.Time{},
		mixin.Remark{},
		mixin.SwitchStatus{},
		appmixin.TenantID{},
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return nil
}

// Indexes of the User.
func (User) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("dept_id"),
	}
}

// Policy 隐私策略占位：显式声明 AlwaysAllow，行为与未定义 Policy 一致；软删/租户过滤仍由各 repo 手动条件与上层鉴权承担。
func (User) Policy() ent.Policy {
	return privacy.Policy{
		Mutation: privacy.MutationPolicy{
			privacy.AlwaysAllowRule(),
		},
		Query: privacy.QueryPolicy{
			privacy.AlwaysAllowRule(),
		},
	}
}
