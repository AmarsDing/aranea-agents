package sessionmemory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/jsonutil"
	"aranea-agents/pkg/loggateway"
)

// --- L1 Task Write Operations ---

// StartL1Task creates a new L1 task or returns existing one if task_key already exists for this session+agent.
func (st *Store) StartL1Task(ctx context.Context, in biz.L1TaskInsert) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	sessID := strings.TrimSpace(in.SessionID)
	if sessID == "" {
		return nil, errors.New("session_id is required")
	}
	id := strings.TrimSpace(in.ID)
	if id == "" {
		id = uuid.NewString()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.client.ExecContext(ctx, `INSERT INTO memory_l1_tasks (
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
	return st.GetL1TaskRow(ctx, sessID, id)
}

// EndL1Task marks a task as completed/failed/cancelled.
func (st *Store) EndL1Task(ctx context.Context, sessionID, taskID, status string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_l1_tasks SET status = ?, ended_at = ?, updated_at = ? WHERE id = ? AND session_id = ?`,
		status, now, now, taskID, sessionID,
	)
	if err != nil {
		return nil, err
	}
	return st.GetL1TaskRow(ctx, sessionID, taskID)
}

// GetL1TaskRow returns a single L1 task row by id and session_id.
func (st *Store) GetL1TaskRow(ctx context.Context, sessionID, id string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	rows, err := st.client.QueryContext(ctx, sqlL1Task+` WHERE id = ? AND session_id = ?`, id, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanL1TaskRowFromRows(rows)
}

// --- L1 Field Write Operations ---

// UpsertL1Field creates or updates an L1 field. On update, it increments revision and archives old value to field_history.
func (st *Store) UpsertL1Field(ctx context.Context, in biz.L1FieldInsert) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
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
		id = uuid.NewString()
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

	// Archive old value to field_history on conflict (best-effort).
	if _, histErr := st.client.ExecContext(ctx, `INSERT INTO memory_l1_field_history (id, field_id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate, changed_by, change_reason, diff_json, metadata_json, created_at)
		SELECT ?, id, task_id, revision, value_text, value_json, value_ref, preview, token_estimate, ?, 'upsert', '{}', '{}', ?
		FROM memory_l1_fields WHERE task_id = ? AND field_path = ?`,
		uuid.NewString(), in.ChangedBy, now, taskID, fieldPath,
	); histErr != nil {
		st.lg.Warn("L1 field history archival failed (best-effort)",
			loggateway.StepID("memory.l1_field_history_fail"),
			loggateway.Str("task_id", taskID),
			loggateway.Str("field_path", fieldPath),
			loggateway.Err(histErr))
	}

	_, err := st.client.ExecContext(ctx, `INSERT INTO memory_l1_fields (
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
		boolToInt(in.PinToPrompt), boolToInt(in.IsRequired),
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
	return st.GetL1FieldRow(ctx, taskID, fieldPath)
}

// DeleteL1Field removes an L1 field by task_id and field_path.
func (st *Store) DeleteL1Field(ctx context.Context, taskID, fieldPath string) error {
	if st == nil || st.client == nil {
		return errors.New("session memory store not wired")
	}
	_, err := st.client.ExecContext(ctx,
		`DELETE FROM memory_l1_fields WHERE task_id = ? AND field_path = ?`, taskID, fieldPath)
	return err
}

// GetL1FieldRow returns a single L1 field row by task_id and field_path.
func (st *Store) GetL1FieldRow(ctx context.Context, taskID, fieldPath string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	rows, err := st.client.QueryContext(ctx, sqlL1Field+` WHERE task_id = ? AND field_path = ?`, taskID, fieldPath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	return scanL1FieldRowFromRows(rows)
}

// PatchL1Fields applies multiple field upserts in a single call.
func (st *Store) PatchL1Fields(ctx context.Context, fields []biz.L1FieldInsert) ([][]byte, error) {
	var results [][]byte
	for _, f := range fields {
		b, err := st.UpsertL1Field(ctx, f)
		if err != nil {
			return results, err
		}
		results = append(results, b)
	}
	return results, nil
}

// ArchiveL1Task marks a task as archived and returns its snapshot JSON.
func (st *Store) ArchiveL1Task(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	if st == nil || st.client == nil {
		return nil, errors.New("session memory store not wired")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err := st.client.ExecContext(ctx,
		`UPDATE memory_l1_tasks SET archived_at = ?, updated_at = ? WHERE id = ? AND session_id = ?`,
		now, now, taskID, sessionID,
	)
	if err != nil {
		return nil, err
	}
	return st.buildL1TaskSnapshot(ctx, sessionID, taskID)
}

// buildL1TaskSnapshot creates a JSON snapshot of the task and all its fields.
func (st *Store) buildL1TaskSnapshot(ctx context.Context, sessionID, taskID string) ([]byte, error) {
	taskRaw, err := st.GetL1TaskRow(ctx, sessionID, taskID)
	if err != nil {
		return nil, err
	}
	taskMap, err := jsonutil.ParseMap(taskRaw)
	if err != nil {
		return nil, err
	}
	fieldsRaw, err := st.ListL1FieldRows(ctx, taskID, true)
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

// ListIdleL1Tasks returns tasks that are active, not archived, and updated before the cutoff.
func (st *Store) ListIdleL1Tasks(ctx context.Context, cutoffRFC3339 string) ([][]byte, error) {
	if st == nil || st.client == nil {
		return nil, nil
	}
	rows, err := st.client.QueryContext(ctx,
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

// --- Helpers ---

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func scanL1TaskRowFromRows(rows *sql.Rows) ([]byte, error) {
	var (
		id, sessID, runID, teamID, agentID string
		taskKey, taskTitle, taskGoal, status string
		schemaVer                          int
		budgetTok, usedTok                 int
		parentTaskID, sharedWithJSON       string
		startedAt, endedAt, archivedAt     string
		metadataJSON, createdAt, updatedAt string
	)
	if err := rows.Scan(
		&id, &sessID, &runID, &teamID, &agentID,
		&taskKey, &taskTitle, &taskGoal, &status,
		&schemaVer, &budgetTok, &usedTok,
		&parentTaskID, &sharedWithJSON,
		&startedAt, &endedAt, &archivedAt,
		&metadataJSON, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "session_id": sessID, "run_id": runID, "team_id": teamID, "agent_id": agentID,
		"task_key": taskKey, "task_title": taskTitle, "task_goal": taskGoal, "status": status,
		"schema_version": schemaVer, "budget_tokens": budgetTok, "used_tokens": usedTok,
		"parent_task_id": parentTaskID, "shared_with_json": sharedWithJSON,
		"started_at": startedAt, "ended_at": endedAt, "archived_at": archivedAt,
		"metadata_json": metadataJSON, "created_at": createdAt, "updated_at": updatedAt,
	}
	return json.Marshal(m)
}

func scanL1FieldRowFromRows(rows *sql.Rows) ([]byte, error) {
	var (
		id, taskID, sessID, agentID             string
		fieldPath, fieldKind, visibility        string
		pinToPrompt, isRequired                 int
		valueText, valueJSON, valueRef, preview string
		tokenEst                                int
		source, sourceRef                       string
		ttlSec                                  int
		expiresAt                               string
		revision                                int
		lastReadAt                              string
		readCount                               int
		metadataJSON, createdAt, updatedAt      string
	)
	if err := rows.Scan(
		&id, &taskID, &sessID, &agentID,
		&fieldPath, &fieldKind, &visibility, &pinToPrompt, &isRequired,
		&valueText, &valueJSON, &valueRef, &preview, &tokenEst,
		&source, &sourceRef, &ttlSec, &expiresAt,
		&revision, &lastReadAt, &readCount,
		&metadataJSON, &createdAt, &updatedAt,
	); err != nil {
		return nil, err
	}
	m := map[string]any{
		"id": id, "task_id": taskID, "session_id": sessID, "agent_id": agentID,
		"field_path": fieldPath, "field_kind": fieldKind, "visibility": visibility,
		"pin_to_prompt": pinToPrompt != 0, "is_required": isRequired != 0,
		"value_text": valueText, "value_json": valueJSON, "value_ref": valueRef,
		"preview": preview, "token_estimate": tokenEst,
		"source": source, "source_ref": sourceRef,
		"ttl_seconds": ttlSec, "expires_at": expiresAt,
		"revision": revision, "last_read_at": lastReadAt, "read_count": readCount,
		"metadata_json": metadataJSON, "created_at": createdAt, "updated_at": updatedAt,
	}
	return json.Marshal(m)
}
