package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

const channelSecretRefPrefix = "enc:"

// EncryptChannelSecretRef stores a channel credential as an AES-GCM blob prefixed with enc:.
func EncryptChannelSecretRef(ctx context.Context, plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	key, _ := credentialAESKey(ctx)
	if len(key) != 32 {
		return "", errors.BadRequest("CHANNEL", credentialKeyRequiredMsg)
	}
	enc, err := encryptCredential(ctx, plain)
	if err != nil {
		return "", err
	}
	if enc == "" {
		return "", errors.BadRequest("CHANNEL", credentialKeyRequiredMsg)
	}
	return channelSecretRefPrefix + enc, nil
}

// DecryptChannelSecretRef resolves enc: secret_ref values for runtime use.
func DecryptChannelSecretRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.BadRequest("CHANNEL", "empty secret_ref")
	}
	if !strings.HasPrefix(ref, channelSecretRefPrefix) {
		return "", errors.BadRequest("CHANNEL", "unsupported channel secret_ref")
	}
	plain, err := decryptCredential(ctx, strings.TrimPrefix(ref, channelSecretRefPrefix))
	if err != nil {
		return "", err
	}
	if plain == "" {
		return "", errors.BadRequest("CHANNEL", "channel credential could not be decrypted")
	}
	return plain, nil
}
