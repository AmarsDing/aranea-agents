package biz

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type CredentialKeyResolver func(ctx context.Context) ([]byte, error)

type SystemSettingCredentialKeyRepo interface {
	EnsureCredentialEncryptionKey(ctx context.Context) (string, error)
}

const invalidCredentialKeyMsg = "ARANEA_CREDENTIAL_KEY must be 32-byte hex or base64"

type CredentialCrypto struct {
	resolver CredentialKeyResolver
	lg       loggateway.Logger
}

func NewCredentialCrypto(resolver CredentialKeyResolver, lg loggateway.Logger) *CredentialCrypto {
	return &CredentialCrypto{resolver: resolver, lg: lg}
}

func (c *CredentialCrypto) IsAvailable() bool {
	if c.resolver != nil {
		return true
	}
	return strings.TrimSpace(os.Getenv(envCredentialKey)) != ""
}

// RequireKeyForSecret G-38 ⑤ 铁律链（④ RequireKeyForSecret 签名化 graft）：仅校验
// aesKey 可解析 32B，作为 UpsertCredential 前置硬错——无 key 拒写、不静默明文
// 降级（Sourcesync 凭据行表唯一加密通道；env: 备选 scheme 不经此闸）。
func (c *CredentialCrypto) RequireKeyForSecret(ctx context.Context) error {
	key, err := c.aesKey(ctx)
	if err != nil {
		return err
	}
	if len(key) != 32 {
		return apierror.BadRequest("CREDENTIAL_KEY", "credential encryption key unavailable or invalid (ARANEA_CREDENTIAL_KEY must be 32-byte hex or base64)")
	}
	return nil
}

func (c *CredentialCrypto) aesKey(ctx context.Context) ([]byte, error) {
	if c.resolver != nil {
		return c.resolver(ctx)
	}
	raw := strings.TrimSpace(os.Getenv(envCredentialKey))
	if raw == "" {
		return nil, nil
	}
	return parseCredentialKeyMaterial(raw)
}

func ResolveCredentialAESKey(ctx context.Context, sys SystemSettingCredentialKeyRepo) ([]byte, error) {
	if raw := strings.TrimSpace(os.Getenv(envCredentialKey)); raw != "" {
		key, err := parseCredentialKeyMaterial(raw)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, apierror.BadRequest("CREDENTIAL_KEY", invalidCredentialKeyMsg)
		}
		return key, nil
	}
	if sys == nil {
		return nil, nil
	}
	hexKey, err := sys.EnsureCredentialEncryptionKey(ctx)
	if err != nil {
		return nil, err
	}
	key, err := parseCredentialKeyMaterial(hexKey)
	if err != nil {
		return nil, apierror.Internal("CREDENTIAL_KEY", "stored credential_encryption_key is invalid")
	}
	if len(key) != 32 {
		return nil, apierror.Internal("CREDENTIAL_KEY", "stored credential_encryption_key is invalid")
	}
	return key, nil
}

func parseCredentialKeyMaterial(raw string) ([]byte, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	if key, err := hex.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	if key, err := base64.StdEncoding.DecodeString(raw); err == nil && len(key) == 32 {
		return key, nil
	}
	return nil, apierror.BadRequest("CREDENTIAL_KEY", invalidCredentialKeyMsg)
}
