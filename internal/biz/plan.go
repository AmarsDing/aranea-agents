package biz

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
)

type PlanStatus string

const (
	PlanStatusDraft     PlanStatus = "draft"
	PlanStatusApproved  PlanStatus = "approved"
	PlanStatusConfirmed PlanStatus = "confirmed"
	PlanStatusExecuting PlanStatus = "executing"
	PlanStatusCompleted PlanStatus = "completed"
	PlanStatusFailed    PlanStatus = "failed"
)

type Plan struct {
	ID        string
	SessionID string
	AgentKey  string
	Goal      string
	Steps     []PlanStep
	Status    PlanStatus
	SurfaceID string
	GraphID   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type PlanStep struct {
	ID          string
	Name        string
	Description string
	AgentName   string
	Tools       []string
	DependsOn   []string
}

type PlanRepository interface {
	Create(ctx context.Context, plan *Plan) (*Plan, error)
	Get(ctx context.Context, id string) (*Plan, error)
	Update(ctx context.Context, plan *Plan) (*Plan, error)
	ListBySession(ctx context.Context, sessionID string) ([]*Plan, error)
}

type PlanUsecase struct {
	repo PlanRepository
}

func NewPlanUsecase(repo PlanRepository) *PlanUsecase {
	return &PlanUsecase{repo: repo}
}

func (uc *PlanUsecase) CreatePlan(ctx context.Context, plan *Plan) (*Plan, error) {
	if plan.Goal == "" {
		return nil, apierror.BadRequest("PLAN", "goal is required")
	}
	if len(plan.Steps) == 0 {
		return nil, apierror.BadRequest("PLAN", "at least one step is required")
	}
	plan.Status = PlanStatusDraft
	plan.CreatedAt = time.Now()
	plan.UpdatedAt = time.Now()
	return uc.repo.Create(ctx, plan)
}

func (uc *PlanUsecase) GetPlan(ctx context.Context, id string) (*Plan, error) {
	if id == "" {
		return nil, apierror.BadRequest("PLAN", "id is required")
	}
	plan, err := uc.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return plan, nil
}

func (uc *PlanUsecase) ApprovePlan(ctx context.Context, id string) (*Plan, error) {
	plan, err := uc.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	// S-01 fix: use state machine instead of hardcoded status check
	if !CanPlanTransition(plan.Status, PlanStatusApproved) {
		return nil, apierror.BadRequest("PLAN", "plan cannot transition from %s to approved", string(plan.Status))
	}
	plan.Status = PlanStatusApproved
	plan.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, plan)
}

func (uc *PlanUsecase) MarkExecuting(ctx context.Context, id string) (*Plan, error) {
	plan, err := uc.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanPlanTransition(plan.Status, PlanStatusExecuting) {
		return nil, apierror.BadRequest("PLAN", "plan cannot transition from %s to executing", string(plan.Status))
	}
	plan.Status = PlanStatusExecuting
	plan.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, plan)
}

func (uc *PlanUsecase) MarkCompleted(ctx context.Context, id string) (*Plan, error) {
	plan, err := uc.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanPlanTransition(plan.Status, PlanStatusCompleted) {
		return nil, apierror.BadRequest("PLAN", "plan cannot transition from %s to completed", string(plan.Status))
	}
	plan.Status = PlanStatusCompleted
	plan.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, plan)
}

func (uc *PlanUsecase) MarkFailed(ctx context.Context, id string) (*Plan, error) {
	plan, err := uc.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	if !CanPlanTransition(plan.Status, PlanStatusFailed) {
		return nil, apierror.BadRequest("PLAN", "plan cannot transition from %s to failed", string(plan.Status))
	}
	plan.Status = PlanStatusFailed
	plan.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, plan)
}

func (uc *PlanUsecase) ListBySession(ctx context.Context, sessionID string) ([]*Plan, error) {
	return uc.repo.ListBySession(ctx, sessionID)
}
