package data

import (
	"context"

	"aranea-agents/internal/data/ent"
)

func ensureDefaultCredentialEncryptionKey(ctx context.Context, client *ent.Client) error {
	_, err := ensureCredentialEncryptionKeyOnClient(ctx, client)
	return err
}
