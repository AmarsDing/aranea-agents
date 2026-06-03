package data

import (
	"context"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/sessionmemory"
)

// l4EntityRepo delegates L4 entity and evolution operations to sessionmemory.Store.
// Implements biz.L4EntityStore + biz.L4EvolutionStore.
type l4EntityRepo struct {
	store *sessionmemory.Store
}

func newL4EntityRepo(store *sessionmemory.Store) *l4EntityRepo {
	if store == nil {
		return nil
	}
	return &l4EntityRepo{store: store}
}

// Compile-time interface checks.
var (
	_ biz.L4EntityStore    = (*l4EntityRepo)(nil)
	_ biz.L4EvolutionStore = (*l4EntityRepo)(nil)
)

// L4EntityStore

func (r *l4EntityRepo) ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error) {
	return r.store.ListEntityRows(ctx, scopeType, scopeID, workspaceID, userID, entityType, status, keyword, limit, offset)
}

func (r *l4EntityRepo) NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32, queryAtRFC3339 string) ([]byte, error) {
	return r.store.NeighborhoodJSON(ctx, centerID, hops, maxNodes, queryAtRFC3339)
}

func (r *l4EntityRepo) AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error) {
	return r.store.AgentIdentityJSON(ctx, agentID)
}

func (r *l4EntityRepo) AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error) {
	return r.store.AgentStrategyJSON(ctx, agentID)
}

func (r *l4EntityRepo) DeleteSessionEventEntities(ctx context.Context, sessionID string) error {
	return r.store.DeleteSessionEventEntities(ctx, sessionID)
}

// L4EvolutionStore

func (r *l4EntityRepo) EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error) {
	return r.store.EvolutionProposalRows(ctx, agentID, status, limit)
}

func (r *l4EntityRepo) EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error) {
	return r.store.EvolutionEventRows(ctx, agentID, limit)
}

func (r *l4EntityRepo) EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error) {
	return r.store.EvolutionMetricsJSON(ctx, agentID)
}

func (r *l4EntityRepo) InsertEvolutionEventRow(ctx context.Context, in biz.EvolutionEventInsert) ([]byte, error) {
	return r.store.InsertEvolutionEventRow(ctx, evolutionEventInsertToStore(in))
}

func evolutionEventInsertToStore(in biz.EvolutionEventInsert) sessionmemory.EvolutionEventInsert {
	return sessionmemory.EvolutionEventInsert{
		AgentID:       in.AgentID,
		WorkspaceID:   in.WorkspaceID,
		EventKind:     in.EventKind,
		Kind:          in.Kind,
		TargetField:   in.TargetField,
		Reason:        in.Reason,
		TriggerKind:   in.TriggerKind,
		TriggerSource: in.TriggerSource,
		MetadataJSON:  in.MetadataJSON,
	}
}
