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
	ID          string
	SessionID   string
	AgentKey    string
	Goal        string
	Steps       []PlanStep
	Status      PlanStatus
	SurfaceID   string
	GraphID     string
	CreatedAt   time.Time
	UpdatedAt   time.Time
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
		return nil, apierror.NotFound("PLAN", "plan not found")
	}
	return plan, nil
}

func (uc *PlanUsecase) ApprovePlan(ctx context.Context, id string) (*Plan, error) {
	plan, err := uc.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	// S-01 fix: use state machine instead of hardcoded status check
	if !canTransitionPlan(plan.Status, PlanStatusApproved) {
		return nil, apierror.BadRequest("PLAN", "plan cannot transition from %s to approved", string(plan.Status))
	}
	plan.Status = PlanStatusApproved
	plan.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, plan)
}

// validPlanTransitions defines the legal state transitions for Plan.
// B-04 fix: add state machine validation to prevent invalid status changes.
var validPlanTransitions = map[PlanStatus][]PlanStatus{
	PlanStatusDraft:     {PlanStatusApproved},
	PlanStatusApproved:  {PlanStatusConfirmed, PlanStatusExecuting},
	PlanStatusConfirmed: {PlanStatusExecuting},
	PlanStatusExecuting: {PlanStatusCompleted, PlanStatusFailed},
	PlanStatusFailed:    {PlanStatusDraft}, // allow retry
	// PlanStatusCompleted is terminal — no outgoing transitions.
}

func canTransitionPlan(from, to PlanStatus) bool {
	allowed, ok := validPlanTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}

func (uc *PlanUsecase) MarkExecuting(ctx context.Context, id string) (*Plan, error) {
	plan, err := uc.GetPlan(ctx, id)
	if err != nil {
		return nil, err
	}
	if !canTransitionPlan(plan.Status, PlanStatusExecuting) {
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
	if !canTransitionPlan(plan.Status, PlanStatusCompleted) {
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
	if !canTransitionPlan(plan.Status, PlanStatusFailed) {
		return nil, apierror.BadRequest("PLAN", "plan cannot transition from %s to failed", string(plan.Status))
	}
	plan.Status = PlanStatusFailed
	plan.UpdatedAt = time.Now()
	return uc.repo.Update(ctx, plan)
}

func (uc *PlanUsecase) ListBySession(ctx context.Context, sessionID string) ([]*Plan, error) {
	return uc.repo.ListBySession(ctx, sessionID)
}
