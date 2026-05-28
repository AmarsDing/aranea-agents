package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

type cascadeGraphStore struct {
	store *sessionmemory.Store
}

func NewCascadeGraphStore(store *sessionmemory.Store) biz.CascadeGraphStore {
	if store == nil {
		return nil
	}
	return &cascadeGraphStore{store: store}
}

func (s *cascadeGraphStore) InsertCascadeProposal(ctx context.Context, in biz.CascadeProposalInsert) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.InsertCascadeProposal(ctx, sessionmemory.CascadeProposalInsert{
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
	})
}

func (s *cascadeGraphStore) ListCascadeProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.ListCascadeProposalRows(ctx, agentID, status, limit)
}

func (s *cascadeGraphStore) GetCascadeProposalRow(ctx context.Context, id string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.GetCascadeProposalRow(ctx, id)
}

func (s *cascadeGraphStore) UpdateCascadeProposalStatus(ctx context.Context, id, status, reviewedBy, reviewNote string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.UpdateCascadeProposalStatus(ctx, id, status, reviewedBy, reviewNote)
}

func (s *cascadeGraphStore) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (s *cascadeGraphStore) GetEntityRow(ctx context.Context, id string) ([]byte, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.GetEntityRow(ctx, id)
}

func (s *cascadeGraphStore) ReplaceNameInAgentFacts(ctx context.Context, agentID, oldName, newName string) ([][]byte, int, error) {
	if s == nil || s.store == nil {
		return nil, 0, biz.ErrCascadeUnavailable
	}
	return s.store.ReplaceNameInAgentFacts(ctx, agentID, oldName, newName)
}

func (s *cascadeGraphStore) InitCascadeSagaSteps(ctx context.Context, proposalID string, steps []biz.CascadeSagaStep) error {
	if s == nil || s.store == nil {
		return biz.ErrCascadeUnavailable
	}
	var ss []sessionmemory.CascadeSagaStep
	for _, step := range steps {
		ss = append(ss, sessionmemory.CascadeSagaStep{
			StepName:    step.StepName,
			IsCritical:  step.IsCritical,
			PayloadJSON: step.PayloadJSON,
		})
	}
	return s.store.InitCascadeSagaSteps(ctx, proposalID, ss)
}

func (s *cascadeGraphStore) GetCascadeSagaSteps(ctx context.Context, proposalID string) ([]biz.CascadeSagaStep, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	raw, err := s.store.GetCascadeSagaSteps(ctx, proposalID)
	if err != nil {
		return nil, err
	}
	out := make([]biz.CascadeSagaStep, len(raw))
	for i, r := range raw {
		out[i] = biz.CascadeSagaStep{
			ID:          r.ID,
			ProposalID:  r.ProposalID,
			StepIndex:   r.StepIndex,
			StepName:    r.StepName,
			State:       r.State,
			IsCritical:  r.IsCritical,
			Attempts:    r.Attempts,
			StartedAt:   r.StartedAt,
			FinishedAt:  r.FinishedAt,
			PayloadJSON: r.PayloadJSON,
			ResultJSON:  r.ResultJSON,
			Error:       r.Error,
		}
	}
	return out, nil
}

func (s *cascadeGraphStore) UpdateSagaStepState(ctx context.Context, stepID int64, state, errMsg string) error {
	if s == nil || s.store == nil {
		return biz.ErrCascadeUnavailable
	}
	return s.store.UpdateSagaStepState(ctx, stepID, state, errMsg)
}

func (s *cascadeGraphStore) UpdateSagaStepResult(ctx context.Context, stepID int64, resultJSON string) error {
	if s == nil || s.store == nil {
		return biz.ErrCascadeUnavailable
	}
	return s.store.UpdateSagaStepResult(ctx, stepID, resultJSON)
}

func (s *cascadeGraphStore) HasCascadeSaga(ctx context.Context, proposalID string) (bool, error) {
	if s == nil || s.store == nil {
		return false, biz.ErrCascadeUnavailable
	}
	return s.store.HasCascadeSaga(ctx, proposalID)
}

func (s *cascadeGraphStore) SaveCascadeOriginalStatements(ctx context.Context, agentID, oldName string, factIDs []string) error {
	if s == nil || s.store == nil {
		return biz.ErrCascadeUnavailable
	}
	return s.store.SaveCascadeOriginalStatements(ctx, agentID, oldName, factIDs)
}

func (s *cascadeGraphStore) RevertCascadeFactStatements(ctx context.Context, agentID string) (int, error) {
	if s == nil || s.store == nil {
		return 0, biz.ErrCascadeUnavailable
	}
	return s.store.RevertCascadeFactStatements(ctx, agentID)
}

func (s *cascadeGraphStore) ListCascadeFactDiffs(ctx context.Context, agentID, oldName, newName string, limit int) ([]map[string]any, error) {
	if s == nil || s.store == nil {
		return nil, biz.ErrCascadeUnavailable
	}
	return s.store.ListCascadeFactDiffs(ctx, agentID, oldName, newName, limit)
}

func (s *cascadeGraphStore) MarkFactsIndexStaleByAgent(ctx context.Context, agentID string) (int64, error) {
	if s == nil || s.store == nil {
		return 0, biz.ErrCascadeUnavailable
	}
	return s.store.MarkFactsIndexStaleByAgent(ctx, agentID)
}
