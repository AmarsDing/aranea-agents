package data

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type planRepo struct {
	data *Data
}

var _ biz.PlanRepository = (*planRepo)(nil)

func NewPlanRepo(d *Data) biz.PlanRepository {
	return &planRepo{data: d}
}

func (r *planRepo) Create(ctx context.Context, plan *biz.Plan) (*biz.Plan, error) {
	stepsJSON, err := json.Marshal(plan.Steps)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", "marshal steps: "+err.Error())
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `INSERT INTO plans (id, session_id, agent_key, goal, steps_json, status, surface_id, graph_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err = r.data.Ent().ExecContext(ctx, q,
		plan.ID, plan.SessionID, plan.AgentKey, plan.Goal, string(stepsJSON),
		string(plan.Status), plan.SurfaceID, plan.GraphID, now, now,
	)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", err.Error())
	}
	return plan, nil
}

func (r *planRepo) Get(ctx context.Context, id string) (*biz.Plan, error) {
	const q = `SELECT id, session_id, agent_key, goal, steps_json, status, surface_id, graph_id, created_at, updated_at FROM plans WHERE id = ?`
	row, qErr := r.data.ReadEnt().QueryContext(ctx, q, id)
	if qErr != nil {
		return nil, kerrors.InternalServer("PLAN", qErr.Error())
	}
	defer row.Close()
	if !row.Next() {
		return nil, kerrors.NotFound("PLAN", "plan not found")
	}
	var p biz.Plan
	var stepsJSON, status, createdAt, updatedAt string
	if err := row.Scan(&p.ID, &p.SessionID, &p.AgentKey, &p.Goal, &stepsJSON, &status, &p.SurfaceID, &p.GraphID, &createdAt, &updatedAt); err != nil {
		return nil, kerrors.InternalServer("PLAN", err.Error())
	}
	p.Status = biz.PlanStatus(status)
	if err := json.Unmarshal([]byte(stepsJSON), &p.Steps); err != nil {
		r.data.lg.Warn("unmarshal plan steps failed", loggateway.StepID("data.plan"), loggateway.Err(err))
		return nil, kerrors.InternalServer("PLAN", "unmarshal steps: "+err.Error())
	}
	cat, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", "parse created_at: "+err.Error())
	}
	p.CreatedAt = cat
	uat, err := time.Parse(time.RFC3339, updatedAt)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", "parse updated_at: "+err.Error())
	}
	p.UpdatedAt = uat
	return &p, nil
}

func (r *planRepo) Update(ctx context.Context, plan *biz.Plan) (*biz.Plan, error) {
	stepsJSON, err := json.Marshal(plan.Steps)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", "marshal steps: "+err.Error())
	}
	now := time.Now().UTC().Format(time.RFC3339)
	const q = `UPDATE plans SET goal=?, steps_json=?, status=?, surface_id=?, graph_id=?, updated_at=? WHERE id=?`
	_, err = r.data.Ent().ExecContext(ctx, q,
		plan.Goal, string(stepsJSON), string(plan.Status), plan.SurfaceID, plan.GraphID, now, plan.ID,
	)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", err.Error())
	}
	return plan, nil
}

func (r *planRepo) ListBySession(ctx context.Context, sessionID string) ([]*biz.Plan, error) {
	const q = `SELECT id, session_id, agent_key, goal, steps_json, status, surface_id, graph_id, created_at, updated_at FROM plans WHERE session_id = ? ORDER BY created_at DESC`
	rows, err := r.data.ReadEnt().QueryContext(ctx, q, sessionID)
	if err != nil {
		return nil, kerrors.InternalServer("PLAN", err.Error())
	}
	defer rows.Close()
	var plans []*biz.Plan
	for rows.Next() {
		var p biz.Plan
		var stepsJSON, status, createdAt, updatedAt string
		if err := rows.Scan(&p.ID, &p.SessionID, &p.AgentKey, &p.Goal, &stepsJSON, &status, &p.SurfaceID, &p.GraphID, &createdAt, &updatedAt); err != nil {
			return nil, kerrors.InternalServer("PLAN", err.Error())
		}
		p.Status = biz.PlanStatus(status)
		if err := json.Unmarshal([]byte(stepsJSON), &p.Steps); err != nil {
			r.data.lg.Warn("unmarshal plan steps failed", loggateway.StepID("data.plan"), loggateway.Err(err))
			return nil, kerrors.InternalServer("PLAN", "unmarshal steps: "+err.Error())
		}
		cat, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			return nil, kerrors.InternalServer("PLAN", "parse created_at: "+err.Error())
		}
		p.CreatedAt = cat
		uat, err := time.Parse(time.RFC3339, updatedAt)
		if err != nil {
			return nil, kerrors.InternalServer("PLAN", "parse updated_at: "+err.Error())
		}
		p.UpdatedAt = uat
		plans = append(plans, &p)
	}
	return plans, nil
}
