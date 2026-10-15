package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

// ReviewFactRow applies a single-fact user governance action via a precise
// column-targeted UPDATE (memory.md §9.4). Unlike UpsertFactRow it never
// touches links/keywords/metadata/quality_score, so feedback actions cannot
// silently wipe A-MEM graph linkages.
//
// Action semantics (L3.md acceptance: confirm +0.10 / reject -0.20, clamped
// to [0,1]):
//   - confirm:   positive_feedback_count+1, confidence bump
//   - reject:    negative_feedback_count+1, confidence drop
//   - archive:   status='archived' (forget)
//   - dispute:   status='disputed'   (conflict governance quarantine)
//   - deprecate: status='deprecated' (conflict arbitration loser)
//   - refine:    statement/details/kind/tags replace, version+1, fingerprint
//     recomputed, PII gate re-run, embedding marked stale
//
// Refine recomputes the fingerprint from the new statement: keeping the stale
// fingerprint would let the next auto-extraction of the OLD text merge via
// ON CONFLICT and silently revert the user's edit.
func (r *l3FactRepo) ReviewFactRow(ctx context.Context, in biz.FactReview) ([]byte, error) {
	factID := strings.TrimSpace(in.FactID)
	if factID == "" {
		return nil, apierror.BadRequest("MEMORY", "fact_id is required")
	}
	action := strings.TrimSpace(in.Action)
	now := time.Now().UTC().Format(time.RFC3339Nano)
	d := r.data.Dialect()

	var updateSQL string
	var args []any
	switch action {
	case biz.FactReviewConfirm:
		updateSQL = `UPDATE memory_facts SET positive_feedback_count = positive_feedback_count + 1, confidence = ` +
			d.Least("1.0", d.Greatest("0.0", "confidence + 0.10")) +
			`, updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewReject:
		updateSQL = `UPDATE memory_facts SET negative_feedback_count = negative_feedback_count + 1, confidence = ` +
			d.Least("1.0", d.Greatest("0.0", "confidence - 0.20")) +
			`, updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewArchive:
		updateSQL = `UPDATE memory_facts SET status = 'archived', archived_at = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, now, factID}
	case biz.FactReviewDispute:
		updateSQL = `UPDATE memory_facts SET status = 'disputed', updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewDeprecate:
		updateSQL = `UPDATE memory_facts SET status = 'deprecated', updated_at = ? WHERE id = ? AND deleted_at = ''`
		args = []any{now, factID}
	case biz.FactReviewRefine:

	default:
		return nil, apierror.BadRequest("MEMORY", "unknown fact review action: "+action)
	}

	var result []byte
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		if action == biz.FactReviewRefine {
			sql_, args_, buildErr := r.buildFactRefineUpdate(txCtx, in, factID, now)
			if buildErr != nil {
				return buildErr
			}
			updateSQL, args = sql_, args_
		}
		res, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx, d.RenumberPlaceholders(updateSQL), args...)
		if execErr != nil {
			return execErr
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return apierror.NotFound("MEMORY", "fact not found or deleted")
		}
		rows, queryErr := r.data.RWDB().ReadDB(txCtx).QueryContext(txCtx,
			d.RenumberPlaceholders(sqlFactSelect+` WHERE id = ?`), factID)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		if !rows.Next() {
			if err := rows.Err(); err != nil {
				return err
			}
			return apierror.NotFound("MEMORY", "fact row not found after review")
		}
		var scanErr error
		result, scanErr = scanFactRowJSON(rows)
		return scanErr
	})
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	return result, nil
}

// buildFactRefineUpdate constructs the UPDATE for the refine action. It reads
// the fact's scope inside the transaction so the fingerprint can be recomputed
// from the refined statement (matching UpsertFactRow's gate-then-fingerprint
// order). A fingerprint collision with an existing fact surfaces as
// CodeConflict via entErrToBizErr, which is the desired duplicate guard.
func (r *l3FactRepo) buildFactRefineUpdate(ctx context.Context, in biz.FactReview, factID, now string) (string, []any, error) {
	var scopeType, scopeID string
	err := QueryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		r.data.Dialect().RenumberPlaceholders(`SELECT scope_type, scope_id FROM memory_facts WHERE id = ? AND deleted_at = ''`),
		[]any{factID}, &scopeType, &scopeID)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil, apierror.NotFound("MEMORY", "fact not found or deleted")
		}
		return "", nil, err
	}
	statement, details, pii, redacted, piiTypesJSON := applyFactPIIGate(biz.FactUpsert{
		Statement:       in.Statement,
		DetailsMarkdown: in.DetailsMarkdown,
	})
	if statement == "" {
		return "", nil, apierror.BadRequest("MEMORY", "statement is required for refine")
	}
	fp := biz.FactFingerprint(statement, scopeType, scopeID)
	tags := strings.TrimSpace(in.TagsJSON)
	if tags == "" {
		tags = "[]"
	}
	updateSQL := `UPDATE memory_facts SET statement = ?, statement_normalized = ?, fingerprint = ?, details_markdown = ?, fact_kind = ?, tags_json = ?, version = version + 1, embedding_status = 'stale', pii_flag = ?, redacted_statement = ?, pii_types = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`
	args := []any{statement, strings.ToLower(statement), fp, details, strings.TrimSpace(in.FactKind), tags, pii, redacted, piiTypesJSON, now, factID}
	return updateSQL, args, nil
}

