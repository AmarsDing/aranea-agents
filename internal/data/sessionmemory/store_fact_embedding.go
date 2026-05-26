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

// MarkFactIndexStale marks a fact's external vector index as stale after a sync failure (MEM-OPT-01 Phase 1).
// Increments attempt counter and stores the last error message for observability.
func (st *Store) MarkFactIndexStale(ctx context.Context, factID, errMsg string) error {
	if st == nil || st.client == nil || strings.TrimSpace(factID) == "" {
		return nil
	}
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	_, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET
 index_status = 'stale', index_last_error = ?, index_attempts = index_attempts + 1
WHERE id = ? AND deleted_at = ''`,
		errMsg, strings.TrimSpace(factID))
	return err
}

// MarkFactIndexSynced marks a fact's external vector index as fresh after a successful sync (MEM-OPT-01 Phase 1).
func (st *Store) MarkFactIndexSynced(ctx context.Context, factID string) error {
	if st == nil || st.client == nil || strings.TrimSpace(factID) == "" {
		return nil
	}
	now := time.Now().UnixMilli()
	_, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET
 index_status = 'fresh', index_synced_at = ?, index_last_error = '', index_attempts = 0
WHERE id = ? AND deleted_at = ''`,
		now, strings.TrimSpace(factID))
	return err
}
