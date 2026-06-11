package biz

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

const channelSecretRefPrefix = "enc:"

func (c *CredentialCrypto) EncryptChannelSecretRef(ctx context.Context, plain string) (string, error) {
	plain = strings.TrimSpace(plain)
	if plain == "" {
		return "", nil
	}
	key, err := c.aesKey(ctx)
	if err != nil {
		return "", apierror.Internal("CHANNEL", err.Error())
	}
	if len(key) != 32 {
		return "", apierror.Internal("CHANNEL", credentialKeyRequiredMsg)
	}
	enc, err := c.encryptCredential(ctx, plain)
	if err != nil {
		return "", err
	}
	if enc == "" {
		return "", apierror.Internal("CHANNEL", credentialKeyRequiredMsg)
	}
	return channelSecretRefPrefix + enc, nil
}

func (c *CredentialCrypto) DecryptChannelSecretRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", apierror.BadRequest("CHANNEL", "empty secret_ref")
	}
	if !strings.HasPrefix(ref, channelSecretRefPrefix) {
		return "", apierror.BadRequest("CHANNEL", "unsupported channel secret_ref")
	}
	plain, err := c.decryptCredential(ctx, strings.TrimPrefix(ref, channelSecretRefPrefix))
	if err != nil {
		return "", err
	}
	if plain == "" {
		return "", apierror.BadRequest("CHANNEL", "channel credential could not be decrypted")
	}
	return plain, nil
}
