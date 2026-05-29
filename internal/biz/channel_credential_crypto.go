package biz

import (
	"context"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

const channelSecretRefPrefix = "enc:"

func (c *CredentialCrypto) EncryptChannelSecretRef(ctx context.Context, plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	key, err := c.aesKey(ctx)
	if err != nil {
		return "", errors.InternalServer("CHANNEL", err.Error())
	}
	if len(key) != 32 {
		return "", errors.InternalServer("CHANNEL", credentialKeyRequiredMsg)
	}
	enc, err := c.encryptCredential(ctx, plain)
	if err != nil {
		return "", err
	}
	if enc == "" {
		return "", errors.InternalServer("CHANNEL", credentialKeyRequiredMsg)
	}
	return channelSecretRefPrefix + enc, nil
}

func (c *CredentialCrypto) DecryptChannelSecretRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.BadRequest("CHANNEL", "empty secret_ref")
	}
	if !strings.HasPrefix(ref, channelSecretRefPrefix) {
		return "", errors.BadRequest("CHANNEL", "unsupported channel secret_ref")
	}
	plain, err := c.decryptCredential(ctx, strings.TrimPrefix(ref, channelSecretRefPrefix))
	if err != nil {
		return "", err
	}
	if plain == "" {
		return "", errors.BadRequest("CHANNEL", "channel credential could not be decrypted")
	}
	return plain, nil
}
