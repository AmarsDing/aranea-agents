package biz

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"os"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// CredentialKeyResolver loads the 32-byte AES key for credential encryption.
type CredentialKeyResolver func(ctx context.Context) ([]byte, error)

var credentialKeyResolver CredentialKeyResolver

// SetCredentialKeyResolver wires DB/env credential key resolution (called from data.NewSystemSettingRepo).
func SetCredentialKeyResolver(r CredentialKeyResolver) {
	credentialKeyResolver = r
}

// SystemSettingCredentialKeyRepo is the subset of SystemSettingRepo used for credential keys.
type SystemSettingCredentialKeyRepo interface {
	EnsureCredentialEncryptionKey(ctx context.Context) (string, error)
}

const invalidCredentialKeyMsg = "ARANEA_CREDENTIAL_KEY must be 32-byte hex or base64"

// ResolveCredentialAESKey prefers ARANEA_CREDENTIAL_KEY env, else system_settings.credential_encryption_key.
func ResolveCredentialAESKey(ctx context.Context, sys SystemSettingCredentialKeyRepo) ([]byte, error) {
	if raw := strings.TrimSpace(os.Getenv(envCredentialKey)); raw != "" {
		key, err := parseCredentialKeyMaterial(raw)
		if err != nil {
			return nil, err
		}
		if len(key) != 32 {
			return nil, errors.BadRequest("CREDENTIAL_KEY", invalidCredentialKeyMsg)
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
		return nil, errors.InternalServer("CREDENTIAL_KEY", "stored credential_encryption_key is invalid")
	}
	if len(key) != 32 {
		return nil, errors.InternalServer("CREDENTIAL_KEY", "stored credential_encryption_key is invalid")
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
	return nil, errors.BadRequest("CREDENTIAL_KEY", invalidCredentialKeyMsg)
}

func credentialAESKey(ctx context.Context) ([]byte, error) {
	if credentialKeyResolver != nil {
		return credentialKeyResolver(ctx)
	}
	raw := strings.TrimSpace(os.Getenv(envCredentialKey))
	if raw == "" {
		return nil, nil
	}
	return parseCredentialKeyMaterial(raw)
}
