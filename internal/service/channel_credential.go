package service

import (
	"strings"

	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ChannelCredentialSecretRef returns secret_ref for a credential key (runtime / tests).
func ChannelCredentialSecretRef(creds []biz.ChannelCredential, key string) (string, error) {
	key = strings.TrimSpace(key)
	for _, c := range creds {
		if strings.EqualFold(strings.TrimSpace(c.CredentialKey), key) {
			ref := strings.TrimSpace(c.SecretRef)
			if ref == "" {
				return "", kerrors.BadRequest("CHANNEL", "credential "+key+" missing secret_ref")
			}
			return ref, nil
		}
	}
	return "", kerrors.BadRequest("CHANNEL", "credential "+key+" not configured")
}
