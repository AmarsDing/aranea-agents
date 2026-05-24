package sessionmemory

import (
	"context"
	"strings"
	"time"
)

// UpsertFactEmbedding stores a recall index vector on one memory_facts row (symmetric to episode embedding).
func (st *Store) UpsertFactEmbedding(ctx context.Context, factID string, embedding []float32, model string, dim int) error {
	if st == nil || st.client == nil || strings.TrimSpace(factID) == "" || len(embedding) == 0 {
		return nil
	}
	blob := encodeFloat32Blob(embedding)
	norm := vectorL2Norm(embedding)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET
 embedding_status = 'ready', embedding_model = ?, embedding_dim = ?,
 embedding_blob = ?, embedding_norm = ?, updated_at = ?
WHERE id = ? AND deleted_at = ''`,
		strings.TrimSpace(model), dim, blob, norm, now, strings.TrimSpace(factID))
	return err
}
