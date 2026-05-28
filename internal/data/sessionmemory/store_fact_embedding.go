package sessionmemory

import (
	"context"
	"database/sql"
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

// FactConsistencyRow holds the minimal fields needed for read-path consistency
// validation when pgvector returns a hit (MEM-OPT-01 Phase 2).
type FactConsistencyRow struct {
	Status      string
	IndexStatus string
	Statement   string
}

// GetFactConsistencyRow returns status, index_status, and statement for a single fact.
// Returns ("", "", "") with nil error if the row does not exist or is soft-deleted.
func (st *Store) GetFactConsistencyRow(ctx context.Context, factID string) (FactConsistencyRow, error) {
	if st == nil || st.client == nil || strings.TrimSpace(factID) == "" {
		return FactConsistencyRow{}, nil
	}
	var row FactConsistencyRow
	err := queryOne(ctx, st.client,
		`SELECT status, index_status, statement FROM memory_facts WHERE id = ? AND deleted_at = ''`,
		[]any{strings.TrimSpace(factID)},
		&row.Status, &row.IndexStatus, &row.Statement)
	if err != nil {
		if err == sql.ErrNoRows {
			return FactConsistencyRow{}, nil
		}
		return FactConsistencyRow{}, err
	}
	return row, nil
}

// ListStaleIndexFacts returns fact rows where index_status='stale' and attempts < maxAttempts,
// ordered by index_synced_at ASC, limited to batchSize (MEM-OPT-01 Phase 3 reconciler).
func (st *Store) ListStaleIndexFacts(ctx context.Context, maxAttempts int, batchSize int) ([][]byte, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if batchSize <= 0 {
		batchSize = 200
	}
	rows, err := st.client.QueryContext(ctx, sqlFactSelect+`
 WHERE index_status = 'stale' AND index_attempts < ? AND deleted_at = ''
 ORDER BY index_synced_at ASC LIMIT ?`, maxAttempts, batchSize)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// CountFactsByIndexStatus returns counts of facts grouped by index_status (MEM-OPT-01 observability).
func (st *Store) CountFactsByIndexStatus(ctx context.Context) (fresh, stale, disabled int64, err error) {
	if st == nil || st.client == nil {
		return 0, 0, 0, nil
	}
	rows, err := st.client.QueryContext(ctx,
		`SELECT index_status, COUNT(*) FROM memory_facts WHERE deleted_at = '' GROUP BY index_status`)
	if err != nil {
		return 0, 0, 0, err
	}
	defer rows.Close()
	for rows.Next() {
		var status string
		var n int64
		if scanErr := rows.Scan(&status, &n); scanErr != nil {
			continue
		}
		switch status {
		case "fresh":
			fresh = n
		case "stale":
			stale = n
		case "disabled":
			disabled = n
		}
	}
	err = rows.Err()
	return
}

// MarkFactIndexDisabled marks a fact's external vector index as permanently disabled
// after max reconciliation attempts (MEM-OPT-01 Phase 3).
func (st *Store) MarkFactIndexDisabled(ctx context.Context, factID string) error {
	if st == nil || st.client == nil || strings.TrimSpace(factID) == "" {
		return nil
	}
	_, err := st.client.ExecContext(ctx, `
UPDATE memory_facts SET index_status = 'disabled' WHERE id = ? AND deleted_at = ''`,
		strings.TrimSpace(factID))
	return err
}

func (st *Store) GetFactRawRow(ctx context.Context, factID string) ([]byte, error) {
	if st == nil || st.client == nil || strings.TrimSpace(factID) == "" {
		return nil, nil
	}
	rows, err := st.client.QueryContext(ctx, sqlFactSelect+` WHERE id = ? AND deleted_at = '' LIMIT 1`,
		strings.TrimSpace(factID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanFactRowJSON(rows)
}
