package data

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/ent/planstepv2"
	"aranea-agents/pkg/loggateway"
)

// planStepV2Repo implements biz.PlanStepV2Repo.
// Stability:evolving
//
// Result/Error fields are stored as Ent JSON (map[string]any) and converted to/from
// biz *StepResult / *StepError via JSON marshal/unmarshal.
type planStepV2Repo struct {
	data *Data
	lg   loggateway.Logger
}

var _ biz.PlanStepV2Repo = (*planStepV2Repo)(nil)

// NewPlanStepV2Repo creates a new PlanStepV2Repo.
// Logger is preset with domain "PLAN_STEP_V2" per loggateway convention.
func NewPlanStepV2Repo(d *Data, lg loggateway.Logger) biz.PlanStepV2Repo {
	return &planStepV2Repo{data: d, lg: lg.With(loggateway.Domain("PLAN_STEP_V2"))}
}

// GetPlanStep returns the PlanStep by ID.
func (r *planStepV2Repo) GetPlanStep(ctx context.Context, id string) (biz.PlanStep, error) {
	if r == nil || r.data == nil {
		return biz.PlanStep{}, fmt.Errorf("plan step v2 repo: database not configured")
	}
	row, err := r.data.RW().Read(ctx).PlanStepV2.Get(ctx, id)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "PLAN_STEP_V2")
	}
	return entPlanStepV2ToBiz(row), nil
}

// ListPlanStepsByPlan returns all plan steps for the given plan, ordered by seq asc.
func (r *planStepV2Repo) ListPlanStepsByPlan(ctx context.Context, planID string) ([]biz.PlanStep, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("plan step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).PlanStepV2.Query().
		Where(planstepv2.PlanIDEQ(planID)).
		Order(ent.Asc(planstepv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "PLAN_STEP_V2")
	}
	return entPlanStepsV2ToBiz(rows), nil
}

// ListPlanStepsByTask returns all plan steps for the given task.
func (r *planStepV2Repo) ListPlanStepsByTask(ctx context.Context, taskID string) ([]biz.PlanStep, error) {
	if r == nil || r.data == nil {
		return nil, fmt.Errorf("plan step v2 repo: database not configured")
	}
	rows, err := r.data.RW().Read(ctx).PlanStepV2.Query().
		Where(planstepv2.TaskIDEQ(taskID)).
		Order(ent.Asc(planstepv2.FieldSeq)).
		All(ctx)
	if err != nil {
		return nil, entErrToBizErr(err, "PLAN_STEP_V2")
	}
	return entPlanStepsV2ToBiz(rows), nil
}

// CreatePlanStep inserts a new PlanStep with the caller's claimed Version.
func (r *planStepV2Repo) CreatePlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	if r == nil || r.data == nil {
		return biz.PlanStep{}, fmt.Errorf("plan step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).PlanStepV2.Create().
		SetID(ps.ID).
		SetPlanID(ps.PlanID).
		SetTaskID(ps.TaskID).
		SetLabel(ps.Label).
		SetDescription(ps.Description).
		SetDependsOn(ps.DependsOn).
		SetMappedTeamStageID(ps.MappedTeamStageID).
		SetStatus(string(ps.Status)).
		SetAutoSynthesis(ps.AutoSynthesis).
		SetStartedAt(ps.StartedAt).
		SetSeq(ps.Seq).
		SetVersion(ps.Version)
	if ps.CompletedAt != nil {
		b.SetCompletedAt(*ps.CompletedAt)
	}
	if ps.Result != nil {
		b.SetResult(stepResultToEnt(ps.Result))
	}
	if ps.Error != nil {
		b.SetError(stepErrorToEnt(ps.Error))
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "PLAN_STEP_V2")
	}
	return entPlanStepV2ToBiz(row), nil
}

// UpdatePlanStep patches mutable fields without version guard.
// Use UpsertPlanStep for concurrent-safe writes.
func (r *planStepV2Repo) UpdatePlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	if r == nil || r.data == nil {
		return biz.PlanStep{}, fmt.Errorf("plan step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).PlanStepV2.UpdateOneID(ps.ID).
		SetPlanID(ps.PlanID).
		SetTaskID(ps.TaskID).
		SetLabel(ps.Label).
		SetDescription(ps.Description).
		SetDependsOn(ps.DependsOn).
		SetMappedTeamStageID(ps.MappedTeamStageID).
		SetStatus(string(ps.Status)).
		SetAutoSynthesis(ps.AutoSynthesis).
		SetSeq(ps.Seq).
		SetVersion(ps.Version)
	if ps.CompletedAt != nil {
		b.SetCompletedAt(*ps.CompletedAt)
	}
	if ps.Result != nil {
		b.SetResult(stepResultToEnt(ps.Result))
	}
	if ps.Error != nil {
		b.SetError(stepErrorToEnt(ps.Error))
	}
	row, err := b.Save(ctx)
	if err != nil {
		return biz.PlanStep{}, entErrToBizErr(err, "PLAN_STEP_V2")
	}
	return entPlanStepV2ToBiz(row), nil
}

// UpsertPlanStep applies optimistic-concurrency upsert (see UpsertTask for semantics).
func (r *planStepV2Repo) UpsertPlanStep(ctx context.Context, ps biz.PlanStep) (biz.PlanStep, error) {
	if r == nil || r.data == nil {
		return biz.PlanStep{}, fmt.Errorf("plan step v2 repo: database not configured")
	}
	b := r.data.RW().Write(ctx).PlanStepV2.UpdateOneID(ps.ID).
		Where(planstepv2.VersionLT(ps.Version)).
		SetPlanID(ps.PlanID).
		SetTaskID(ps.TaskID).
		SetLabel(ps.Label).
		SetDescription(ps.Description).
		SetDependsOn(ps.DependsOn).
		SetMappedTeamStageID(ps.MappedTeamStageID).
		SetStatus(string(ps.Status)).
		SetAutoSynthesis(ps.AutoSynthesis).
		SetSeq(ps.Seq).
		SetVersion(ps.Version)
	if ps.CompletedAt != nil {
		b.SetCompletedAt(*ps.CompletedAt)
	}
	if ps.Result != nil {
		b.SetResult(stepResultToEnt(ps.Result))
	}
	if ps.Error != nil {
		b.SetError(stepErrorToEnt(ps.Error))
	}
	if err := b.Exec(ctx); err == nil {
		row, getErr := r.data.RW().Read(ctx).PlanStepV2.Get(ctx, ps.ID)
		if getErr != nil {
			return biz.PlanStep{}, entErrToBizErr(getErr, "PLAN_STEP_V2")
		}
		return entPlanStepV2ToBiz(row), nil
	}
	cb := r.data.RW().Write(ctx).PlanStepV2.Create().
		SetID(ps.ID).
		SetPlanID(ps.PlanID).
		SetTaskID(ps.TaskID).
		SetLabel(ps.Label).
		SetDescription(ps.Description).
		SetDependsOn(ps.DependsOn).
		SetMappedTeamStageID(ps.MappedTeamStageID).
		SetStatus(string(ps.Status)).
		SetAutoSynthesis(ps.AutoSynthesis).
		SetStartedAt(ps.StartedAt).
		SetSeq(ps.Seq).
		SetVersion(ps.Version)
	if ps.CompletedAt != nil {
		cb.SetCompletedAt(*ps.CompletedAt)
	}
	if ps.Result != nil {
		cb.SetResult(stepResultToEnt(ps.Result))
	}
	if ps.Error != nil {
		cb.SetError(stepErrorToEnt(ps.Error))
	}
	row, err := cb.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			existing, getErr := r.data.RW().Read(ctx).PlanStepV2.Get(ctx, ps.ID)
			if getErr != nil {
				return biz.PlanStep{}, entErrToBizErr(getErr, "PLAN_STEP_V2")
			}
			return entPlanStepV2ToBiz(existing), nil
		}
		return biz.PlanStep{}, entErrToBizErr(err, "PLAN_STEP_V2")
	}
	return entPlanStepV2ToBiz(row), nil
}

