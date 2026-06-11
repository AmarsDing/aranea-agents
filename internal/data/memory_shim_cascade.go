package data

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type cascadeRepo struct {
	data *Data
}

var (
	_ biz.CascadeProposalStore = (*cascadeRepo)(nil)
	_ biz.CascadeGraphReader   = (*cascadeRepo)(nil)
	_ biz.CascadeFactMutator   = (*cascadeRepo)(nil)
	_ biz.CascadeSagaStore     = (*cascadeRepo)(nil)
)

func NewCascadeRepo(data *Data) *cascadeRepo {
	if data == nil {
		return nil
	}
	return &cascadeRepo{data: data}
}

// --- CascadeProposalStore ---

func (r *cascadeRepo) InsertCascadeProposal(ctx context.Context, in biz.CascadeProposalInsert) ([]byte, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	id := newUUIDString()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	meta := strings.TrimSpace(in.MetadataJSON)
	if meta == "" {
		meta = "{}"
	}
	affected := strings.TrimSpace(in.AffectedJSON)
	if affected == "" {
		affected = "[]"
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_cascade_proposals (
		id, agent_id, workspace_id, trigger_entity_id, trigger_entity_name, trigger_attribute,
		old_value, new_value, affected_json, status, risk_level, rationale, metadata_json,
		reviewed_by, reviewed_at, expires_at, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id,
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.WorkspaceID),
		strings.TrimSpace(in.TriggerEntityID),
		strings.TrimSpace(in.TriggerEntityName),
		strings.TrimSpace(in.TriggerAttribute),
		strings.TrimSpace(in.OldValue),
		strings.TrimSpace(in.NewValue),
		affected, "pending",
		strings.TrimSpace(in.RiskLevel),
		strings.TrimSpace(in.Rationale),
		meta, "", "",
		strings.TrimSpace(in.ExpiresAt), now, now,
	)
	if err != nil {
		return nil, err
	}
	return r.GetCascadeProposalRow(ctx, id)
}

func (r *cascadeRepo) ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	lim := int(limit)
	if lim <= 0 {
		lim = 20
	}
	clauses := []string{}
	args := []any{}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	}
	where := ""
	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}
	q := cascadeProposalSelect + where + ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, lim)
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanCascadeProposalJSON(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *cascadeRepo) GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, cascadeProposalSelect+` WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound("MEMORY", "cascade proposal not found: %s", id)
	}
	return scanCascadeProposalJSON(rows)
}

func (r *cascadeRepo) UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Merge review note into metadata
	var meta string
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), `SELECT metadata_json FROM memory_cascade_proposals WHERE id = ?`, []any{id}, &meta); err != nil {
		return nil, err
	}
	lg := r.data.lg
	meta = mergeCascadeReviewNote(meta, status, reviewNote, lg)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_cascade_proposals SET status = ?, reviewed_by = ?, reviewed_at = ?, metadata_json = ?, updated_at = ? WHERE id = ?`,
		status, reviewedBy, now, meta, now, id,
	)
	if err != nil {
		return nil, err
	}
	return r.GetCascadeProposalRow(ctx, id)
}

func (r *cascadeRepo) CompareAndSwapProposalStatus(ctx context.Context, id string, fromStatuses []string, toStatus, reviewedBy, reviewNote string) ([]byte, bool, error) {
	if r == nil {
		return nil, false, biz.ErrCascadeUnavailable
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Merge review note into metadata
	var meta string
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx), `SELECT metadata_json FROM memory_cascade_proposals WHERE id = ?`, []any{id}, &meta); err != nil {
		return nil, false, err
	}
	lg := r.data.lg
	meta = mergeCascadeReviewNote(meta, toStatus, reviewNote, lg)
	// Build WHERE clause with status IN (fromStatuses) for atomic CAS.
	placeholders := make([]string, len(fromStatuses))
	args := make([]any, 0, len(fromStatuses)+5)
	for i, s := range fromStatuses {
		placeholders[i] = "?"
		args = append(args, s)
	}
	args = append(args, toStatus, reviewedBy, now, meta, now, id)
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		fmt.Sprintf(`UPDATE memory_cascade_proposals SET status = ?, reviewed_by = ?, reviewed_at = ?, metadata_json = ?, updated_at = ? WHERE id = ? AND status IN (%s)`,
			strings.Join(placeholders, ",")),
		args...,
	)
	if err != nil {
		return nil, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// CAS failed: status was already changed by another process.
		current, err := r.GetCascadeProposalRow(ctx, id)
		return current, false, err
	}
	current, err := r.GetCascadeProposalRow(ctx, id)
	return current, true, err
}

