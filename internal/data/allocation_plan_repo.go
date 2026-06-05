package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

var _ biz.AllocationPlanRepository = (*allocationPlanRepo)(nil)

type allocationPlanRepo struct {
	data *Data
	lg   loggateway.Logger
}

// NewAllocationPlanRepo implements biz.AllocationPlanRepository.
func NewAllocationPlanRepo(d *Data, lg loggateway.Logger) biz.AllocationPlanRepository {
	return &allocationPlanRepo{data: d, lg: lg}
}

func (r *allocationPlanRepo) Create(ctx context.Context, plan *biz.AllocationPlan) (*biz.AllocationPlan, error) {
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return nil, kerrors.BadRequest("ALLOCATION_PLAN", "plan id is required")
	}
	now := time.Now().UTC()
	if plan.CreatedAt == "" {
		plan.CreatedAt = now.Format(time.RFC3339)
	}
	plan.UpdatedAt = now.Format(time.RFC3339)
	if plan.Status == "" {
		plan.Status = biz.AllocationStatusDraft
	}

	allocationsJSON, err := json.Marshal(plan.Allocations)
	if err != nil {
		return nil, kerrors.InternalServer("ALLOCATION_PLAN", "marshal allocations: "+err.Error())
	}

	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		`INSERT INTO allocation_plans (id, task_plan_id, spirit_session_id, trace_id, allocations_json,
			status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID, plan.TaskPlanID, plan.SpiritSessionID, plan.TraceID, string(allocationsJSON),
		string(plan.Status), plan.CreatedAt, plan.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, plan.ID)
}

func (r *allocationPlanRepo) GetByID(ctx context.Context, id string) (*biz.AllocationPlan, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, kerrors.BadRequest("ALLOCATION_PLAN", "id is required")
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, task_plan_id, spirit_session_id, trace_id, allocations_json,
			status, created_at, updated_at
		 FROM allocation_plans WHERE id = ?`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	if !rows.Next() {
		return nil, sql.ErrNoRows
	}
	plan, err := scanAllocationPlanFromRows(rows)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (r *allocationPlanRepo) Update(ctx context.Context, plan *biz.AllocationPlan) (*biz.AllocationPlan, error) {
	if plan == nil || strings.TrimSpace(plan.ID) == "" {
		return nil, kerrors.BadRequest("ALLOCATION_PLAN", "plan id is required")
	}
	plan.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	allocationsJSON, err := json.Marshal(plan.Allocations)
	if err != nil {
		return nil, kerrors.InternalServer("ALLOCATION_PLAN", "marshal allocations: "+err.Error())
	}

	_, err = r.data.RW().Write(ctx).ExecContext(ctx,
		`UPDATE allocation_plans SET
			task_plan_id=?, spirit_session_id=?, trace_id=?, allocations_json=?,
			status=?, updated_at=?
		 WHERE id = ?`,
		plan.TaskPlanID, plan.SpiritSessionID, plan.TraceID, string(allocationsJSON),
		string(plan.Status), plan.UpdatedAt,
		plan.ID,
	)
	if err != nil {
		return nil, err
	}
	return r.GetByID(ctx, plan.ID)
}

func (r *allocationPlanRepo) ListBySpiritSessionID(ctx context.Context, spiritSessionID string) ([]*biz.AllocationPlan, error) {
	spiritSessionID = strings.TrimSpace(spiritSessionID)
	if spiritSessionID == "" {
		return nil, nil
	}
	rows, err := r.data.RWDB().ReadDB(ctx).QueryContext(ctx,
		`SELECT id, task_plan_id, spirit_session_id, trace_id, allocations_json,
			status, created_at, updated_at
		 FROM allocation_plans WHERE spirit_session_id = ? ORDER BY created_at DESC`, spiritSessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var plans []*biz.AllocationPlan
	for rows.Next() {
		plan, err := scanAllocationPlanFromRows(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

// EnsureAllocationPlanSchema creates the allocation_plans table if it does not exist.
// Called during DDL migration.
func EnsureAllocationPlanSchema(ctx context.Context, db *sql.DB, lg loggateway.Logger) error {
	if db == nil {
		return nil
	}
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS allocation_plans (
		id TEXT PRIMARY KEY,
		task_plan_id TEXT DEFAULT '',
		spirit_session_id TEXT DEFAULT '',
		trace_id TEXT DEFAULT '',
		allocations_json TEXT DEFAULT '[]',
		status TEXT DEFAULT 'draft',
		created_at TEXT DEFAULT '',
		updated_at TEXT DEFAULT ''
	)`)
	if err != nil {
		return fmt.Errorf("create allocation_plans table: %w", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS idx_allocation_plans_spirit_session_id ON allocation_plans(spirit_session_id)`); err != nil {
		return fmt.Errorf("create allocation_plans index: %w", err)
	}
	return nil
}

func scanAllocationPlanFromRows(rows *sql.Rows) (*biz.AllocationPlan, error) {
	var plan biz.AllocationPlan
	var status string
	var allocationsJSON string

	err := rows.Scan(
		&plan.ID, &plan.TaskPlanID, &plan.SpiritSessionID, &plan.TraceID, &allocationsJSON,
		&status, &plan.CreatedAt, &plan.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	plan.Status = biz.AllocationStatus(status)

	if err := json.Unmarshal([]byte(allocationsJSON), &plan.Allocations); err != nil {
		plan.Allocations = nil
	}

	return &plan, nil
}
