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

var _ biz.TaskPlanRepository = (*taskPlanRepo)(nil)

type taskPlanRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NewTaskPlanRepo implements biz.TaskPlanRepository.
func NewTaskPlanRepo(d *Data, lg loggateway.Logger) biz.TaskPlanRepository {
	return &taskPlanRepo{data: d, lg: lg}
}

func (r *taskPlanRepo) Create(ctx context.Context, plan *biz.TaskPlan) (*biz.TaskPlan, error) {
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return nil, apierror.BadRequest("TASK_PLAN", "plan id is required")
	}
	now := time.Now().UTC()
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = now
	}
	plan.UpdatedAt = now
	if plan.Status == "" {
		plan.Status = biz.PlanStatusDraft
	}

	dimensionsJSON, err := json.Marshal(plan.Dimensions)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_PLAN")
	}
	subTasksJSON, err := json.Marshal(plan.SubTasks)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_PLAN")
	}
	dagJSON := "{}"
	if plan.TaskDAG != nil {
		b, err := json.Marshal(plan.TaskDAG)
		if err != nil {
			return nil, entErrToBizErr(err, "TASK_PLAN")
		}
		dagJSON = string(b)
	}
	memoryHitJSON := "{}"
	if plan.MemoryHit != nil {
		b, err := json.Marshal(plan.MemoryHit)
		if err != nil {
			return nil, entErrToBizErr(err, "TASK_PLAN")
		}
		memoryHitJSON = string(b)
	}
	intentArtifactJSON := plan.IntentArtifactJSON
	if intentArtifactJSON == "" {
		intentArtifactJSON = "{}"
	}

	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		`INSERT INTO task_plans (id, spirit_session_id, trace_id, user_message, intent_artifact_json,
			complexity_level, complexity_score, dimensions_json, sub_tasks_json, dag_json,
			decompose_reason, strategy, strategy_reason, topology_hint, memory_hit_json,
			status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.SpiritSessionID, plan.TraceID, plan.UserMessage, intentArtifactJSON,
		string(plan.ComplexityLevel), plan.ComplexityScore, string(dimensionsJSON), string(subTasksJSON), dagJSON,
		plan.DecomposeReason, string(plan.Strategy), plan.StrategyReason, string(plan.TopologyHint), memoryHitJSON,
		string(plan.Status), plan.CreatedAt.Format(time.RFC3339), plan.UpdatedAt.Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, plan.ID)
}

func (r *taskPlanRepo) GetByID(ctx context.Context, id string) (*biz.TaskPlan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, apierror.BadRequest("TASK_PLAN", "id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, spirit_session_id, trace_id, user_message, intent_artifact_json,
			complexity_level, complexity_score, dimensions_json, sub_tasks_json, dag_json,
			decompose_reason, strategy, strategy_reason, topology_hint, memory_hit_json,
			status, created_at, updated_at
		 FROM task_plans WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, apierror.NotFound(apierror.DomainData, "not found")
	}
	plan, err := scanTaskPlanFromRows(rows)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (r *taskPlanRepo) Update(ctx context.Context, plan *biz.TaskPlan) (*biz.TaskPlan, error) {
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return nil, apierror.BadRequest("TASK_PLAN", "plan id is required")
	}
	plan.UpdatedAt = time.Now().UTC()

	dimensionsJSON, err := json.Marshal(plan.Dimensions)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_PLAN")
	}
	subTasksJSON, err := json.Marshal(plan.SubTasks)
	if err != nil {
		return nil, entErrToBizErr(err, "TASK_PLAN")
	}
	dagJSON := "{}"
	if plan.TaskDAG != nil {
		b, err := json.Marshal(plan.TaskDAG)
		if err != nil {
			return nil, entErrToBizErr(err, "TASK_PLAN")
		}
		dagJSON = string(b)
	}
	memoryHitJSON := "{}"
	if plan.MemoryHit != nil {
		b, err := json.Marshal(plan.MemoryHit)
		if err != nil {
			return nil, entErrToBizErr(err, "TASK_PLAN")
		}
		memoryHitJSON = string(b)
	}

	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE task_plans SET
			spirit_session_id=?, trace_id=?, user_message=?, intent_artifact_json=?,
			complexity_level=?, complexity_score=?, dimensions_json=?, sub_tasks_json=?, dag_json=?,
			decompose_reason=?, strategy=?, strategy_reason=?, topology_hint=?, memory_hit_json=?,
			status=?, updated_at=?
		 WHERE id = ?`,
		plan.SpiritSessionID, plan.TraceID, plan.UserMessage, plan.IntentArtifactJSON,
		string(plan.ComplexityLevel), plan.ComplexityScore, string(dimensionsJSON), string(subTasksJSON), dagJSON,
		plan.DecomposeReason, string(plan.Strategy), plan.StrategyReason, string(plan.TopologyHint), memoryHitJSON,
		string(plan.Status), plan.UpdatedAt.Format(time.RFC3339),
		plan.ID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, plan.ID)
}

