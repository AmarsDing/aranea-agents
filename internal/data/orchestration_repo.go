package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

var _ biz.OrchestrationRepository = (*orchestrationRepo)(nil)

type orchestrationRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NewOrchestrationRepo implements biz.OrchestrationRepository.
func NewOrchestrationRepo(d *Data, lg loggateway.Logger) biz.OrchestrationRepository {
	return &orchestrationRepo{data: d, lg: lg}
}

func (r *orchestrationRepo) Create(ctx context.Context, handle *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	if handle == nil || strings.TrimSpace(handle.ID) == "" {
		return nil, apierror.BadRequest("ORCHESTRATION", "orchestration id is required")
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if handle.CreatedAt == "" {
		handle.CreatedAt = now
	}
	handle.UpdatedAt = now
	if handle.Status == "" {
		handle.Status = biz.OrchestrationStatusPending
	}

	teamIDsJSON, err := json.Marshal(handle.TeamIDs)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	agentKeysJSON, err := json.Marshal(handle.AgentKeys)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	synthesisResultJSON := handle.SynthesisResultJSON
	if synthesisResultJSON == "" {
		synthesisResultJSON = "{}"
	}

	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		`INSERT INTO orchestrations (id, task_plan_id, allocation_id, spirit_session_id, trace_id,
			strategy, graph_execution_id, team_ids_json, agent_keys_json, status, checkpoint_id,
			synthesis_result_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		handle.ID, handle.TaskPlanID, handle.AllocationID, handle.SpiritSessionID, handle.TraceID,
		string(handle.Strategy), handle.GraphExecutionID, string(teamIDsJSON), string(agentKeysJSON), string(handle.Status), handle.CheckpointID,
		synthesisResultJSON, handle.CreatedAt, handle.UpdatedAt,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	return r.GetByID(ctx, handle.ID)
}

func (r *orchestrationRepo) GetByID(ctx context.Context, id string) (*biz.OrchestrationHandle, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apierror.BadRequest("ORCHESTRATION", "id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, task_plan_id, allocation_id, spirit_session_id, trace_id,
			strategy, graph_execution_id, team_ids_json, agent_keys_json, status, checkpoint_id,
			synthesis_result_json, created_at, updated_at
		 FROM orchestrations WHERE id = ?`, id)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound(apierror.DomainData, "not found")
	}
	handle, err := scanOrchestrationFromRows(rows)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	return handle, nil
}

func (r *orchestrationRepo) Update(ctx context.Context, handle *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	if handle == nil || strings.TrimSpace(handle.ID) == "" {
		return nil, apierror.BadRequest("ORCHESTRATION", "orchestration id is required")
	}
	handle.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	teamIDsJSON, err := json.Marshal(handle.TeamIDs)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	agentKeysJSON, err := json.Marshal(handle.AgentKeys)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	synthesisResultJSON := handle.SynthesisResultJSON
	if synthesisResultJSON == "" {
		synthesisResultJSON = "{}"
	}

	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE orchestrations SET
			task_plan_id=?, allocation_id=?, spirit_session_id=?, trace_id=?,
			strategy=?, graph_execution_id=?, team_ids_json=?, agent_keys_json=?, status=?, checkpoint_id=?,
			synthesis_result_json=?, updated_at=?
		 WHERE id = ?`,
		handle.TaskPlanID, handle.AllocationID, handle.SpiritSessionID, handle.TraceID,
		string(handle.Strategy), handle.GraphExecutionID, string(teamIDsJSON), string(agentKeysJSON), string(handle.Status), handle.CheckpointID,
		synthesisResultJSON, handle.UpdatedAt,
		handle.ID,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	return r.GetByID(ctx, handle.ID)
}

func (r *orchestrationRepo) ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*biz.OrchestrationHandle, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, task_plan_id, allocation_id, spirit_session_id, trace_id,
			strategy, graph_execution_id, team_ids_json, agent_keys_json, status, checkpoint_id,
			synthesis_result_json, created_at, updated_at
		 FROM orchestrations WHERE spirit_session_id = ? ORDER BY created_at DESC`, spiritSessionID)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	defer rows.Close()
	var handles []*biz.OrchestrationHandle
	for rows.Next() {
		handle, err := scanOrchestrationFromRows(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "ORCHESTRATION")
		}
		handles = append(handles, handle)
	}
	return handles, nil
}

