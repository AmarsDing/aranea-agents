package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
)

// l1WorkingMemoryRepo implements biz.L1TaskWriter + biz.L1FieldWriter + biz.L1AdminReader + biz.L1IdleTaskReader using direct Raw SQL.
type l1WorkingMemoryRepo struct {
	data *Data
}

var _ biz.L1TaskWriter = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1FieldWriter = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1AdminReader = (*l1WorkingMemoryRepo)(nil)
var _ biz.L1IdleTaskReader = (*l1WorkingMemoryRepo)(nil)

func newL1WorkingMemoryRepo(data *Data) *l1WorkingMemoryRepo {
	if data == nil {
		return nil
	}
	return &l1WorkingMemoryRepo{data: data}
}

// --- L1AdminReader ---

func (r *l1WorkingMemoryRepo) ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error) {
	if sessionID == "" {
		return nil, errors.New("session id is required")
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

func (r *l1WorkingMemoryRepo) ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error) {
	if taskID == "" {
		return nil, errors.New("task id is required")
	}
	q := sqlL1Field + ` WHERE task_id = ?`
	if !includeInternal {
		q += ` AND visibility != 'internal'`
	}
	q += ` ORDER BY field_path ASC`
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx, q, taskID)
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
		return nil, errors.New("session_id is required")
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
		status = excluded.status, updated_at = excluded.updated_at`,
		id, sessID,
		strings.TrimSpace(in.RunID),
		strings.TrimSpace(in.TeamID),
		strings.TrimSpace(in.AgentID),
		strings.TrimSpace(in.TaskKey),
		strings.TrimSpace(in.TaskTitle),
		strings.TrimSpace(in.TaskGoal),
		"active",
		1, in.BudgetTokens, 0,
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

func (r *l1WorkingMemoryRepo) buildL1TaskSnapshot(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	taskRaw, err := r.GetL1TaskRow(ctx, sessionID, taskID)
	if err != nil {
		return nil, err
	}
	taskMap, err := jsonutil.ParseMap(taskRaw)
	if err != nil {
		return nil, err
	}
	fieldsRaw, err := r.ListL1FieldRows(ctx, taskID, true)
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
		return nil, errors.New("task_id is required")
	}
	fieldPath := strings.TrimSpace(in.FieldPath)
	if fieldPath == "" {
		return nil, errors.New("field_path is required")
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
	lg := r.data.lg

	// Archive old value to field_history on conflict (best-effort).
	if _, histErr := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_l1_field_history (id, field_id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate, changed_by, change_reason, diff_json, metadata_json, created_at)
		SELECT ?, id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate, ?, 'upsert', '{}', '{}', ?
		FROM memory_l1_fields WHERE task_id = ? AND field_path = ?`,
		newUUIDString(), in.ChangedBy, now, taskID, fieldPath,
	); histErr != nil {
		lg.Warn("L1 field history archival failed (best-effort)",
			loggateway.StepID("memory.l1_field_history_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Str("field_path", fieldPath),
			loggateway.Err(histErr))
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
		revision = revision + 1, updated_at = excluded.updated_at`,
		id, taskID,
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
		in.TTLSeconds, "",
		1, "", 0, "{}", now, now,
	)
	if err != nil {
		return nil, err
	}
	return r.GetL1FieldRow(ctx, taskID, fieldPath)
}

func (r *l1WorkingMemoryRepo) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`DELETE FROM memory_l1_fields WHERE task_id = ? AND field_path = ?`, taskID, fieldPath)
	return err
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
