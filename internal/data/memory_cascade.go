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
