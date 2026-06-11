package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
)

// l1WorkingMemoryRepo implements biz.L1TaskWriter + biz.L1FieldWriter + biz.L1AdminReader + biz.L1IdleTaskReader using direct Raw SQL.
// Uses d.RWDB() for read-write separated raw SQL because L1 tables are not managed by Ent schema.
type l1WorkingMemoryRepo struct {
	data *Data
}

var _ biz.L1TaskWriter = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1FieldWriter = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1AdminReader = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1IdleTaskReader = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1SchemaReader = (*l1WorkingMemoryRepo)(nil)

const (
	l1SchemaVersion    = 1
	l1FieldEstimateMin = 1 // minimum token estimate for non-empty content
)

func newL1WorkingMemoryRepo(data *Data) *l1WorkingMemoryRepo {
	if data == nil {
		return nil
	}
	return &l1WorkingMemoryRepo{data: data}
}

// --- L1AdminReader ---

func (r *l1WorkingMemoryRepo) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	if sessionID == "" {
		return nil, apierror.BadRequest("MEMORY", "session id is required")
	}
	clauses := []string{"session_id = ?"}
	args := []any{sessionID}
	if agentID != "" {
		clauses = append(clauses, "agent_id = ?")
		args = append(args, agentID)
	}
	if status != "" {
		clauses = append(clauses, "status = ?")
		args = append(args, status)
	} else if strings.TrimSpace(includeEnded) != "true" {
		clauses = append(clauses, "status IN ('active','paused')")
	}
	where := " WHERE " + strings.Join(clauses, " AND ")
	q := sqlL1Task + where + ` ORDER BY updated_at DESC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanL1TaskRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *l1WorkingMemoryRepo) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool, requestingAgentID ...string) ([][]byte, error) {
	return r.listL1FieldRows(ctx, taskID, includeInternal, false, requestingAgentID...)
}

// listL1FieldRowsInternal includes expired fields (used for snapshots/archives).
func (r *l1WorkingMemoryRepo) listL1FieldRowsInternal(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	return r.listL1FieldRows(ctx, taskID, includeInternal, true)
}

func (r *l1WorkingMemoryRepo) listL1FieldRows(ctx context.Context, taskID string, includeInternal, includeExpired bool, requestingAgentID ...string) ([][]byte, error) {
	if taskID == "" {
		return nil, apierror.BadRequest("MEMORY", "task id is required")
	}
	q := sqlL1Field + ` WHERE task_id = ?`
	args := []any{taskID}
	if !includeInternal {
		q += ` AND visibility != 'internal'`
	}
	// Filter out expired fields unless explicitly included (e.g. for snapshots).
	if !includeExpired {
		nowUTC := time.Now().UTC().Format(time.RFC3339)
		q += ` AND (expires_at = '' OR expires_at > ?)`
		args = append(args, nowUTC)
	}
	// Filter by requesting agent: if specified, only return fields owned by that agent
	// or fields in tasks shared with that agent.
	if len(requestingAgentID) > 0 && requestingAgentID[0] != "" {
		q += ` AND (agent_id = ? OR agent_id = '' OR ? IN (SELECT value FROM json_each((SELECT shared_with_json FROM memory_l1_tasks WHERE id = ?))))`
		args = append(args, requestingAgentID[0], requestingAgentID[0], taskID)
	}
	q += ` ORDER BY field_path ASC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		b, err := scanL1FieldRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (r *l1WorkingMemoryRepo) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlL1Task+` WHERE id = ? AND session_id = ?`, id, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanL1TaskRow(rows)
}

func (r *l1WorkingMemoryRepo) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, sqlL1Field+` WHERE task_id = ? AND field_path = ?`, taskID, fieldPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanL1FieldRow(rows)
}

// --- L1TaskWriter ---

