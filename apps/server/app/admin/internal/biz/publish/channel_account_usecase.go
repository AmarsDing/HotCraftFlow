package publish

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"

	"framework/pkg/mediacontract"
)

// CapabilityProvider 能力源端口（publishadapters 实现：注册适配器声明优先，platforms.yaml 兜底）。
type CapabilityProvider interface {
	// CapabilityFor 返回平台能力与「是否有任何来源声明」。
	CapabilityFor(platformKey string) (Capability, bool)
}

// ChannelUsecase 渠道用例（platform:slot 槽位管理；能力快照在建渠道时固化，运营可调，
// 发布时快照优先于适配器声明——见 CapabilityOf）。
type ChannelUsecase struct {
	conf     *Conf
	channels ChannelRepo
	caps     CapabilityProvider
	log      *log.Helper
}

// NewChannelUsecase 构造渠道用例。
func NewChannelUsecase(conf *Conf, channels ChannelRepo, caps CapabilityProvider, logger log.Logger) *ChannelUsecase {
	return &ChannelUsecase{
		conf:     conf,
		channels: channels,
		caps:     caps,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish-channel")),
	}
}

// CreateChannelInput 建渠道输入。
type CreateChannelInput struct {
	PlatformKey    string
	Slot           string
	Region         string
	Enabled        bool
	Priority       int
	AdapterVersion string
}

// Create 建渠道（platform:slot 唯一；能力快照取适配器声明 / platforms.yaml）。
func (uc *ChannelUsecase) Create(ctx context.Context, in CreateChannelInput) (*ChannelRow, error) {
	platform := strings.TrimSpace(in.PlatformKey)
	slot := strings.TrimSpace(in.Slot)
	if platform == "" || slot == "" {
		return nil, fmt.Errorf("%w: platform_key 与 slot 必填", ErrBadRequest)
	}
	region := strings.TrimSpace(in.Region)
	if region == "" {
		region = string(mediacontract.RegionGlobal)
	}
	if !mediacontract.Region(region).Valid() {
		return nil, fmt.Errorf("%w: 非法 region %q（仅 cn/intl/global）", ErrBadRequest, region)
	}

	row := &ChannelRow{
		ChannelID:   mediacontract.NewID("ch"),
		PlatformKey: platform,
		Slot:        slot,
		Region:      region,
		Enabled:     in.Enabled,
		Priority:    in.Priority,
	}
	if in.AdapterVersion != "" {
		row.AdapterVersion = strPtr(in.AdapterVersion)
	}
	// 能力快照（best-effort：来源缺失时留空，发布时回退适配器声明/报错）。
	if cap, ok := uc.caps.CapabilityFor(platform); ok {
		row.Capability = capabilitySnapshot(cap)
	}
	if err := uc.channels.CreateChannel(ctx, row); err != nil {
		return nil, err
	}
	return row, nil
}

// List 分页列出渠道。
func (uc *ChannelUsecase) List(ctx context.Context, pageNo, pageSize int) ([]*ChannelRow, int, error) {
	return uc.channels.ListChannels(ctx, pageNo, pageSize)
}

// Get 按 channel_id 取渠道（账号列表冗余字段装配用）。
func (uc *ChannelUsecase) Get(ctx context.Context, channelID string) (*ChannelRow, error) {
	return uc.channels.GetChannel(ctx, channelID)
}

// capabilitySnapshot Capability → 可入库的 JSON map（键名对齐 capabilityFromSnapshot）。
func capabilitySnapshot(c Capability) map[string]any {
	out := map[string]any{
		"video":           c.Video,
		"image_text":      c.ImageText,
		"native_schedule": c.NativeSchedule,
		"draft_state":     c.DraftState,
		"analytics":       c.Analytics,
	}
	if c.MaxDurationSec > 0 {
		out["max_duration_sec"] = c.MaxDurationSec
	}
	if c.MaxSizeBytes > 0 {
		out["max_size_bytes"] = c.MaxSizeBytes
	}
	if len(c.AspectRatios) > 0 {
		arr := make([]any, 0, len(c.AspectRatios))
		for _, r := range c.AspectRatios {
			arr = append(arr, r)
		}
		out["aspect_ratios"] = arr
	}
	if c.TitleMaxLen > 0 {
		out["title_max_len"] = c.TitleMaxLen
	}
	if c.DescMaxLen > 0 {
		out["desc_max_len"] = c.DescMaxLen
	}
	return out
}

// AccountUsecase 账号池用例（FR-PUB-05）。
//
// 🔴 凭据纪律：凭据明文只在 Store 期间存在于本用例作用域；vault.Store 返回引用后
// 本用例立即丢弃明文（不记日志、不回传）；重新授权覆盖引用旧密文自然失效。
type AccountUsecase struct {
	conf     *Conf
	accounts AccountRepo
	channels ChannelRepo
	vault    CredentialVault
	clock    func() time.Time
	log      *log.Helper
}

