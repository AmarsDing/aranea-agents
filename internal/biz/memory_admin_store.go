package biz

import (
	"context"

	"aranea-agents/internal/data/sessionmemory"
)

type FactUpsert = sessionmemory.MemoryFactUpsert

type EvolutionEventInsert = sessionmemory.EvolutionEventInsert

type SessionAdminStore interface {
	ListL0SnapshotRows(ctx context.Context, sessionID string, limit int32) ([][]byte, error)
	ListL1TaskRows(ctx context.Context, sessionID, agentID, status, includeEnded string) ([][]byte, error)
	ListL1FieldRows(ctx context.Context, taskID string, includeInternal bool) ([][]byte, error)
	ListFactRows(ctx context.Context, scopeType, scopeID, kind, status, keyword string, limit, offset int32) ([][]byte, int32, int32, int32, error)
	ListEntityRows(ctx context.Context, scopeType, scopeID, workspaceID, userID, entityType, status, keyword string, limit, offset int32) ([][]byte, int32, error)
	NeighborhoodJSON(ctx context.Context, centerID string, hops, maxNodes int32) ([]byte, error)
	AgentIdentityJSON(ctx context.Context, agentID string) ([]byte, error)
	AgentStrategyJSON(ctx context.Context, agentID string) ([]byte, error)
	EvolutionProposalRows(ctx context.Context, agentID, status string, limit int32) ([][]byte, error)
	EvolutionEventRows(ctx context.Context, agentID string, limit int32) ([][]byte, error)
	EvolutionMetricsJSON(ctx context.Context, agentID string) ([]byte, error)
	UpsertFactRow(ctx context.Context, in FactUpsert) ([]byte, error)
	InsertEvolutionEventRow(ctx context.Context, in EvolutionEventInsert) ([]byte, error)
	DeleteSessionEventEntities(ctx context.Context, sessionID string) error
}

func WrapSessionAdminStore(store *sessionmemory.Store) SessionAdminStore {
	if store == nil {
		return nil
	}
	return store
}