func (r *l1WorkingMemoryRepo) StartL1Task(ctx context.Context, in biz.L1TaskInsert) ([]byte, error) {
	sessID := strings.TrimSpace(in.SessionID)
	if sessID == "" {
		return nil, apierror.BadRequest("MEMORY", "session_id is required")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_l1_tasks (
		id, session_id, run_id, team_id, agent_id, task_key, task_title, task_goal, status,
		schema_version, budget_tokens, used_tokens, parent_task_id, shared_with_json,
		started_at, ended_at, archived_at, metadata_json, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(session_id, task_key, agent_id) DO UPDATE SET
		task_title = excluded.task_title, task_goal = excluded.task_goal,
		status = excluded.status, ended_at = '', updated_at = excluded.updated_at`,
		id, sessID,
		strings.TrimSpace(in.RunID),
		strings.TrimSpace(in.TeamID),
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.TaskKey),
		strings.TrimSpace(in.TaskTitle),
		strings.TrimSpace(in.TaskGoal),
		"active",
		l1SchemaVersion, in.BudgetTokens, 0,
		strings.TrimSpace(in.ParentTaskID),
		"[]",
		now, "", "", "{}", now, now,
	)
	if err != nil {
		return nil, err
	}
	return r.GetL1TaskRow(ctx, sessID, id)
}

func (r *l1WorkingMemoryRepo) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_l1_tasks SET status = ?, ended_at = ?, updated_at = ? WHERE id = ? AND session_id = ?`,
		status, now, now, taskID, sessionID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetL1TaskRow(ctx, sessionID, taskID)
}

func (r *l1WorkingMemoryRepo) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_l1_tasks SET archived_at = ?, updated_at = ? WHERE id = ? AND session_id = ?`,
		now, now, taskID, sessionID,
	)
	if err != nil {
		return nil, err
	}
	return r.buildL1TaskSnapshot(ctx, sessionID, taskID)
}

func (r *l1WorkingMemoryRepo) UnarchiveL1Task(ctx context.Context, sessionID, taskID string) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_l1_tasks SET archived_at = '', updated_at = ? WHERE id = ? AND session_id = ?`,
		now, taskID, sessionID,
	)
	return err
}

func (r *l1WorkingMemoryRepo) buildL1TaskSnapshot(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	taskRaw, err := r.GetL1TaskRow(ctx, sessionID, taskID)
	if err != nil {
		return nil, err
	}
	taskMap, err := jsonutil.ParseMap(taskRaw)
	if err != nil {
		return nil, err
	}
	fieldsRaw, err := r.listL1FieldRowsInternal(ctx, taskID, true)
	if err != nil {
		return nil, err
	}
	var fields []map[string]any
	for _, raw := range fieldsRaw {
		m, _ := jsonutil.ParseMap(raw)
		if m != nil {
			fields = append(fields, m)
		}
	}
	snapshot := map[string]any{
		"task":   taskMap,
		"fields": fields,
	}
	return json.Marshal(snapshot)
}

// --- L1FieldWriter ---