// --- CascadeGraphReader ---

func (r *cascadeRepo) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	// Delegate to l4EntityRepo's NeighborhoodJSON
	l4 := newL4EntityRepo(r.data)
	return l4.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (r *cascadeRepo) GetEntityRow(ctx context.Context, id string) ([]byte, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		"SELECT"+sqlEntityCols+" FROM memory_entities WHERE id = ? AND status = 'active' AND deleted_at = ''", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lg := r.data.lg
	if !rows.Next() {
		return nil, apierror.NotFound("MEMORY", "entity not found: %s", id)
	}
	return scanEntityRowJSON(rows, lg)
}

// --- CascadeFactMutator ---

func (r *cascadeRepo) ReplaceNameInAgentFacts(ctx context.Context, agentID, oldName, newName string) ([][]byte, int, error) {
	if r == nil {
		return nil, 0, biz.ErrCascadeUnavailable
	}
	re, err := nameReplacePattern(oldName)
	if err != nil {
		return nil, 0, fmt.Errorf("invalid old name pattern: %w", err)
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		sqlFactSelect+` WHERE agent_id = ? AND status = 'active' AND deleted_at = ''`, agentID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	// Phase 1: Collect all matching facts (CS-B10: no per-row writes in loop).
	type factUpdate struct {
		ID       string
		NewStmt  string
		NewMeta  string
	}
	var updates []factUpdate
	now := time.Now().UTC().Format(time.RFC3339Nano)
	lg := r.data.lg
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if err := json.Unmarshal(b, &row); err != nil {
			lg.Warn("ReplaceNameInAgentFacts: unmarshal failed",
				loggateway.StepID("memory.cascade_fact_unmarshal_fail"),
				loggateway.Err(err))
			continue
		}
		stmt, _ := row["statement"].(string)
		if !re.MatchString(stmt) {
			continue
		}
		newStmt := replaceNameWordBoundary(stmt, re, newName)
		id, _ := row["id"].(string)
		meta, _ := row["metadata_json"].(string)
		newMeta := mergeCascadeFactMeta(meta, oldName, newName, lg)
		updates = append(updates, factUpdate{ID: id, NewStmt: newStmt, NewMeta: newMeta})
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	if len(updates) == 0 {
		return nil, 0, nil
	}

	// Phase 2: Batch UPDATE all matching facts in a single transaction.
	writeDB := r.data.RWDB().WriteHandle()
	tx, err := writeDB.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY")
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	for _, u := range updates {
		if _, err := tx.ExecContext(ctx,
			`UPDATE memory_facts SET statement = ?, statement_normalized = ?, metadata_json = ?, updated_at = ? WHERE id = ?`,
			u.NewStmt, strings.ToLower(u.NewStmt), u.NewMeta, now, u.ID,
		); err != nil {
			return nil, 0, entErrToBizErr(err, "MEMORY")
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, 0, entErrToBizErr(err, "MEMORY")
	}
	committed = true

	// Phase 3: Batch read-back updated rows.
	ids := make([]any, len(updates))
	for i, u := range updates {
		ids[i] = u.ID
	}
	placeholders := make([]string, len(ids))
	for i := range placeholders {
		placeholders[i] = "?"
	}
	readRows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		fmt.Sprintf("%s WHERE id IN (%s)", sqlFactSelect, strings.Join(placeholders, ",")),
		ids...)
	if err != nil {
		return nil, len(updates), nil
	}
	defer readRows.Close()
	var updated [][]byte
	for readRows.Next() {
		if b, err := scanFactRowJSON(readRows); err == nil {
			updated = append(updated, b)
		}
	}
	return updated, len(updated), nil
}

func (r *cascadeRepo) SaveCascadeOriginalStatements(ctx context.Context, agentID, oldName string, factIDs []string) error {
	if r == nil {
		return biz.ErrCascadeUnavailable
	}
	if len(factIDs) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	placeholders := make([]string, len(factIDs))
	args := make([]any, 0, len(factIDs)+2)
	args = append(args, agentID, now)
	for i, id := range factIDs {
		placeholders[i] = "?"
		args = append(args, id)
	}
	q := fmt.Sprintf(`UPDATE memory_facts SET metadata_json = json_set(COALESCE(metadata_json,'{}'), '$.cascade_original_statement', statement, '$.cascade_original_name', ?, '$.cascade_saved_at', ?) WHERE id IN (%s)`,
		strings.Join(placeholders, ","))
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, q, args...)
	return err
}

