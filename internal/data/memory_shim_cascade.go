package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

type cascadeRepo struct {
	store *sessionmemory.Store
}

var (
	_ biz.CascadeProposalStore = (*cascadeRepo)(nil)
	_ biz.CascadeGraphReader   = (*cascadeRepo)(nil)
	_ biz.CascadeFactMutator   = (*cascadeRepo)(nil)
	_ biz.CascadeSagaStore     = (*cascadeRepo)(nil)
)

func NewCascadeRepo(store *sessionmemory.Store) *cascadeRepo {
	if store == nil {
		return nil
	}
	return &cascadeRepo{store: store}
}

// --- CascadeProposalStore ---

func (r *cascadeRepo) InsertCascadeProposal(ctx context.Context, in biz.CascadeProposalInsert) ([]byte, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.InsertCascadeProposal(ctx, bizToStoreProposalInsert(in))
}

func (r *cascadeRepo) ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.ListCascadeProposalRows(ctx, agentID, status, limit)
}

func (r *cascadeRepo) GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.GetCascadeProposalRow(ctx, id)
}

func (r *cascadeRepo) UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.UpdateCascadeProposalStatus(ctx, id, status, reviewedBy, reviewNote)
}

// --- CascadeGraphReader ---

func (r *cascadeRepo) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (r *cascadeRepo) GetEntityRow(ctx context.Context, id string) ([]byte, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.GetEntityRow(ctx, id)
}

// --- CascadeFactMutator ---

func (r *cascadeRepo) ReplaceNameInAgentFacts(ctx context.Context, agentID, oldName, newName string) ([][]byte, int, error) {
	if r == nil || r.store == nil {
		return nil, 0, biz.ErrCascadeUnavailable
	}
	return r.store.ReplaceNameInAgentFacts(ctx, agentID, oldName, newName)
}

func (r *cascadeRepo) SaveCascadeOriginalStatements(ctx context.Context, agentID, oldName string, factIDs []string) error {
	if r == nil || r.store == nil {
		return biz.ErrCascadeUnavailable
	}
	return r.store.SaveCascadeOriginalStatements(ctx, agentID, oldName, factIDs)
}

func (r *cascadeRepo) RevertCascadeFactStatements(ctx context.Context, agentID string) (int, error) {
	if r == nil || r.store == nil {
		return 0, biz.ErrCascadeUnavailable
	}
	return r.store.RevertCascadeFactStatements(ctx, agentID)
}

func (r *cascadeRepo) ListCascadeFactDiffs(ctx context.Context, agentID, oldName, newName string, limit int) ([]map[string]any, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return r.store.ListCascadeFactDiffs(ctx, agentID, oldName, newName, limit)
}

func (r *cascadeRepo) MarkFactsIndexStaleByAgent(ctx context.Context, agentID string) (int64, error) {
	if r == nil || r.store == nil {
		return 0, biz.ErrCascadeUnavailable
	}
	return r.store.MarkFactsIndexStaleByAgent(ctx, agentID)
}

// --- CascadeSagaStore ---

func (r *cascadeRepo) InitCascadeSagaSteps(ctx context.Context, proposalID string, steps []biz.CascadeSagaStep) error {
	if r == nil || r.store == nil {
		return biz.ErrCascadeUnavailable
	}
	ss := make([]sessionmemory.CascadeSagaStep, len(steps))
	for i, s := range steps {
		ss[i] = bizToStoreSagaStep(s)
	}
	return r.store.InitCascadeSagaSteps(ctx, proposalID, ss)
}

func (r *cascadeRepo) GetCascadeSagaSteps(ctx context.Context, proposalID string) ([]biz.CascadeSagaStep, error) {
	if r == nil || r.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	raw, err := r.store.GetCascadeSagaSteps(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	out := make([]biz.CascadeSagaStep, len(raw))
	for i, s := range raw {
		out[i] = storeToBizSagaStep(s)
	}
	return out, nil
}

func (r *cascadeRepo) UpdateSagaStepState(ctx context.Context, stepID int64, state, errMsg string) error {
	if r == nil || r.store == nil {
		return biz.ErrCascadeUnavailable
	}
	return r.store.UpdateSagaStepState(ctx, stepID, state, errMsg)
}

func (r *cascadeRepo) UpdateSagaStepResult(ctx context.Context, stepID int64, resultJSON string) error {
	if r == nil || r.store == nil {
		return biz.ErrCascadeUnavailable
	}
	return r.store.UpdateSagaStepResult(ctx, stepID, resultJSON)
}

func (r *cascadeRepo) HasCascadeSaga(ctx context.Context, proposalID string) (bool, error) {
	if r == nil || r.store == nil {
		return false, biz.ErrCascadeUnavailable
	}
	return r.store.HasCascadeSaga(ctx, proposalID)
}

// --- conversion helpers ---

func bizToStoreProposalInsert(in biz.CascadeProposalInsert) sessionmemory.CascadeProposalInsert {
	return sessionmemory.CascadeProposalInsert{
		AgentID:           in.AgentID,
		WorkspaceID:       in.WorkspaceID,
		TriggerEntityID:   in.TriggerEntityID,
		TriggerEntityName: in.TriggerEntityName,
		TriggerAttribute:  in.TriggerAttribute,
		OldValue:          in.OldValue,
		NewValue:          in.NewValue,
		AffectedJSON:      in.AffectedJSON,
		RiskLevel:         in.RiskLevel,
		Rationale:         in.Rationale,
		MetadataJSON:      in.MetadataJSON,
		ExpiresAt:         in.ExpiresAt,
	}
}

func bizToStoreSagaStep(in biz.CascadeSagaStep) sessionmemory.CascadeSagaStep {
	return sessionmemory.CascadeSagaStep{
		ID:          in.ID,
		ProposalID:  in.ProposalID,
		StepIndex:   in.StepIndex,
		StepName:    in.StepName,
		State:       in.State,
		IsCritical:  in.IsCritical,
		Attempts:    in.Attempts,
		StartedAt:   in.StartedAt,
		FinishedAt:  in.FinishedAt,
		PayloadJSON: in.PayloadJSON,
		ResultJSON:  in.ResultJSON,
		Error:       in.Error,
	}
}

func storeToBizSagaStep(in sessionmemory.CascadeSagaStep) biz.CascadeSagaStep {
	return biz.CascadeSagaStep{
		ID:          in.ID,
		ProposalID:  in.ProposalID,
		StepIndex:   in.StepIndex,
		StepName:    in.StepName,
		State:       in.State,
		IsCritical:  in.IsCritical,
		Attempts:    in.Attempts,
		StartedAt:   in.StartedAt,
		FinishedAt:  in.FinishedAt,
		PayloadJSON: in.PayloadJSON,
		ResultJSON:  in.ResultJSON,
		Error:       in.Error,
	}
}
