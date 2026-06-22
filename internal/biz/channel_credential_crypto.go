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
		return "", apierror.Internal(apierror.DomainChannel, err.Error())
	}
	if len(key) != 32 {
		return "", apierror.Internal(apierror.DomainChannel, credentialKeyRequiredMsg)
	}
	enc, err := c.encryptCredential(ctx, plain)
	if err != nil {
		return "", err
	}
	if enc == "" {
		return "", apierror.Internal(apierror.DomainChannel, credentialKeyRequiredMsg)
	}
	return channelSecretRefPrefix + enc, nil
}

func (c *CredentialCrypto) DecryptChannelSecretRef(ctx context.Context, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", nil
	}
	if !strings.HasPrefix(ref, channelSecretRefPrefix) {
		return "", apierror.BadRequest(apierror.DomainChannel, "unsupported channel secret_ref")
	}
	plain, err := c.decryptCredential(ctx, strings.TrimPrefix(ref, channelSecretRefPrefix))
	if err != nil {
		return "", apierror.Wrap(err, apierror.CodeInternal, apierror.DomainChannel)
	}
	if plain == "" {
		return "", apierror.BadRequest(apierror.DomainChannel, "channel credential could not be decrypted")
	}
	return plain, nil
}
