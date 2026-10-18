package pubdata

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/go-kratos/kratos/v2/log"

	bizpub "framework/app/admin/internal/biz/publish"
	"framework/pkg/crypto"
)

// credentialRefPrefix 凭据引用格式：enc:v1:<base64(nonce+ciphertext)>。
// v1 = AES-256-GCM（pkg/crypto）+ JSON 载荷；未来轮换算法/格式时 bump 版本号，
// Load 按前缀路由，保证旧引用仍可解。
const credentialRefPrefix = "enc:v1:"

// devFallbackKey 开发兜底密钥（32B）——🔴 独立密钥源：不复用 pkg/crypto 的
// ASSET_ENCRYPTION_KEY 单例与其内置默认串；生产必须设 MAP_CREDENTIAL_KEY，
// 缺失时启动即 Warn（与 pkg/crypto 的 dev-key 路径一致策略，但密钥材料隔离）。
const devFallbackKey = "MapDevCredentialKey2026HotCraft!"

// MapCredentialVault bizpub.CredentialVault 的本地加密实现（详细设计 §5）。
//
// 🔴 纪律：
//   - 密钥只来自环境变量（默认 MAP_CREDENTIAL_KEY），绝不入库、绝不进日志。
//   - DB 中 credential_ref 存的是「密文引用串」；明文仅在 Load 返回后的调用方作用域存活，
//     调用方必须 defer cred.Zero()（biz 层已有该约定）。
//   - 本类型方法永不记录 cred.Secret（日志仅出 accountID/ref 前缀）。
type MapCredentialVault struct {
	cipher *crypto.AESGCMCrypto
	log    *log.Helper
}

// NewMapCredentialVault 从 conf 指定的环境变量取 32B 密钥构造加密器。
// 缺省/长度异常走 dev 兜底（Warn 醒目提示），与 pkg/crypto 的 getKey 策略对齐但密钥独立。
func NewMapCredentialVault(conf *bizpub.Conf, logger log.Logger) (*MapCredentialVault, error) {
	l := log.NewHelper(log.With(logger, "module", "data/map-credential-vault"))

	keyEnv := conf.Credential.KeyEnv
	if keyEnv == "" {
		keyEnv = "MAP_CREDENTIAL_KEY"
	}
	key := []byte(os.Getenv(keyEnv))
	switch {
	case len(key) == 32:
		// 正解：生产路径。
	case len(key) > 0:
		l.Warnf("环境变量 %s 长度 %d 字节，非 32（AES-256），将零 padding/截断（建议配置 32 字节密钥）", keyEnv, len(key))
		b := make([]byte, 32)
		copy(b, key)
		key = b
	default:
		l.Warnf("🔴 未设置环境变量 %s，使用开发兜底密钥——生产环境必须设置该变量（密钥不入库）", keyEnv)
		key = []byte(devFallbackKey)
	}

	c, err := crypto.NewAESGCMCrypto(key)
	// key 使用后尽量清引用（GC 无法保证时机，comments 即约定）。
	for i := range key {
		key[i] = 0
	}
	if err != nil {
		return nil, fmt.Errorf("init map credential vault: %w", err)
	}
	return &MapCredentialVault{cipher: c, log: l}, nil
}

// credentialPayload 加密前的序列化载荷（JSON，便于未来扩展字段不破坏旧引用）。
type credentialPayload struct {
	Type   string `json:"t"` // oauth / cookie / api_key
	Secret string `json:"s"` // base64 编码的明文字节（避免二进制进 JSON）
}

// Store 加密存储凭据，返回引用（enc:v1:...）。
// 🔴 调用方（biz 层）负责在 Store 返回后 cred.Zero()；本方法内的中间明文同步清零。
func (v *MapCredentialVault) Store(_ context.Context, accountID string, cred *bizpub.Credential) (string, error) {
	if cred == nil || len(cred.Secret) == 0 {
		return "", fmt.Errorf("%w: 凭据为空，无法加密存储", bizpub.ErrBadRequest)
	}
	payload, err := json.Marshal(credentialPayload{Type: cred.Type, Secret: base64.StdEncoding.EncodeToString(cred.Secret)})
	if err != nil {
		return "", fmt.Errorf("%w: 凭据序列化失败", bizpub.ErrInternal)
	}
	ciphertext, err := v.cipher.Encrypt(string(payload))
	// 🔴 中间明文缓冲清零（序列化副本）。
	for i := range payload {
		payload[i] = 0
	}
	if err != nil {
		v.log.Errorf("[map-vault] 凭据加密失败 account=%s err=%v", accountID, err)
		return "", fmt.Errorf("%w: 凭据加密失败", bizpub.ErrInternal)
	}
	return credentialRefPrefix + ciphertext, nil
}

// Load 按引用解密（🔴 调用方必须 defer cred.Zero()；ref 非法/解密失败均报凭据类错误，不透内部细节）。
func (v *MapCredentialVault) Load(_ context.Context, ref string) (*bizpub.Credential, error) {
	if !strings.HasPrefix(ref, credentialRefPrefix) {
		return nil, fmt.Errorf("%w: 凭据引用格式不支持（期望 %s 前缀）", bizpub.ErrCredentialInvalid, "enc:v1:")
	}
	plain, err := v.cipher.Decrypt(strings.TrimPrefix(ref, credentialRefPrefix))
	if err != nil {
		return nil, fmt.Errorf("%w: 凭据解密失败（密钥不匹配或密文损坏）", bizpub.ErrCredentialInvalid)
	}
	var p credentialPayload
	if err := json.Unmarshal([]byte(plain), &p); err != nil {
		return nil, fmt.Errorf("%w: 凭据载荷解析失败", bizpub.ErrCredentialInvalid)
	}
	secret, err := base64.StdEncoding.DecodeString(p.Secret)
	if err != nil {
		return nil, fmt.Errorf("%w: 凭据内容损坏", bizpub.ErrCredentialInvalid)
	}
	return &bizpub.Credential{Type: p.Type, Secret: secret}, nil
}

// Delete 删除凭据（账号删除场景）。
// V1 实现说明：密文本体寄生在账号行 credential_ref 字段内， vault 本身无独立存储，
// 因此 Delete 为空操作——真正删除 = 清空账号行 credential_ref（V1 未开放账号删除 API，
// 届时由 AccountRepo 清字段；凭据失效 🔴 不调用本方法，保留便于排查，见详细设计 §5）。
func (v *MapCredentialVault) Delete(_ context.Context, ref string) error {
	if ref != "" {
		v.log.Warnf("[map-vault] Delete 调用（V1 无独立密文存储，实际清理由账号行字段完成）refPrefix=%.12s…", ref)
	}
	return nil
}