func (r *l3FactRepo) IncrementConflictCount(ctx context.Context, factID string) (int32, error) {
	var count int32
	err := r.data.ExecInTx(ctx, func(txCtx context.Context) error {
		_, execErr := r.data.RWDB().WriteDB(txCtx).ExecContext(txCtx,
			r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET conflict_count = conflict_count + 1, updated_at = ? WHERE id = ?`),
			time.Now().UTC().Format(time.RFC3339Nano), factID)
		if execErr != nil {
			return execErr
		}
		return queryRowScan(txCtx, r.data.RWDB().ReadDB(txCtx),
			r.data.Dialect().RenumberPlaceholders(`SELECT conflict_count FROM memory_facts WHERE id = ?`), []any{factID}, &count)
	})
	return count, entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) BatchIncrementConflictCounts(ctx context.Context, factIDs []string) error {
	if len(factIDs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(factIDs))
	args := make([]any, 0, len(factIDs)+1)
	args = append(args, now)
	for i, id := range factIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`UPDATE memory_facts SET conflict_count = conflict_count + 1, updated_at = ? WHERE id IN (%s)`, strings.Join(placeholders, ","))
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) ListConflictingFacts(ctx context.Context, scopeType, scopeID, agentID string, limit, offset int32) ([][]byte, int32, error) {
	clauses := []string{"conflict_count > 0", "status = 'active'", "deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}

	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY conflict_count DESC, updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, total, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

// SupersedeFact marks oldID as superseded by newID so the old statement no
// longer participates in recall or conflict arbitration.
func (r *l3FactRepo) SupersedeFact(ctx context.Context, oldID, newID string) error {
	if oldID == "" || newID == "" || oldID == newID {
		return nil
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET status = 'superseded', superseded_by = ?, updated_at = ? WHERE id = ? AND status = 'active'`),
		newID, time.Now().UTC().Format(time.RFC3339Nano), oldID)
	return entErrToBizErr(err, "MEMORY_L3")
}

// ListActivePreferenceFacts returns active facts of the given kinds within the
// user+agent scopes, ordered by importance DESC, updated_at DESC (FR-M3 pinned
// preference injection).
func (r *l3FactRepo) ListActivePreferenceFacts(ctx context.Context, agentID, userID string, kinds []string, limit int32) ([][]byte, error) {
	if len(kinds) == 0 {
		return nil, nil
	}
	kindPH := make([]string, len(kinds))
	args := make([]any, 0, len(kinds)+4)
	for i, k := range kinds {
		kindPH[i] = "?"
		args = append(args, k)
	}
	where := " WHERE status = 'active' AND deleted_at = '' AND valid_until = ''" +
		" AND fact_kind IN (" + strings.Join(kindPH, ",") + ")" +
		" AND ((scope_type = 'user' AND scope_id = ?) OR (scope_type = 'agent' AND scope_id = ?))"
	args = append(args, userID, agentID)
	lim := int(limit)
	if lim <= 0 {
		lim = 10
	}
	if lim > 50 {
		lim = 50
	}
	q := sqlFactSelect + where + ` ORDER BY importance DESC, updated_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func (r *l3FactRepo) ListPIIFlaggedFacts(ctx context.Context, scopeType, scopeID, agentID string, limit, offset int32) ([][]byte, int32, error) {
	clauses := []string{"pii_flag = 1", "status = 'active'", "deleted_at = ''"}
	args := []any{}
	if scopeType != "" {
		clauses = append(clauses, "scope_type = ?")
		args = append(args, scopeType)
	}
	if scopeID != "" {
		clauses = append(clauses, "scope_id = ?")
		args = append(args, scopeID)
	}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	var total int32
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), r.data.Dialect().RenumberPlaceholders("SELECT COUNT(*) FROM memory_facts"+where), args, &total); err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	off := int(offset)
	if off < 0 {
		off = 0
	}
	q := sqlFactSelect + where + ` ORDER BY updated_at DESC LIMIT ? OFFSET ?`
	args = append(args, lim, off)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, r.data.Dialect().RenumberPlaceholders(q), args...)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY_L3")
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			return nil, 0, entErrToBizErr(err, "MEMORY_L3")
		}
		out = append(out, b)
	}
	return out, total, entErrToBizErr(rows.Err(), "MEMORY_L3")
}

func (r *l3FactRepo) ApprovePIIFact(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET pii_flag = 0, statement = COALESCE(NULLIF(redacted_statement, ''), statement), redacted_statement = '', pii_types = '[]', updated_at = ? WHERE id = ?`),
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}

func (r *l3FactRepo) RejectPIIFact(ctx context.Context, factID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		r.data.Dialect().RenumberPlaceholders(`UPDATE memory_facts SET status = 'redacted', updated_at = ? WHERE id = ?`),
		time.Now().UTC().Format(time.RFC3339Nano), factID)
	return entErrToBizErr(err, "MEMORY_L3")
}