// NewAccountUsecase 构造账号用例。
func NewAccountUsecase(
	conf *Conf,
	accounts AccountRepo,
	channels ChannelRepo,
	vault CredentialVault,
	logger log.Logger,
) *AccountUsecase {
	return &AccountUsecase{
		conf:     conf,
		accounts: accounts,
		channels: channels,
		vault:    vault,
		clock:    time.Now,
		log:      log.NewHelper(log.With(logger, "module", "biz/publish-account")),
	}
}

// CreateAccountInput 建账号输入。
type CreateAccountInput struct {
	ChannelID      string
	DisplayName    string
	CredentialType string   // oauth / cookie / api_key
	Credential     string   // 🔴 明文仅经 TLS 传输一次；加密后仅引用入库
	Enabled        bool
}

// Create 建账号（凭据可选；未配置时 has_credential=false，发布于该渠道前必须 reauth）。
func (uc *AccountUsecase) Create(ctx context.Context, in CreateAccountInput) (*AccountRow, error) {
	name := strings.TrimSpace(in.DisplayName)
	if name == "" {
		return nil, fmt.Errorf("%w: display_name 必填", ErrBadRequest)
	}
	ch, err := uc.channels.GetChannel(ctx, in.ChannelID)
	if err != nil {
		return nil, fmt.Errorf("%w: 渠道 %s 不存在", ErrChannelNotFound, in.ChannelID)
	}
	credType := strings.TrimSpace(in.CredentialType)

	//
	row := &AccountRow{
		AccountID:   mediacontract.NewPublishAccountID(),
		ChannelID:   ch.ChannelID,
		DisplayName: name,
		Enabled:     in.Enabled,
		Health:      AccountHealthHealthy,
		PlatformKey: ch.PlatformKey,
		Slot:        ch.Slot,
	}
	plain := strings.TrimSpace(in.Credential)
	if plain != "" {
		if credType == "" {
			return nil, fmt.Errorf("%w: 提供凭据时 credential_type 必填（oauth/cookie/api_key）", ErrBadRequest)
		}
		cred := &Credential{Type: credType, Secret: []byte(plain)}
		ref, serr := uc.vault.Store(ctx, row.AccountID, cred)
		cred.Zero() // 🔴 用完即清
		if serr != nil {
			return nil, fmt.Errorf("%w: 凭据加密失败", ErrInternal)
		}
		row.CredentialRef = strPtr(ref)
		row.CredentialType = credType
		row.HasCredential = true
	}
	if err := uc.accounts.CreateAccount(ctx, row); err != nil {
		return nil, err
	}
	uc.log.Infof("[map-account] 账号创建 account=%s channel=%s has_credential=%v", row.AccountID, ch.ChannelID, row.HasCredential)
	return row, nil
}

// List 分页列出账号。
func (uc *AccountUsecase) List(ctx context.Context, pageNo, pageSize int) ([]*AccountRow, int, error) {
	return uc.accounts.ListAccounts(ctx, pageNo, pageSize)
}

// Toggle 人工启停。
func (uc *AccountUsecase) Toggle(ctx context.Context, accountID string, enabled bool) (*AccountRow, error) {
	if _, err := uc.accounts.GetAccount(ctx, accountID); err != nil {
		return nil, err
	}
	if err := uc.accounts.SetEnabled(ctx, accountID, enabled); err != nil {
		return nil, err
	}
	return uc.accounts.GetAccount(ctx, accountID)
}

// Reauth 重新授权（凭据失效后的恢复路径）：
// 🔴 覆盖新凭据引用 + 健康复位 healthy + 连续失败清零；旧凭据在失效期间从不删除
// （便于排查），重新授权后旧密文引用被自然替换。
func (uc *AccountUsecase) Reauth(ctx context.Context, accountID, credType, credential string) error {
	acct, err := uc.accounts.GetAccount(ctx, accountID)
	if err != nil {
		return err
	}
	plain := strings.TrimSpace(credential)
	if plain == "" {
		return fmt.Errorf("%w: 重新授权必须提供新凭据", ErrBadRequest)
	}
	credType = strings.TrimSpace(credType)
	if credType == "" {
		credType = acct.CredentialType
	}
	if credType == "" {
		return fmt.Errorf("%w: credential_type 必填（oauth/cookie/api_key）", ErrBadRequest)
	}
	cred := &Credential{Type: credType, Secret: []byte(plain)}
	ref, serr := uc.vault.Store(ctx, accountID, cred)
	cred.Zero() // 🔴 用完即清
	if serr != nil {
		return fmt.Errorf("%w: 凭据加密失败", ErrInternal)
	}
	if err := uc.accounts.SetCredentialRef(ctx, accountID, ref, credType); err != nil {
		return err
	}
	if err := uc.accounts.ResetFailures(ctx, accountID); err != nil {
		return err
	}
	if err := uc.accounts.SetHealth(ctx, accountID, AccountHealthHealthy, nil); err != nil {
		return err
	}
	uc.log.Infof("[map-account] 账号重新授权 account=%s（健康复位 healthy）", accountID)
	return nil
}
