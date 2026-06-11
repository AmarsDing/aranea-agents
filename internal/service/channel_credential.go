package service

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ChannelCredentialSecretRef returns secret_ref for a credential key (runtime / tests).
func ChannelCredentialSecretRef(creds []biz.ChannelCredential, key string) (string, error) {
	key = strings.TrimSpace(key)
	for _, c := range creds {
		if strings.EqualFold(strings.TrimSpace(c.CredentialKey), key) {
			ref := strings.TrimSpace(c.SecretRef)
			if ref == "" {
				return "", apierror.BadRequest("CHANNEL", "credential %s missing secret_ref", key)
			}
			return ref, nil
		}
	}
	return "", apierror.BadRequest("CHANNEL", "credential %s not configured", key)
}