func (r *l1WorkingMemoryRepo) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) ([]byte, error) {
	taskID := strings.TrimSpace(in.TaskID)
	if taskID == "" {
		return nil, apierror.BadRequest("MEMORY", "task_id is required")
	}
	fieldPath := strings.TrimSpace(in.FieldPath)
	if fieldPath == "" {
		return nil, apierror.BadRequest("MEMORY", "field_path is required")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = newUUIDString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	visibility := strings.TrimSpace(in.Visibility)
	if visibility == "" {
		visibility = "prompt"
	}
	fieldKind := strings.TrimSpace(in.FieldKind)
	if fieldKind == "" {
		fieldKind = "string"
	}
	// Auto-calculate token_estimate if not provided by caller.
	if in.TokenEstimate <= 0 {
		in.TokenEstimate = estimateTokens(in.ValueText, in.ValueJSON, in.ValueRef)
	}

	lg := r.data.lg

	// Read budget for post-upsert check.
	budgetTokens, _, budgetErr := r.getL1TaskBudget(ctx, taskID)
	if budgetErr != nil {
		return nil, budgetErr
	}

	// Archive old value to field_history on conflict (best-effort).
	if in.HistoryEnabled {
		r.archiveL1FieldHistory(ctx, taskID, fieldPath, in.ChangedBy, now, lg)
	}

	// Upsert the field (optimistic — budget checked after).
	if err := r.insertL1FieldRow(ctx, id, in, fieldPath, fieldKind, visibility, now); err != nil {
		return nil, err
	}

	// Sync used_tokens aggregation for the parent task.
	if syncErr := r.syncL1TaskUsedTokens(ctx, taskID); syncErr != nil {
		lg.Warn("L1 used_tokens sync failed after upsert",
			loggateway.StepID("memory.l1_used_tokens_sync_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Err(syncErr))
	}

	// Post-upsert budget check: if budget is set, verify we didn't exceed it.
	// This eliminates the TOCTOU race — SQLite serializes writes, so the
	// sync above reflects the true state after our upsert.
	if budgetTokens > 0 {
		_, currentUsed, checkErr := r.getL1TaskBudget(ctx, taskID)
		if checkErr != nil {
			lg.Warn("L1 budget verification read failed",
				loggateway.StepID("memory.l1_budget_verify_fail"),
				loggateway.Str("task_id", taskID),
				loggateway.Err(checkErr))
		} else if currentUsed > budgetTokens {
			// Roll back: delete the field we just inserted.
			if _, delErr := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
				`DELETE FROM memory_l1_fields WHERE id = ?`, id); delErr != nil {
				lg.Error("L1 budget rollback DELETE failed",
					loggateway.StepID("memory.l1_budget_rollback_fail"),
					loggateway.Str("task_id", taskID),
					loggateway.Str("field_id", id),
					loggateway.Err(delErr))
			}
			r.syncL1TaskUsedTokens(ctx, taskID) // re-sync after rollback
			return nil, biz.ErrL1BudgetOverflow
		}
	}

	return r.GetL1FieldRow(ctx, taskID, fieldPath)
}

// archiveL1FieldHistory copies the current field value to field_history (best-effort).
func (r *l1WorkingMemoryRepo) archiveL1FieldHistory(ctx context.Context, taskID, fieldPath, changedBy, now string, lg loggateway.Logger) {
	if _, histErr := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_l1_field_history (id, field_id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate, changed_by, change_reason, diff_json, metadata_json, created_at)
		SELECT ?, id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate, ?, 'upsert', '{}', '{}', ?
		FROM memory_l1_fields WHERE task_id = ? AND field_path = ?`,
		newUUIDString(), changedBy, now, taskID, fieldPath,
	); histErr != nil {
		lg.Warn("L1 field history archival failed (best-effort)",
			loggateway.StepID("memory.l1_field_history_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Str("field_path", fieldPath),
			loggateway.Err(histErr))
	}
}

// insertL1FieldRow executes the INSERT … ON CONFLICT DO UPDATE for a single field.
func (r *l1WorkingMemoryRepo) insertL1FieldRow(ctx context.Context, id string, in biz.L1FieldInsert, fieldPath, fieldKind, visibility, now string) error {
	// Calculate expires_at from ttl_seconds + creation time.
	expiresAt := ""
	if in.TTLSeconds > 0 {
		t, _ := time.Parse(time.RFC3339Nano, now)
		if t.IsZero() {
			t = time.Now().UTC()
		}
		expiresAt = t.Add(time.Duration(in.TTLSeconds) * time.Second).Format(time.RFC3339Nano)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_l1_fields (
		id, task_id, session_id, agent_id, field_path, field_kind, visibility,
		pin_to_prompt, is_required, value_text, value_json, value_ref, preview,
		token_estimate, source, source_ref, ttl_seconds, expires_at,
		revision, last_read_at, read_count, metadata_json, created_at, updated_at
	) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
	ON CONFLICT(task_id, field_path) DO UPDATE SET
		value_text = excluded.value_text, value_json = excluded.value_json,
		value_ref = excluded.value_ref, preview = excluded.preview,
		token_estimate = excluded.token_estimate, visibility = excluded.visibility,
		pin_to_prompt = excluded.pin_to_prompt, source = excluded.source,
		ttl_seconds = excluded.ttl_seconds, expires_at = excluded.expires_at,
		revision = revision + 1, updated_at = excluded.updated_at`,
		id, strings.TrimSpace(in.TaskID),
		strings.TrimSpace(in.SessionID),
		strings.TrimSpace(in.AgentID),
		fieldPath, fieldKind, visibility,
		memBoolToInt(in.PinToPrompt), memBoolToInt(in.IsRequired),
		strings.TrimSpace(in.ValueText),
		strings.TrimSpace(in.ValueJSON),
		strings.TrimSpace(in.ValueRef),
		strings.TrimSpace(in.Preview),
		in.TokenEstimate,
		strings.TrimSpace(in.Source),
		strings.TrimSpace(in.SourceRef),
		in.TTLSeconds, expiresAt,
		1, "", 0, "{}", now, now,
	)
	return err
}

func (r *l1WorkingMemoryRepo) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`DELETE FROM memory_l1_fields WHERE task_id = ? AND field_path = ?`, taskID, fieldPath)
	if err != nil {
		return err
	}
	// Sync used_tokens aggregation for the parent task.
	return r.syncL1TaskUsedTokens(ctx, taskID)
}