func (r *orchestrationRepo) ListByStatus(ctx context.Context, status biz.OrchestrationStatus) ([]*biz.OrchestrationHandle, error) {
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, task_plan_id, allocation_id, spirit_session_id, trace_id,
			strategy, graph_execution_id, team_ids_json, agent_keys_json, status, checkpoint_id,
			synthesis_result_json, created_at, updated_at
		 FROM orchestrations WHERE status = ? ORDER BY created_at DESC`, string(status))
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}
	defer rows.Close()
	var handles []*biz.OrchestrationHandle
	for rows.Next() {
		handle, err := scanOrchestrationFromRows(rows)
		if err != nil {
			return nil, entErrToBizErr(err, "ORCHESTRATION")
		}
		handles = append(handles, handle)
	}
	return handles, nil
}

func scanOrchestrationFromRows(rows *sql.Rows) (*biz.OrchestrationHandle, error) {
	var handle biz.OrchestrationHandle
	var strategy, status string
	var teamIDsJSON, agentKeysJSON, synthesisResultJSON string

	err := rows.Scan(
		&handle.ID, &handle.TaskPlanID, &handle.AllocationID, &handle.SpiritSessionID, &handle.TraceID,
		&strategy, &handle.GraphExecutionID, &teamIDsJSON, &agentKeysJSON, &status, &handle.CheckpointID,
		&synthesisResultJSON, &handle.CreatedAt, &handle.UpdatedAt,
	)
	if err != nil {
		return nil, entErrToBizErr(err, "ORCHESTRATION")
	}

	handle.Strategy = biz.OrchestrationStrategy(strategy)
	handle.Status = biz.OrchestrationStatus(status)

	if err := json.Unmarshal([]byte(teamIDsJSON), &handle.TeamIDs); err != nil {
		handle.TeamIDs = nil
	}
	if err := json.Unmarshal([]byte(agentKeysJSON), &handle.AgentKeys); err != nil {
		handle.AgentKeys = nil
	}
	handle.SynthesisResultJSON = synthesisResultJSON

	return &handle, nil
}

// EnsureOrchestrationSchema creates the orchestrations table if it does not exist.
// Called during DDL migration.
func EnsureOrchestrationSchema(ctx context.Context, db *sql.DB, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS orchestrations (
		id TEXT PRIMARY KEY,
		task_plan_id TEXT DEFAULT '',
		allocation_id TEXT DEFAULT '',
		spirit_session_id TEXT DEFAULT '',
		trace_id TEXT DEFAULT '',
		strategy TEXT DEFAULT 'direct',
		graph_execution_id TEXT DEFAULT '',
		team_ids_json TEXT DEFAULT '[]',
		agent_keys_json TEXT DEFAULT '[]',
		status TEXT DEFAULT 'pending',
		checkpoint_id TEXT DEFAULT '',
		synthesis_result_json TEXT DEFAULT '{}',
		created_at TEXT DEFAULT '',
		updated_at TEXT DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create orchestrations table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_orchestrations_spirit_session_id ON orchestrations(spirit_session_id)`); err != nil {
		return fmt.Errorf("create orchestrations index: %w", err)
	}
	// Migration: add agent_keys_json column. SQLite ALTER TABLE ADD COLUMN
	// fails if the column already exists; we catch and ignore that error.
	if _, err := db.ExecContext(ctx, `ALTER TABLE orchestrations ADD COLUMN agent_keys_json TEXT DEFAULT '[]'`); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "duplicate column") {
			return fmt.Errorf("alter orchestrations add agent_keys_json: %w", err)
		}
	}
	return nil
}
