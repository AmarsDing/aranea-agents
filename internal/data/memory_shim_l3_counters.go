package data

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// UpdateDecayScores batch-updates the persisted Ebbinghaus decay score (R_t)
// for the given fact IDs. The updates are wrapped in a single transaction so
// the batch is atomic (red line #24: cross-table writes must use ExecInTx).
//
// scores maps fact ID → R_t ∈ (0, 1]. A zero-length map is a no-op.
func (r *l3FactRepo) UpdateDecayScores(ctx context.Context, scores map[string]float64) error {
	if r == nil || r.data == nil || len(scores) == 0 {
		return nil
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for factID, score := range scores {
			factID = strings.TrimSpace(factID)
			if factID == "" {
				continue
			}
			_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET decay_score = ?, updated_at = ? WHERE id = ?`),
				score, now, factID)
			if execErr != nil {
				return entErrToBizErr(execErr, "MEMORY_L3")
			}
		}
		return nil
	})
}

// IncrementFactRecalledCount batch-increments recalled_count and updates
// last_used_at for the given fact IDs. Called by the scored recall adapter so
// the Ebbinghaus decay worker has accurate access-recency signals. The updates
// are wrapped in a single transaction to minimize write load on the recall
// path.
//
// FR-12.6: this is the "recalled" stage of the three-stage counters — the
// fact entered a recall result set (previously mis-labelled use_count).
//
// factIDs must be non-empty; an empty slice is a no-op.
func (r *l3FactRepo) IncrementFactRecalledCount(ctx context.Context, factIDs []string) error {
	return r.incrementFactCounter(ctx, factIDs, "recalled_count")
}

// IncrementFactInjectedCount batch-increments injected_count and updates
// last_used_at for the given fact IDs. Called by the before-model memory
// inject hook once per turn for the facts actually written into the prompt
// (FR-12.6: the "injected" stage — the only usage count shown to users).
func (r *l3FactRepo) IncrementFactInjectedCount(ctx context.Context, factIDs []string) error {
	return r.incrementFactCounter(ctx, factIDs, "injected_count")
}

// incrementFactCounter is the shared batch-increment helper for the
// recalled/injected stages. column is a compile-time constant from the two
// callers above — never user input (SQL injection safe by construction).
func (r *l3FactRepo) incrementFactCounter(ctx context.Context, factIDs []string, column string) error {
	if r == nil || r.data == nil || len(factIDs) == 0 {
		return nil
	}

	seen := make(map[string]struct{}, len(factIDs))
	ids := make([]string, 0, len(factIDs))
	for _, id := range factIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return nil
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		placeholders := make([]string, len(ids))
		args := make([]any, 0, len(ids)+1)
		args = append(args, now)
		for i, id := range ids {
			placeholders[i] = "?"
			args = append(args, id)
		}
		q := fmt.Sprintf(`UPDATE memory_facts SET %s = %s + 1, last_used_at = ? WHERE id IN (%s)`,
			column, column, strings.Join(placeholders, ","))
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(q), args...)
		return entErrToBizErr(execErr, "MEMORY_L3")
	})
}

// RecordFactCitations records (fact, turn) citations into the dedup ledger
// and increments cited_count only for pairs not seen before (FR-12.6: the
// "cited" stage). Idempotent: re-recording the same (fact_id, turn_id) pair
// is a no-op, so the backfill worker may re-scan overlapping windows freely.
// last_used_at is updated only for newly-cited facts.
func (r *l3FactRepo) RecordFactCitations(ctx context.Context, citations []biz.FactCitation) error {
	if r == nil || r.data == nil || len(citations) == 0 {
		return nil
	}
	insertSQL := `INSERT INTO memory_fact_citations (fact_id, turn_id, created_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`
	if !r.data.Dialect().IsPostgres() {
		insertSQL = `INSERT OR IGNORE INTO memory_fact_citations (fact_id, turn_id, created_at) VALUES (?, ?, ?)`
	}
	return r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		for _, c := range citations {
			factID := strings.TrimSpace(c.FactID)
			turnID := strings.TrimSpace(c.TurnID)
			if factID == "" || turnID == "" {
				continue
			}
			res, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(insertSQL), factID, turnID, now)
			if execErr != nil {
				return entErrToBizErr(execErr, "MEMORY_L3")
			}
			affected, _ := res.RowsAffected()
			if affected == 0 {
				continue
			}
			if _, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
				r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET cited_count = cited_count + 1, last_used_at = ? WHERE id = ?`),
				now, factID); execErr != nil {
				return entErrToBizErr(execErr, "MEMORY_L3")
			}
		}
		return nil
	})
}

func (r *l3FactRepo) listFactsBySourceSession(ctx context.Context, scopeType, scopeID, userID, sessionID string, limit int) ([][]byte, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 20
	}
	clauses := []string{"status = 'active'", "deleted_at = ''", "valid_until = ''", "source_session_id = ?"}
	args := []any{sessionID}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if userID != "" {
		clauses = append(clauses, "user_id = ?")
		args = append(args, userID)
	}
	q := sqlFactSelect + " WHERE " + strings.Join(clauses, " AND ") +
		` ORDER BY COALESCE(NULLIF(valid_from, ''), created_at) DESC LIMIT ?`
	args = append(args, limit)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, scanErr := scanFactRowJSON(rows)
		if scanErr != nil {
			return nil, entErrToBizErr(scanErr, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}
