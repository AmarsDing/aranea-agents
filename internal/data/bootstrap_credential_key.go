package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

func ensureDefaultCredentialEncryptionKey(ctx context.Context, client *ent.Client, d Dialect) error {
	_, err := ensureCredentialEncryptionKeyOnClient(ctx, client, d)
	return err
}