func (r *cascadeRepo) RevertCascadeFactStatements(ctx context.Context, agentID string) (int, error) {
	if r == nil {
		return 0, biz.ErrCascadeUnavailable
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	// Find facts with cascade_original_statement in metadata
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		sqlFactSelect+` WHERE agent_id = ? AND status = 'active' AND deleted_at = '' AND metadata_json LIKE '%cascade_original_statement%'`, agentID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	// Phase 1: Collect all facts to revert (CS-B10: no per-row writes in loop).
	type revertItem struct {
		ID       string
		OrigStmt string
	}
	var items []revertItem
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(b, &row) != nil {
			continue
		}
		id, _ := row["id"].(string)
		meta, _ := row["metadata_json"].(string)
		metaMap := decodeJSONObject(meta, r.data.lg)
		origStmt := anyStr(metaMap["cascade_original_statement"])
		if origStmt == "" || id == "" {
			continue
		}
		items = append(items, revertItem{ID: id, OrigStmt: origStmt})
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if len(items) == 0 {
		return 0, nil
	}

	// Phase 2: Batch UPDATE in a single transaction.
	writeDB := r.data.RWDB().WriteHandle()
	tx, err := writeDB.BeginTx(ctx, nil)
	if err != nil {
		return 0, entErrToBizErr(err, "MEMORY")
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	var reverted int
	for _, item := range items {
		if _, err := tx.ExecContext(ctx,
			`UPDATE memory_facts SET statement = ?, statement_normalized = ?, metadata_json = json_remove(metadata_json, '$.cascade_original_statement', '$.cascade_original_name', '$.cascade_saved_at'), updated_at = ? WHERE id = ?`,
			item.OrigStmt, strings.ToLower(item.OrigStmt), now, item.ID,
		); err != nil {
			return 0, entErrToBizErr(err, "MEMORY")
		}
		reverted++
	}
	if err := tx.Commit(); err != nil {
		return 0, entErrToBizErr(err, "MEMORY")
	}
	committed = true
	return reverted, nil
}

func (r *cascadeRepo) ListCascadeFactDiffs(ctx context.Context, agentID, oldName, newName string, limit int) ([]map[string]any, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	re, err := nameReplacePattern(oldName)
	if err != nil {
		return nil, fmt.Errorf("invalid old name pattern: %w", err)
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		sqlFactSelect+` WHERE agent_id = ? AND status = 'active' AND deleted_at = ''`, agentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var diffs []map[string]any
	for rows.Next() {
		b, err := scanFactRowJSON(rows)
		if err != nil {
			continue
		}
		var row map[string]any
		if json.Unmarshal(b, &row) != nil {
			continue
		}
		stmt, _ := row["statement"].(string)
		if !re.MatchString(stmt) {
			continue
		}
		newStmt := replaceNameWordBoundary(stmt, re, newName)
		scopeType, _ := row["scope_type"].(string)
		scopeID, _ := row["scope_id"].(string)
		scope := scopeType
		if scopeID != "" {
			scope = scopeType + ":" + scopeID
		}
		diffs = append(diffs, map[string]any{
			"fact_id":          row["id"],
			"before_statement": stmt,
			"after_statement":  newStmt,
			"scope":            scope,
		})
		if len(diffs) >= limit {
			break
		}
	}
	return diffs, nil
}

func (r *cascadeRepo) MarkFactsIndexStaleByAgent(ctx context.Context, agentID string) (int64, error) {
	if r == nil {
		return 0, biz.ErrCascadeUnavailable
	}
	res, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET embedding_status = 'stale' WHERE agent_id = ? AND status = 'active' AND deleted_at = ''`, agentID)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// --- CascadeSagaStore ---

func (r *cascadeRepo) InitCascadeSagaSteps(ctx context.Context, proposalID string, steps []biz.CascadeSagaStep) error {
	if r == nil {
		return biz.ErrCascadeUnavailable
	}
	if len(steps) == 0 {
		return nil
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Batch INSERT in a single transaction (CS-B10).
	writeDB := r.data.RWDB().WriteHandle()
	tx, err := writeDB.BeginTx(ctx, nil)
	if err != nil {
		return entErrToBizErr(err, "MEMORY")
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()
	for i, s := range steps {
		id := newUUIDString()
		payload := strings.TrimSpace(s.PayloadJSON)
		if payload == "" {
			payload = "{}"
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO memory_cascade_saga_steps (
			id, proposal_id, step_index, step_name, state, is_critical, attempts,
			started_at, finished_at, payload_json, result_json, error, created_at
		) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			id, proposalID, i, s.StepName, s.State, memBoolToInt(s.IsCritical), s.Attempts,
			s.StartedAt, s.FinishedAt, payload, "", "", now,
		); err != nil {
			return entErrToBizErr(err, "MEMORY")
		}
	}
	if err := tx.Commit(); err != nil {
		return entErrToBizErr(err, "MEMORY")
	}
	committed = true
	return nil
}

func (r *cascadeRepo) GetCascadeSagaSteps(ctx context.Context, proposalID string) ([]biz.CascadeSagaStep, error) {
	if r == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, proposal_id, step_index, step_name, state, is_critical, attempts, started_at, finished_at, payload_json, result_json, error FROM memory_cascade_saga_steps WHERE proposal_id = ? ORDER BY step_index ASC`, proposalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var steps []biz.CascadeSagaStep
	for rows.Next() {
		var s biz.CascadeSagaStep
		var isCrit int
		if err := rows.Scan(&s.ID, &s.ProposalID, &s.StepIndex, &s.StepName, &s.State, &isCrit, &s.Attempts, &s.StartedAt, &s.FinishedAt, &s.PayloadJSON, &s.ResultJSON, &s.Error); err != nil {
			continue
		}
		s.IsCritical = isCrit != 0
		steps = append(steps, s)
	}
	return steps, rows.Err()
}

func (r *cascadeRepo) UpdateSagaStepState(ctx context.Context, stepID string, state, errMsg string) error {
	if r == nil {
		return biz.ErrCascadeUnavailable
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if state == "running" {
		_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			`UPDATE memory_cascade_saga_steps SET state = ?, started_at = ?, attempts = attempts + 1, error = ? WHERE id = ?`,
			state, now, errMsg, stepID)
		return err
	}
	if state == "done" || state == "failed" {
		_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
			`UPDATE memory_cascade_saga_steps SET state = ?, finished_at = ?, error = ? WHERE id = ?`,
			state, now, errMsg, stepID)
		return err
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_cascade_saga_steps SET state = ?, error = ? WHERE id = ?`,
		state, errMsg, stepID)
	return err
}

func (r *cascadeRepo) UpdateSagaStepResult(ctx context.Context, stepID string, resultJSON string) error {
	if r == nil {
		return biz.ErrCascadeUnavailable
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_cascade_saga_steps SET result_json = ? WHERE id = ?`, resultJSON, stepID)
	return err
}

func (r *cascadeRepo) HasCascadeSaga(ctx context.Context, proposalID string) (bool, error) {
	if r == nil {
		return false, biz.ErrCascadeUnavailable
	}
	var count int
	if err := queryRowScan(ctx, r.data.RWDB().ReadDB(ctx),
		`SELECT COUNT(*) FROM memory_cascade_saga_steps WHERE proposal_id = ?`, []any{proposalID}, &count); err != nil {
		return false, err
	}
	return count > 0, nil
}

// ensure loggateway and regexp are referenced
var _ loggateway.Logger
var _ = (*regexp.Regexp)(nil)