// entPlanStepV2ToBiz converts an Ent PlanStepV2 row to biz.PlanStep.
// Result/Error JSON maps are decoded into biz *StepResult / *StepError.
func entPlanStepV2ToBiz(row *ent.PlanStepV2) biz.PlanStep {
	var completedAt *time.Time
	if row.CompletedAt != nil {
		t := *row.CompletedAt
		completedAt = &t
	}
	return biz.PlanStep{
		ID:                row.ID,
		PlanID:            row.PlanID,
		TaskID:            row.TaskID,
		Label:             row.Label,
		Description:       row.Description,
		DependsOn:         row.DependsOn,
		MappedTeamStageID: row.MappedTeamStageID,
		Status:            biz.PlanStepStatus(row.Status),
		AutoSynthesis:     row.AutoSynthesis,
		StartedAt:         row.StartedAt,
		CompletedAt:       completedAt,
		Seq:               row.Seq,
		Version:           row.Version,
		Result:            stepResultFromEnt(row.Result),
		Error:             stepErrorFromEnt(row.Error),
	}
}

func entPlanStepsV2ToBiz(rows []*ent.PlanStepV2) []biz.PlanStep {
	out := make([]biz.PlanStep, 0, len(rows))
	for _, r := range rows {
		out = append(out, entPlanStepV2ToBiz(r))
	}
	return out
}

// stepResultToEnt converts biz *StepResult to Ent's map[string]any for JSON field.
// Returns nil if r is nil (field will be NULL/empty in DB).
func stepResultToEnt(r *biz.StepResult) map[string]any {
	if r == nil {
		return nil
	}
	data, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// stepResultFromEnt converts Ent's map[string]any back to biz *StepResult.
// Returns nil if m is nil or empty (no result set).
func stepResultFromEnt(m map[string]any) *biz.StepResult {
	if len(m) == 0 {
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var r biz.StepResult
	if err := json.Unmarshal(data, &r); err != nil {
		return nil
	}
	return &r
}

// stepErrorToEnt converts biz *StepError to Ent's map[string]any for JSON field.
// Returns nil if e is nil (field will be NULL/empty in DB).
func stepErrorToEnt(e *biz.StepError) map[string]any {
	if e == nil {
		return nil
	}
	data, err := json.Marshal(e)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil
	}
	return m
}

// stepErrorFromEnt converts Ent's map[string]any back to biz *StepError.
// Returns nil if m is nil or empty (no error set).
func stepErrorFromEnt(m map[string]any) *biz.StepError {
	if len(m) == 0 {
		return nil
	}
	data, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	var e biz.StepError
	if err := json.Unmarshal(data, &e); err != nil {
		return nil
	}
	return &e
}
