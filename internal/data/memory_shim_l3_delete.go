package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/pkg/loggateway"
)

func (r *l3FactRepo) DeleteFactRow(ctx context.Context, factID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE id = ?`), now, factID)
	if err != nil {
		return entErrToBizErr(err, "MEMORY_L3")
	}

	if r.vectorStore != nil {
		if delErr := r.vectorStore.Delete(ctx, factID); delErr != nil {
			r.data.lg.Warn("cascade: delete fact vector failed, marking stale",
				loggateway.StepID("memory.l3.vector_delete"),
				loggateway.Str("fact_id", factID),
				loggateway.Err(delErr))

			if markErr := r.markFactEmbeddingStale(ctx, factID); markErr != nil {
				r.data.lg.Warn("cascade: mark fact embedding stale failed",
					loggateway.StepID("memory.l3.vector_mark_stale"),
					loggateway.Str("fact_id", factID),
					loggateway.Err(markErr))
			}
		}
	}
	return nil
}

func (r *l3FactRepo) DeleteFactRowsByIDs(ctx context.Context, factIDs []string) (int, error) {
	if len(factIDs) == 0 {
		return 0, nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(factIDs))
	args := make([]any, 0, len(factIDs)+1)
	args = append(args, now)
	for i, id := range factIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`UPDATE memory_facts SET deleted_at = ?, status = 'deleted' WHERE id IN (%s)`, strings.Join(placeholders, ","))
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return 0, entErrToBizErr(err, "MEMORY_L3")
	}
	n, _ := res.RowsAffected()

	if r.vectorStore != nil {
		for _, fid := range factIDs {
			if delErr := r.vectorStore.Delete(ctx, fid); delErr != nil {
				r.data.lg.Warn("cascade: delete fact vector failed, marking stale",
					loggateway.StepID("memory.l3.vector_delete_batch"),
					loggateway.Str("fact_id", fid),
					loggateway.Err(delErr))

				if markErr := r.markFactEmbeddingStale(ctx, fid); markErr != nil {
					r.data.lg.Warn("cascade: mark fact embedding stale failed",
						loggateway.StepID("memory.l3.vector_mark_stale"),
						loggateway.Str("fact_id", fid),
						loggateway.Err(markErr))
				}
			}
		}
	}
	return int(n), nil
}

func (r *l3FactRepo) ClearFactsByScope(ctx context.Context, scopeType, scopeID, userID string) ([]string, error) {
	// Read IDs + soft-delete in a single transaction to prevent new facts
	// from escaping cleanup (read-write consistency under separation).
	var factIDs []string
	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		rows, queryErr := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`SELECT id FROM memory_facts WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`),
			scopeType, scopeID, userID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if scanErr := rows.Scan(&id); scanErr != nil {
				return scanErr
			}
			factIDs = append(factIDs, id)
		}
		if err := rows.Err(); err != nil {
			return entErrToBizErr(err, "MEMORY_L3")
		}
		if len(factIDs) == 0 {
			return nil
		}
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET deleted_at = ?, status = 'deleted', valid_until = ? WHERE scope_type = ? AND scope_id = ? AND user_id = ? AND status = 'active' AND deleted_at = ''`),
			now, now, scopeType, scopeID, userID)
		return execErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	if len(factIDs) == 0 {
		return nil, nil
	}

	if r.vectorStore != nil {
		for _, fid := range factIDs {
			if delErr := r.vectorStore.Delete(ctx, fid); delErr != nil {
				r.data.lg.Warn("cascade: clear fact vector failed, marking stale",
					loggateway.StepID("memory.l3.vector_clear"),
					loggateway.Str("fact_id", fid),
					loggateway.Err(delErr))

				if markErr := r.markFactEmbeddingStale(ctx, fid); markErr != nil {
					r.data.lg.Warn("cascade: mark fact embedding stale failed",
						loggateway.StepID("memory.l3.vector_mark_stale"),
						loggateway.Str("fact_id", fid),
						loggateway.Err(markErr))
				}
			}
		}
	}
	return factIDs, nil
}
