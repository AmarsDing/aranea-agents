package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"aranea-agents/internal/biz"
)

// CredentialsRevision hashes credential keys and revision fields for runtime fingerprinting.
func CredentialsRevision(creds []biz.ChannelCredential) string {
	if len(creds) == 0 {
		return ""
	}
	parts := make([]string, 0, len(creds))
	for _, c := range creds {
		parts = append(parts, strings.Join([]string{
			strings.ToLower(strings.TrimSpace(c.CredentialKey)),
			strings.TrimSpace(c.UpdatedAt),
			strings.TrimSpace(c.SecretRef),
			strings.TrimSpace(c.Status),
		}, ":"))
	}
	sort.Strings(parts)
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return hex.EncodeToString(sum[:8])
}