func (r *taskPlanRepo) ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*biz.TaskPlan, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, spirit_session_id, trace_id, user_message, intent_artifact_json,
			complexity_level, complexity_score, dimensions_json, sub_tasks_json, dag_json,
			decompose_reason, strategy, strategy_reason, topology_hint, memory_hit_json,
			status, created_at, updated_at
		 FROM task_plans WHERE spirit_session_id = ? ORDER BY created_at DESC`, spiritSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plans []*biz.TaskPlan
	for rows.Next() {
		plan, err := scanTaskPlanFromRows(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// EnsureTaskPlanSchema creates the task_plans table if it does not exist.
// Called during DDL migration.
func EnsureTaskPlanSchema(ctx context.Context, db *sql.DB, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS task_plans (
		id TEXT PRIMARY KEY,
		spirit_session_id TEXT DEFAULT '',
		trace_id TEXT DEFAULT '',
		user_message TEXT DEFAULT '',
		intent_artifact_json TEXT DEFAULT '{}',
		complexity_level TEXT DEFAULT 'simple',
		complexity_score REAL DEFAULT 0,
		dimensions_json TEXT DEFAULT '{}',
		sub_tasks_json TEXT DEFAULT '[]',
		dag_json TEXT DEFAULT '{}',
		decompose_reason TEXT DEFAULT '',
		strategy TEXT DEFAULT 'direct',
		strategy_reason TEXT DEFAULT '',
		topology_hint TEXT DEFAULT '',
		memory_hit_json TEXT DEFAULT '{}',
		status TEXT DEFAULT 'draft',
		created_at TEXT DEFAULT '',
		updated_at TEXT DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create task_plans table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_task_plans_spirit_session_id ON task_plans(spirit_session_id)`); err != nil {
		return fmt.Errorf("create task_plans index: %w", err)
	}
	return nil
}

func scanTaskPlanFromRows(rows *sql.Rows) (*biz.TaskPlan, error) {
	var plan biz.TaskPlan
	var complexityLevel, strategy, topologyHint, status string
	var createdAtStr, updatedAtStr string
	var dimensionsJSON, subTasksJSON, dagJSON, memoryHitJSON string

	err := rows.Scan(
		&plan.ID, &plan.SpiritSessionID, &plan.TraceID, &plan.UserMessage, &plan.IntentArtifactJSON,
		&complexityLevel, &plan.ComplexityScore, &dimensionsJSON, &subTasksJSON, &dagJSON,
		&plan.DecomposeReason, &strategy, &plan.StrategyReason, &topologyHint, &memoryHitJSON,
		&status, &createdAtStr, &updatedAtStr,
	)
	if err != nil {
		return nil, err
	}

	plan.ComplexityLevel = biz.ComplexityLevel(complexityLevel)
	plan.Strategy = biz.OrchestrationStrategy(strategy)
	plan.TopologyHint = biz.TopologyType(topologyHint)
	plan.Status = biz.PlanStatus(status)
	plan.CreatedAt = parseTimeRFC3339(createdAtStr)
	plan.UpdatedAt = parseTimeRFC3339(updatedAtStr)

	if err := json.Unmarshal([]byte(dimensionsJSON), &plan.Dimensions); err != nil {
		plan.Dimensions = biz.DimensionScores{}
	}
	if err := json.Unmarshal([]byte(subTasksJSON), &plan.SubTasks); err != nil {
		plan.SubTasks = nil
	}
	if dagJSON != "" && dagJSON != "{}" {
		var dag biz.PlanTaskDAG
		if err := json.Unmarshal([]byte(dagJSON), &dag); err == nil {
			plan.TaskDAG = &dag
		}
	}
	if memoryHitJSON != "" && memoryHitJSON != "{}" {
		var hit biz.MemoryHit
		if err := json.Unmarshal([]byte(memoryHitJSON), &hit); err == nil {
			plan.MemoryHit = &hit
		}
	}

	return &plan, nil
}

func parseTimeRFC3339(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}
	}
	return t
}
