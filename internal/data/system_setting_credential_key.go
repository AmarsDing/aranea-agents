package data

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/apierror"
)

// ensureCredentialEncryptionKeyOnClient returns the platform AES key (hex), generating it once if empty.
// Uses a conditional UPDATE to avoid races when multiple goroutines initialize concurrently.
func ensureCredentialEncryptionKeyOnClient(ctx context.Context, c *ent.Client) (string, error) {
	if c == nil {
		return "", fmt.Errorf("ent client is nil")
	}
	row, err := c.SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		if ent.IsNotFound(err) {
			return "", apierror.NotFound(apierror.DomainData, "not found")
		}
		return "", err
	}
	if key := strings.TrimSpace(row.CredentialEncryptionKey); key != "" {
		return key, nil
	}
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	newKey := hex.EncodeToString(buf)
	res, err := c.ExecContext(ctx,
		`UPDATE system_settings SET credential_encryption_key = ? WHERE id = ? AND (credential_encryption_key = '' OR credential_encryption_key IS NULL)`,
		newKey, systemSettingSingletonID,
	)
	if err != nil {
		return "", err
	}
	affected, _ := res.RowsAffected()
	if affected > 0 {
		return newKey, nil
	}
	row, err = c.SystemSetting.Get(ctx, systemSettingSingletonID)
	if err != nil {
		return "", err
	}
	key := strings.TrimSpace(row.CredentialEncryptionKey)
	if key == "" {
		return "", fmt.Errorf("credential_encryption_key: failed to persist generated key")
	}
	return key, nil
}