func (r *l1WorkingMemoryRepo) PatchL1Fields(ctx context.Context, fields []biz.L1FieldInsert) ([][]byte, error) {
	var results [][]byte
	for _, f := range fields {
		b, err := r.UpsertL1Field(ctx, f)
		if err != nil {
			return results, err
		}
		results = append(results, b)
	}
	return results, nil
}

// --- Budget helpers ---

// getL1TaskBudget returns the budget_tokens and used_tokens for the given task.
func (r *l1WorkingMemoryRepo) getL1TaskBudget(ctx context.Context, taskID string) (budget, used int, err error) {
	rows, qErr := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT budget_tokens, used_tokens FROM memory_l1_tasks WHERE id = ?`, taskID)
	if qErr != nil {
		return 0, 0, qErr
	}
	defer rows.Close()
	if !rows.Next() {
		return 0, 0, sql.ErrNoRows
	}
	if sErr := rows.Scan(&budget, &used); sErr != nil {
		return 0, 0, sErr
	}
	return budget, used, rows.Err()
}

// syncL1TaskUsedTokens recalculates and updates used_tokens for the given task.
func (r *l1WorkingMemoryRepo) syncL1TaskUsedTokens(ctx context.Context, taskID string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_l1_tasks SET used_tokens = (
            SELECT COALESCE(SUM(token_estimate), 0) FROM memory_l1_fields WHERE task_id = ?
        ), updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		taskID, taskID)
	return err
}

// estimateTokens provides a rough token count estimate.
// CJK characters are estimated at ~1 token per rune (conservative),
// non-CJK characters at ~1 token per 4 runes (standard tokenizer behavior).
func estimateTokens(texts ...string) int {
	var cjkCount, otherCount int
	for _, text := range texts {
		for _, r := range text {
			if isCJKRune(r) {
				cjkCount++
			} else {
				otherCount++
			}
		}
	}
	est := cjkCount + otherCount/4
	if est == 0 {
		for _, text := range texts {
			if text != "" {
				return l1FieldEstimateMin
			}
		}
	}
	return est
}

// isCJKRune reports whether r is a CJK ideograph or syllable.
func isCJKRune(r rune) bool {
	return (r >= 0x4E00 && r <= 0x9FFF) || // CJK Unified Ideographs
		(r >= 0x3040 && r <= 0x309F) || // Hiragana
		(r >= 0x30A0 && r <= 0x30FF) || // Katakana
		(r >= 0xAC00 && r <= 0xD7AF) // Hangul Syllables
}

// --- L1IdleTaskReader ---

func (r *l1WorkingMemoryRepo) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, session_id, agent_id, task_title, status, updated_at FROM memory_l1_tasks WHERE status = 'active' AND archived_at = '' AND updated_at < ?`,
		cutoffRFC3339,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][]byte
	for rows.Next() {
		var id, sessID, agentID, title, status, updatedAt string
		if err := rows.Scan(&id, &sessID, &agentID, &title, &status, &updatedAt); err != nil {
			return nil, err
		}
		m := map[string]any{
			"id": id, "session_id": sessID, "agent_id": agentID,
			"task_title": title, "status": status, "updated_at": updatedAt,
		}
		b, _ := json.Marshal(m)
		out = append(out, b)
	}
	return out, rows.Err()
}

// --- L1SchemaReader ---

func (r *l1WorkingMemoryRepo) GetL1SchemaRow(ctx context.Context, schemaID string) ([]byte, error) {
	if schemaID == "" {
		return nil, apierror.BadRequest("MEMORY", "schema id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, scope_type, scope_id, schema_key, schema_version, schema_json, description, enabled, metadata_json, created_at FROM memory_l1_schemas WHERE id = ?`,
		schemaID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	var (
		id, scopeType, scopeID, schemaKey string
		schemaVersion                     int
		schemaJSON, description           string
		enabled                           int
		metadataJSON, createdAt           string
	)
	if err := rows.Scan(&id, &scopeType, &scopeID, &schemaKey, &schemaVersion, &schemaJSON, &description, &enabled, &metadataJSON, &createdAt); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "scope_type": scopeType, "scope_id": scopeID,
		"schema_key": schemaKey, "schema_version": schemaVersion,
		"schema_json": schemaJSON, "description": description,
		"enabled": enabled, "metadata_json": metadataJSON, "created_at": createdAt,
	}
	return json.Marshal(m)
}
